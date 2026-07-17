package wallet

import (
	"errors"
	"fmt"
	"strings"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OwnerRef struct {
	Type     model.WalletOwnerType
	ID       uint
	TenantID uint
}

func ResolveOwnerRef(tx *gorm.DB, ownerType model.WalletOwnerType, ownerID, tenantID uint) (OwnerRef, error) {
	if !ownerType.IsValid() || ownerID == 0 {
		return OwnerRef{}, errors.New("invalid wallet owner")
	}

	switch ownerType {
	case model.WalletOwnerUser:
		resolvedTenantID, err := resolveEffectiveTenantID(tx, ownerID, tenantID)
		if err != nil {
			return OwnerRef{}, err
		}
		tenantID = resolvedTenantID
	case model.WalletOwnerTeam:
		var team model.Team
		if err := tx.Select("id", "tenant_id").First(&team, ownerID).Error; err != nil {
			return OwnerRef{}, err
		}
		if tenantID != 0 && team.TenantID != tenantID {
			return OwnerRef{}, errors.New("team does not belong to requested tenant")
		}
		tenantID = team.TenantID
	case model.WalletOwnerApp:
		var app model.App
		if err := tx.Select("id", "tenant_id").First(&app, ownerID).Error; err != nil {
			return OwnerRef{}, err
		}
		if tenantID != 0 && app.TenantID != tenantID {
			return OwnerRef{}, errors.New("app does not belong to requested tenant")
		}
		tenantID = app.TenantID
	case model.WalletOwnerTenant:
		if tenantID != 0 && tenantID != ownerID {
			return OwnerRef{}, errors.New("tenant wallet owner must match tenant context")
		}
		var count int64
		if err := tx.Model(&model.Tenant{}).Where("id = ?", ownerID).Count(&count).Error; err != nil {
			return OwnerRef{}, err
		}
		if count == 0 {
			return OwnerRef{}, gorm.ErrRecordNotFound
		}
		tenantID = ownerID
	}

	if tenantID == 0 {
		return OwnerRef{}, errors.New("wallet owner has no tenant identity")
	}
	return OwnerRef{Type: ownerType, ID: ownerID, TenantID: tenantID}, nil
}

func walletOwnerQuery(tx *gorm.DB, owner OwnerRef) *gorm.DB {
	return tx.Where(
		"tenant_id = ? AND owner_type = ? AND owner_id = ?",
		owner.TenantID,
		owner.Type,
		owner.ID,
	)
}

func findCurrencyByCode(tx *gorm.DB, code string) (model.Currency, error) {
	var curr model.Currency
	if err := tx.Where("code = ? AND is_active = ?", strings.ToUpper(strings.TrimSpace(code)), true).First(&curr).Error; err != nil {
		return model.Currency{}, errors.New("invalid currency code")
	}
	return curr, nil
}

func ensureOwnerWalletTx(tx *gorm.DB, owner OwnerRef, currencyID uint) (model.Wallet, error) {
	var current model.Wallet
	err := walletOwnerQuery(tx, owner).Where("currency_id = ?", currencyID).First(&current).Error
	if err == nil {
		return current, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Wallet{}, err
	}

	current = model.Wallet{
		TenantID:   owner.TenantID,
		OwnerType:  owner.Type,
		OwnerID:    owner.ID,
		CurrencyID: &currencyID,
	}
	if err := tx.Create(&current).Error; err != nil {
		// A concurrent request may have created the unique owner/currency row.
		if queryErr := walletOwnerQuery(tx, owner).Where("currency_id = ?", currencyID).First(&current).Error; queryErr == nil {
			return current, nil
		}
		return model.Wallet{}, err
	}
	return current, nil
}

func EnsureOwnerCreditWalletTx(tx *gorm.DB, ownerType model.WalletOwnerType, ownerID, tenantID uint) error {
	owner, err := ResolveOwnerRef(tx, ownerType, ownerID, tenantID)
	if err != nil {
		return err
	}
	curr, err := resolveCreditCurrency(tx)
	if err != nil {
		return err
	}
	_, err = ensureOwnerWalletTx(tx, owner, curr.ID)
	return err
}

func GetOwnerBalanceByCode(ownerType model.WalletOwnerType, ownerID, tenantID uint, currencyCode string) (model.Wallet, error) {
	db := common.DB()
	owner, err := ResolveOwnerRef(db, ownerType, ownerID, tenantID)
	if err != nil {
		return model.Wallet{}, err
	}
	curr, err := findCurrencyByCode(db, currencyCode)
	if err != nil {
		return model.Wallet{}, err
	}

	var result model.Wallet
	err = db.Transaction(func(tx *gorm.DB) error {
		walletModel, err := ensureOwnerWalletTx(tx, owner, curr.ID)
		if err != nil {
			return err
		}
		result = walletModel
		return nil
	})
	if err != nil {
		return model.Wallet{}, err
	}
	result.Currency = &curr
	return result, nil
}

func ListOwnerWallets(ownerType model.WalletOwnerType, ownerID, tenantID uint) ([]model.Wallet, error) {
	db := common.DB()
	owner, err := ResolveOwnerRef(db, ownerType, ownerID, tenantID)
	if err != nil {
		return nil, err
	}

	var wallets []model.Wallet
	err = walletOwnerQuery(db.Preload("Currency"), owner).
		Order("currency_id ASC").
		Find(&wallets).Error
	return wallets, err
}

func OwnerHistoryByCode(ownerType model.WalletOwnerType, ownerID, tenantID uint, currencyCode string, limit int) ([]model.WalletTx, error) {
	if limit <= 0 {
		limit = 20
	}
	db := common.DB()
	owner, err := ResolveOwnerRef(db, ownerType, ownerID, tenantID)
	if err != nil {
		return nil, err
	}
	curr, err := findCurrencyByCode(db, currencyCode)
	if err != nil {
		return nil, err
	}

	var walletModel model.Wallet
	if err := walletOwnerQuery(db, owner).Where("currency_id = ?", curr.ID).First(&walletModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []model.WalletTx{}, nil
		}
		return nil, err
	}

	var txs []model.WalletTx
	err = db.Preload("Wallet.Currency").Where("wallet_id = ?", walletModel.ID).
		Order("created_at DESC").Limit(limit).Find(&txs).Error
	return txs, err
}

func AdjustOwnerByCode(ownerType model.WalletOwnerType, ownerID, tenantID uint, currencyCode string, delta int64, txType, reference string) (model.Wallet, error) {
	if delta == 0 {
		return model.Wallet{}, errors.New("amount must not be zero")
	}
	db := common.DB()
	owner, err := ResolveOwnerRef(db, ownerType, ownerID, tenantID)
	if err != nil {
		return model.Wallet{}, err
	}
	curr, err := findCurrencyByCode(db, currencyCode)
	if err != nil {
		return model.Wallet{}, err
	}

	var updated model.Wallet
	err = db.Transaction(func(tx *gorm.DB) error {
		walletModel, err := ensureOwnerWalletTx(tx, owner, curr.ID)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&walletModel, walletModel.ID).Error; err != nil {
			return err
		}

		normalizedReference := strings.TrimSpace(reference)
		if len(normalizedReference) > 128 {
			return errors.New("reference must not exceed 128 characters")
		}
		normalizedType := strings.TrimSpace(txType)
		if normalizedType == "" {
			if delta > 0 {
				normalizedType = "adjust_increase"
			} else {
				normalizedType = "adjust_decrease"
			}
		}
		if normalizedReference != "" {
			var existing model.WalletTx
			err := tx.Where("wallet_id = ? AND idempotency_key = ?", walletModel.ID, normalizedReference).First(&existing).Error
			if err == nil {
				if existing.Amount != delta || existing.Type != normalizedType {
					return errors.New("idempotency key was already used for a different wallet adjustment")
				}
				walletModel.Currency = &curr
				updated = walletModel
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		newBalance := walletModel.Balance + delta
		if newBalance < 0 {
			return errors.New("insufficient funds")
		}
		if err := tx.Model(&walletModel).Update("balance", newBalance).Error; err != nil {
			return err
		}

		walletTx := model.WalletTx{
			WalletID:  walletModel.ID,
			Type:      normalizedType,
			Amount:    delta,
			Status:    "success",
			Reference: normalizedReference,
		}
		if normalizedReference != "" {
			walletTx.IdempotencyKey = &normalizedReference
		}
		if err := tx.Create(&walletTx).Error; err != nil {
			return err
		}

		walletModel.Balance = newBalance
		walletModel.Currency = &curr
		updated = walletModel
		return nil
	})
	if err != nil {
		return model.Wallet{}, err
	}
	return updated, nil
}

func EnsureCreditWalletsForAllTeams() (int64, error) {
	db := common.DB()
	var teams []model.Team
	if err := db.Select("id", "tenant_id").Where("tenant_id > 0 AND is_active = ?", true).Find(&teams).Error; err != nil {
		return 0, err
	}

	var created int64
	err := db.Transaction(func(tx *gorm.DB) error {
		curr, err := resolveCreditCurrency(tx)
		if err != nil {
			return err
		}
		for _, team := range teams {
			owner := OwnerRef{Type: model.WalletOwnerTeam, ID: team.ID, TenantID: team.TenantID}
			var count int64
			if err := walletOwnerQuery(tx.Model(&model.Wallet{}), owner).Where("currency_id = ?", curr.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if _, err := ensureOwnerWalletTx(tx, owner, curr.ID); err != nil {
				return fmt.Errorf("ensure team %d credit wallet: %w", team.ID, err)
			}
			created++
		}
		return nil
	})
	return created, err
}

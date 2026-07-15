package migration

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
)

// MigrateWalletCurrencyField migrates existing wallet data from currency/currency_code to currency_id field
func MigrateWalletCurrencyField() {
	db := common.DB()

	// 确保 market_currencies 表存在且有数据
	if !db.Migrator().HasTable("market_currencies") {
		log.Println("[Migration] Currencies table doesn't exist, skipping wallet migration")
		return
	}

	// 检查是否有货币数据
	var currencyCount int64
	db.Model(&model.Currency{}).Count(&currencyCount)
	if currencyCount == 0 {
		log.Println("[Migration] No currencies found, skipping wallet migration")
		return
	}

	// 检查 market_wallets 表是否存在
	if !db.Migrator().HasTable("market_wallets") {
		log.Println("[Migration] Wallets table doesn't exist yet, skipping currency migration")
		return
	} // 检查是否存在旧的currency字段或currency_code字段
	hasCurrency := db.Migrator().HasColumn("wallets", "currency")
	hasCurrencyCode := db.Migrator().HasColumn("wallets", "currency_code")
	hasCurrencyID := db.Migrator().HasColumn("wallets", "currency_id")

	if !hasCurrency && !hasCurrencyCode {
		log.Println("[Migration] No old currency fields found in wallets table, skipping migration")
		return
	}

	if hasCurrencyID {
		log.Println("[Migration] currency_id field already exists, cleaning up old fields")
		dropOldCurrencyColumns(db, hasCurrency, hasCurrencyCode)
		return
	}

	log.Println("[Migration] Migrating wallet currency field to currency_id...")

	// 先添加 currency_id 字段（允许为空）
	if err := db.Exec("ALTER TABLE wallets ADD COLUMN currency_id INTEGER").Error; err != nil {
		log.Printf("[Migration] Failed to add currency_id column: %v", err)
		return
	}

	// 查询现有钱包记录
	var wallets []struct {
		ID           uint
		Currency     *string // 可能为空
		CurrencyCode *string // 可能为空
	}

	// 构建查询SQL
	var query string
	if hasCurrency && hasCurrencyCode {
		query = "SELECT id, currency, currency_code FROM market_wallets WHERE (currency IS NOT NULL AND currency != '') OR (currency_code IS NOT NULL AND currency_code != '')"
	} else if hasCurrency {
		query = "SELECT id, currency FROM market_wallets WHERE currency IS NOT NULL AND currency != ''"
	} else {
		query = "SELECT id, currency_code FROM market_wallets WHERE currency_code IS NOT NULL AND currency_code != ''"
	}

	if err := db.Raw(query).Scan(&wallets).Error; err != nil {
		log.Printf("[Migration] Failed to query existing wallet currency data: %v", err)
		return
	}

	if len(wallets) == 0 {
		log.Println("[Migration] No existing wallet data to migrate")
	} else {
		log.Printf("[Migration] Found %d wallet records to migrate", len(wallets))

		// 创建货币代码到ID的映射
		currencyMap := make(map[string]uint)
		var currencies []model.Currency
		db.Find(&currencies)
		for _, curr := range currencies {
			currencyMap[curr.Code] = curr.ID
		}

		// 更新每个钱包记录的currency_id字段
		migrated := 0
		for _, wallet := range wallets {
			var currencyCode string

			// 优先使用currency_code，如果没有则使用currency
			if wallet.CurrencyCode != nil && *wallet.CurrencyCode != "" {
				currencyCode = *wallet.CurrencyCode
			} else if wallet.Currency != nil && *wallet.Currency != "" {
				currencyCode = *wallet.Currency
			} else {
				continue
			}

			if currencyID, exists := currencyMap[currencyCode]; exists {
				if err := db.Exec("UPDATE market_wallets SET currency_id = ? WHERE id = ?", currencyID, wallet.ID).Error; err != nil {
					log.Printf("[Migration] Failed to update wallet %d: %v", wallet.ID, err)
				} else {
					migrated++
				}
			} else {
				// 如果找不到对应的货币，使用默认货币（USD）
				if defaultCurrencyID, exists := currencyMap["USD"]; exists {
					if err := db.Exec("UPDATE market_wallets SET currency_id = ? WHERE id = ?", defaultCurrencyID, wallet.ID).Error; err != nil {
						log.Printf("[Migration] Failed to update wallet %d with default currency: %v", wallet.ID, err)
					} else {
						migrated++
						log.Printf("[Migration] Wallet %d migrated to default currency (USD) from unknown currency '%s'", wallet.ID, currencyCode)
					}
				} else {
					log.Printf("[Migration] Currency code '%s' not found and no default currency available for wallet %d", currencyCode, wallet.ID)
				}
			}
		}

		log.Printf("[Migration] Successfully migrated %d wallet records", migrated)
	}

	// 现在设置 currency_id 为 NOT NULL
	if err := db.Exec("UPDATE market_wallets SET currency_id = (SELECT id FROM market_currencies WHERE code = 'USD' LIMIT 1) WHERE currency_id IS NULL").Error; err != nil {
		log.Printf("[Migration] Failed to set default currency for null records: %v", err)
	}

	// 删除旧的字段
	dropOldCurrencyColumns(db, hasCurrency, hasCurrencyCode)

	log.Println("[Migration] Wallet currency migration completed")
}

func dropOldCurrencyColumns(db *gorm.DB, hasCurrency, hasCurrencyCode bool) {
	if hasCurrency {
		if err := db.Migrator().DropColumn(&model.Wallet{}, "currency"); err != nil {
			log.Printf("[Migration] Failed to drop old currency column: %v", err)
		} else {
			log.Println("[Migration] Dropped old currency column")
		}
	}

	if hasCurrencyCode {
		if err := db.Migrator().DropColumn(&model.Wallet{}, "currency_code"); err != nil {
			log.Printf("[Migration] Failed to drop old currency_code column: %v", err)
		} else {
			log.Println("[Migration] Dropped old currency_code column")
		}
	}
}

// MigrateWalletTenantField ensures tenant_id exists and backfills data for historical rows.
func MigrateWalletTenantField() {
	db := common.DB()

	if !db.Migrator().HasTable("market_wallets") {
		log.Println("[Migration] Wallets table doesn't exist, skipping tenant_id migration")
		return
	}

	if !db.Migrator().HasColumn("market_wallets", "tenant_id") {
		if err := db.Migrator().AddColumn(&model.Wallet{}, "TenantID"); err != nil {
			log.Printf("[Migration] Failed to add tenant_id to wallets: %v", err)
			return
		}
		log.Println("[Migration] Added tenant_id column to wallets")
	}

	type walletRow struct {
		ID       uint
		UserID   *uint
		TeamID   *uint
		TenantID uint
	}

	var wallets []walletRow
	if err := db.Table("market_wallets").
		Select("id", "user_id", "team_id", "tenant_id").
		Where("tenant_id = ?", 0).
		Find(&wallets).Error; err != nil {
		log.Printf("[Migration] Failed to query wallets for tenant backfill: %v", err)
		return
	}

	if len(wallets) == 0 {
		log.Println("[Migration] Wallet tenant_id already populated")
		return
	}

	migrated := 0
	for _, w := range wallets {
		tenantID := uint(0)

		if w.UserID != nil && *w.UserID != 0 {
			var membership model.TenantUser
			err := db.Select("tenant_id").Where("user_id = ?", *w.UserID).Order("created_at ASC").First(&membership).Error
			if err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					log.Printf("[Migration] Failed to load tenant membership for user %d wallet %d: %v", *w.UserID, w.ID, err)
				}
			} else {
				tenantID = membership.TenantID
			}
		}

		if tenantID == 0 && w.TeamID != nil && *w.TeamID != 0 {
			var team model.Team
			err := db.Select("tenant_id").First(&team, *w.TeamID).Error
			if err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					log.Printf("[Migration] Failed to load team %d for wallet %d: %v", *w.TeamID, w.ID, err)
				}
			} else {
				tenantID = team.TenantID
			}
		}

		if tenantID == 0 {
			continue
		}

		if err := db.Table("market_wallets").Where("id = ?", w.ID).Update("tenant_id", tenantID).Error; err != nil {
			log.Printf("[Migration] Failed to backfill tenant_id for wallet %d: %v", w.ID, err)
			continue
		}
		migrated++
	}

	log.Printf("[Migration] Wallet tenant_id backfill completed: %d rows updated", migrated)
}

// MigrateTeamTenantFields assigns pre-tenancy teams to a real tenant. Owner
// membership is authoritative when available; the active default tenant is the
// compatibility home for teams created by legacy global users.
func MigrateTeamTenantFields() error {
	db := common.DB()
	if !db.Migrator().HasTable(model.Team{}.TableName()) {
		return nil
	}

	var teams []model.Team
	if err := db.Where("tenant_id = ? AND is_active = ?", 0, true).Find(&teams).Error; err != nil {
		return err
	}
	for _, team := range teams {
		tenantID := uint(0)
		var owner model.TeamMember
		if err := db.Where("team_id = ? AND role = ? AND status = ?", team.ID, model.TeamRoleOwner, "active").
			Order("created_at ASC").First(&owner).Error; err == nil {
			var membership model.TenantUser
			if err := db.Where("user_id = ?", owner.UserID).Order("created_at ASC").First(&membership).Error; err == nil {
				tenantID = membership.TenantID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if tenantID == 0 {
				var user model.User
				if err := db.Select("enforced_tenant_id").First(&user, owner.UserID).Error; err == nil {
					tenantID = user.EnforcedTenantID
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if tenantID == 0 {
			var defaultTenant model.Tenant
			err := db.Where("code = ? AND status = ?", "default", model.TenantStatusActive).First(&defaultTenant).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = db.Where("status = ?", model.TenantStatusActive).Order("id ASC").First(&defaultTenant).Error
			}
			if err != nil {
				return fmt.Errorf("resolve tenant for legacy team %d: %w", team.ID, err)
			}
			tenantID = defaultTenant.ID
		}

		if err := db.Model(&model.Team{}).Where("id = ? AND tenant_id = ?", team.ID, 0).Update("tenant_id", tenantID).Error; err != nil {
			return err
		}
		log.Printf("[Migration] Assigned legacy team %d to tenant %d", team.ID, tenantID)
	}
	return nil
}

// MigrateWalletOwnerFields promotes the historical user_id/team_id columns to
// the unified owner_type/owner_id identity before AutoMigrate adds uniqueness.
func MigrateWalletOwnerFields() error {
	db := common.DB()
	if !db.Migrator().HasTable("market_wallets") {
		return nil
	}

	if !db.Migrator().HasColumn("market_wallets", "owner_type") {
		if err := db.Migrator().AddColumn(&model.Wallet{}, "OwnerType"); err != nil {
			return err
		}
	}
	if !db.Migrator().HasColumn("market_wallets", "owner_id") {
		if err := db.Migrator().AddColumn(&model.Wallet{}, "OwnerID"); err != nil {
			return err
		}
	}

	if err := db.Exec(`UPDATE market_wallets SET owner_type = 'user', owner_id = user_id
		WHERE (owner_type IS NULL OR owner_type = '' OR owner_id = 0) AND user_id IS NOT NULL AND user_id > 0`).Error; err != nil {
		return err
	}
	if err := db.Exec(`UPDATE market_wallets SET owner_type = 'team', owner_id = team_id
		WHERE (owner_type IS NULL OR owner_type = '' OR owner_id = 0) AND team_id IS NOT NULL AND team_id > 0`).Error; err != nil {
		return err
	}
	if err := db.Exec(`UPDATE market_wallets SET owner_type = 'tenant', owner_id = tenant_id
		WHERE (owner_type IS NULL OR owner_type = '' OR owner_id = 0)
		AND user_id IS NULL AND team_id IS NULL AND tenant_id > 0`).Error; err != nil {
		return err
	}

	var invalid int64
	if err := db.Unscoped().Model(&model.Wallet{}).
		Where("owner_type NOT IN ? OR owner_id = 0", []string{"user", "app", "tenant", "team"}).
		Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return errors.New("wallet owner migration left rows without a valid owner")
	}

	type duplicateGroup struct {
		TenantID   uint
		OwnerType  model.WalletOwnerType
		OwnerID    uint
		CurrencyID uint
		Count      int64
	}
	var groups []duplicateGroup
	if err := db.Unscoped().Model(&model.Wallet{}).
		Select("tenant_id, owner_type, owner_id, currency_id, COUNT(*) AS count").
		Where("deleted_at IS NULL").
		Group("tenant_id, owner_type, owner_id, currency_id").
		Having("COUNT(*) > 1").Scan(&groups).Error; err != nil {
		return err
	}

	for _, group := range groups {
		err := db.Transaction(func(tx *gorm.DB) error {
			var rows []model.Wallet
			if err := tx.Unscoped().
				Where("tenant_id = ? AND owner_type = ? AND owner_id = ? AND currency_id = ? AND deleted_at IS NULL", group.TenantID, group.OwnerType, group.OwnerID, group.CurrencyID).
				Order("id ASC").Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) < 2 {
				return nil
			}

			canonical := rows[0]
			balance := canonical.Balance
			freeze := canonical.Freeze
			for _, duplicate := range rows[1:] {
				balance += duplicate.Balance
				freeze += duplicate.Freeze
				if err := tx.Model(&model.WalletTx{}).Where("wallet_id = ?", duplicate.ID).Update("wallet_id", canonical.ID).Error; err != nil {
					return err
				}
				if err := tx.Unscoped().Delete(&duplicate).Error; err != nil {
					return err
				}
			}
			return tx.Model(&canonical).Updates(map[string]interface{}{"balance": balance, "freeze": freeze}).Error
		})
		if err != nil {
			return err
		}
		log.Printf("[Migration] Merged %d duplicate wallets for tenant=%d owner=%s:%d currency=%d", group.Count, group.TenantID, group.OwnerType, group.OwnerID, group.CurrencyID)
	}
	return nil
}

package model

import (
	"errors"

	"gorm.io/gorm"
)

type WalletOwnerType string

const (
	WalletOwnerUser   WalletOwnerType = "user"
	WalletOwnerApp    WalletOwnerType = "app"
	WalletOwnerTenant WalletOwnerType = "tenant"
	WalletOwnerTeam   WalletOwnerType = "team"
)

func (ownerType WalletOwnerType) IsValid() bool {
	switch ownerType {
	case WalletOwnerUser, WalletOwnerApp, WalletOwnerTenant, WalletOwnerTeam:
		return true
	default:
		return false
	}
}

// Wallet represents one currency account owned by a user, app, tenant, or team.
type Wallet struct {
	gorm.Model
	TenantID   uint            `gorm:"index;not null;default:0;uniqueIndex:uidx_wallet_owner_currency,priority:1" json:"tenant_id"`
	OwnerType  WalletOwnerType `gorm:"size:16;not null;default:'';index;uniqueIndex:uidx_wallet_owner_currency,priority:2" json:"owner_type"`
	OwnerID    uint            `gorm:"not null;default:0;index;uniqueIndex:uidx_wallet_owner_currency,priority:3" json:"owner_id"`
	CurrencyID *uint           `gorm:"index;not null;uniqueIndex:uidx_wallet_owner_currency,priority:4" json:"currency_id"`

	// Deprecated compatibility columns. OwnerType and OwnerID are authoritative.
	UserID  *uint `gorm:"index" json:"user_id,omitempty"`
	TeamID  *uint `gorm:"index" json:"team_id,omitempty"`
	Balance int64 // in smallest unit (e.g. cents, satoshi)
	Freeze  int64 // frozen amount

	// 关联
	User     *User     `gorm:"foreignKey:UserID"`
	Team     *Team     `gorm:"foreignKey:TeamID"`
	Currency *Currency `gorm:"foreignKey:CurrencyID"`
	Txns     []WalletTx
}

func (w *Wallet) BeforeSave(_ *gorm.DB) error {
	if w.OwnerType == "" {
		switch {
		case w.UserID != nil && *w.UserID > 0:
			w.OwnerType = WalletOwnerUser
			w.OwnerID = *w.UserID
		case w.TeamID != nil && *w.TeamID > 0:
			w.OwnerType = WalletOwnerTeam
			w.OwnerID = *w.TeamID
		case w.TenantID > 0:
			w.OwnerType = WalletOwnerTenant
			w.OwnerID = w.TenantID
		}
	}

	if !w.OwnerType.IsValid() || w.OwnerID == 0 {
		return errors.New("wallet owner is invalid")
	}

	switch w.OwnerType {
	case WalletOwnerUser:
		ownerID := w.OwnerID
		w.UserID = &ownerID
		w.TeamID = nil
	case WalletOwnerTeam:
		ownerID := w.OwnerID
		w.TeamID = &ownerID
		w.UserID = nil
	default:
		w.UserID = nil
		w.TeamID = nil
	}
	return nil
}

// TableName 指定表名
func (Wallet) TableName() string {
	return "market_wallets"
}

// WalletTx represents a transaction on a wallet.
type WalletTx struct {
	gorm.Model
	WalletID       uint    `gorm:"index;index:idx_wallet_tx_idempotency,priority:1"`
	Type           string  `gorm:"size:32"` // recharge, withdraw, transfer
	Amount         int64   // positive or negative depending on Type
	Status         string  `gorm:"size:32"`  // pending, success, fail
	Reference      string  `gorm:"size:128"` // optional external ref
	IdempotencyKey *string `gorm:"size:128;index:idx_wallet_tx_idempotency,priority:2" json:"idempotency_key,omitempty"`
	Wallet         Wallet  `gorm:"foreignKey:WalletID"`
}

// TableName 指定表名
func (WalletTx) TableName() string {
	return "market_wallet_txes"
}

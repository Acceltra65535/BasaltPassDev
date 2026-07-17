package model

import "gorm.io/gorm"

// Currency represents a supported currency in the system
type Currency struct {
	gorm.Model
	Code            string  `gorm:"size:16;uniqueIndex;not null" json:"code"` // 货币代码，如 USD, CNY, BTC
	Name            string  `gorm:"size:64;not null" json:"name"`             // 货币名称，如 US Dollar, Chinese Yuan, Bitcoin
	NameCN          string  `gorm:"size:64" json:"name_cn"`                   // 中文名称，如 美元, 人民币, 比特币
	Symbol          string  `gorm:"size:8" json:"symbol"`                     // 货币符号，如 $, ¥, ₿
	DecimalPlaces   int     `gorm:"default:2" json:"decimal_places"`          // 小数位数，如 2 (美元), 8 (比特币)
	Type            string  `gorm:"size:16;default:'fiat'" json:"type"`       // 货币类型：fiat(法币), crypto(加密货币), points(积分)
	ExchangeRateUSD float64 `gorm:"default:0" json:"exchange_rate_usd"`       // 1 个该币种折合多少 USD，用于钱包充值换算
	PaymentEnabled  bool    `gorm:"default:false" json:"payment_enabled"`     // 是否可作为收银台付款币种
	IsActive        bool    `gorm:"default:true" json:"is_active"`            // 是否启用
	SortOrder       int     `gorm:"default:0" json:"sort_order"`              // 排序顺序
	Description     string  `gorm:"size:255" json:"description"`              // 描述
	IconURL         string  `gorm:"size:255" json:"icon_url"`                 // 图标URL

	// 关联
	Wallets []Wallet `gorm:"foreignKey:CurrencyID"`
}

// TableName returns the table name for Currency model
func (Currency) TableName() string {
	return "market_currencies"
}

// CurrencyRate represents a directional exchange rate pair.
// Rate means 1 BaseCurrencyCode = Rate QuoteCurrencyCode.
type CurrencyRate struct {
	gorm.Model
	BaseCurrencyCode  string  `gorm:"size:16;not null;uniqueIndex:idx_currency_rate_pair" json:"base_currency_code"`
	QuoteCurrencyCode string  `gorm:"size:16;not null;uniqueIndex:idx_currency_rate_pair" json:"quote_currency_code"`
	Rate              float64 `gorm:"not null" json:"rate"`
	Source            string  `gorm:"size:64;default:'manual'" json:"source"`
	IsActive          bool    `gorm:"default:true" json:"is_active"`
	Description       string  `gorm:"size:255" json:"description"`
}

func (CurrencyRate) TableName() string {
	return "market_currency_rates"
}

// AppWalletCurrency links an application to wallet currencies it uses.
type AppWalletCurrency struct {
	gorm.Model
	TenantID       uint   `gorm:"not null;index;uniqueIndex:idx_app_wallet_currency" json:"tenant_id"`
	AppID          uint   `gorm:"not null;index;uniqueIndex:idx_app_wallet_currency" json:"app_id"`
	CurrencyID     uint   `gorm:"not null;index;uniqueIndex:idx_app_wallet_currency" json:"currency_id"`
	WalletCategory string `gorm:"size:32;not null;default:'top_up';uniqueIndex:idx_app_wallet_currency" json:"wallet_category"`
	SortOrder      int    `gorm:"default:0" json:"sort_order"`
	IsDefault      bool   `gorm:"default:false" json:"is_default"`

	App      App      `gorm:"foreignKey:AppID" json:"app,omitempty"`
	Currency Currency `gorm:"foreignKey:CurrencyID" json:"currency,omitempty"`
}

func (AppWalletCurrency) TableName() string {
	return "app_wallet_currencies"
}

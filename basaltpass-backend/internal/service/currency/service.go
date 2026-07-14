package currency

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"fmt"
	"strings"
)

// GetAllCurrencies returns all active currencies
func GetAllCurrencies() ([]model.Currency, error) {
	var currencies []model.Currency
	db := common.DB()
	err := db.Where("is_active = ?", true).Order("sort_order, code").Find(&currencies).Error
	return currencies, err
}

// GetCurrencyByCode returns a currency by its code
func GetCurrencyByCode(code string) (model.Currency, error) {
	var currency model.Currency
	db := common.DB()
	err := db.Where("code = ? AND is_active = ?", code, true).First(&currency).Error
	return currency, err
}

// GetCurrencyByID returns a currency by its ID
func GetCurrencyByID(id uint) (model.Currency, error) {
	var currency model.Currency
	db := common.DB()
	err := db.Where("id = ? AND is_active = ?", id, true).First(&currency).Error
	return currency, err
}

// GetAllCurrencyRates returns all active directional currency rates.
func GetAllCurrencyRates() ([]model.CurrencyRate, error) {
	var rates []model.CurrencyRate
	db := common.DB()
	err := db.Where("is_active = ?", true).Order("base_currency_code, quote_currency_code").Find(&rates).Error
	return rates, err
}

// GetExchangeRate returns the rate for 1 base currency in quote currency.
func GetExchangeRate(baseCode string, quoteCode string) (float64, error) {
	baseCode = strings.ToUpper(strings.TrimSpace(baseCode))
	quoteCode = strings.ToUpper(strings.TrimSpace(quoteCode))
	if baseCode == "" || quoteCode == "" {
		return 0, fmt.Errorf("currency pair is required")
	}
	if baseCode == quoteCode {
		return 1, nil
	}

	db := common.DB()
	var exact model.CurrencyRate
	if err := db.Where("base_currency_code = ? AND quote_currency_code = ? AND is_active = ?", baseCode, quoteCode, true).First(&exact).Error; err == nil && exact.Rate > 0 {
		return exact.Rate, nil
	}

	var inverse model.CurrencyRate
	if err := db.Where("base_currency_code = ? AND quote_currency_code = ? AND is_active = ?", quoteCode, baseCode, true).First(&inverse).Error; err == nil && inverse.Rate > 0 {
		return 1 / inverse.Rate, nil
	}

	baseUSD, baseErr := getRateToUSD(baseCode)
	quoteUSD, quoteErr := getRateToUSD(quoteCode)
	if baseErr == nil && quoteErr == nil && baseUSD > 0 && quoteUSD > 0 {
		return baseUSD / quoteUSD, nil
	}

	return 0, fmt.Errorf("exchange rate is not configured for %s/%s", baseCode, quoteCode)
}

func getRateToUSD(code string) (float64, error) {
	if code == "USD" {
		return 1, nil
	}
	db := common.DB()
	var rate model.CurrencyRate
	if err := db.Where("base_currency_code = ? AND quote_currency_code = ? AND is_active = ?", code, "USD", true).First(&rate).Error; err == nil && rate.Rate > 0 {
		return rate.Rate, nil
	}
	if err := db.Where("base_currency_code = ? AND quote_currency_code = ? AND is_active = ?", "USD", code, true).First(&rate).Error; err == nil && rate.Rate > 0 {
		return 1 / rate.Rate, nil
	}
	currency, err := GetCurrencyByCode(code)
	if err != nil {
		return 0, err
	}
	if currency.ExchangeRateUSD <= 0 {
		return 0, fmt.Errorf("exchange rate is not configured for %s", code)
	}
	return currency.ExchangeRateUSD, nil
}

// CreateCurrency creates a new currency
func CreateCurrency(currency *model.Currency) error {
	db := common.DB()
	return db.Create(currency).Error
}

// UpdateCurrency updates an existing currency
func UpdateCurrency(currency *model.Currency) error {
	db := common.DB()
	return db.Save(currency).Error
}

// DeleteCurrency soft deletes a currency (sets is_active to false)
func DeleteCurrency(id uint) error {
	db := common.DB()
	return db.Model(&model.Currency{}).Where("id = ?", id).Update("is_active", false).Error
}

// InitDefaultCurrencies initializes the system with default currencies
func InitDefaultCurrencies() error {
	db := common.DB()

	// Check if currencies already exist
	var count int64
	db.Model(&model.Currency{}).Count(&count)
	if count > 0 {
		return nil // Already initialized
	}

	defaultCurrencies := []model.Currency{
		{
			Code:            "USD",
			Name:            "US Dollar",
			NameCN:          "美元",
			Symbol:          "$",
			DecimalPlaces:   2,
			Type:            "fiat",
			ExchangeRateUSD: 1,
			PaymentEnabled:  true,
			IsActive:        true,
			SortOrder:       1,
			Description:     "United States Dollar",
		},
		{
			Code:            "CNY",
			Name:            "Chinese Yuan",
			NameCN:          "人民币",
			Symbol:          "¥",
			DecimalPlaces:   2,
			Type:            "fiat",
			ExchangeRateUSD: 0.14,
			PaymentEnabled:  true,
			IsActive:        true,
			SortOrder:       2,
			Description:     "Chinese Yuan Renminbi",
		},
		{
			Code:            "EUR",
			Name:            "Euro",
			NameCN:          "欧元",
			Symbol:          "€",
			DecimalPlaces:   2,
			Type:            "fiat",
			ExchangeRateUSD: 1.08,
			PaymentEnabled:  true,
			IsActive:        true,
			SortOrder:       3,
			Description:     "European Euro",
		},
		{
			Code:            "BTC",
			Name:            "Bitcoin",
			NameCN:          "比特币",
			Symbol:          "₿",
			DecimalPlaces:   8,
			Type:            "crypto",
			ExchangeRateUSD: 1,
			PaymentEnabled:  false,
			IsActive:        true,
			SortOrder:       10,
			Description:     "Bitcoin cryptocurrency",
		},
		{
			Code:            "ETH",
			Name:            "Ethereum",
			NameCN:          "以太坊",
			Symbol:          "Ξ",
			DecimalPlaces:   18,
			Type:            "crypto",
			ExchangeRateUSD: 1,
			PaymentEnabled:  false,
			IsActive:        true,
			SortOrder:       11,
			Description:     "Ethereum cryptocurrency",
		},
		{
			Code:            "CREDIT",
			Name:            "Credit",
			NameCN:          "信用点",
			Symbol:          "C",
			DecimalPlaces:   0,
			Type:            "points",
			ExchangeRateUSD: 0.000001,
			PaymentEnabled:  false,
			IsActive:        true,
			SortOrder:       19,
			Description:     "User credit point wallet unit; 1 USD = 1,000,000 CREDIT",
		},
		{
			Code:            "POINTS",
			Name:            "System Points",
			NameCN:          "系统积分",
			Symbol:          "P",
			DecimalPlaces:   0,
			Type:            "points",
			ExchangeRateUSD: 0.01,
			PaymentEnabled:  false,
			IsActive:        true,
			SortOrder:       20,
			Description:     "System reward points",
		},
	}

	for _, currency := range defaultCurrencies {
		if err := db.Create(&currency).Error; err != nil {
			return err
		}
	}

	return nil
}

// predefinedCurrencyCatalog 提供一份可初始化的货币目录（代码 -> Currency 模板）
func predefinedCurrencyCatalog() map[string]model.Currency {
	return map[string]model.Currency{
		"USD":    {Code: "USD", Name: "US Dollar", NameCN: "美元", Symbol: "$", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 1, PaymentEnabled: true, IsActive: true, SortOrder: 1, Description: "United States Dollar"},
		"CNY":    {Code: "CNY", Name: "Chinese Yuan", NameCN: "人民币", Symbol: "¥", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 0.14, PaymentEnabled: true, IsActive: true, SortOrder: 2, Description: "Chinese Yuan Renminbi"},
		"EUR":    {Code: "EUR", Name: "Euro", NameCN: "欧元", Symbol: "€", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 1.08, PaymentEnabled: true, IsActive: true, SortOrder: 3, Description: "European Euro"},
		"GBP":    {Code: "GBP", Name: "British Pound", NameCN: "英镑", Symbol: "£", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 1.26, PaymentEnabled: true, IsActive: true, SortOrder: 4, Description: "Great Britain Pound"},
		"JPY":    {Code: "JPY", Name: "Japanese Yen", NameCN: "日元", Symbol: "¥", DecimalPlaces: 0, Type: "fiat", ExchangeRateUSD: 0.0063, PaymentEnabled: true, IsActive: true, SortOrder: 5, Description: "Japanese Yen"},
		"KRW":    {Code: "KRW", Name: "Korean Won", NameCN: "韩元", Symbol: "₩", DecimalPlaces: 0, Type: "fiat", ExchangeRateUSD: 0.00073, PaymentEnabled: true, IsActive: true, SortOrder: 6, Description: "Korean Won"},
		"HKD":    {Code: "HKD", Name: "Hong Kong Dollar", NameCN: "港币", Symbol: "$", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 0.13, PaymentEnabled: true, IsActive: true, SortOrder: 7, Description: "Hong Kong Dollar"},
		"INR":    {Code: "INR", Name: "Indian Rupee", NameCN: "印度卢比", Symbol: "₹", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 0.012, PaymentEnabled: true, IsActive: true, SortOrder: 8, Description: "Indian Rupee"},
		"AUD":    {Code: "AUD", Name: "Australian Dollar", NameCN: "澳元", Symbol: "$", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 0.66, PaymentEnabled: true, IsActive: true, SortOrder: 9, Description: "Australian Dollar"},
		"CAD":    {Code: "CAD", Name: "Canadian Dollar", NameCN: "加元", Symbol: "$", DecimalPlaces: 2, Type: "fiat", ExchangeRateUSD: 0.73, PaymentEnabled: true, IsActive: true, SortOrder: 10, Description: "Canadian Dollar"},
		"BTC":    {Code: "BTC", Name: "Bitcoin", NameCN: "比特币", Symbol: "₿", DecimalPlaces: 8, Type: "crypto", ExchangeRateUSD: 1, IsActive: true, SortOrder: 50, Description: "Bitcoin cryptocurrency"},
		"ETH":    {Code: "ETH", Name: "Ethereum", NameCN: "以太坊", Symbol: "Ξ", DecimalPlaces: 18, Type: "crypto", ExchangeRateUSD: 1, IsActive: true, SortOrder: 51, Description: "Ethereum cryptocurrency"},
		"CREDIT": {Code: "CREDIT", Name: "Credit", NameCN: "信用点", Symbol: "C", DecimalPlaces: 0, Type: "points", ExchangeRateUSD: 0.000001, IsActive: true, SortOrder: 89, Description: "User credit point wallet unit; 1 USD = 1,000,000 CREDIT"},
		"POINTS": {Code: "POINTS", Name: "System Points", NameCN: "系统积分", Symbol: "P", DecimalPlaces: 0, Type: "points", ExchangeRateUSD: 0.01, IsActive: true, SortOrder: 90, Description: "System reward points"},
	}
}

// EnsurePaymentDefaults backfills exchange rates and checkout-enabled flags for existing currency rows.
func EnsurePaymentDefaults() error {
	db := common.DB()
	defaults := predefinedCurrencyCatalog()
	for code, tpl := range defaults {
		if code == "CREDIT" {
			if err := db.Model(&model.Currency{}).
				Where("code = ?", code).
				Updates(map[string]interface{}{
					"decimal_places":    tpl.DecimalPlaces,
					"type":              tpl.Type,
					"exchange_rate_usd": tpl.ExchangeRateUSD,
					"description":       tpl.Description,
					"payment_enabled":   tpl.PaymentEnabled,
				}).Error; err != nil {
				return err
			}
			continue
		}
		updates := map[string]interface{}{
			"payment_enabled": tpl.PaymentEnabled,
		}
		if tpl.ExchangeRateUSD > 0 {
			updates["exchange_rate_usd"] = tpl.ExchangeRateUSD
		}
		rateNeedsBackfill := "exchange_rate_usd IS NULL OR exchange_rate_usd <= 0"
		if code != "USD" && code != "CREDIT" && tpl.ExchangeRateUSD != 1 {
			rateNeedsBackfill = rateNeedsBackfill + " OR exchange_rate_usd = 1"
		}
		if err := db.Model(&model.Currency{}).
			Where("code = ? AND ("+rateNeedsBackfill+")", code).
			Updates(updates).Error; err != nil {
			return err
		}
		if err := db.Model(&model.Currency{}).
			Where("code = ? AND exchange_rate_usd > 0", code).
			Update("payment_enabled", tpl.PaymentEnabled).Error; err != nil {
			return err
		}
	}
	return ensureDefaultCurrencyRateOverrides(defaults)
}

// EnsureDefaultCurrencyRates seeds missing directional pair rates from the built-in USD rates.
func EnsureDefaultCurrencyRates() error {
	db := common.DB()
	defaults := predefinedCurrencyCatalog()
	for baseCode, base := range defaults {
		if base.ExchangeRateUSD <= 0 {
			continue
		}
		for quoteCode, quote := range defaults {
			if quote.ExchangeRateUSD <= 0 || baseCode == quoteCode {
				continue
			}
			rate := base.ExchangeRateUSD / quote.ExchangeRateUSD
			var count int64
			if err := db.Model(&model.CurrencyRate{}).
				Where("base_currency_code = ? AND quote_currency_code = ?", baseCode, quoteCode).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			item := model.CurrencyRate{
				BaseCurrencyCode:  baseCode,
				QuoteCurrencyCode: quoteCode,
				Rate:              rate,
				Source:            "system_default",
				IsActive:          true,
				Description:       fmt.Sprintf("Default rate: 1 %s = %g %s", baseCode, rate, quoteCode),
			}
			if err := db.Create(&item).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureDefaultCurrencyRateOverrides(defaults map[string]model.Currency) error {
	db := common.DB()
	for _, pair := range [][2]string{{"CREDIT", "USD"}, {"USD", "CREDIT"}} {
		baseCode := pair[0]
		quoteCode := pair[1]
		base := defaults[baseCode]
		quote := defaults[quoteCode]
		if base.ExchangeRateUSD <= 0 || quote.ExchangeRateUSD <= 0 {
			continue
		}
		rate := base.ExchangeRateUSD / quote.ExchangeRateUSD
		description := fmt.Sprintf("Default rate: 1 %s = %g %s", baseCode, rate, quoteCode)
		var existing model.CurrencyRate
		err := db.Where("base_currency_code = ? AND quote_currency_code = ?", baseCode, quoteCode).First(&existing).Error
		if err == nil {
			if updateErr := db.Model(&existing).Updates(map[string]interface{}{
				"rate":        rate,
				"source":      "system_default",
				"is_active":   true,
				"description": description,
			}).Error; updateErr != nil {
				return updateErr
			}
			continue
		}
		item := model.CurrencyRate{
			BaseCurrencyCode:  baseCode,
			QuoteCurrencyCode: quoteCode,
			Rate:              rate,
			Source:            "system_default",
			IsActive:          true,
			Description:       description,
		}
		if createErr := db.Create(&item).Error; createErr != nil {
			return createErr
		}
	}
	return nil
}

// InitCurrenciesByCodes 根据管理员选择的代码列表初始化货币。如果已存在则跳过。
func InitCurrenciesByCodes(codes []string) (created int, skipped int, err error) {
	db := common.DB()
	catalog := predefinedCurrencyCatalog()

	for _, raw := range codes {
		code := raw
		if code == "" {
			continue
		}
		// 统一为大写
		if code != strings.ToUpper(code) {
			code = strings.ToUpper(code)
		}
		tpl, ok := catalog[code]
		if !ok {
			// 对未收录的代码，按最小模板创建（可后续在后台编辑完善）
			tpl = model.Currency{Code: code, Name: code, NameCN: code, Symbol: code, DecimalPlaces: 2, Type: "fiat", IsActive: true}
		}
		var cnt int64
		if err2 := db.Model(&model.Currency{}).Where("code = ?", code).Count(&cnt).Error; err2 != nil {
			return created, skipped, err2
		}
		if cnt > 0 {
			skipped++
			continue
		}
		if err2 := db.Create(&tpl).Error; err2 != nil {
			return created, skipped, err2
		}
		created++
	}
	return created, skipped, nil
}

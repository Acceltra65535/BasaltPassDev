package user

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func serializeAppRechargeCurrency(link model.AppWalletCurrency) fiber.Map {
	curr := link.Currency
	return fiber.Map{
		"id":              curr.ID,
		"code":            curr.Code,
		"name":            curr.Name,
		"name_cn":         curr.NameCN,
		"symbol":          curr.Symbol,
		"decimal_places":  curr.DecimalPlaces,
		"type":            curr.Type,
		"payment_enabled": curr.PaymentEnabled,
		"is_active":       curr.IsActive,
		"description":     curr.Description,
		"icon_url":        curr.IconURL,
		"wallet_category": link.WalletCategory,
		"is_default":      link.IsDefault,
		"sort_order":      link.SortOrder,
	}
}

// GetAppRechargeConfigHandler returns wallet currencies linked to an app.
// GET /api/v1/user/apps/recharge-config?app_id=1
// GET /api/v1/user/apps/recharge-config?client_id=oauth-client-id
func GetAppRechargeConfigHandler(c *fiber.Ctx) error {
	activeTenantID, _ := c.Locals("tenantID").(uint)
	category := strings.TrimSpace(c.Query("category", "top_up"))
	if category == "" {
		category = "top_up"
	}

	var app model.App
	db := common.DB()
	if clientID := strings.TrimSpace(c.Query("client_id")); clientID != "" {
		var client model.OAuthClient
		query := db.Preload("App").Where("client_id = ? AND is_active = ?", clientID, true)
		if activeTenantID != 0 {
			query = query.Where("tenant_id = ?", activeTenantID)
		}
		if err := query.First(&client).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "app not found"})
		}
		app = client.App
	} else {
		appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
		if err != nil || appID == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "app_id or client_id is required"})
		}
		query := db.Where("id = ?", uint(appID))
		if activeTenantID != 0 {
			query = query.Where("tenant_id = ?", activeTenantID)
		}
		if err := query.First(&app).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "app not found"})
		}
	}

	if app.Status != model.AppStatusActive {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "app not found"})
	}

	var links []model.AppWalletCurrency
	if err := db.
		Preload("Currency").
		Where("tenant_id = ? AND app_id = ? AND wallet_category = ?", app.TenantID, app.ID, category).
		Order("sort_order ASC, id ASC").
		Find(&links).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	currencies := make([]fiber.Map, 0, len(links))
	for _, link := range links {
		if link.Currency.ID == 0 || !link.Currency.IsActive {
			continue
		}
		currencies = append(currencies, serializeAppRechargeCurrency(link))
	}

	return c.JSON(fiber.Map{
		"app": fiber.Map{
			"id":           app.ID,
			"tenant_id":    app.TenantID,
			"name":         app.Name,
			"description":  app.Description,
			"icon_url":     app.IconURL,
			"logo_url":     app.LogoURL,
			"homepage_url": app.HomepageURL,
		},
		"wallet_category": category,
		"currencies":      currencies,
	})
}

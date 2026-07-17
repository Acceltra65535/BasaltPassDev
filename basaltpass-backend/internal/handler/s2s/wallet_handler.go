package s2s

import (
	"errors"
	"strconv"
	"strings"

	"basaltpass-backend/internal/model"
	"basaltpass-backend/internal/service/wallet"

	"github.com/gofiber/fiber/v2"
)

type adjustOwnerWalletRequest struct {
	Operation string `json:"operation"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Reference string `json:"reference"`
}

func parseWalletOwner(c *fiber.Ctx) (model.WalletOwnerType, uint, uint, error) {
	ownerType := model.WalletOwnerType(strings.ToLower(strings.TrimSpace(c.Params("owner_type"))))
	if !ownerType.IsValid() {
		return "", 0, 0, errors.New("owner_type must be user, app, tenant, or team")
	}
	ownerID64, err := strconv.ParseUint(c.Params("owner_id"), 10, 32)
	if err != nil || ownerID64 == 0 {
		return "", 0, 0, errors.New("invalid owner_id")
	}
	tenantID, err := s2sTenantID(c)
	if err != nil {
		return "", 0, 0, err
	}
	return ownerType, uint(ownerID64), tenantID, nil
}

// GetOwnerWalletHandler reads one currency wallet for any tenant-scoped owner.
func GetOwnerWalletHandler(c *fiber.Ctx) error {
	ownerType, ownerID, tenantID, err := parseWalletOwner(c)
	if err != nil {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "invalid_parameter", "message": err.Error()})
	}
	currencyCode := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	if currencyCode == "" {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "invalid_parameter", "message": "currency is required"})
	}
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	walletModel, err := wallet.GetOwnerBalanceByCode(ownerType, ownerID, tenantID, currencyCode)
	if err != nil {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "wallet_error", "message": err.Error()})
	}
	txs, err := wallet.OwnerHistoryByCode(ownerType, ownerID, tenantID, currencyCode, limit)
	if err != nil {
		return unifiedResponse(c, fiber.StatusInternalServerError, nil, fiber.Map{"code": "wallet_error", "message": err.Error()})
	}
	return unifiedResponse(c, fiber.StatusOK, fiber.Map{
		"owner_type": ownerType, "owner_id": ownerID, "tenant_id": tenantID,
		"currency": currencyCode, "wallet_id": walletModel.ID,
		"balance": walletModel.Balance, "transactions": txs,
	}, nil)
}

// AdjustOwnerWalletHandler atomically adjusts one owner wallet.
func AdjustOwnerWalletHandler(c *fiber.Ctx) error {
	ownerType, ownerID, tenantID, err := parseWalletOwner(c)
	if err != nil {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "invalid_parameter", "message": err.Error()})
	}
	var req adjustOwnerWalletRequest
	if err := c.BodyParser(&req); err != nil {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "invalid_parameter", "message": "invalid JSON body"})
	}
	req.Operation = strings.ToLower(strings.TrimSpace(req.Operation))
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	req.Reference = strings.TrimSpace(req.Reference)
	if req.Operation != "increase" && req.Operation != "decrease" {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "invalid_parameter", "message": "operation must be increase or decrease"})
	}
	if req.Amount <= 0 || req.Currency == "" {
		return unifiedResponse(c, fiber.StatusBadRequest, nil, fiber.Map{"code": "invalid_parameter", "message": "positive amount and currency are required"})
	}

	delta := req.Amount
	txType := "s2s_wallet_increase"
	if req.Operation == "decrease" {
		delta = -req.Amount
		txType = "s2s_wallet_decrease"
	}
	walletModel, err := wallet.AdjustOwnerByCode(ownerType, ownerID, tenantID, req.Currency, delta, txType, req.Reference)
	if err != nil {
		status := fiber.StatusBadRequest
		if err.Error() == "insufficient funds" {
			status = fiber.StatusConflict
		}
		return unifiedResponse(c, status, nil, fiber.Map{"code": "wallet_error", "message": err.Error()})
	}
	return unifiedResponse(c, fiber.StatusOK, fiber.Map{
		"owner_type": ownerType, "owner_id": ownerID, "tenant_id": tenantID,
		"wallet_id": walletModel.ID, "currency": req.Currency,
		"operation": req.Operation, "amount": req.Amount,
		"balance_delta": delta, "balance": walletModel.Balance,
		"reference": req.Reference,
	}, nil)
}

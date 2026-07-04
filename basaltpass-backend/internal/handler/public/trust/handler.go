package trust

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type tokenRequest struct {
	ServiceID string   `json:"service_id"`
	Scopes    []string `json:"scopes"`
}

// IssueTrustTokenHandler provides the minimal trust-token contract used by
// APICred-gated services. The identity check is backed by a real BasaltPass
// OAuth client named infopipe-<service_id>; callers pass that client's secret
// as a bearer token.
func IssueTrustTokenHandler(c *fiber.Ctx) error {
	var req tokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid_json"})
	}
	serviceID := strings.TrimSpace(req.ServiceID)
	if serviceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "service_id_required"})
	}
	secret := bearer(c.Get("Authorization"))
	if secret == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing_service_secret"})
	}

	var client model.OAuthClient
	clientID := "infopipe-" + serviceID
	if err := common.DB().Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "service_not_registered"})
	}
	if !client.VerifyClientSecret(secret) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "service_secret_invalid"})
	}
	if !scopesAllowed(client.GetScopeList(), req.Scopes) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "scope_not_allowed"})
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "token_generation_failed"})
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	return c.JSON(fiber.Map{
		"token":            "trust:" + serviceID + ":" + hex.EncodeToString(nonce),
		"expires_at":       expiresAt.Format(time.RFC3339),
		"service_identity": serviceID,
		"scopes":           req.Scopes,
	})
}

func bearer(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func scopesAllowed(have []string, requested []string) bool {
	if len(requested) == 0 {
		return true
	}
	set := map[string]struct{}{}
	for _, scope := range have {
		set[strings.TrimSpace(scope)] = struct{}{}
	}
	for _, scope := range requested {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := set[scope]; !ok {
			return false
		}
	}
	return true
}

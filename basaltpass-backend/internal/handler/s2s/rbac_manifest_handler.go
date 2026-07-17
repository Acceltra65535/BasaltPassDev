package s2s

import (
	"errors"
	"strconv"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/service/rbacmanifest"

	"github.com/gofiber/fiber/v2"
)

// SubmitRBACManifestHandler accepts only the RBAC manifest schema. App,
// tenant, OAuth settings and user assignments are derived from authentication
// context or rejected as unknown JSON fields.
func SubmitRBACManifestHandler(c *fiber.Ctx) error {
	if authSource, _ := c.Locals("s2s_auth_source").(string); authSource == "query" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "query-string credentials are forbidden for RBAC manifest submission"})
	}
	tenantID, tenantOK := c.Locals("s2s_tenant_id").(uint)
	appID, appOK := c.Locals("s2s_app_id").(uint)
	clientID, clientOK := c.Locals("s2s_client_id").(string)
	if !tenantOK || !appOK || !clientOK || tenantID == 0 || appID == 0 || clientID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authenticated app context is required"})
	}
	if len(c.Body()) > rbacmanifest.MaxManifestBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": "RBAC manifest is too large"})
	}

	result, err := rbacmanifest.New(common.DB()).Submit(tenantID, appID, clientID, c.Body())
	if err != nil {
		return rbacManifestError(c, err)
	}
	status := fiber.StatusCreated
	if !result.Created {
		status = fiber.StatusOK
	}
	return c.Status(status).JSON(fiber.Map{"data": result})
}

func GetOwnRBACManifestHandler(c *fiber.Ctx) error {
	if authSource, _ := c.Locals("s2s_auth_source").(string); authSource == "query" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "query-string credentials are forbidden for RBAC manifest access"})
	}
	tenantID, tenantOK := c.Locals("s2s_tenant_id").(uint)
	appID, appOK := c.Locals("s2s_app_id").(uint)
	clientID, clientOK := c.Locals("s2s_client_id").(string)
	manifestID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid manifest ID"})
	}
	if !tenantOK || !appOK || !clientOK || tenantID == 0 || appID == 0 || clientID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authenticated app context is required"})
	}
	view, err := rbacmanifest.New(common.DB()).GetManifestForClient(tenantID, appID, clientID, uint(manifestID))
	if err != nil {
		return rbacManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": view})
}

func rbacManifestError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, rbacmanifest.ErrValidation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, rbacmanifest.ErrConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, rbacmanifest.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "RBAC manifest not found"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "RBAC manifest operation failed"})
	}
}

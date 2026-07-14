package tenant

import (
	"errors"
	"strconv"
	"strings"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/service/rbacmanifest"

	"github.com/gofiber/fiber/v2"
)

type rejectRBACManifestRequest struct {
	Note string `json:"note"`
}

func ListAppRBACManifestsHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := manifestTenantAppContext(c)
	if !ok {
		return nil
	}
	views, err := rbacmanifest.New(common.DB()).ListManifests(tenantID, appID)
	if err != nil {
		return tenantManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"manifests": views}})
}

func GetAppRBACManifestHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := manifestTenantAppContext(c)
	if !ok {
		return nil
	}
	manifestID, err := strconv.ParseUint(c.Params("manifest_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid manifest ID"})
	}
	view, err := rbacmanifest.New(common.DB()).GetManifest(tenantID, appID, uint(manifestID))
	if err != nil {
		return tenantManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": view})
}

func ApproveAppRBACManifestHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := manifestTenantAppContext(c)
	if !ok {
		return nil
	}
	manifestID, err := strconv.ParseUint(c.Params("manifest_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid manifest ID"})
	}
	reviewerID, ok := c.Locals("userID").(uint)
	if !ok || reviewerID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "reviewer context is required"})
	}
	view, err := rbacmanifest.New(common.DB()).Approve(tenantID, appID, uint(manifestID), reviewerID)
	if err != nil {
		return tenantManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": view, "message": "RBAC manifest approved and published"})
}

func RejectAppRBACManifestHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := manifestTenantAppContext(c)
	if !ok {
		return nil
	}
	manifestID, err := strconv.ParseUint(c.Params("manifest_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid manifest ID"})
	}
	var request rejectRBACManifestRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&request); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid rejection request"})
		}
	}
	reviewerID, ok := c.Locals("userID").(uint)
	if !ok || reviewerID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "reviewer context is required"})
	}
	view, err := rbacmanifest.New(common.DB()).Reject(tenantID, appID, uint(manifestID), reviewerID, strings.TrimSpace(request.Note))
	if err != nil {
		return tenantManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": view, "message": "RBAC manifest rejected"})
}

func ListAppRBACRevisionsHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := manifestTenantAppContext(c)
	if !ok {
		return nil
	}
	views, err := rbacmanifest.New(common.DB()).ListRevisions(tenantID, appID)
	if err != nil {
		return tenantManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"revisions": views}})
}

func RollbackAppRBACRevisionHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := manifestTenantAppContext(c)
	if !ok {
		return nil
	}
	revisionID, err := strconv.ParseUint(c.Params("revision_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid revision ID"})
	}
	reviewerID, ok := c.Locals("userID").(uint)
	if !ok || reviewerID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "reviewer context is required"})
	}
	view, err := rbacmanifest.New(common.DB()).Rollback(tenantID, appID, uint(revisionID), reviewerID)
	if err != nil {
		return tenantManifestError(c, err)
	}
	return c.JSON(fiber.Map{"data": view, "message": "RBAC revision rolled back atomically"})
}

func manifestTenantAppContext(c *fiber.Ctx) (uint, uint, bool) {
	tenantID, ok := c.Locals("tenantID").(uint)
	if !ok || tenantID == 0 {
		_ = c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "tenant context is required"})
		return 0, 0, false
	}
	appID, err := strconv.ParseUint(c.Params("app_id"), 10, 32)
	if err != nil || appID == 0 {
		_ = c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid app ID"})
		return 0, 0, false
	}
	return tenantID, uint(appID), true
}

func tenantManifestError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, rbacmanifest.ErrValidation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, rbacmanifest.ErrConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, rbacmanifest.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "RBAC resource not found"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "RBAC manifest operation failed"})
	}
}

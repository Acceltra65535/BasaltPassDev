package tenant

import (
	"errors"
	"strconv"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/service/aduit"
	"basaltpass-backend/internal/service/appgrant"
	"basaltpass-backend/internal/utils"

	"github.com/gofiber/fiber/v2"
)

var appGrantNow = func() time.Time { return time.Now().UTC() }

func ListAppGrantMappingsHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	items, err := appgrant.NewService(common.DB()).ListMappings(tenantID, appID)
	if err != nil {
		return appGrantError(c, err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"mappings": items}})
}

func GetAppGrantMappingOptionsHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	options, err := appgrant.NewService(common.DB()).Options(tenantID, appID)
	if err != nil {
		return appGrantError(c, err)
	}
	return c.JSON(fiber.Map{"data": options})
}

func PreviewAppGrantMappingHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	var input appgrant.MappingInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	count, err := appgrant.NewService(common.DB()).PreviewAffectedUsers(tenantID, appID, input)
	if err != nil {
		return appGrantError(c, err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"affected_user_count": count}})
}

func CreateAppGrantMappingHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	actorID, err := utils.PositiveUintFromAny(c.Locals("userID"))
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
	}
	var input appgrant.MappingInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	created, err := appgrant.NewService(common.DB()).CreateMapping(tenantID, appID, actorID, input)
	if err != nil {
		return appGrantError(c, err)
	}
	aduit.LogAuditWithDetails(actorID, "tenant_app_grant_mapping_create", "tenant_app_grant_mapping", strconv.FormatUint(uint64(created.ID), 10), c.IP(), c.Get("User-Agent"), created)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": created})
}

func UpdateAppGrantMappingHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	mappingID, err := utils.ParsePositiveUint(c.Params("mapping_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid mapping id"})
	}
	actorID, err := utils.PositiveUintFromAny(c.Locals("userID"))
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
	}
	var input appgrant.MappingInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	updated, err := appgrant.NewService(common.DB()).UpdateMapping(tenantID, appID, mappingID, actorID, input)
	if err != nil {
		return appGrantError(c, err)
	}
	aduit.LogAuditWithDetails(actorID, "tenant_app_grant_mapping_update", "tenant_app_grant_mapping", strconv.FormatUint(uint64(updated.ID), 10), c.IP(), c.Get("User-Agent"), updated)
	return c.JSON(fiber.Map{"data": updated})
}

func DeleteAppGrantMappingHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	mappingID, err := utils.ParsePositiveUint(c.Params("mapping_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid mapping id"})
	}
	actorID, err := utils.PositiveUintFromAny(c.Locals("userID"))
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user context"})
	}
	deleted, err := appgrant.NewService(common.DB()).DeleteMappingWithView(tenantID, appID, mappingID)
	if err != nil {
		return appGrantError(c, err)
	}
	aduit.LogAuditWithDetails(actorID, "tenant_app_grant_mapping_delete", "tenant_app_grant_mapping", strconv.FormatUint(uint64(mappingID), 10), c.IP(), c.Get("User-Agent"), deleted)
	return c.SendStatus(fiber.StatusNoContent)
}

func GetEffectiveAppUserGrantsHandler(c *fiber.Ctx) error {
	tenantID, appID, ok := appGrantContext(c)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid tenant or app context"})
	}
	userID, err := utils.ParsePositiveUint(c.Params("user_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	result, err := appgrant.NewService(common.DB()).Resolve(userID, tenantID, appID, appGrantNow())
	if err != nil {
		return appGrantError(c, err)
	}
	return c.JSON(fiber.Map{"data": result})
}

func appGrantContext(c *fiber.Ctx) (uint, uint, bool) {
	tenantID, err := utils.PositiveUintFromAny(c.Locals("tenantID"))
	if err != nil {
		return 0, 0, false
	}
	appID, err := utils.ParsePositiveUint(c.Params("app_id"))
	if err != nil {
		return 0, 0, false
	}
	return tenantID, appID, true
}

func appGrantError(c *fiber.Ctx, err error) error {
	var validation *appgrant.ValidationError
	switch {
	case errors.Is(err, appgrant.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, appgrant.ErrConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.As(err, &validation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": validation.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "app grant mapping operation failed"})
	}
}

package tenant

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func setupTenantUserUUIDHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.TenantUser{},
		&model.TenantRbacRole{},
		&model.TenantRbacPermission{},
		&model.TenantRbacRolePermission{},
		&model.TenantUserRbacRole{},
		&model.TenantUserRbacPermission{},
		&model.App{},
		&model.AppUser{},
		&model.AppRole{},
		&model.AppPermission{},
		&model.AppUserRole{},
		&model.AppUserPermission{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	common.SetDBForTest(db)
	return db
}

func newTenantUserUUIDTestApp(tenantID uint) *fiber.App {
	app := fiber.New()
	app.Get("/tenant/users/by-uuid/:user_uuid", func(c *fiber.Ctx) error {
		c.Locals("tenantID", tenantID)
		return GetTenantUserByUUIDHandler(c)
	})
	return app
}

func newTenantUsersTestApp(tenantID uint) *fiber.App {
	app := fiber.New()
	app.Get("/tenant/users", func(c *fiber.Ctx) error {
		c.Locals("tenantID", tenantID)
		return GetTenantUsersHandler(c)
	})
	app.Get("/tenant/users/:id", func(c *fiber.Ctx) error {
		c.Locals("tenantID", tenantID)
		return GetTenantUserHandler(c)
	})
	return app
}

func TestGetTenantUserByUUIDHandler_Success(t *testing.T) {
	db := setupTenantUserUUIDHandlerTestDB(t)
	tenantID := uint(2001)

	u := model.User{
		EnforcedTenantID: tenantID,
		Email:            "tenant-uuid-user@example.com",
		PasswordHash:     "x",
		Nickname:         "tenant-user",
		EmailVerified:    true,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&model.TenantUser{UserID: u.ID, TenantID: tenantID, Role: model.TenantRoleMember}).Error; err != nil {
		t.Fatalf("create tenant user failed: %v", err)
	}

	app := newTenantUserUUIDTestApp(tenantID)
	req := httptest.NewRequest(http.MethodGet, "/tenant/users/by-uuid/"+u.UserUUID, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		User TenantUserResponse `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.User.ID != u.ID {
		t.Fatalf("expected id %d, got %d", u.ID, payload.User.ID)
	}
	if payload.User.UserUUID != u.UserUUID {
		t.Fatalf("expected user_uuid %q, got %q", u.UserUUID, payload.User.UserUUID)
	}
}

func TestGetTenantUserByUUIDHandler_InvalidUUID(t *testing.T) {
	setupTenantUserUUIDHandlerTestDB(t)
	app := newTenantUserUUIDTestApp(1)

	req := httptest.NewRequest(http.MethodGet, "/tenant/users/by-uuid/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetTenantUserByUUIDHandler_WrongTenant(t *testing.T) {
	db := setupTenantUserUUIDHandlerTestDB(t)
	ownerTenantID := uint(3001)
	otherTenantID := uint(3002)

	u := model.User{
		EnforcedTenantID: ownerTenantID,
		Email:            "other-tenant-user@example.com",
		PasswordHash:     "x",
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	app := newTenantUserUUIDTestApp(otherTenantID)
	req := httptest.NewRequest(http.MethodGet, "/tenant/users/by-uuid/"+u.UserUUID, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetTenantUsersHandlerIncludesAuthorizedAppCount(t *testing.T) {
	db := setupTenantUserUUIDHandlerTestDB(t)
	tenantID := uint(4001)
	otherTenantID := uint(4002)

	u := model.User{
		EnforcedTenantID: tenantID,
		Email:            "tenant-app-count@example.com",
		PasswordHash:     "x",
		Nickname:         "app-count-user",
		EmailVerified:    true,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&model.TenantUser{UserID: u.ID, TenantID: tenantID, Role: model.TenantRoleMember}).Error; err != nil {
		t.Fatalf("create tenant user failed: %v", err)
	}

	apps := []model.App{
		{TenantID: tenantID, Name: "App One", Status: model.AppStatusActive},
		{TenantID: tenantID, Name: "App Two", Status: model.AppStatusActive},
		{TenantID: otherTenantID, Name: "Other Tenant App", Status: model.AppStatusActive},
	}
	if err := db.Create(&apps).Error; err != nil {
		t.Fatalf("create apps failed: %v", err)
	}

	now := time.Now()
	appUsers := []model.AppUser{
		{AppID: apps[0].ID, UserID: u.ID, FirstAuthorizedAt: now, LastAuthorizedAt: now, LastActiveAt: &now, Status: model.AppUserStatusActive},
		{AppID: apps[1].ID, UserID: u.ID, FirstAuthorizedAt: now, LastAuthorizedAt: now, LastActiveAt: &now, Status: model.AppUserStatusActive},
		{AppID: apps[2].ID, UserID: u.ID, FirstAuthorizedAt: now, LastAuthorizedAt: now, LastActiveAt: &now, Status: model.AppUserStatusActive},
	}
	if err := db.Create(&appUsers).Error; err != nil {
		t.Fatalf("create app users failed: %v", err)
	}

	app := newTenantUsersTestApp(tenantID)
	req := httptest.NewRequest(http.MethodGet, "/tenant/users", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Users []TenantUserResponse `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(payload.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(payload.Users))
	}
	if payload.Users[0].AppCount != 2 {
		t.Fatalf("expected app_count 2, got %d", payload.Users[0].AppCount)
	}
}

func TestGetTenantUserHandlerIncludesAccessDetail(t *testing.T) {
	db := setupTenantUserUUIDHandlerTestDB(t)
	tenantID := uint(5001)

	u := model.User{
		EnforcedTenantID: tenantID,
		Email:            "tenant-detail@example.com",
		PasswordHash:     "x",
		Nickname:         "detail-user",
		EmailVerified:    true,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&model.TenantUser{UserID: u.ID, TenantID: tenantID, Role: model.TenantRoleMember}).Error; err != nil {
		t.Fatalf("create tenant user failed: %v", err)
	}

	tenantPerm := model.TenantRbacPermission{TenantID: tenantID, Code: "tenant.users.read", Name: "Read Users", Category: "users"}
	if err := db.Create(&tenantPerm).Error; err != nil {
		t.Fatalf("create tenant permission failed: %v", err)
	}
	tenantRole := model.TenantRbacRole{TenantID: tenantID, Code: "tenant-reader", Name: "Tenant Reader"}
	if err := db.Create(&tenantRole).Error; err != nil {
		t.Fatalf("create tenant role failed: %v", err)
	}
	if err := db.Create(&model.TenantRbacRolePermission{RoleID: tenantRole.ID, PermissionID: tenantPerm.ID}).Error; err != nil {
		t.Fatalf("create tenant role permission failed: %v", err)
	}
	if err := db.Create(&model.TenantUserRbacRole{UserID: u.ID, TenantID: tenantID, RoleID: tenantRole.ID, AssignedBy: u.ID, AssignedAt: time.Now()}).Error; err != nil {
		t.Fatalf("assign tenant role failed: %v", err)
	}

	app := model.App{TenantID: tenantID, Name: "Connected App", Status: model.AppStatusActive}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app failed: %v", err)
	}
	now := time.Now()
	if err := db.Create(&model.AppUser{AppID: app.ID, UserID: u.ID, FirstAuthorizedAt: now, LastAuthorizedAt: now, Status: model.AppUserStatusActive}).Error; err != nil {
		t.Fatalf("create app user failed: %v", err)
	}
	appPerm := model.AppPermission{TenantID: tenantID, AppID: app.ID, Code: "app.profile.read", Name: "Read Profile", Category: "profile"}
	if err := db.Create(&appPerm).Error; err != nil {
		t.Fatalf("create app permission failed: %v", err)
	}
	appRole := model.AppRole{TenantID: tenantID, AppID: app.ID, Code: "app-reader", Name: "App Reader"}
	if err := db.Create(&appRole).Error; err != nil {
		t.Fatalf("create app role failed: %v", err)
	}
	if err := db.Model(&appRole).Association("Permissions").Append(&appPerm); err != nil {
		t.Fatalf("append app permission failed: %v", err)
	}
	if err := db.Create(&model.AppUserRole{UserID: u.ID, AppID: app.ID, RoleID: appRole.ID, AssignedBy: u.ID, AssignedAt: now}).Error; err != nil {
		t.Fatalf("assign app role failed: %v", err)
	}

	fiberApp := newTenantUsersTestApp(tenantID)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tenant/users/%d", u.ID), nil)
	resp, err := fiberApp.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload TenantUserDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(payload.TenantRoles) != 1 {
		t.Fatalf("expected 1 tenant role, got %d", len(payload.TenantRoles))
	}
	if len(payload.TenantPermissions) != 1 {
		t.Fatalf("expected 1 tenant permission, got %d", len(payload.TenantPermissions))
	}
	if len(payload.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(payload.Apps))
	}
	if len(payload.Apps[0].Roles) != 1 {
		t.Fatalf("expected 1 app role, got %d", len(payload.Apps[0].Roles))
	}
	if len(payload.Apps[0].Permissions) != 1 {
		t.Fatalf("expected 1 app permission, got %d", len(payload.Apps[0].Permissions))
	}
}

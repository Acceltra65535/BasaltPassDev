package s2s

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func TestS2SPermissionsUseDynamicTenantMappingAndStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:s2s-grants-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Tenant{}, &model.TenantUser{}, &model.App{}, &model.AppUser{},
		&model.AppRole{}, &model.AppPermission{}, &model.AppUserRole{}, &model.AppUserPermission{},
		&model.TenantRbacRole{}, &model.TenantRbacPermission{}, &model.TenantUserRbacRole{}, &model.TenantUserRbacPermission{},
		&model.TenantRbacRolePermission{}, &model.TenantAppGrantMapping{}); err != nil {
		t.Fatal(err)
	}
	common.SetDBForTest(db)

	tenant := model.Tenant{Name: "Tenant", Code: "s2s-dynamic", Status: model.TenantStatusActive}
	db.Create(&tenant)
	appModel := model.App{TenantID: tenant.ID, Name: "App", Status: model.AppStatusActive}
	db.Create(&appModel)
	user := model.User{Email: "s2s-dynamic@example.com", EmailVerified: true}
	db.Create(&user)
	membership := model.TenantUser{TenantID: tenant.ID, UserID: user.ID, Role: model.TenantRoleMember}
	db.Create(&membership)
	now := time.Now().UTC()
	db.Create(&model.AppUser{AppID: appModel.ID, UserID: user.ID, Status: model.AppUserStatusActive, FirstAuthorizedAt: now, LastAuthorizedAt: now})
	permission := model.AppPermission{TenantID: tenant.ID, AppID: appModel.ID, Code: "demo.read", Name: "Read", Category: "demo"}
	role := model.AppRole{TenantID: tenant.ID, AppID: appModel.ID, Code: "demo.user", Name: "User"}
	db.Create(&permission)
	db.Create(&role)
	db.Model(&role).Association("Permissions").Append(&permission)
	mapping := model.TenantAppGrantMapping{TenantID: tenant.ID, AppID: appModel.ID, SourceType: model.TenantAppGrantSourceMembershipRole,
		SourceCode: "member", TargetType: model.TenantAppGrantTargetAppRole, TargetID: role.ID, Enabled: true, CreatedBy: user.ID, UpdatedBy: user.ID}
	db.Create(&mapping)

	api := fiber.New()
	api.Get("/users/:id/permissions", func(c *fiber.Ctx) error {
		c.Locals("s2s_tenant_id", tenant.ID)
		c.Locals("s2s_app_id", appModel.ID)
		return GetUserPermissionsHandler(c)
	})

	request := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/users/%d/permissions", user.ID), nil)
		resp, requestErr := api.Test(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status %d", resp.StatusCode)
		}
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload["data"].(map[string]any)
	}

	data := request()
	permissions := data["permission_codes"].([]any)
	roles := data["role_codes"].([]any)
	if len(permissions) != 1 || permissions[0] != "demo.read" || len(roles) != 1 || roles[0] != "demo.user" || data["eligible"] != true {
		t.Fatalf("unexpected dynamic grants: %+v", data)
	}
	var physical int64
	db.Model(&model.AppUserRole{}).Count(&physical)
	if physical != 0 {
		t.Fatalf("S2S materialized inherited role rows: %d", physical)
	}

	db.Model(&membership).Update("role", model.TenantRoleBanned)
	data = request()
	if data["eligible"] != false || len(data["permission_codes"].([]any)) != 0 || data["denial_reason"] != "tenant_membership_banned" {
		t.Fatalf("banned membership retained grants: %+v", data)
	}
}

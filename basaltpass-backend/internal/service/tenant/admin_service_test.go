package tenant

import (
	"testing"

	"basaltpass-backend/internal/common"
	admindto "basaltpass-backend/internal/dto/tenant"
	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAdminTenantServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Tenant{},
		&model.TenantUser{},
		&model.TenantQuota{},
		&model.TenantAuthSetting{},
		&model.TenantRbacPermission{},
		&model.TenantRbacRole{},
		&model.TenantRbacRolePermission{},
		&model.TenantUserRbacRole{},
		&model.ManualAPIKey{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	common.SetDBForTest(db)
	return db
}

func TestAdminCreateTenantBootstrapsTenantRBAC(t *testing.T) {
	db := setupAdminTenantServiceTestDB(t)

	owner := model.User{
		Email:        "owner@example.com",
		PasswordHash: "x",
	}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}

	svc := NewAdminTenantService(db)
	created, err := svc.CreateTenant(admindto.AdminCreateTenantRequest{
		Name:             "Admin Tenant",
		Code:             "admin-tenant",
		Description:      "test tenant",
		OwnerEmail:       owner.Email,
		MaxApps:          10,
		MaxUsers:         50,
		MaxTokensPerHour: 5000,
	})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}
	tenant := created.Tenant
	if created.AutomationKey == "" {
		t.Fatalf("expected automation key to be returned")
	}

	var adminRole model.TenantRbacRole
	if err := db.Where("tenant_id = ? AND code = ?", tenant.ID, "admin").First(&adminRole).Error; err != nil {
		t.Fatalf("load admin role failed: %v", err)
	}

	var updatedOwner model.User
	if err := db.First(&updatedOwner, owner.ID).Error; err != nil {
		t.Fatalf("reload owner failed: %v", err)
	}
	if updatedOwner.TenantID != 0 {
		t.Fatalf("expected owner to remain a system-level user, got tenant_id %d", updatedOwner.TenantID)
	}

	var roleCount int64
	if err := db.Model(&model.TenantRbacRole{}).Where("tenant_id = ?", tenant.ID).Count(&roleCount).Error; err != nil {
		t.Fatalf("count roles failed: %v", err)
	}
	if roleCount < 5 {
		t.Fatalf("expected default tenant roles to be bootstrapped, got %d", roleCount)
	}

	var ownerRole model.TenantRbacRole
	if err := db.Where("tenant_id = ? AND code = ?", tenant.ID, "owner").First(&ownerRole).Error; err != nil {
		t.Fatalf("load owner role failed: %v", err)
	}

	var ownerAssignment model.TenantUserRbacRole
	if err := db.Where("tenant_id = ? AND user_id = ? AND role_id = ?", tenant.ID, owner.ID, ownerRole.ID).First(&ownerAssignment).Error; err != nil {
		t.Fatalf("expected owner rbac role assignment: %v", err)
	}

	var permissionCount int64
	if err := db.Model(&model.TenantRbacPermission{}).Where("tenant_id = ?", tenant.ID).Count(&permissionCount).Error; err != nil {
		t.Fatalf("count permissions failed: %v", err)
	}
	if permissionCount == 0 {
		t.Fatalf("expected bootstrapped permissions for admin tenant")
	}

	var adminLinks int64
	if err := db.Model(&model.TenantRbacRolePermission{}).Where("role_id = ?", adminRole.ID).Count(&adminLinks).Error; err != nil {
		t.Fatalf("count admin role permissions failed: %v", err)
	}
	if adminLinks != permissionCount {
		t.Fatalf("expected admin role to have all permissions, got %d of %d", adminLinks, permissionCount)
	}
}

func TestAdminCreateTenantCreatesSystemOwnerWhenMissing(t *testing.T) {
	db := setupAdminTenantServiceTestDB(t)
	svc := NewAdminTenantService(db)

	created, err := svc.CreateTenant(admindto.AdminCreateTenantRequest{
		Name:             "New Owner Tenant",
		Code:             "new-owner-tenant",
		OwnerEmail:       "new-owner@example.com",
		OwnerUsername:    "New Owner",
		OwnerPassword:    "StrongPassw0rd!",
		MaxApps:          10,
		MaxUsers:         50,
		MaxTokensPerHour: 5000,
	})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}
	if created.Tenant == nil || created.AutomationKey == "" {
		t.Fatalf("expected tenant and automation key")
	}

	var owner model.User
	if err := db.Where("email = ?", "new-owner@example.com").First(&owner).Error; err != nil {
		t.Fatalf("expected owner user to be created: %v", err)
	}
	if owner.TenantID != 0 {
		t.Fatalf("expected owner to remain system-level, got tenant_id %d", owner.TenantID)
	}
	if owner.Nickname != "New Owner" {
		t.Fatalf("expected nickname to be set, got %q", owner.Nickname)
	}

	var membership model.TenantUser
	if err := db.Where("tenant_id = ? AND user_id = ? AND role = ?", created.Tenant.ID, owner.ID, model.TenantRoleOwner).First(&membership).Error; err != nil {
		t.Fatalf("expected owner tenant membership: %v", err)
	}
}

package appgrant

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type grantFixture struct {
	db               *gorm.DB
	service          *Service
	tenant           model.Tenant
	otherTenant      model.Tenant
	app              model.App
	otherApp         model.App
	user             model.User
	appUser          model.AppUser
	readPermission   model.AppPermission
	adminPermission  model.AppPermission
	userRole         model.AppRole
	adminRole        model.AppRole
	tenantRole       model.TenantRbacRole
	tenantPermission model.TenantRbacPermission
	now              time.Time
}

func newGrantFixture(t *testing.T) grantFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:appgrant-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Tenant{}, &model.TenantUser{}, &model.App{}, &model.AppUser{},
		&model.AppPermission{}, &model.AppRole{}, &model.AppUserPermission{}, &model.AppUserRole{},
		&model.TenantRbacRole{}, &model.TenantRbacPermission{}, &model.TenantUserRbacRole{},
		&model.TenantUserRbacPermission{}, &model.TenantRbacRolePermission{}, &model.TenantAppGrantMapping{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)
	tenant := model.Tenant{Name: "Tenant A", Code: "tenant-a"}
	otherTenant := model.Tenant{Name: "Tenant B", Code: "tenant-b"}
	db.Create(&tenant)
	db.Create(&otherTenant)
	app := model.App{TenantID: tenant.ID, Name: "App A", Status: model.AppStatusActive}
	otherApp := model.App{TenantID: otherTenant.ID, Name: "App B", Status: model.AppStatusActive}
	db.Create(&app)
	db.Create(&otherApp)
	user := model.User{Email: "mapped@example.com", EmailVerified: true}
	db.Create(&user)
	db.Create(&model.TenantUser{TenantID: tenant.ID, UserID: user.ID, Role: model.TenantRoleMember})
	appUser := model.AppUser{AppID: app.ID, UserID: user.ID, Status: model.AppUserStatusActive, FirstAuthorizedAt: now, LastAuthorizedAt: now}
	db.Create(&appUser)

	readPermission := model.AppPermission{TenantID: tenant.ID, AppID: app.ID, Code: "app.read", Name: "Read", Category: "app"}
	adminPermission := model.AppPermission{TenantID: tenant.ID, AppID: app.ID, Code: "app.admin", Name: "Admin", Category: "app"}
	db.Create(&readPermission)
	db.Create(&adminPermission)
	userRole := model.AppRole{TenantID: tenant.ID, AppID: app.ID, Code: "app.user", Name: "User"}
	adminRole := model.AppRole{TenantID: tenant.ID, AppID: app.ID, Code: "app.admin", Name: "Admin"}
	db.Create(&userRole)
	db.Create(&adminRole)
	db.Model(&userRole).Association("Permissions").Append(&readPermission)
	db.Model(&adminRole).Association("Permissions").Append(&adminPermission)

	tenantPermission := model.TenantRbacPermission{TenantID: tenant.ID, Code: "tenant.apps.manage", Name: "Manage apps", Category: "app"}
	tenantRole := model.TenantRbacRole{TenantID: tenant.ID, Code: "operators", Name: "Operators"}
	db.Create(&tenantPermission)
	db.Create(&tenantRole)
	db.Create(&model.TenantRbacRolePermission{RoleID: tenantRole.ID, PermissionID: tenantPermission.ID})
	db.Create(&model.TenantUserRbacRole{TenantID: tenant.ID, UserID: user.ID, RoleID: tenantRole.ID, AssignedAt: now, AssignedBy: user.ID})

	return grantFixture{db: db, service: NewService(db), tenant: tenant, otherTenant: otherTenant, app: app, otherApp: otherApp,
		user: user, appUser: appUser, readPermission: readPermission, adminPermission: adminPermission, userRole: userRole,
		adminRole: adminRole, tenantRole: tenantRole, tenantPermission: tenantPermission, now: now}
}

func boolPtr(value bool) *bool { return &value }

func TestResolveMergesExplicitAndDynamicGrantsWithoutMaterializing(t *testing.T) {
	f := newGrantFixture(t)
	explicit := model.AppUserRole{UserID: f.user.ID, AppID: f.app.ID, RoleID: f.userRole.ID, AssignedAt: f.now, AssignedBy: f.user.ID}
	if err := f.db.Create(&explicit).Error; err != nil {
		t.Fatalf("create explicit role: %v", err)
	}

	membership, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member",
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.userRole.ID,
	})
	if err != nil {
		t.Fatalf("create membership mapping: %v", err)
	}
	if membership.AffectedUserCount != 1 {
		t.Fatalf("expected one affected user, got %d", membership.AffectedUserCount)
	}
	roleMapping, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceTenantRole, SourceID: f.tenantRole.ID,
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.adminRole.ID,
	})
	if err != nil {
		t.Fatalf("create role mapping: %v", err)
	}
	_, err = f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceTenantPermission, SourceID: f.tenantPermission.ID,
		TargetType: model.TenantAppGrantTargetAppPermission, TargetID: f.readPermission.ID,
	})
	if err != nil {
		t.Fatalf("create permission mapping: %v", err)
	}

	grants, err := f.service.Resolve(f.user.ID, f.tenant.ID, f.app.ID, f.now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !grants.Eligible || len(grants.Roles) != 2 || len(grants.Permissions) != 2 {
		t.Fatalf("unexpected grants: %+v", grants)
	}
	var userSources, adminSources int
	for _, role := range grants.Roles {
		switch role.Code {
		case "app.user":
			userSources = len(role.Sources)
		case "app.admin":
			adminSources = len(role.Sources)
		}
	}
	if userSources != 2 || adminSources != 1 {
		t.Fatalf("expected explicit+mapped user sources and one admin source, got user=%d admin=%d", userSources, adminSources)
	}
	var physicalCount int64
	f.db.Model(&model.AppUserRole{}).Where("user_id = ? AND app_id = ?", f.user.ID, f.app.ID).Count(&physicalCount)
	if physicalCount != 1 {
		t.Fatalf("resolver materialized inherited roles: count=%d", physicalCount)
	}

	if err := f.service.DeleteMapping(f.tenant.ID, f.app.ID, roleMapping.ID); err != nil {
		t.Fatalf("delete mapping: %v", err)
	}
	grants, err = f.service.Resolve(f.user.ID, f.tenant.ID, f.app.ID, f.now)
	if err != nil {
		t.Fatalf("resolve after delete: %v", err)
	}
	for _, role := range grants.Roles {
		if role.Code == "app.admin" {
			t.Fatal("mapped admin role remained after policy deletion")
		}
	}
}

func TestMappingValidationAndDuplicateProtection(t *testing.T) {
	f := newGrantFixture(t)
	input := MappingInput{SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member", TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.userRole.ID}
	if _, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, input); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}

	otherPermission := model.AppPermission{TenantID: f.otherTenant.ID, AppID: f.otherApp.ID, Code: "other.read", Name: "Other", Category: "other"}
	f.db.Create(&otherPermission)
	_, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member",
		TargetType: model.TenantAppGrantTargetAppPermission, TargetID: otherPermission.ID,
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected cross-tenant validation error, got %v", err)
	}

	until := f.now
	from := f.now.Add(time.Hour)
	_, err = f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceTenantRole, SourceID: f.tenantRole.ID,
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.adminRole.ID, ValidFrom: &from, ValidUntil: &until,
	})
	if !errors.As(err, &validation) {
		t.Fatalf("expected invalid validity window, got %v", err)
	}
}

func TestCreateDisabledMappingRemainsDisabled(t *testing.T) {
	f := newGrantFixture(t)
	created, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member",
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.userRole.ID, Enabled: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("create disabled mapping: %v", err)
	}
	if created.Enabled || created.AffectedUserCount != 0 {
		t.Fatalf("disabled mapping unexpectedly active: %+v", created)
	}
	var stored model.TenantAppGrantMapping
	if err := f.db.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("load disabled mapping: %v", err)
	}
	if stored.Enabled {
		t.Fatal("explicit enabled=false was replaced by the database default")
	}
	grants, err := f.service.Resolve(f.user.ID, f.tenant.ID, f.app.ID, f.now)
	if err != nil {
		t.Fatalf("resolve disabled mapping: %v", err)
	}
	if len(grants.Roles) != 0 || len(grants.Permissions) != 0 {
		t.Fatalf("disabled mapping granted access: %+v", grants)
	}
}

func TestResolveEnforcesRuntimeStatusAndExpiry(t *testing.T) {
	f := newGrantFixture(t)
	_, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member",
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.userRole.ID, Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	assertDenied := func(reason string) {
		t.Helper()
		grants, resolveErr := f.service.Resolve(f.user.ID, f.tenant.ID, f.app.ID, f.now)
		if resolveErr != nil || grants.Eligible || grants.DenialReason != reason || len(grants.Roles) != 0 {
			t.Fatalf("expected denial %s, got grants=%+v err=%v", reason, grants, resolveErr)
		}
	}

	f.db.Model(&model.AppUser{}).Where("id = ?", f.appUser.ID).Updates(map[string]any{"status": model.AppUserStatusSuspended, "banned_until": nil})
	assertDenied("app_user_inactive")
	past := f.now.Add(-time.Minute)
	f.db.Model(&model.AppUser{}).Where("id = ?", f.appUser.ID).Updates(map[string]any{"banned_until": past})
	grants, err := f.service.Resolve(f.user.ID, f.tenant.ID, f.app.ID, f.now)
	if err != nil || !grants.Eligible || len(grants.Roles) != 1 {
		t.Fatalf("expired suspension should be dynamically inactive, got %+v err=%v", grants, err)
	}

	f.db.Model(&model.TenantUser{}).Where("user_id = ? AND tenant_id = ?", f.user.ID, f.tenant.ID).Update("role", model.TenantRoleBanned)
	assertDenied("tenant_membership_banned")
	f.db.Model(&model.TenantUser{}).Where("user_id = ? AND tenant_id = ?", f.user.ID, f.tenant.ID).Update("role", model.TenantRoleMember)
	f.db.Model(&model.User{}).Where("id = ?", f.user.ID).Update("banned", true)
	assertDenied("user_banned")
	f.db.Model(&model.User{}).Where("id = ?", f.user.ID).Update("banned", false)
	f.db.Model(&model.Tenant{}).Where("id = ?", f.tenant.ID).Update("status", model.TenantStatusSuspended)
	assertDenied("tenant_inactive")
	f.db.Model(&model.Tenant{}).Where("id = ?", f.tenant.ID).Update("status", model.TenantStatusActive)
	f.db.Model(&model.App{}).Where("id = ?", f.app.ID).Update("status", model.AppStatusSuspended)
	assertDenied("app_inactive")
}

func TestPreviewUsesSameStatusEligibilityAsResolver(t *testing.T) {
	f := newGrantFixture(t)
	input := MappingInput{
		SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member",
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.userRole.ID,
	}
	assertCount := func(want int64) {
		t.Helper()
		got, err := f.service.PreviewAffectedUsers(f.tenant.ID, f.app.ID, input)
		if err != nil || got != want {
			t.Fatalf("preview count: want=%d got=%d err=%v", want, got, err)
		}
	}
	assertCount(1)
	f.db.Model(&model.User{}).Where("id = ?", f.user.ID).Update("banned", true)
	assertCount(0)
	f.db.Model(&model.User{}).Where("id = ?", f.user.ID).Update("banned", false)
	f.db.Model(&model.Tenant{}).Where("id = ?", f.tenant.ID).Update("status", model.TenantStatusSuspended)
	assertCount(0)
	f.db.Model(&model.Tenant{}).Where("id = ?", f.tenant.ID).Update("status", model.TenantStatusActive)
	f.db.Model(&model.App{}).Where("id = ?", f.app.ID).Update("status", model.AppStatusSuspended)
	assertCount(0)
}

func TestExpiredMappingAndTenantRoleDoNotGrant(t *testing.T) {
	f := newGrantFixture(t)
	past := f.now.Add(-time.Hour)
	_, err := f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceMembershipRole, SourceCode: "member",
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.userRole.ID, ValidUntil: &past,
	})
	if err != nil {
		t.Fatalf("create expired mapping: %v", err)
	}
	f.db.Model(&model.TenantUserRbacRole{}).Where("user_id = ? AND role_id = ?", f.user.ID, f.tenantRole.ID).Update("expires_at", past)
	_, err = f.service.CreateMapping(f.tenant.ID, f.app.ID, f.user.ID, MappingInput{
		SourceType: model.TenantAppGrantSourceTenantRole, SourceID: f.tenantRole.ID,
		TargetType: model.TenantAppGrantTargetAppRole, TargetID: f.adminRole.ID,
	})
	if err != nil {
		t.Fatalf("create role mapping: %v", err)
	}
	grants, err := f.service.Resolve(f.user.ID, f.tenant.ID, f.app.ID, f.now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(grants.Roles) != 0 || len(grants.Permissions) != 0 {
		t.Fatalf("expired sources granted access: %+v", grants)
	}
}

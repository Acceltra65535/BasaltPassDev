package tenantquota

import (
	"testing"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Tenant{},
		&model.TenantQuota{},
		&model.TenantUser{},
		&model.App{},
		&model.Team{},
		&model.OAuthAccessToken{},
	); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestEnsureAppsWithinLimit(t *testing.T) {
	db := newTestDB(t)
	tenant := model.Tenant{Name: "Acme", Code: "acme"}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&model.TenantQuota{TenantID: tenant.ID, MaxApps: 1, MaxUsers: 10, MaxTeams: 10, MaxTokensPerHour: 10}).Error; err != nil {
		t.Fatalf("create quota: %v", err)
	}
	if err := db.Create(&model.App{TenantID: tenant.ID, Name: "One", Status: model.AppStatusActive}).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}

	if err := EnsureAppsWithinLimit(db, tenant.ID); err == nil {
		t.Fatal("expected app quota error")
	}
}

func TestEnsureUserCanJoinDoesNotDoubleCountExistingPrimaryTenant(t *testing.T) {
	db := newTestDB(t)
	tenant := model.Tenant{Name: "Acme", Code: "acme"}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user := model.User{Email: "owner@example.com", EnforcedTenantID: tenant.ID}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.TenantUser{UserID: user.ID, TenantID: tenant.ID, Role: model.TenantRoleMember}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := db.Create(&model.TenantQuota{TenantID: tenant.ID, MaxApps: 10, MaxUsers: 1, MaxTeams: 10, MaxTokensPerHour: 10}).Error; err != nil {
		t.Fatalf("create quota: %v", err)
	}

	if err := EnsureUserCanJoin(db, tenant.ID, user.ID); err != nil {
		t.Fatalf("existing primary tenant user should not consume another slot: %v", err)
	}
}

func TestEnsureTokensWithinLimit(t *testing.T) {
	db := newTestDB(t)
	tenant := model.Tenant{Name: "Acme", Code: "acme"}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&model.TenantQuota{TenantID: tenant.ID, MaxApps: 10, MaxUsers: 10, MaxTeams: 10, MaxTokensPerHour: 1}).Error; err != nil {
		t.Fatalf("create quota: %v", err)
	}
	now := time.Now()
	if err := db.Create(&model.OAuthAccessToken{
		Token:     "tok",
		ClientID:  "client",
		UserID:    1,
		TenantID:  tenant.ID,
		AppID:     1,
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(-30 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	if err := EnsureTokensWithinLimit(db, tenant.ID, now); err == nil {
		t.Fatal("expected token quota error")
	}
}

package rbacmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"basaltpass-backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type manifestFixture struct {
	tenant   model.Tenant
	app      model.App
	client   model.OAuthClient
	reviewer model.User
}

func setupManifestTestDB(t *testing.T) (*gorm.DB, manifestFixture) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Tenant{}, &model.App{}, &model.OAuthClient{},
		&model.AppPermission{}, &model.AppRole{}, &model.AppUserPermission{}, &model.AppUserRole{},
		&model.AppRBACManifest{}, &model.AppRBACRevision{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tenant := model.Tenant{Name: "Acme", Code: "acme-" + strings.ReplaceAll(t.Name(), "/", "-"), Status: model.TenantStatusActive}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	reviewer := model.User{Email: "reviewer-" + strings.ReplaceAll(t.Name(), "/", "-") + "@example.com", PasswordHash: "x", EnforcedTenantID: tenant.ID}
	if err := db.Create(&reviewer).Error; err != nil {
		t.Fatalf("create reviewer: %v", err)
	}
	app := model.App{TenantID: tenant.ID, Name: "Demo", Status: model.AppStatusActive}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	client := model.OAuthClient{AppID: app.ID, ClientID: fmt.Sprintf("client-%d", app.ID), ClientSecret: "unused", IsActive: true, CreatedBy: reviewer.ID}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	return db, manifestFixture{tenant: tenant, app: app, client: client, reviewer: reviewer}
}

func testManifest(revision uint64, permissionCodes []string, roles map[string][]string) []byte {
	manifest := Manifest{SchemaVersion: "1.0.0", Type: "basalt_rbac_bundle", Revision: revision}
	for _, code := range permissionCodes {
		manifest.Permissions = append(manifest.Permissions, PermissionDef{
			PermissionKey: code, DisplayName: strings.ToUpper(code), Resource: "demo", Action: "read", Scope: "app", Status: "active",
		})
	}
	for code, permissions := range roles {
		manifest.Roles = append(manifest.Roles, RoleDef{RoleKey: code, DisplayName: strings.ToUpper(code), Assignable: true, Status: "active"})
		for _, permission := range permissions {
			manifest.RolePermissions = append(manifest.RolePermissions, RolePermissionLink{RoleKey: code, PermissionKey: permission, Effect: "allow"})
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestDecodeManifestRejectsNonRBACAndReservedFields(t *testing.T) {
	nonRBAC := []byte(`{"schema_version":"1.0.0","type":"basalt_rbac_bundle","revision":1,"permissions":[],"roles":[],"role_permissions":[],"oauth_clients":[]}`)
	if _, err := DecodeManifest(nonRBAC); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected unknown OAuth field to be rejected, got %v", err)
	}
	assignments := []byte(`{"schema_version":"1.0.0","type":"basalt_rbac_bundle","revision":1,"permissions":[],"roles":[],"role_permissions":[],"user_assignments":[]}`)
	if _, err := DecodeManifest(assignments); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected user assignments to be rejected, got %v", err)
	}

	reserved := testManifest(1, []string{"tenant.users.delete"}, nil)
	if _, err := DecodeManifest(reserved); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected reserved permission prefix to be rejected, got %v", err)
	}
}

func TestApproveRejectsStaleDiffWithoutPublishing(t *testing.T) {
	db, fixture := setupManifestTestDB(t)
	svc := New(db)
	submitted, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, testManifest(1, []string{"demo.read"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	manual := model.AppPermission{Code: "demo.manual", Name: "Manual", Category: "demo", AppID: fixture.app.ID, TenantID: fixture.tenant.ID}
	if err := db.Create(&manual).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(fixture.tenant.ID, fixture.app.ID, submitted.Manifest.ID, fixture.reviewer.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected baseline drift to block approval, got %v", err)
	}
	var manualCount, proposedCount int64
	db.Model(&model.AppPermission{}).Where("id = ?", manual.ID).Count(&manualCount)
	db.Model(&model.AppPermission{}).Where("app_id = ? AND code = ?", fixture.app.ID, "demo.read").Count(&proposedCount)
	if manualCount != 1 || proposedCount != 0 {
		t.Fatalf("stale approval changed effective RBAC: manual=%d proposed=%d", manualCount, proposedCount)
	}
}

func TestSubmitIsStrictlyBoundAndIdempotent(t *testing.T) {
	db, fixture := setupManifestTestDB(t)
	svc := New(db)
	raw := testManifest(1, []string{"demo.read"}, map[string][]string{"demo.viewer": {"demo.read"}})

	if _, err := svc.Submit(fixture.tenant.ID, fixture.app.ID+999, fixture.client.ClientID, raw); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected cross-app submission to fail, got %v", err)
	}
	created, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, raw)
	if err != nil || !created.Created {
		t.Fatalf("submit failed: result=%+v err=%v", created, err)
	}
	duplicate, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, raw)
	if err != nil || duplicate.Created || duplicate.Manifest.ID != created.Manifest.ID {
		t.Fatalf("expected idempotent duplicate, result=%+v err=%v", duplicate, err)
	}
	stale := testManifest(1, []string{"demo.write"}, map[string][]string{"demo.editor": {"demo.write"}})
	if _, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected same revision with different digest to fail, got %v", err)
	}
}

func TestApprovePublishesAtomicallyAndCreatesAuditAndRollbackBaseline(t *testing.T) {
	db, fixture := setupManifestTestDB(t)
	seedPermission := model.AppPermission{Code: "demo.legacy", Name: "Legacy", Category: "demo", AppID: fixture.app.ID, TenantID: fixture.tenant.ID}
	if err := db.Create(&seedPermission).Error; err != nil {
		t.Fatal(err)
	}
	svc := New(db)
	raw := testManifest(1, []string{"demo.read", "demo.write"}, map[string][]string{
		"demo.viewer": {"demo.read"}, "demo.editor": {"demo.read", "demo.write"},
	})
	submitted, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, raw)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	approved, err := svc.Approve(fixture.tenant.ID, fixture.app.ID, submitted.Manifest.ID, fixture.reviewer.ID)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != model.AppRBACManifestApproved || approved.ActiveRevisionID == nil {
		t.Fatalf("unexpected approval: %+v", approved)
	}

	var permissions []model.AppPermission
	if err := db.Where("app_id = ?", fixture.app.ID).Order("code").Find(&permissions).Error; err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 2 || permissions[0].Code != "demo.read" || permissions[1].Code != "demo.write" {
		t.Fatalf("unexpected effective permissions: %+v", permissions)
	}
	var revisions []model.AppRBACRevision
	if err := db.Where("app_id = ?", fixture.app.ID).Order("revision").Find(&revisions).Error; err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 || revisions[0].Action != "baseline" || revisions[1].Action != "manifest" || !revisions[1].IsActive {
		t.Fatalf("unexpected revisions: %+v", revisions)
	}
	var audits int64
	if err := db.Model(&model.AuditLog{}).Where("action IN ?", []string{"rbac_manifest_submit", "rbac_manifest_approve"}).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("expected submit and approval audit records, got %d", audits)
	}
}

func TestApproveRefusesToDeleteAssignedRoleWithoutPartialChanges(t *testing.T) {
	db, fixture := setupManifestTestDB(t)
	permission := model.AppPermission{Code: "demo.read", Name: "Read", Category: "demo", AppID: fixture.app.ID, TenantID: fixture.tenant.ID}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	role := model.AppRole{Code: "demo.viewer", Name: "Viewer", AppID: fixture.app.ID, TenantID: fixture.tenant.ID}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&role).Association("Permissions").Append(&permission); err != nil {
		t.Fatal(err)
	}
	assignment := model.AppUserRole{UserID: fixture.reviewer.ID, AppID: fixture.app.ID, RoleID: role.ID, AssignedAt: time.Now(), AssignedBy: fixture.reviewer.ID}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}

	svc := New(db)
	submitted, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, testManifest(1, []string{"demo.write"}, nil))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := svc.Approve(fixture.tenant.ID, fixture.app.ID, submitted.Manifest.ID, fixture.reviewer.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected assigned role deletion to be blocked, got %v", err)
	}
	var roleCount, writePermissionCount, assignmentCount int64
	db.Model(&model.AppRole{}).Where("id = ?", role.ID).Count(&roleCount)
	db.Model(&model.AppPermission{}).Where("code = ? AND app_id = ?", "demo.write", fixture.app.ID).Count(&writePermissionCount)
	db.Model(&model.AppUserRole{}).Where("id = ?", assignment.ID).Count(&assignmentCount)
	if roleCount != 1 || writePermissionCount != 0 || assignmentCount != 1 {
		t.Fatalf("approval was not atomic: role=%d write=%d assignment=%d", roleCount, writePermissionCount, assignmentCount)
	}
}

func TestRollbackCreatesNewRevisionAndPreservesAssignments(t *testing.T) {
	db, fixture := setupManifestTestDB(t)
	svc := New(db)
	first, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, testManifest(1, []string{"demo.read"}, map[string][]string{"demo.viewer": {"demo.read"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(fixture.tenant.ID, fixture.app.ID, first.Manifest.ID, fixture.reviewer.ID); err != nil {
		t.Fatal(err)
	}
	second, err := svc.Submit(fixture.tenant.ID, fixture.app.ID, fixture.client.ClientID, testManifest(2, []string{"demo.read", "demo.write"}, map[string][]string{"demo.viewer": {"demo.read"}, "demo.editor": {"demo.write"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(fixture.tenant.ID, fixture.app.ID, second.Manifest.ID, fixture.reviewer.ID); err != nil {
		t.Fatal(err)
	}
	revisions, err := svc.ListRevisions(fixture.tenant.ID, fixture.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	var firstPublished RevisionView
	for _, revision := range revisions {
		if revision.Action == "manifest" && revision.ManifestID != nil && *revision.ManifestID == first.Manifest.ID {
			firstPublished = revision
		}
	}
	if firstPublished.ID == 0 {
		t.Fatalf("first published revision not found: %+v", revisions)
	}
	var editor model.AppRole
	if err := db.Where("app_id = ? AND code = ?", fixture.app.ID, "demo.editor").First(&editor).Error; err != nil {
		t.Fatal(err)
	}
	assignment := model.AppUserRole{UserID: fixture.reviewer.ID, AppID: fixture.app.ID, RoleID: editor.ID, AssignedAt: time.Now(), AssignedBy: fixture.reviewer.ID}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rollback(fixture.tenant.ID, fixture.app.ID, firstPublished.ID, fixture.reviewer.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected rollback that deletes an assigned role to be blocked, got %v", err)
	}
	var assignmentCount int64
	db.Model(&model.AppUserRole{}).Where("id = ?", assignment.ID).Count(&assignmentCount)
	if assignmentCount != 1 {
		t.Fatal("blocked rollback modified the user assignment")
	}
	if err := db.Delete(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	rolledBack, err := svc.Rollback(fixture.tenant.ID, fixture.app.ID, firstPublished.ID, fixture.reviewer.ID)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolledBack.Action != "rollback" || !rolledBack.IsActive || rolledBack.TargetRevisionID == nil || *rolledBack.TargetRevisionID != firstPublished.ID {
		t.Fatalf("unexpected rollback revision: %+v", rolledBack)
	}
	var writeCount, editorCount int64
	db.Model(&model.AppPermission{}).Where("app_id = ? AND code = ?", fixture.app.ID, "demo.write").Count(&writeCount)
	db.Model(&model.AppRole{}).Where("app_id = ? AND code = ?", fixture.app.ID, "demo.editor").Count(&editorCount)
	if writeCount != 0 || editorCount != 0 {
		t.Fatalf("rollback did not restore snapshot: write=%d editor=%d", writeCount, editorCount)
	}
}

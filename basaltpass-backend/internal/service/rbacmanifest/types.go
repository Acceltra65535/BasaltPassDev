package rbacmanifest

import (
	"errors"
	"time"

	"basaltpass-backend/internal/model"
)

var (
	ErrValidation = errors.New("rbac manifest validation failed")
	ErrConflict   = errors.New("rbac manifest conflict")
	ErrNotFound   = errors.New("rbac manifest not found")
)

// Keep the normalized payload below MySQL TEXT's portable storage limit.
const MaxManifestBytes = 60 * 1024

type Manifest struct {
	SchemaVersion   string               `json:"schema_version"`
	Type            string               `json:"type"`
	Revision        uint64               `json:"revision"`
	Permissions     []PermissionDef      `json:"permissions"`
	Roles           []RoleDef            `json:"roles"`
	RolePermissions []RolePermissionLink `json:"role_permissions"`
}

type PermissionDef struct {
	PermissionKey string `json:"permission_key"`
	DisplayName   string `json:"display_name"`
	Resource      string `json:"resource"`
	Action        string `json:"action"`
	Scope         string `json:"scope"`
	Description   string `json:"description"`
	Status        string `json:"status"`
}

type RoleDef struct {
	RoleKey     string `json:"role_key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Assignable  bool   `json:"assignable"`
	Priority    int    `json:"priority"`
	Status      string `json:"status"`
}

type RolePermissionLink struct {
	RoleKey       string `json:"role_key"`
	PermissionKey string `json:"permission_key"`
	Effect        string `json:"effect"`
}

type Snapshot struct {
	Permissions []SnapshotPermission `json:"permissions"`
	Roles       []SnapshotRole       `json:"roles"`
}

type SnapshotPermission struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type SnapshotRole struct {
	Code            string   `json:"code"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permission_codes"`
}

type Diff struct {
	HasChanges              bool     `json:"has_changes"`
	PermissionsAdded        []string `json:"permissions_added"`
	PermissionsUpdated      []string `json:"permissions_updated"`
	PermissionsRemoved      []string `json:"permissions_removed"`
	RolesAdded              []string `json:"roles_added"`
	RolesUpdated            []string `json:"roles_updated"`
	RolesRemoved            []string `json:"roles_removed"`
	RolePermissionsAdded    []string `json:"role_permissions_added"`
	RolePermissionsRemoved  []string `json:"role_permissions_removed"`
	AssignedRolesAffected   []string `json:"assigned_roles_affected"`
	RemovalAssignmentBlocks []string `json:"removal_assignment_blocks"`
}

type ManifestView struct {
	ID               uint                        `json:"id"`
	TenantID         uint                        `json:"tenant_id"`
	AppID            uint                        `json:"app_id"`
	SourceClientID   string                      `json:"source_client_id"`
	SchemaVersion    string                      `json:"schema_version"`
	SourceRevision   uint64                      `json:"source_revision"`
	Digest           string                      `json:"digest"`
	BaseDigest       string                      `json:"base_digest"`
	Status           model.AppRBACManifestStatus `json:"status"`
	Diff             Diff                        `json:"diff"`
	Manifest         Manifest                    `json:"manifest"`
	SubmittedAt      time.Time                   `json:"submitted_at"`
	ReviewedAt       *time.Time                  `json:"reviewed_at,omitempty"`
	ReviewedBy       *uint                       `json:"reviewed_by,omitempty"`
	ReviewNote       string                      `json:"review_note,omitempty"`
	ActiveRevisionID *uint                       `json:"active_revision_id,omitempty"`
}

type RevisionView struct {
	ID               uint      `json:"id"`
	TenantID         uint      `json:"tenant_id"`
	AppID            uint      `json:"app_id"`
	Revision         uint64    `json:"revision"`
	Digest           string    `json:"digest"`
	ManifestID       *uint     `json:"manifest_id,omitempty"`
	Action           string    `json:"action"`
	TargetRevisionID *uint     `json:"target_revision_id,omitempty"`
	IsActive         bool      `json:"is_active"`
	CreatedBy        uint      `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

type SubmitResult struct {
	Manifest ManifestView `json:"manifest"`
	Created  bool         `json:"created"`
}

package model

import "time"

type AppRBACManifestStatus string

const (
	AppRBACManifestPending    AppRBACManifestStatus = "pending"
	AppRBACManifestApproved   AppRBACManifestStatus = "approved"
	AppRBACManifestRejected   AppRBACManifestStatus = "rejected"
	AppRBACManifestSuperseded AppRBACManifestStatus = "superseded"
)

// AppRBACManifest is an app-submitted, RBAC-only proposal. Payload never
// contains OAuth client settings or user assignments, and it is not applied
// until a tenant administrator approves it.
type AppRBACManifest struct {
	ID               uint                  `gorm:"primaryKey" json:"id"`
	TenantID         uint                  `gorm:"not null;index;uniqueIndex:idx_app_rbac_manifest_source_revision" json:"tenant_id"`
	AppID            uint                  `gorm:"not null;index;uniqueIndex:idx_app_rbac_manifest_source_revision" json:"app_id"`
	SourceClientID   string                `gorm:"size:64;not null;index" json:"source_client_id"`
	SchemaVersion    string                `gorm:"size:16;not null" json:"schema_version"`
	SourceRevision   uint64                `gorm:"not null;uniqueIndex:idx_app_rbac_manifest_source_revision" json:"source_revision"`
	Digest           string                `gorm:"size:64;not null;index:idx_app_rbac_manifest_digest" json:"digest"`
	BaseDigest       string                `gorm:"size:64;not null" json:"base_digest"`
	Status           AppRBACManifestStatus `gorm:"size:20;not null;index" json:"status"`
	Payload          string                `gorm:"type:text;not null" json:"-"`
	Diff             string                `gorm:"type:text;not null" json:"-"`
	SubmittedAt      time.Time             `gorm:"not null;index" json:"submitted_at"`
	ReviewedAt       *time.Time            `json:"reviewed_at,omitempty"`
	ReviewedBy       *uint                 `gorm:"index" json:"reviewed_by,omitempty"`
	ReviewNote       string                `gorm:"size:500" json:"review_note,omitempty"`
	ActiveRevisionID *uint                 `gorm:"index" json:"active_revision_id,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

func (AppRBACManifest) TableName() string { return "app_rbac_manifests" }

// AppRBACRevision is an immutable full snapshot of the effective app RBAC
// catalog. Rollback creates a new revision from an older snapshot.
type AppRBACRevision struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TenantID         uint      `gorm:"not null;index;uniqueIndex:idx_app_rbac_revision_number" json:"tenant_id"`
	AppID            uint      `gorm:"not null;index;uniqueIndex:idx_app_rbac_revision_number" json:"app_id"`
	Revision         uint64    `gorm:"not null;uniqueIndex:idx_app_rbac_revision_number" json:"revision"`
	Snapshot         string    `gorm:"type:text;not null" json:"-"`
	Digest           string    `gorm:"size:64;not null" json:"digest"`
	ManifestID       *uint     `gorm:"index" json:"manifest_id,omitempty"`
	Action           string    `gorm:"size:20;not null" json:"action"`
	TargetRevisionID *uint     `gorm:"index" json:"target_revision_id,omitempty"`
	IsActive         bool      `gorm:"not null;default:false;index" json:"is_active"`
	CreatedBy        uint      `gorm:"not null;index" json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

func (AppRBACRevision) TableName() string { return "app_rbac_revisions" }

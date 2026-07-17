package appgrant

import (
	"errors"
	"time"

	"basaltpass-backend/internal/model"
)

var (
	ErrNotFound = errors.New("app grant mapping not found")
	ErrConflict = errors.New("app grant mapping conflicts with existing policy")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

type MappingInput struct {
	SourceType model.TenantAppGrantSourceType `json:"source_type"`
	SourceID   uint                           `json:"source_id"`
	SourceCode string                         `json:"source_code"`
	TargetType model.TenantAppGrantTargetType `json:"target_type"`
	TargetID   uint                           `json:"target_id"`
	Enabled    *bool                          `json:"enabled,omitempty"`
	ValidFrom  *time.Time                     `json:"valid_from,omitempty"`
	ValidUntil *time.Time                     `json:"valid_until,omitempty"`
}

type GrantEndpoint struct {
	Type string `json:"type"`
	ID   uint   `json:"id,omitempty"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type MappingView struct {
	ID                uint          `json:"id"`
	TenantID          uint          `json:"tenant_id"`
	AppID             uint          `json:"app_id"`
	Source            GrantEndpoint `json:"source"`
	Target            GrantEndpoint `json:"target"`
	Enabled           bool          `json:"enabled"`
	ValidFrom         *time.Time    `json:"valid_from,omitempty"`
	ValidUntil        *time.Time    `json:"valid_until,omitempty"`
	AffectedUserCount int64         `json:"affected_user_count"`
	CreatedBy         uint          `json:"created_by"`
	UpdatedBy         uint          `json:"updated_by"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type GrantSource struct {
	Type         string `json:"type"`
	AssignmentID uint   `json:"assignment_id,omitempty"`
	MappingID    uint   `json:"mapping_id,omitempty"`
	SourceType   string `json:"source_type,omitempty"`
	SourceID     uint   `json:"source_id,omitempty"`
	SourceCode   string `json:"source_code,omitempty"`
	ViaRoleCode  string `json:"via_role_code,omitempty"`
}

type EffectiveRole struct {
	ID          uint                  `json:"id"`
	Code        string                `json:"code"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	AppID       uint                  `json:"app_id"`
	Permissions []model.AppPermission `json:"permissions"`
	Sources     []GrantSource         `json:"sources"`
}

type EffectivePermission struct {
	ID          uint          `json:"id"`
	Code        string        `json:"code"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Category    string        `json:"category"`
	AppID       uint          `json:"app_id"`
	Sources     []GrantSource `json:"sources"`
}

type EffectiveGrants struct {
	Eligible     bool                  `json:"eligible"`
	DenialReason string                `json:"denial_reason,omitempty"`
	Roles        []EffectiveRole       `json:"roles"`
	Permissions  []EffectivePermission `json:"permissions"`
}

type Options struct {
	MembershipRoles   []GrantEndpoint `json:"membership_roles"`
	TenantRoles       []GrantEndpoint `json:"tenant_roles"`
	TenantPermissions []GrantEndpoint `json:"tenant_permissions"`
	AppRoles          []GrantEndpoint `json:"app_roles"`
	AppPermissions    []GrantEndpoint `json:"app_permissions"`
}

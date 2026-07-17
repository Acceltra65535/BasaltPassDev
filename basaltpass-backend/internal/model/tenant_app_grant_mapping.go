package model

import "time"

type TenantAppGrantSourceType string

const (
	TenantAppGrantSourceMembershipRole   TenantAppGrantSourceType = "membership_role"
	TenantAppGrantSourceTenantRole       TenantAppGrantSourceType = "tenant_role"
	TenantAppGrantSourceTenantPermission TenantAppGrantSourceType = "tenant_permission"
)

type TenantAppGrantTargetType string

const (
	TenantAppGrantTargetAppRole       TenantAppGrantTargetType = "app_role"
	TenantAppGrantTargetAppPermission TenantAppGrantTargetType = "app_permission"
)

// TenantAppGrantMapping is tenant-owned policy. It never materializes an
// AppUserRole/AppUserPermission; effective app grants are derived at read time.
type TenantAppGrantMapping struct {
	ID         uint                     `json:"id" gorm:"primaryKey"`
	TenantID   uint                     `json:"tenant_id" gorm:"not null;index;uniqueIndex:idx_tenant_app_grant_mapping"`
	AppID      uint                     `json:"app_id" gorm:"not null;index;uniqueIndex:idx_tenant_app_grant_mapping"`
	SourceType TenantAppGrantSourceType `json:"source_type" gorm:"type:varchar(32);not null;uniqueIndex:idx_tenant_app_grant_mapping"`
	SourceID   uint                     `json:"source_id" gorm:"not null;default:0;uniqueIndex:idx_tenant_app_grant_mapping"`
	SourceCode string                   `json:"source_code" gorm:"size:100;not null;default:'';uniqueIndex:idx_tenant_app_grant_mapping"`
	TargetType TenantAppGrantTargetType `json:"target_type" gorm:"type:varchar(32);not null;uniqueIndex:idx_tenant_app_grant_mapping"`
	TargetID   uint                     `json:"target_id" gorm:"not null;uniqueIndex:idx_tenant_app_grant_mapping"`
	Enabled    bool                     `json:"enabled" gorm:"not null;index"`
	ValidFrom  *time.Time               `json:"valid_from,omitempty"`
	ValidUntil *time.Time               `json:"valid_until,omitempty"`
	CreatedBy  uint                     `json:"created_by" gorm:"not null"`
	UpdatedBy  uint                     `json:"updated_by" gorm:"not null"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`

	// Tenant/App are the stable ownership boundary for a mapping. Cascades only
	// apply to hard deletes; ordinary app suspension/deletion remains status based.
	Tenant Tenant `json:"-" gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	App    App    `json:"-" gorm:"foreignKey:AppID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (TenantAppGrantMapping) TableName() string {
	return "tenant_app_grant_mappings"
}

package model

import (
	"time"

	"gorm.io/gorm"
)

// AuthRefreshToken stores server-side state for application console refresh JWTs.
// The JWT itself remains opaque to clients; only its hash is persisted.
type AuthRefreshToken struct {
	gorm.Model
	JTI        string     `gorm:"size:64;uniqueIndex;not null"`
	FamilyID   string     `gorm:"size:64;index;not null"`
	TokenHash  string     `gorm:"size:64;uniqueIndex;not null"`
	UserID     uint       `gorm:"not null;index"`
	TenantID   uint       `gorm:"not null;index"`
	Scope      string     `gorm:"size:32;not null;index"`
	ExpiresAt  time.Time  `gorm:"not null;index"`
	ConsumedAt *time.Time `gorm:"index"`
	RevokedAt  *time.Time `gorm:"index"`
}

func (AuthRefreshToken) TableName() string {
	return "auth_refresh_tokens"
}

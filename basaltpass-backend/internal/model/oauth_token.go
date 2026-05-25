package model

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

// OAuthAccessToken OAuth2访问令牌
type OAuthAccessToken struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Token      string    `gorm:"size:128;uniqueIndex;not null" json:"token"`
	ClientID   string    `gorm:"size:64;not null;index" json:"client_id"`
	UserID     uint      `gorm:"not null;index" json:"user_id"`
	TenantID   uint      `gorm:"not null;index" json:"tenant_id"`
	AppID      uint      `gorm:"not null;index" json:"app_id"`
	Scopes     string    `gorm:"type:text" json:"scopes"`
	Claims     string    `gorm:"type:text" json:"claims,omitempty"`
	ExpiresAt  time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	AuthCodeID *uint     `gorm:"index" json:"auth_code_id,omitempty"`

	// Token Exchange (RFC 8693) actor context — populated when this token was
	// created via a token-exchange grant.
	ActorClientID string `gorm:"size:64;index" json:"actor_client_id,omitempty"` // the client that initiated the exchange
	ActorAppID    uint   `gorm:"index" json:"actor_app_id,omitempty"`            // the app that initiated the exchange
	IsExchanged   bool   `gorm:"default:false" json:"is_exchanged"`              // true when created via token exchange

	// 关联
	User   User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Tenant Tenant      `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	App    App         `gorm:"foreignKey:AppID" json:"app,omitempty"`
	Client OAuthClient `gorm:"foreignKey:ClientID;references:ClientID" json:"client,omitempty"`
}

// OAuthRefreshToken OAuth2刷新令牌
type OAuthRefreshToken struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Token         string    `gorm:"size:128;uniqueIndex;not null" json:"token"`
	ClientID      string    `gorm:"size:64;not null;index" json:"client_id"`
	UserID        uint      `gorm:"not null;index" json:"user_id"`
	TenantID      uint      `gorm:"not null;index" json:"tenant_id"`
	AppID         uint      `gorm:"not null;index" json:"app_id"`
	Scopes        string    `gorm:"type:text" json:"scopes"`
	Nonce         string    `gorm:"size:255" json:"nonce,omitempty"`
	AuthTime      time.Time `json:"auth_time"`
	ACR           string    `gorm:"size:128" json:"acr,omitempty"`
	AMR           string    `gorm:"type:text" json:"amr,omitempty"`
	ExpiresAt     time.Time `gorm:"not null;index" json:"expires_at"`
	AccessTokenID *uint     `gorm:"index" json:"access_token_id,omitempty"` // 关联的访问令牌ID
	CreatedAt     time.Time `json:"created_at"`

	// 关联
	User        User              `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Tenant      Tenant            `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	App         App               `gorm:"foreignKey:AppID" json:"app,omitempty"`
	Client      OAuthClient       `gorm:"foreignKey:ClientID;references:ClientID" json:"client,omitempty"`
	AccessToken *OAuthAccessToken `gorm:"foreignKey:AccessTokenID" json:"access_token,omitempty"`
}

// OAuthAuthorizationCode OAuth2授权码（增强版）
type OAuthAuthorizationCode struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Code                string    `gorm:"size:128;uniqueIndex;not null" json:"code"`
	ClientID            string    `gorm:"size:64;not null;index" json:"client_id"`
	UserID              uint      `gorm:"not null;index" json:"user_id"`
	TenantID            uint      `gorm:"not null;index" json:"tenant_id"`
	AppID               uint      `gorm:"not null;index" json:"app_id"`
	RedirectURI         string    `gorm:"size:500;not null" json:"redirect_uri"`
	Scopes              string    `gorm:"type:text" json:"scopes"`
	Claims              string    `gorm:"type:text" json:"claims,omitempty"`
	CodeChallenge       string    `gorm:"size:128" json:"code_challenge"`       // PKCE支持
	CodeChallengeMethod string    `gorm:"size:16" json:"code_challenge_method"` // PKCE方法
	Nonce               string    `gorm:"size:255" json:"nonce"`                // OIDC nonce
	ACRValues           string    `gorm:"size:255" json:"acr_values,omitempty"`
	Prompt              string    `gorm:"size:128" json:"prompt,omitempty"`
	MaxAge              *int      `json:"max_age,omitempty"`
	LoginHint           string    `gorm:"size:255" json:"login_hint,omitempty"`
	AuthTime            time.Time `json:"auth_time"`
	ACR                 string    `gorm:"size:128" json:"acr,omitempty"`
	AMR                 string    `gorm:"type:text" json:"amr,omitempty"`
	ExpiresAt           time.Time `gorm:"not null;index" json:"expires_at"`
	Used                bool      `gorm:"default:false;index" json:"used"` // 是否已使用
	CreatedAt           time.Time `json:"created_at"`

	// 关联
	User   User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Tenant Tenant      `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	App    App         `gorm:"foreignKey:AppID" json:"app,omitempty"`
	Client OAuthClient `gorm:"foreignKey:ClientID;references:ClientID" json:"client,omitempty"`
}

const (
	OIDCSigningKeyStatusActive  = "active"
	OIDCSigningKeyStatusStandby = "standby"
	OIDCSigningKeyStatusRetired = "retired"
)

// OIDCSigningKey stores persistent signing material for OIDC id_tokens.
type OIDCSigningKey struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	KID                 string     `gorm:"column:kid;size:128;uniqueIndex;not null" json:"kid"`
	Algorithm           string     `gorm:"size:32;not null;default:RS256" json:"algorithm"`
	PrivateKeyEncrypted string     `gorm:"type:text;not null" json:"-"`
	PublicJWK           string     `gorm:"type:text;not null" json:"public_jwk"`
	Status              string     `gorm:"size:32;not null;index" json:"status"`
	NotBefore           *time.Time `json:"not_before,omitempty"`
	NotAfter            *time.Time `json:"not_after,omitempty"`
	RotatedAt           *time.Time `json:"rotated_at,omitempty"`
	RetiredAt           *time.Time `json:"retired_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (OIDCSigningKey) TableName() string {
	return "oidc_signing_keys"
}

// 生成令牌的辅助方法

// GenerateAccessToken 生成访问令牌
func GenerateAccessToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "bp_at_" + hex.EncodeToString(bytes), nil
}

// GenerateRefreshToken 生成刷新令牌
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "bp_rt_" + hex.EncodeToString(bytes), nil
}

// GenerateAuthCode 生成授权码
func GenerateAuthCode() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateCrossAppToken generates a token for cross-app exchange (bp_xat_ prefix).
func GenerateCrossAppToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "bp_xat_" + hex.EncodeToString(bytes), nil
}

// IsExpired 检查令牌是否过期
func (t *OAuthAccessToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsExpired 检查刷新令牌是否过期
func (t *OAuthRefreshToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsExpired 检查授权码是否过期
func (c *OAuthAuthorizationCode) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// GetScopeList 获取权限范围列表
func (t *OAuthAccessToken) GetScopeList() []string {
	if t.Scopes == "" {
		return []string{}
	}
	return strings.Split(t.Scopes, " ")
}

// GetScopeList 获取权限范围列表
func (c *OAuthAuthorizationCode) GetScopeList() []string {
	if c.Scopes == "" {
		return []string{}
	}
	return strings.Split(c.Scopes, " ")
}

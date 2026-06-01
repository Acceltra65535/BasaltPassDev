package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	tenantservice "basaltpass-backend/internal/service/tenant"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		if os.Getenv("BASALTPASS_DYNO_MODE") == "test" {
			return "test-secret" // Allow test mode
		}
		log.Printf("[auth][error] environment variable %s is required", key)
		return ""
	}
	return v
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// TokenPair contains access and refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

const (
	ConsoleScopeUser   = "user"
	ConsoleScopeTenant = "tenant"
	ConsoleScopeAdmin  = "admin"

	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
	TokenTypePreAuth = "pre_auth"
)

// GenerateTokenPair creates JWT access and refresh tokens for a user id.
// 自动从用户记录中获取tenant_id
func GenerateTokenPair(userID uint) (TokenPair, error) {
	// 从数据库获取用户的tenant_id
	var user model.User
	if err := common.DB().Select("id", "tenant_id").First(&user, userID).Error; err != nil {
		return TokenPair{}, err
	}
	tenantID, err := resolveTokenTenantID(userID, user.TenantID, ConsoleScopeUser)
	if err != nil {
		return TokenPair{}, err
	}
	return GenerateTokenPairWithTenantAndScope(userID, tenantID, ConsoleScopeUser)
}

// GenerateTokenPairWithTenant creates JWT tokens with tenant context
func GenerateTokenPairWithTenant(userID uint, tenantID uint) (TokenPair, error) {
	return GenerateTokenPairWithTenantAndScope(userID, tenantID, ConsoleScopeUser)
}

// GenerateTokenPairWithTenantAndScope creates JWT tokens with tenant context and console scope.
//
// Scopes:
// - user: default, minimal privilege
// - tenant: tenant console
// - admin: global admin console
func GenerateTokenPairWithTenantAndScope(userID uint, tenantID uint, scope string) (TokenPair, error) {
	return GenerateTokenPairWithTenantScopeAndAuthMethods(userID, tenantID, scope, []string{"pwd"})
}

func GenerateTokenPairWithTenantScopeAndAuthMethods(userID uint, tenantID uint, scope string, methods []string) (TokenPair, error) {
	return generateTokenPairWithTenantScopeAuthMethodsAndFamily(userID, tenantID, scope, methods, "")
}

func generateTokenPairWithTenantScopeAuthMethodsAndFamily(userID uint, tenantID uint, scope string, methods []string, familyID string) (TokenPair, error) {
	if scope == "" {
		scope = ConsoleScopeUser
	}

	if scope != ConsoleScopeAdmin && tenantID > 0 {
		allowed, err := tenantservice.IsTenantLoginAllowed(tenantID)
		if err != nil {
			log.Printf("[auth][error] IsTenantLoginAllowed failed for tenantID=%d, userID=%d, scope=%s: %v", tenantID, userID, scope, err)
			return TokenPair{}, err
		}
		if !allowed {
			log.Printf("[auth][warn] Tenant login disabled for tenantID=%d", tenantID)
			return TokenPair{}, ErrTenantLoginDisabled
		}
	}

	now := time.Now()
	amr := normalizeAuthMethods(methods)
	acr := acrForAuthMethods(amr)
	accessClaims := jwt.MapClaims{
		"sub":       userID,
		"tid":       tenantID, // 租户ID - 现在直接使用user.tenant_id
		"scp":       scope,    // console scope
		"typ":       TokenTypeAccess,
		"iat":       now.Unix(),
		"auth_time": now.Unix(),
		"acr":       acr,
		"amr":       amr,
		"exp":       now.Add(15 * time.Minute).Unix(),
	}
	secret, err := common.JWTSecret()
	if err != nil {
		log.Printf("[auth][error] JWT secret unavailable: %v", err)
		return TokenPair{}, err
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(secret)
	if err != nil {
		log.Printf("[auth][error] Failed to sign access token for userID=%d, tenantID=%d, scope=%s: %v", userID, tenantID, scope, err)
		return TokenPair{}, err
	}

	refreshJTI, err := randomTokenID()
	if err != nil {
		return TokenPair{}, err
	}
	if strings.TrimSpace(familyID) == "" {
		familyID = refreshJTI
	}
	refreshExpiresAt := now.Add(7 * 24 * time.Hour)
	refreshClaims := jwt.MapClaims{
		"sub":       userID,
		"tid":       tenantID,
		"scp":       scope,
		"jti":       refreshJTI,
		"fam":       familyID,
		"iat":       now.Unix(),
		"auth_time": now.Unix(),
		"acr":       acr,
		"amr":       amr,
		"exp":       refreshExpiresAt.Unix(),
		"typ":       TokenTypeRefresh,
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(secret)
	if err != nil {
		log.Printf("[auth][error] Failed to sign refresh token for userID=%d, tenantID=%d, scope=%s: %v", userID, tenantID, scope, err)
		return TokenPair{}, err
	}
	if err := storeRefreshToken(refreshJTI, familyID, refreshToken, userID, tenantID, scope, refreshExpiresAt); err != nil {
		log.Printf("[auth][error] Failed to persist refresh token for userID=%d, tenantID=%d, scope=%s: %v", userID, tenantID, scope, err)
		return TokenPair{}, err
	}

	log.Printf("[auth][debug] Tokens generated successfully for userID=%d, tenantID=%d, scope=%s", userID, tenantID, scope)
	return TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

func randomTokenID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func tokenSHA256(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func storeRefreshToken(jti, familyID, rawToken string, userID, tenantID uint, scope string, expiresAt time.Time) error {
	db := common.DB()
	if db == nil {
		return errors.New("database unavailable")
	}
	return db.Create(&model.AuthRefreshToken{
		JTI:       jti,
		FamilyID:  familyID,
		TokenHash: tokenSHA256(rawToken),
		UserID:    userID,
		TenantID:  tenantID,
		Scope:     scope,
		ExpiresAt: expiresAt,
	}).Error
}

func consumeRefreshToken(jti, familyID, rawToken string) error {
	jti = strings.TrimSpace(jti)
	familyID = strings.TrimSpace(familyID)
	if jti == "" || familyID == "" {
		return errors.New("invalid refresh token state")
	}
	db := common.DB()
	if db == nil {
		return errors.New("database unavailable")
	}

	now := time.Now()
	tokenHash := tokenSHA256(rawToken)
	consume := db.Model(&model.AuthRefreshToken{}).
		Where("jti = ? AND token_hash = ? AND family_id = ?", jti, tokenHash, familyID).
		Where("consumed_at IS NULL AND revoked_at IS NULL AND expires_at > ?", now).
		Updates(map[string]interface{}{
			"consumed_at": now,
			"revoked_at":  now,
		})
	if consume.Error != nil {
		return consume.Error
	}
	if consume.RowsAffected == 1 {
		return nil
	}

	var record model.AuthRefreshToken
	if err := db.Where("jti = ? AND token_hash = ?", jti, tokenHash).First(&record).Error; err != nil {
		return errors.New("invalid refresh token")
	}
	if record.FamilyID != familyID {
		return errors.New("invalid refresh token family")
	}
	if record.RevokedAt != nil || record.ConsumedAt != nil {
		_ = revokeRefreshTokenFamily(db, record.FamilyID, now)
		return errors.New("refresh token reuse detected")
	}
	if !record.ExpiresAt.After(now) {
		return errors.New("refresh token expired")
	}

	return errors.New("invalid refresh token state")
}

func revokeRefreshTokenFamily(db *gorm.DB, familyID string, revokedAt time.Time) error {
	return db.Model(&model.AuthRefreshToken{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", revokedAt).Error
}

func normalizeAuthMethods(methods []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		method = strings.TrimSpace(method)
		if method == "" {
			continue
		}
		if _, exists := seen[method]; exists {
			continue
		}
		seen[method] = struct{}{}
		out = append(out, method)
	}
	if len(out) == 0 {
		return []string{"pwd"}
	}
	return out
}

func acrForAuthMethods(methods []string) string {
	if len(methods) > 1 {
		return "urn:basaltpass:acr:mfa"
	}
	for _, method := range methods {
		switch method {
		case "webauthn":
			return "urn:basaltpass:acr:webauthn"
		case "otp":
			return "urn:basaltpass:acr:mfa"
		}
	}
	return "urn:basaltpass:acr:password"
}

func resolveTokenTenantID(userID uint, claimedTenantID uint, scope string) (uint, error) {
	if scope == ConsoleScopeAdmin {
		return 0, nil
	}

	var user model.User
	if err := common.DB().Select("id", "tenant_id").First(&user, userID).Error; err != nil {
		return 0, err
	}

	if claimedTenantID > 0 {
		if user.TenantID == claimedTenantID {
			return claimedTenantID, nil
		}

		var membershipCount int64
		if err := common.DB().Model(&model.TenantUser{}).
			Where("user_id = ? AND tenant_id = ?", userID, claimedTenantID).
			Count(&membershipCount).Error; err != nil {
			return 0, err
		}
		if membershipCount > 0 {
			return claimedTenantID, nil
		}
	}

	if user.TenantID > 0 {
		return user.TenantID, nil
	}

	// 平台级账号（tenant_id=0）在 user scope 下保持 tenant_id=0，
	// 避免因为历史 tenant_users 记录被错误带入某个租户上下文。
	return 0, nil
}

// ParseToken validates a JWT and returns claims.
func ParseToken(tokenStr string) (*jwt.Token, error) {
	secret, err := common.JWTSecret()
	if err != nil {
		return nil, err
	}
	return jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return secret, nil
	})
}

// ValidateAccessTokenType accepts explicit access tokens and, during the
// compatibility window, legacy access tokens without a typ claim. Refresh
// tokens and all other typed tokens are rejected.
func ValidateAccessTokenType(claims jwt.MapClaims) error {
	if claims == nil {
		return errors.New("invalid token claims")
	}

	typ, hasType := claims["typ"]
	if !hasType || typ == nil {
		return nil
	}

	typStr, ok := typ.(string)
	if !ok {
		return errors.New("invalid token type")
	}

	switch typStr {
	case "", TokenTypeAccess:
		return nil
	case TokenTypeRefresh:
		return errors.New("refresh token not allowed")
	default:
		return errors.New("invalid token type")
	}
}

// GeneratePreAuthToken issues a short-lived (5 min) one-time token emitted after the
// first-factor (password) check succeeds when 2FA is required.
// The token carries the verified user identity so the 2FA step never trusts a
// client-supplied user_id.
func GeneratePreAuthToken(userID uint, tenantID uint) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"tid": tenantID,
		"typ": TokenTypePreAuth,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	secret, err := common.JWTSecret()
	if err != nil {
		return "", err
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ParsePreAuthToken validates a pre_auth token and extracts the embedded user / tenant IDs.
// Returns an error if the token is expired, tampered with, or not of type "pre_auth".
func ParsePreAuthToken(tokenStr string) (userID uint, tenantID uint, err error) {
	secret, secretErr := common.JWTSecret()
	if secretErr != nil {
		return 0, 0, secretErr
	}
	token, parseErr := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return secret, nil
	})
	if parseErr != nil || token == nil || !token.Valid {
		return 0, 0, errors.New("invalid or expired 2FA session token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != TokenTypePreAuth {
		return 0, 0, errors.New("invalid token type")
	}
	subFloat, ok := claims["sub"].(float64)
	if !ok {
		return 0, 0, errors.New("missing subject in token")
	}
	tidFloat, _ := claims["tid"].(float64)
	return uint(subFloat), uint(tidFloat), nil
}

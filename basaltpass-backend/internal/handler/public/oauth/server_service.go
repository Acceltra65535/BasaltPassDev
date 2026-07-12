package oauth

import (
	"basaltpass-backend/internal/handler/public/app/app_user"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"basaltpass-backend/internal/service/tenantquota"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// OAuthServerService OAuth2授权服务器服务
type OAuthServerService struct {
	db *gorm.DB
}

type UserTenantAuthorizationDecision struct {
	Allowed      bool
	JoinRequired bool
	TenantID     uint
	Reason       string
}

// NewOAuthServerService 创建新的OAuth2服务器服务
func NewOAuthServerService() *OAuthServerService {
	return &OAuthServerService{
		db: common.DB(),
	}
}

func (s *OAuthServerService) database() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	s.db = common.DB()
	return s.db
}

// AuthorizeRequest 授权请求结构
type AuthorizeRequest struct {
	ClientID            string `form:"client_id" binding:"required"`
	RedirectURI         string `form:"redirect_uri" binding:"required"`
	ResponseType        string `form:"response_type" binding:"required"`
	Scope               string `form:"scope"`
	State               string `form:"state"`
	CodeChallenge       string `form:"code_challenge"`        // PKCE
	CodeChallengeMethod string `form:"code_challenge_method"` // PKCE
	Nonce               string `form:"nonce"`                 // OIDC
	Prompt              string `form:"prompt"`                // OIDC prompt: none/login/consent
	MaxAge              string `form:"max_age"`               // OIDC max_age seconds
	LoginHint           string `form:"login_hint"`            // OIDC login_hint
	Claims              string `form:"claims"`                // OIDC claims parameter
	ACRValues           string `form:"acr_values"`            // OIDC requested ACR values
}

// TokenRequest 令牌请求结构
type TokenRequest struct {
	GrantType           string `form:"grant_type" binding:"required"`
	Code                string `form:"code"`
	RedirectURI         string `form:"redirect_uri"`
	ClientID            string `form:"client_id"`
	CodeVerifier        string `form:"code_verifier"` // PKCE
	ClientAuthenticated bool   `json:"-"`
}

// TokenResponse 令牌响应结构
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// AuthorizeResponse 授权响应结构
type AuthorizeResponse struct {
	Code  string `json:"code"`
	State string `json:"state,omitempty"`
}

type OIDCAuthContext struct {
	AuthTime time.Time
	ACR      string
	AMR      []string
}

// ValidateAuthorizeRequest 验证授权请求
func (s *OAuthServerService) ValidateAuthorizeRequest(req *AuthorizeRequest) (*model.OAuthClient, error) {
	// 1. 验证response_type
	if req.ResponseType != "code" {
		return nil, errors.New("unsupported_response_type")
	}
	if err := validateOIDCAuthorizeParameters(req); err != nil {
		return nil, err
	}

	// 2. 查找并验证客户端（预加载App信息以显示应用详情）
	var client model.OAuthClient
	if err := s.db.Where("client_id = ? AND is_active = ?", req.ClientID, true).
		Preload("App").
		Preload("App.Tenant").
		First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid_client")
		}
		return nil, err
	}

	// 3. 验证redirect_uri
	if !client.ValidateRedirectURI(req.RedirectURI) {
		return nil, errors.New("invalid_redirect_uri")
	}

	// 4. 验证scope（可选）
	if req.Scope != "" {
		requestedScopes := strings.Split(req.Scope, " ")
		allowedScopes := client.GetScopeList()

		for _, reqScope := range requestedScopes {
			found := false
			for _, allowedScope := range allowedScopes {
				if strings.TrimSpace(reqScope) == strings.TrimSpace(allowedScope) {
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("invalid_scope")
			}
		}
	}

	if strings.TrimSpace(req.CodeChallengeMethod) != "" && req.CodeChallengeMethod != "S256" {
		return nil, errors.New("invalid_request")
	}
	if client.GetTokenEndpointAuthMethod() == model.OAuthTokenEndpointAuthNone {
		if strings.TrimSpace(req.CodeChallenge) == "" || req.CodeChallengeMethod != "S256" {
			return nil, errors.New("invalid_request")
		}
	}

	return &client, nil
}

func validateOIDCAuthorizeParameters(req *AuthorizeRequest) error {
	for _, prompt := range strings.Fields(req.Prompt) {
		switch prompt {
		case "none", "login", "consent", "select_account":
		default:
			return errors.New("invalid_request")
		}
	}
	if hasPrompt(req.Prompt, "none") && len(strings.Fields(req.Prompt)) > 1 {
		return errors.New("invalid_request")
	}
	if strings.TrimSpace(req.MaxAge) != "" {
		maxAge, err := strconv.Atoi(strings.TrimSpace(req.MaxAge))
		if err != nil || maxAge < 0 {
			return errors.New("invalid_request")
		}
	}
	return nil
}

func hasPrompt(promptText string, target string) bool {
	for _, prompt := range strings.Fields(promptText) {
		if prompt == target {
			return true
		}
	}
	return false
}

func maxAgeDuration(req *AuthorizeRequest) (time.Duration, bool) {
	if strings.TrimSpace(req.MaxAge) == "" {
		return 0, false
	}
	maxAge, err := strconv.Atoi(strings.TrimSpace(req.MaxAge))
	if err != nil || maxAge < 0 {
		return 0, false
	}
	return time.Duration(maxAge) * time.Second, true
}

// ValidateClientCredentials verifies OAuth client_id/client_secret.
func (s *OAuthServerService) ValidateClientCredentials(clientID string, clientSecret string) (*model.OAuthClient, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("invalid_client")
	}

	var client model.OAuthClient
	db := s.database()
	if db == nil {
		return nil, errors.New("database_unavailable")
	}
	if err := db.Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid_client")
		}
		return nil, err
	}

	if !client.VerifyClientSecret(clientSecret) {
		return nil, errors.New("invalid_client")
	}

	return &client, nil
}

// resolveClientTenantID resolves tenant ownership for an OAuth client.
// Fallback order:
// 1) client.App.TenantID (preloaded)
// 2) apps.tenant_id by client.AppID
// 3) latest authorization record tenant_id for this client
// 4) creator's tenant_user tenant_id
func (s *OAuthServerService) resolveClientTenantID(client *model.OAuthClient) uint {
	if client == nil {
		return 0
	}

	if client.App.TenantID > 0 {
		return client.App.TenantID
	}

	db := s.database()
	if db == nil {
		return 0
	}

	if client.AppID > 0 {
		var app model.App
		if err := db.Select("tenant_id").Where("id = ?", client.AppID).First(&app).Error; err == nil && app.TenantID > 0 {
			return app.TenantID
		}
	}

	if strings.TrimSpace(client.ClientID) != "" {
		var code model.OAuthAuthorizationCode
		if err := db.Select("tenant_id").
			Where("client_id = ? AND tenant_id > 0", client.ClientID).
			Order("id DESC").
			First(&code).Error; err == nil && code.TenantID > 0 {
			return code.TenantID
		}
	}

	if client.CreatedBy > 0 {
		var admin model.TenantUser
		if err := db.Select("tenant_id").
			Where("user_id = ? AND tenant_id > 0", client.CreatedBy).
			Order("id ASC").
			First(&admin).Error; err == nil && admin.TenantID > 0 {
			return admin.TenantID
		}
	}

	return 0
}

// GenerateAuthorizationCode 生成授权码
func (s *OAuthServerService) GenerateAuthorizationCode(userID uint, req *AuthorizeRequest, client *model.OAuthClient, authContexts ...OIDCAuthContext) (string, error) {
	// 生成授权码
	codeBytes := make([]byte, 32)
	if _, err := rand.Read(codeBytes); err != nil {
		return "", err
	}
	code := hex.EncodeToString(codeBytes)
	authContext := normalizeOIDCAuthContext(time.Now(), firstOIDCAuthContext(authContexts))
	if acr := firstRequestedACR(req.ACRValues); acr != "" {
		authContext.ACR = acr
	}

	// 获取租户ID（支持历史脏数据的兜底恢复）
	tenantID := s.resolveClientTenantID(client)
	maxAge := maxAgePointer(req)

	// 创建授权码记录（包含AppID和TenantID）
	authCode := &model.OAuthAuthorizationCode{
		Code:                code,
		ClientID:            req.ClientID,
		UserID:              userID,
		TenantID:            tenantID,
		AppID:               client.AppID,
		RedirectURI:         req.RedirectURI,
		Scopes:              req.Scope,
		Claims:              strings.TrimSpace(req.Claims),
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		Nonce:               strings.TrimSpace(req.Nonce),
		ACRValues:           strings.TrimSpace(req.ACRValues),
		Prompt:              strings.TrimSpace(req.Prompt),
		MaxAge:              maxAge,
		LoginHint:           strings.TrimSpace(req.LoginHint),
		AuthTime:            authContext.AuthTime,
		ACR:                 authContext.ACR,
		AMR:                 strings.Join(authContext.AMR, " "),
		ExpiresAt:           time.Now().Add(10 * time.Minute), // 授权码10分钟有效期
		Used:                false,
	}

	if err := s.db.Create(authCode).Error; err != nil {
		return "", err
	}

	return code, nil
}

func firstOIDCAuthContext(authContexts []OIDCAuthContext) OIDCAuthContext {
	if len(authContexts) == 0 {
		return OIDCAuthContext{}
	}
	return authContexts[0]
}

func normalizeOIDCAuthContext(fallback time.Time, ctx OIDCAuthContext) OIDCAuthContext {
	if ctx.AuthTime.IsZero() {
		ctx.AuthTime = fallback
	}
	ctx.AMR = normalizeOIDCAMR(ctx.AMR)
	if strings.TrimSpace(ctx.ACR) == "" {
		ctx.ACR = acrForOIDCAMR(ctx.AMR)
	}
	return ctx
}

func normalizeOIDCAMR(methods []string) []string {
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

func acrForOIDCAMR(methods []string) string {
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

func maxAgePointer(req *AuthorizeRequest) *int {
	if strings.TrimSpace(req.MaxAge) == "" {
		return nil
	}
	maxAge, err := strconv.Atoi(strings.TrimSpace(req.MaxAge))
	if err != nil || maxAge < 0 {
		return nil
	}
	return &maxAge
}

func firstRequestedACR(acrValues string) string {
	for _, value := range strings.Fields(acrValues) {
		if value != "" {
			return value
		}
	}
	return ""
}

// HasAppUserAuthorization checks whether a user has already authorized an app.
func (s *OAuthServerService) HasAppUserAuthorization(appID, userID uint) (bool, error) {
	if appID == 0 || userID == 0 {
		return false, nil
	}

	var count int64
	if err := s.db.Model(&model.AppUser{}).
		Where("app_id = ? AND user_id = ?", appID, userID).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

func hasScope(scopeText string, target string) bool {
	for _, s := range strings.Fields(scopeText) {
		if s == target {
			return true
		}
	}
	return false
}

func clientAllowsGrant(client model.OAuthClient, grantType string) bool {
	for _, grant := range client.GetGrantTypeList() {
		if strings.TrimSpace(grant) == grantType {
			return true
		}
	}
	return false
}

func isJWTClientAuthMethod(method string) bool {
	return method == model.OAuthTokenEndpointAuthClientSecretJWT || method == model.OAuthTokenEndpointAuthPrivateKeyJWT
}

func (s *OAuthServerService) oidcSubject(clientID string, userID uint) string {
	var client model.OAuthClient
	if err := s.db.Select("client_id", "subject_type", "sector_identifier_uri").Where("client_id = ?", clientID).First(&client).Error; err != nil {
		return strconv.FormatUint(uint64(userID), 10)
	}
	return oidcSubjectForClient(client, userID)
}

func oidcSubjectForClient(client model.OAuthClient, userID uint) string {
	publicSub := strconv.FormatUint(uint64(userID), 10)
	if client.GetSubjectType() != model.OAuthSubjectTypePairwise {
		return publicSub
	}
	sector := strings.TrimSpace(client.SectorIdentifierURI)
	if sector == "" {
		sector = strings.TrimSpace(client.ClientID)
	}
	sum := sha256.Sum256([]byte(oidcIssuer() + "|" + sector + "|" + publicSub))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *OAuthServerService) buildIDToken(clientID string, userID uint, nonce string, scopes string, issuedAt time.Time, authContexts ...OIDCAuthContext) (string, error) {
	privateKey, err := GetPrivateKey()
	if err != nil {
		return "", err
	}
	authContext := normalizeOIDCAuthContext(issuedAt, firstOIDCAuthContext(authContexts))

	sub := s.oidcSubject(clientID, userID)
	claims := jwt.MapClaims{
		"iss":       oidcIssuer(),
		"sub":       sub,
		"aud":       clientID,
		"azp":       clientID,
		"exp":       issuedAt.Add(time.Hour).Unix(),
		"iat":       issuedAt.Unix(),
		"auth_time": authContext.AuthTime.Unix(),
		"acr":       authContext.ACR,
		"amr":       authContext.AMR,
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	if hasScope(scopes, "profile") {
		var user model.User
		if err := s.db.First(&user, userID).Error; err != nil {
			return "", err
		}
		displayName := oidcDisplayName(user, sub)
		givenName, familyName, middleName := oidcNameParts(user, displayName)
		claims["name"] = displayName
		claims["nickname"] = oidcNickname(user, displayName)
		claims["preferred_username"] = oidcPreferredUsername(user, sub)
		claims["given_name"] = givenName
		claims["family_name"] = familyName
		if middleName != "" {
			claims["middle_name"] = middleName
		}
		if user.AvatarURL != "" {
			claims["picture"] = user.AvatarURL
		}
	}
	if hasScope(scopes, "email") {
		var user model.User
		if err := s.db.Select("email", "email_verified").First(&user, userID).Error; err != nil {
			return "", err
		}
		if email := strings.TrimSpace(user.Email); email != "" {
			claims["email"] = email
			claims["email_verified"] = user.EmailVerified
		}
	}
	if hasScope(scopes, "groups") {
		groups, err := s.oidcGroups(clientID, userID)
		if err != nil {
			return "", err
		}
		claims["groups"] = groups
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = GetKeyID()
	return token.SignedString(privateKey)
}

// oidcGroups returns stable, tenant-aware group names suitable for relying
// parties such as Harbor. System administrators receive a dedicated group
// that can be mapped directly to an administrator group at the RP.
func (s *OAuthServerService) oidcGroups(clientID string, userID uint) ([]string, error) {
	groups := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(group string) {
		group = strings.TrimSpace(group)
		if group == "" {
			return
		}
		if _, ok := seen[group]; ok {
			return
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}

	var user model.User
	if err := s.db.Select("id", "is_system_admin").First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.IsSuperAdmin() {
		add("basaltpass-system-admin")
	}

	var globalRoleCodes []string
	if err := s.db.Table("system_auth_roles").
		Select("system_auth_roles.code").
		Joins("JOIN system_auth_user_roles ON system_auth_user_roles.role_id = system_auth_roles.id").
		Where("system_auth_user_roles.user_id = ?", userID).
		Pluck("system_auth_roles.code", &globalRoleCodes).Error; err != nil {
		return nil, err
	}
	for _, code := range globalRoleCodes {
		add("role:" + code)
	}

	var client model.OAuthClient
	if err := s.db.Select("client_id", "app_id").Where("client_id = ?", clientID).First(&client).Error; err != nil {
		return nil, err
	}
	tenantID := s.resolveClientTenantID(&client)
	if tenantID > 0 {
		var tenant model.Tenant
		if err := s.db.Select("id", "code").First(&tenant, tenantID).Error; err != nil {
			return nil, err
		}
		prefix := "tenant:" + strings.TrimSpace(tenant.Code) + ":"
		var membership model.TenantUser
		if err := s.db.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&membership).Error; err == nil {
			add(prefix + string(membership.Role))
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		var tenantRoleCodes []string
		if err := s.db.Table("tenant_roles").
			Select("tenant_roles.code").
			Joins("JOIN tenant_user_roles ON tenant_user_roles.role_id = tenant_roles.id").
			Where("tenant_user_roles.user_id = ? AND tenant_user_roles.tenant_id = ?", userID, tenantID).
			Pluck("tenant_roles.code", &tenantRoleCodes).Error; err != nil {
			return nil, err
		}
		for _, code := range tenantRoleCodes {
			add(prefix + "role:" + code)
		}
	}

	sort.Strings(groups)
	return groups, nil
}

func oidcDisplayName(user model.User, sub string) string {
	if name := strings.TrimSpace(user.Nickname); name != "" {
		return name
	}
	parts := make([]string, 0, 3)
	for _, part := range []string{user.GivenName, user.MiddleName, user.FamilyName} {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		if at := strings.Index(email, "@"); at > 0 {
			return email[:at]
		}
		return email
	}
	return sub
}

func oidcPreferredUsername(user model.User, sub string) string {
	if email := strings.TrimSpace(user.Email); email != "" {
		if at := strings.Index(email, "@"); at > 0 {
			return email[:at]
		}
		return email
	}
	if username := strings.TrimSpace(user.Nickname); username != "" {
		return username
	}
	return sub
}

func oidcNickname(user model.User, fallback string) string {
	if nickname := strings.TrimSpace(user.Nickname); nickname != "" {
		return nickname
	}
	return fallback
}

func oidcNameParts(user model.User, displayName string) (string, string, string) {
	givenName := strings.TrimSpace(user.GivenName)
	familyName := strings.TrimSpace(user.FamilyName)
	middleName := strings.TrimSpace(user.MiddleName)
	if givenName != "" || familyName != "" || middleName != "" {
		if givenName == "" {
			givenName = oidcNickname(user, displayName)
		}
		if familyName == "" {
			familyName = givenName
		}
		return givenName, familyName, middleName
	}

	parts := strings.Fields(displayName)
	if len(parts) >= 2 {
		givenName = parts[0]
		familyName = parts[len(parts)-1]
		if len(parts) > 2 {
			middleName = strings.Join(parts[1:len(parts)-1], " ")
		}
		return givenName, familyName, middleName
	}
	if displayName == "" {
		return "", "", ""
	}
	return displayName, displayName, ""
}

// ExchangeCodeForToken 用授权码换取访问令牌
func (s *OAuthServerService) ExchangeCodeForToken(req *TokenRequest, clientID string, clientSecret string) (*TokenResponse, error) {
	// 1. 验证grant_type
	if req.GrantType != "authorization_code" {
		return nil, errors.New("unsupported_grant_type")
	}

	// 2. 查找授权码
	var authCode model.OAuthAuthorizationCode
	if err := s.db.Where("code = ? AND used = ?", req.Code, false).First(&authCode).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.revokeTokensForReusedAuthorizationCode(req.Code, clientID)
			return nil, errors.New("invalid_grant")
		}
		return nil, err
	}

	// 3. 检查授权码是否过期
	if authCode.IsExpired() {
		return nil, errors.New("invalid_grant")
	}

	// 4. 验证客户端ID匹配
	if authCode.ClientID != clientID {
		return nil, errors.New("invalid_client")
	}

	// 5. 查找并验证客户端
	var client model.OAuthClient
	if err := s.db.Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		return nil, errors.New("invalid_client")
	}

	// 6. 验证客户端认证方式。公共客户端必须使用 S256 PKCE，不能靠缺省 plain。
	if client.GetTokenEndpointAuthMethod() == model.OAuthTokenEndpointAuthNone {
		if clientSecret != "" || authCode.CodeChallenge == "" || authCode.CodeChallengeMethod != "S256" {
			return nil, errors.New("invalid_client")
		}
	} else if isJWTClientAuthMethod(client.GetTokenEndpointAuthMethod()) {
		if !req.ClientAuthenticated {
			return nil, errors.New("invalid_client")
		}
	} else if clientSecret == "" || !client.VerifyClientSecret(clientSecret) {
		return nil, errors.New("invalid_client")
	}

	// 7. 验证redirect_uri
	if req.RedirectURI != authCode.RedirectURI {
		return nil, errors.New("invalid_grant")
	}

	// 8. PKCE验证（如果使用）
	if authCode.CodeChallenge != "" {
		if req.CodeVerifier == "" {
			return nil, errors.New("invalid_request")
		}

		var challenge string
		switch authCode.CodeChallengeMethod {
		case "S256", "":
			// RFC 7636 §4.2: code_challenge = BASE64URL(SHA256(ASCII(code_verifier)))
			hash := sha256.Sum256([]byte(req.CodeVerifier))
			challenge = base64.RawURLEncoding.EncodeToString(hash[:])
		default:
			return nil, errors.New("invalid_request")
		}

		if challenge != authCode.CodeChallenge {
			return nil, errors.New("invalid_grant")
		}
	}

	// 9. 生成访问令牌
	if err := tenantquota.EnsureTokensWithinLimit(s.db, authCode.TenantID, time.Now()); err != nil {
		return nil, err
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	accessToken := hex.EncodeToString(tokenBytes)

	// 11. 保存访问令牌
	oauthToken := &model.OAuthAccessToken{
		Token:      accessToken,
		ClientID:   authCode.ClientID,
		UserID:     authCode.UserID,
		TenantID:   authCode.TenantID,
		AppID:      authCode.AppID,
		Scopes:     authCode.Scopes,
		Claims:     authCode.Claims,
		ExpiresAt:  time.Now().Add(1 * time.Hour), // 访问令牌1小时有效期
		AuthCodeID: &authCode.ID,
	}

	if err := s.db.Create(oauthToken).Error; err != nil {
		return nil, err
	}

	// 12. 标记授权码为已使用
	if err := s.db.Model(&authCode).Update("used", true).Error; err != nil {
		return nil, err
	}

	// 13. 记录用户对应用的授权关系
	if authCode.AppID > 0 {
		appUserService := app_user.NewAppUserService(s.db)
		if err := appUserService.RecordUserAppAuthorization(authCode.AppID, authCode.UserID, authCode.Scopes); err != nil {
			// 记录日志但不失败，因为这不是关键功能
			// TODO: 可以考虑添加日志记录
			_ = err
		}
	}

	// 14. 更新客户端最后使用时间
	now := time.Now()
	s.db.Model(&client).Update("last_used_at", &now)

	issuedAt := time.Now()
	resp := &TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600, // 1小时
		Scope:       authCode.Scopes,
	}
	authContext := normalizeOIDCAuthContext(issuedAt, OIDCAuthContext{
		AuthTime: authCode.AuthTime,
		ACR:      authCode.ACR,
		AMR:      strings.Fields(authCode.AMR),
	})
	if hasScope(authCode.Scopes, "offline_access") && clientAllowsGrant(client, "refresh_token") {
		refreshTokenBytes := make([]byte, 32)
		if _, err := rand.Read(refreshTokenBytes); err != nil {
			return nil, err
		}
		refreshToken := hex.EncodeToString(refreshTokenBytes)
		refreshTokenModel := &model.OAuthRefreshToken{
			Token:         refreshToken,
			ClientID:      authCode.ClientID,
			UserID:        authCode.UserID,
			TenantID:      authCode.TenantID,
			AppID:         authCode.AppID,
			Scopes:        authCode.Scopes,
			Nonce:         authCode.Nonce,
			AuthTime:      authContext.AuthTime,
			ACR:           authContext.ACR,
			AMR:           strings.Join(authContext.AMR, " "),
			ExpiresAt:     time.Now().Add(7 * 24 * time.Hour),
			AccessTokenID: &oauthToken.ID,
		}
		if err := s.db.Create(refreshTokenModel).Error; err != nil {
			return nil, err
		}
		resp.RefreshToken = refreshToken
	}
	if hasScope(authCode.Scopes, "openid") {
		idToken, err := s.buildIDToken(authCode.ClientID, authCode.UserID, authCode.Nonce, authCode.Scopes, issuedAt, authContext)
		if err != nil {
			return nil, err
		}
		resp.IDToken = idToken
	}
	return resp, nil
}

func (s *OAuthServerService) revokeTokensForReusedAuthorizationCode(code string, clientID string) {
	code = strings.TrimSpace(code)
	clientID = strings.TrimSpace(clientID)
	if code == "" || clientID == "" {
		return
	}
	var authCode model.OAuthAuthorizationCode
	if err := s.db.Select("id", "client_id", "used").Where("code = ?", code).First(&authCode).Error; err != nil {
		return
	}
	if !authCode.Used || authCode.ClientID != clientID {
		return
	}
	_ = s.db.Where("auth_code_id = ?", authCode.ID).Delete(&model.OAuthAccessToken{}).Error
}

// RefreshAccessToken 刷新访问令牌
func (s *OAuthServerService) RefreshAccessToken(refreshToken string, clientID string, clientSecret string, authenticated ...bool) (*TokenResponse, error) {
	// 1. 查找刷新令牌
	var refreshTokenModel model.OAuthRefreshToken
	if err := s.db.Where("token = ? AND client_id = ?", refreshToken, clientID).First(&refreshTokenModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid_grant")
		}
		return nil, err
	}

	// 1.1 检查刷新令牌是否过期
	if refreshTokenModel.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("expired_token")
	}

	// 2. 验证客户端
	var client model.OAuthClient
	if err := s.db.Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		return nil, errors.New("invalid_client")
	}

	if client.GetTokenEndpointAuthMethod() == model.OAuthTokenEndpointAuthNone {
		if clientSecret != "" {
			return nil, errors.New("invalid_client")
		}
	} else if isJWTClientAuthMethod(client.GetTokenEndpointAuthMethod()) {
		if len(authenticated) == 0 || !authenticated[0] {
			return nil, errors.New("invalid_client")
		}
	} else if clientSecret == "" || !client.VerifyClientSecret(clientSecret) {
		return nil, errors.New("invalid_client")
	}

	// 3. 生成新的访问令牌
	if err := tenantquota.EnsureTokensWithinLimit(s.db, refreshTokenModel.TenantID, time.Now()); err != nil {
		return nil, err
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	newAccessToken := hex.EncodeToString(tokenBytes)

	// 4. 生成新的刷新令牌
	refreshTokenBytes := make([]byte, 32)
	if _, err := rand.Read(refreshTokenBytes); err != nil {
		return nil, err
	}
	newRefreshTokenStr := hex.EncodeToString(refreshTokenBytes)

	// 5. 仅删除与本 refresh token 直接关联的那一个 access token，
	//    避免误杀同一用户在其他浏览器/设备上的并发会话。
	if refreshTokenModel.AccessTokenID != nil {
		s.db.Delete(&model.OAuthAccessToken{}, *refreshTokenModel.AccessTokenID)
	}

	// 6. 创建新的访问令牌
	newToken := model.OAuthAccessToken{
		Token:     newAccessToken,
		ClientID:  clientID,
		UserID:    refreshTokenModel.UserID,
		TenantID:  refreshTokenModel.TenantID,
		AppID:     refreshTokenModel.AppID,
		Scopes:    refreshTokenModel.Scopes,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	if err := s.db.Create(&newToken).Error; err != nil {
		return nil, err
	}

	// 7. 删除旧的刷新令牌
	if err := s.db.Delete(&refreshTokenModel).Error; err != nil {
		return nil, err
	}

	// 8. 创建新的刷新令牌
	newRefreshTokenModel := model.OAuthRefreshToken{
		Token:         newRefreshTokenStr,
		ClientID:      clientID,
		UserID:        refreshTokenModel.UserID,
		TenantID:      refreshTokenModel.TenantID,
		AppID:         refreshTokenModel.AppID,
		Scopes:        refreshTokenModel.Scopes,
		Nonce:         refreshTokenModel.Nonce,
		AuthTime:      refreshTokenModel.AuthTime,
		ACR:           refreshTokenModel.ACR,
		AMR:           refreshTokenModel.AMR,
		ExpiresAt:     time.Now().Add(time.Hour * 24 * 30), // 30天
		AccessTokenID: &newToken.ID,
	}

	if err := s.db.Create(&newRefreshTokenModel).Error; err != nil {
		return nil, err
	}

	resp := &TokenResponse{
		AccessToken:  newAccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: newRefreshTokenStr,
		Scope:        refreshTokenModel.Scopes,
	}
	if hasScope(refreshTokenModel.Scopes, "openid") {
		issuedAt := time.Now()
		idToken, err := s.buildIDToken(clientID, refreshTokenModel.UserID, "", refreshTokenModel.Scopes, issuedAt, OIDCAuthContext{
			AuthTime: refreshTokenModel.AuthTime,
			ACR:      refreshTokenModel.ACR,
			AMR:      strings.Fields(refreshTokenModel.AMR),
		})
		if err != nil {
			return nil, err
		}
		resp.IDToken = idToken
	}
	return resp, nil
}

// ValidateAccessToken 验证访问令牌
func (s *OAuthServerService) ValidateAccessToken(token string) (*model.OAuthAccessToken, error) {
	var oauthToken model.OAuthAccessToken
	db := s.database()
	if db == nil {
		return nil, errors.New("database_unavailable")
	}
	if err := db.
		Preload("User").
		Preload("Client").
		Preload("Tenant").
		Where("token = ?", token).
		First(&oauthToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid_token")
		}
		return nil, err
	}

	if oauthToken.IsExpired() {
		return nil, errors.New("token_expired")
	}

	return &oauthToken, nil
}

func (s *OAuthServerService) ValidateRefreshToken(token string) (*model.OAuthRefreshToken, error) {
	var refreshToken model.OAuthRefreshToken
	db := s.database()
	if db == nil {
		return nil, errors.New("database_unavailable")
	}
	if err := db.
		Preload("User").
		Preload("Client").
		Preload("Tenant").
		Where("token = ?", token).
		First(&refreshToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid_token")
		}
		return nil, err
	}
	if refreshToken.IsExpired() {
		return nil, errors.New("token_expired")
	}
	return &refreshToken, nil
}

type TokenIntrospectionResponse struct {
	Active     bool        `json:"active"`
	TokenType  string      `json:"token_type,omitempty"`
	ClientID   string      `json:"client_id,omitempty"`
	Username   string      `json:"username,omitempty"`
	Scope      string      `json:"scope,omitempty"`
	Exp        int64       `json:"exp,omitempty"`
	Iat        int64       `json:"iat,omitempty"`
	Nbf        int64       `json:"nbf,omitempty"`
	Sub        string      `json:"sub,omitempty"`
	Aud        string      `json:"aud,omitempty"`
	Iss        string      `json:"iss,omitempty"`
	TenantID   string      `json:"tenant_id,omitempty"`
	TenantCode string      `json:"tenant_code,omitempty"`
	Tenant     interface{} `json:"tenant,omitempty"`
	Act        interface{} `json:"act,omitempty"`
}

func (s *OAuthServerService) IntrospectToken(token string, authenticatedClientID string) (*TokenIntrospectionResponse, error) {
	if accessToken, err := s.ValidateAccessToken(token); err == nil {
		if accessToken.ClientID != authenticatedClientID {
			return nil, errors.New("access_denied")
		}
		resp := tokenIntrospectionFromAccessToken(accessToken)
		return resp, nil
	} else if err != nil && err.Error() != "invalid_token" && err.Error() != "token_expired" {
		return nil, err
	}

	if refreshToken, err := s.ValidateRefreshToken(token); err == nil {
		if refreshToken.ClientID != authenticatedClientID {
			return nil, errors.New("access_denied")
		}
		resp := tokenIntrospectionFromRefreshToken(refreshToken)
		return resp, nil
	} else if err != nil && err.Error() != "invalid_token" && err.Error() != "token_expired" {
		return nil, err
	}

	return &TokenIntrospectionResponse{Active: false}, nil
}

func tokenIntrospectionFromAccessToken(token *model.OAuthAccessToken) *TokenIntrospectionResponse {
	sub := strconv.FormatUint(uint64(token.UserID), 10)
	if strings.TrimSpace(token.Client.ClientID) != "" {
		sub = oidcSubjectForClient(token.Client, token.UserID)
	}
	resp := &TokenIntrospectionResponse{
		Active:    true,
		TokenType: "access_token",
		ClientID:  token.ClientID,
		Username:  token.User.Email,
		Scope:     token.Scopes,
		Exp:       token.ExpiresAt.Unix(),
		Iat:       token.CreatedAt.Unix(),
		Nbf:       token.CreatedAt.Unix(),
		Sub:       sub,
		Aud:       token.ClientID,
		Iss:       oidcIssuer(),
	}
	if token.IsExchanged && token.ActorClientID != "" {
		resp.Act = fiberMapTokenActor(token.ActorClientID, token.ActorAppID)
	}
	applyTokenTenantIntrospection(resp, token.TenantID, token.Tenant)
	return resp
}

func tokenIntrospectionFromRefreshToken(token *model.OAuthRefreshToken) *TokenIntrospectionResponse {
	sub := strconv.FormatUint(uint64(token.UserID), 10)
	if strings.TrimSpace(token.Client.ClientID) != "" {
		sub = oidcSubjectForClient(token.Client, token.UserID)
	}
	resp := &TokenIntrospectionResponse{
		Active:    true,
		TokenType: "refresh_token",
		ClientID:  token.ClientID,
		Username:  token.User.Email,
		Scope:     token.Scopes,
		Exp:       token.ExpiresAt.Unix(),
		Iat:       token.CreatedAt.Unix(),
		Nbf:       token.CreatedAt.Unix(),
		Sub:       sub,
		Aud:       token.ClientID,
		Iss:       oidcIssuer(),
	}
	applyTokenTenantIntrospection(resp, token.TenantID, token.Tenant)
	return resp
}

func fiberMapTokenActor(clientID string, appID uint) map[string]interface{} {
	return map[string]interface{}{
		"client_id": clientID,
		"app_id":    appID,
	}
}

func applyTokenTenantIntrospection(resp *TokenIntrospectionResponse, tenantID uint, tenant model.Tenant) {
	if tenantID > 0 {
		resp.TenantID = strconv.FormatUint(uint64(tenantID), 10)
	}
	if strings.TrimSpace(tenant.Code) != "" {
		resp.TenantCode = tenant.Code
		resp.Tenant = map[string]interface{}{
			"id":   tenantID,
			"code": tenant.Code,
			"name": tenant.Name,
		}
	}
}

// GetUserInfo 获取用户信息（OpenID Connect）
func (s *OAuthServerService) GetUserInfo(token string) (*UserInfoResponse, error) {
	oauthToken, err := s.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	if !hasScope(oauthToken.Scopes, "openid") {
		return nil, errors.New("insufficient_scope")
	}

	// 更新用户在应用中的最后活跃时间
	if oauthToken.AppID > 0 {
		appUserService := app_user.NewAppUserService(s.db)
		if err := appUserService.UpdateUserLastActivity(oauthToken.AppID, oauthToken.UserID); err != nil {
			// 记录日志但不失败
			_ = err
		}
	}

	user := oauthToken.User
	displayName := oidcDisplayName(user, fmt.Sprintf("%d", user.ID))
	givenName, familyName, middleName := oidcNameParts(user, displayName)
	response := &UserInfoResponse{
		Sub:       oidcSubjectForClient(oauthToken.Client, user.ID),
		UpdatedAt: user.UpdatedAt.Unix(),
	}
	if hasScope(oauthToken.Scopes, "profile") || userInfoClaimRequested(oauthToken.Claims, "name") {
		response.Name = displayName
	}
	if hasScope(oauthToken.Scopes, "profile") {
		response.Nickname = oidcNickname(user, displayName)
		response.NickName = response.Nickname
		response.PreferredUsername = oidcPreferredUsername(user, response.Sub)
		response.GivenName = givenName
		response.FamilyName = familyName
		response.MiddleName = middleName
		response.Locale = strings.TrimSpace(user.Locale)
		response.Zoneinfo = strings.TrimSpace(user.Zoneinfo)
		response.Picture = user.AvatarURL
		response.Profile = oidcProfileURL(user)
		s.applyExtendedProfileClaims(user.ID, response)
	}
	if hasScope(oauthToken.Scopes, "email") {
		response.Email = user.Email
		response.EmailVerified = &user.EmailVerified
	}
	if hasScope(oauthToken.Scopes, "phone") {
		response.Phone = user.Phone
		response.PhoneVerified = &user.PhoneVerified
	}
	if hasScope(oauthToken.Scopes, "address") {
		if address := s.oidcAddressClaim(user.ID); address != nil {
			response.Address = address
		}
	}
	if hasScope(oauthToken.Scopes, "groups") {
		groups, err := s.oidcGroups(oauthToken.ClientID, user.ID)
		if err != nil {
			return nil, err
		}
		response.Groups = groups
	}

	return response, nil
}

func userInfoClaimRequested(claimsJSON string, claim string) bool {
	if strings.TrimSpace(claimsJSON) == "" || strings.TrimSpace(claim) == "" {
		return false
	}
	var claims struct {
		Userinfo map[string]json.RawMessage `json:"userinfo"`
	}
	if err := json.Unmarshal([]byte(claimsJSON), &claims); err != nil {
		return false
	}
	_, ok := claims.Userinfo[claim]
	return ok
}

func oidcProfileURL(user model.User) string {
	if email := strings.TrimSpace(user.Email); email != "" {
		return "mailto:" + email
	}
	if user.UserUUID != "" {
		return oidcIssuer() + "/users/" + user.UserUUID
	}
	if user.ID > 0 {
		return oidcIssuer() + "/users/" + strconv.FormatUint(uint64(user.ID), 10)
	}
	return ""
}

func (s *OAuthServerService) applyExtendedProfileClaims(userID uint, response *UserInfoResponse) {
	var profile model.UserProfile
	if err := s.db.Preload("Gender").Where("user_id = ?", userID).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		return
	}

	if website := strings.TrimSpace(profile.Website); website != "" {
		response.Website = website
	}
	if profile.Gender != nil {
		response.Gender = strings.TrimSpace(profile.Gender.Code)
	}
	if profile.BirthDate != nil {
		response.Birthdate = profile.BirthDate.Format("2006-01-02")
	}
}

func (s *OAuthServerService) oidcAddressClaim(userID uint) *OIDCAddressClaim {
	var profile model.UserProfile
	if err := s.db.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		return nil
	}
	formatted := strings.TrimSpace(profile.Location)
	if formatted == "" {
		formatted = strings.TrimSpace(profile.Website)
	}
	if formatted == "" {
		return nil
	}
	return &OIDCAddressClaim{
		Formatted:     formatted,
		StreetAddress: formatted,
		Locality:      formatted,
		Region:        formatted,
		PostalCode:    "00000",
		Country:       formatted,
	}
}

// UserInfoResponse 用户信息响应（OpenID Connect标准）
type UserInfoResponse struct {
	Sub               string            `json:"sub"`                             // 用户唯一标识
	Name              string            `json:"name,omitempty"`                  // 用户姓名
	Nickname          string            `json:"nickname,omitempty"`              // 昵称
	NickName          string            `json:"nick_name,omitempty"`             // 兼容别名
	PreferredUsername string            `json:"preferred_username,omitempty"`    // 首选用户名
	Profile           string            `json:"profile,omitempty"`               // 个人资料URL
	Website           string            `json:"website,omitempty"`               // 网站
	GivenName         string            `json:"given_name,omitempty"`            // 名
	FamilyName        string            `json:"family_name,omitempty"`           // 姓
	MiddleName        string            `json:"middle_name,omitempty"`           // 中间名
	Gender            string            `json:"gender,omitempty"`                // 性别
	Birthdate         string            `json:"birthdate,omitempty"`             // 生日
	Locale            string            `json:"locale,omitempty"`                // 区域
	Zoneinfo          string            `json:"zoneinfo,omitempty"`              // 时区
	Email             string            `json:"email,omitempty"`                 // 邮箱
	EmailVerified     *bool             `json:"email_verified,omitempty"`        // 邮箱是否验证
	Phone             string            `json:"phone_number,omitempty"`          // 手机号
	PhoneVerified     *bool             `json:"phone_number_verified,omitempty"` // 手机号是否验证
	Address           *OIDCAddressClaim `json:"address,omitempty"`               // 地址
	Picture           string            `json:"picture,omitempty"`               // 头像
	UpdatedAt         int64             `json:"updated_at"`                      // 更新时间
	Groups            []string          `json:"groups,omitempty"`                // 用户所属角色组
}

type OIDCAddressClaim struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"street_address,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	Country       string `json:"country,omitempty"`
}

// BuildAuthorizeURL 构建授权URL
func (s *OAuthServerService) BuildAuthorizeURL(baseURL string, req *AuthorizeRequest) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("client_id", req.ClientID)
	q.Set("redirect_uri", req.RedirectURI)
	q.Set("response_type", req.ResponseType)
	if req.Scope != "" {
		q.Set("scope", req.Scope)
	}
	if req.State != "" {
		q.Set("state", req.State)
	}
	if req.Nonce != "" {
		q.Set("nonce", req.Nonce)
	}
	if req.Prompt != "" {
		q.Set("prompt", req.Prompt)
	}
	if req.MaxAge != "" {
		q.Set("max_age", req.MaxAge)
	}
	if req.LoginHint != "" {
		q.Set("login_hint", req.LoginHint)
	}
	if req.CodeChallenge != "" {
		q.Set("code_challenge", req.CodeChallenge)
		q.Set("code_challenge_method", req.CodeChallengeMethod)
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ResolveTokenClientID resolves the owner client_id of a token.
// It checks both access token and refresh token tables.
func (s *OAuthServerService) ResolveTokenClientID(token string) (clientID string, found bool, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false, nil
	}

	var accessToken model.OAuthAccessToken
	if err := s.db.Select("client_id").Where("token = ?", token).First(&accessToken).Error; err == nil {
		return accessToken.ClientID, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, err
	}

	var refreshToken model.OAuthRefreshToken
	if err := s.db.Select("client_id").Where("token = ?", token).First(&refreshToken).Error; err == nil {
		return refreshToken.ClientID, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, err
	}

	return "", false, nil
}

// RevokeToken 撤销令牌
func (s *OAuthServerService) RevokeToken(token string) error {
	// 删除访问令牌
	accessTokenErr := s.db.Where("token = ?", token).Delete(&model.OAuthAccessToken{}).Error

	// 删除刷新令牌
	refreshTokenErr := s.db.Where("token = ?", token).Delete(&model.OAuthRefreshToken{}).Error

	// 只要有一个删除成功就认为操作成功
	if accessTokenErr != nil && refreshTokenErr != nil {
		return accessTokenErr
	}

	return nil
}

func (s *OAuthServerService) EnsureUserTenantIdentity(userID, tenantID uint, role model.TenantRole) error {
	if userID == 0 || tenantID == 0 {
		return errors.New("invalid_identity_context")
	}
	if role == "" {
		role = model.TenantRoleMember
	}

	var existing model.TenantUser
	err := s.db.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if err := tenantquota.EnsureUserCanJoin(s.db, tenantID, userID); err != nil {
		return err
	}
	return s.db.Create(&model.TenantUser{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
	}).Error
}

func (s *OAuthServerService) EvaluateUserTenantAuthorization(userID uint, client *model.OAuthClient) (*UserTenantAuthorizationDecision, error) {
	var user model.User
	if err := s.db.Select("id", "enforced_tenant_id", "is_system_admin").First(&user, userID).Error; err != nil {
		return nil, errors.New("user_not_found")
	}

	tenantID := s.resolveClientTenantID(client)
	if tenantID == 0 {
		return nil, errors.New("app_tenant_not_found")
	}

	if user.IsSystemAdmin != nil && *user.IsSystemAdmin {
		return &UserTenantAuthorizationDecision{Allowed: true, TenantID: tenantID}, nil
	}

	if user.EnforcedTenantID != 0 && user.EnforcedTenantID != tenantID {
		return &UserTenantAuthorizationDecision{
			Allowed:      false,
			JoinRequired: false,
			TenantID:     tenantID,
			Reason:       "tenant_mismatch",
		}, nil
	}

	var membershipCount int64
	if err := s.db.Model(&model.TenantUser{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Count(&membershipCount).Error; err != nil {
		return nil, err
	}
	if membershipCount > 0 {
		return &UserTenantAuthorizationDecision{Allowed: true, TenantID: tenantID}, nil
	}

	return &UserTenantAuthorizationDecision{
		Allowed:      false,
		JoinRequired: true,
		TenantID:     tenantID,
		Reason:       "join_required",
	}, nil
}

// ValidateUserTenant 验证用户是否属于应用所在的租户
func (s *OAuthServerService) ValidateUserTenant(userID uint, client *model.OAuthClient) error {
	decision, err := s.EvaluateUserTenantAuthorization(userID, client)
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	if decision.JoinRequired {
		return errors.New("join_required")
	}
	if decision.Reason != "" {
		return errors.New(decision.Reason)
	}
	return errors.New("tenant_mismatch")
}

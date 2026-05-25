package oauth

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/config"
	"basaltpass-backend/internal/service/aduit"
	serviceauth "basaltpass-backend/internal/service/auth"
	"basaltpass-backend/internal/utils"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"basaltpass-backend/internal/model"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

var oauthServerService = NewOAuthServerService()

func oidcRequestValue(c *fiber.Ctx, requestObjectClaims map[string]string, key string) string {
	if value := strings.TrimSpace(requestObjectClaims[key]); value != "" {
		return value
	}
	return c.Query(key)
}

func parseUnsignedOIDCRequestObject(requestObject string) (map[string]string, error) {
	parts := strings.Split(requestObject, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid request object")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, err
	}
	if alg, _ := header["alg"].(string); alg != "none" {
		return nil, errors.New("unsupported request object alg")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, err
	}
	claims := make(map[string]string, len(payload))
	for key, value := range payload {
		switch v := value.(type) {
		case string:
			claims[key] = v
		default:
			encoded, err := json.Marshal(v)
			if err == nil {
				claims[key] = string(encoded)
			}
		}
	}
	return claims, nil
}

// AuthorizeHandler 处理OAuth2授权请求
// GET /oauth/authorize
func AuthorizeHandler(c *fiber.Ctx) error {
	requestObjectClaims := map[string]string{}
	if requestObject := strings.TrimSpace(c.Query("request")); requestObject != "" {
		claims, err := parseUnsignedOIDCRequestObject(requestObject)
		if err != nil {
			return redirectWithErrorIfAllowed(c, c.Query("client_id"), c.Query("redirect_uri"), "invalid_request", c.Query("state"))
		}
		requestObjectClaims = claims
	}

	// 解析授权请求参数
	req := &AuthorizeRequest{
		ClientID:            oidcRequestValue(c, requestObjectClaims, "client_id"),
		RedirectURI:         oidcRequestValue(c, requestObjectClaims, "redirect_uri"),
		ResponseType:        oidcRequestValue(c, requestObjectClaims, "response_type"),
		Scope:               oidcRequestValue(c, requestObjectClaims, "scope"),
		State:               oidcRequestValue(c, requestObjectClaims, "state"),
		CodeChallenge:       oidcRequestValue(c, requestObjectClaims, "code_challenge"),
		CodeChallengeMethod: oidcRequestValue(c, requestObjectClaims, "code_challenge_method"),
		Nonce:               oidcRequestValue(c, requestObjectClaims, "nonce"),
		Prompt:              oidcRequestValue(c, requestObjectClaims, "prompt"),
		MaxAge:              oidcRequestValue(c, requestObjectClaims, "max_age"),
		LoginHint:           oidcRequestValue(c, requestObjectClaims, "login_hint"),
		Claims:              oidcRequestValue(c, requestObjectClaims, "claims"),
		ACRValues:           oidcRequestValue(c, requestObjectClaims, "acr_values"),
	}

	// 验证授权请求
	client, err := oauthServerService.ValidateAuthorizeRequest(req)
	if err != nil {
		return redirectWithErrorIfAllowed(c, req.ClientID, req.RedirectURI, err.Error(), req.State)
	}

	// 检查用户是否已登录
	userID := c.Locals("userID")
	sessionContext := OIDCAuthContext{}
	if userID == nil {
		// Hosted login flow uses an HttpOnly cookie; browser redirects can't attach Authorization headers.
		if uid, ctx, ok := tryUserIDFromAccessTokenCookie(c); ok {
			c.Locals("userID", uid)
			userID = uid
			sessionContext = ctx
		}
	}
	if userID == nil {
		if hasPrompt(req.Prompt, "none") {
			return redirectWithErrorIfAllowed(c, req.ClientID, req.RedirectURI, "login_required", req.State)
		}
		// 用户未登录，重定向到登录页面
		// 构建租户特定的登录URL
		loginURL := buildLoginURLWithTenant(c, req, client)
		return c.Redirect(loginURL, http.StatusFound)
	}
	if hasPrompt(req.Prompt, "login") || authContextTooOld(sessionContext, req) {
		loginURL := buildLoginURLWithTenant(c, req, client)
		return c.Redirect(loginURL, http.StatusFound)
	}

	// 用户已登录，验证用户是否属于该租户
	uid := userID.(uint)
	decision, err := oauthServerService.EvaluateUserTenantAuthorization(uid, client)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":             "tenant_context_error",
			"error_description": err.Error(),
		})
	}

	// 用户已登录且属于正确的租户，重定向到前端托管的授权同意页面
	if decision.Allowed {
		alreadyAuthorized, err := oauthServerService.HasAppUserAuthorization(client.AppID, uid)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":             "server_error",
				"error_description": "Failed to check existing app authorization",
			})
		}

		// Skip consent when the user has already authorized this app.
		if alreadyAuthorized && !hasPrompt(req.Prompt, "consent") {
			code, err := oauthServerService.GenerateAuthorizationCode(uid, req, client, sessionContext)
			if err != nil {
				return redirectWithError(c, req.RedirectURI, "server_error", req.State)
			}

			aduit.LogAudit(uid, "OAuth2授权(免再次确认)", "oauth_client", req.ClientID, c.IP(), c.Get("User-Agent"))
			return redirectWithCode(c, req.RedirectURI, code, req.State)
		}
	}
	if hasPrompt(req.Prompt, "none") {
		return redirectWithErrorIfAllowed(c, req.ClientID, req.RedirectURI, "consent_required", req.State)
	}

	consentURL := buildConsentURL(req, client, decision)
	return c.Redirect(consentURL, http.StatusFound)
}

func tryUserIDFromAccessTokenCookie(c *fiber.Ctx) (uint, OIDCAuthContext, bool) {
	// OAuth hosted flow can come from different consoles (user/tenant/admin),
	// so we need to accept scoped cookie names as well.
	cookieNames := []string{
		"access_token",
		"access_token_user",
		"access_token_tenant",
		"access_token_admin",
	}

	for _, name := range cookieNames {
		tokenStr := strings.TrimSpace(c.Cookies(name))
		if tokenStr == "" {
			continue
		}
		if uid, ctx, ok := parseOIDCSessionFromJWT(tokenStr); ok {
			return uid, ctx, true
		}
	}

	return 0, OIDCAuthContext{}, false
}

func parseUserIDFromJWT(tokenStr string) (uint, bool) {
	uid, _, ok := parseOIDCSessionFromJWT(tokenStr)
	return uid, ok
}

func parseOIDCSessionFromJWT(tokenStr string) (uint, OIDCAuthContext, bool) {
	secret, err := common.JWTSecret()
	if err != nil {
		return 0, OIDCAuthContext{}, false
	}

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return secret, nil
	})
	if err != nil || token == nil || !token.Valid {
		return 0, OIDCAuthContext{}, false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, OIDCAuthContext{}, false
	}
	if err := serviceauth.ValidateAccessTokenType(claims); err != nil {
		return 0, OIDCAuthContext{}, false
	}
	sub, exists := claims["sub"]
	if !exists {
		return 0, OIDCAuthContext{}, false
	}
	ctx := authContextFromJWTClaims(claims)
	if subFloat, ok := sub.(float64); ok {
		return uint(subFloat), ctx, true
	}
	if subStr, ok := sub.(string); ok {
		uid, err := strconv.ParseUint(subStr, 10, 64)
		if err == nil {
			return uint(uid), ctx, true
		}
	}
	return 0, OIDCAuthContext{}, false
}

func authContextFromJWTClaims(claims jwt.MapClaims) OIDCAuthContext {
	ctx := OIDCAuthContext{ACR: claimString(claims["acr"]), AMR: claimStringList(claims["amr"])}
	if authTime := unixClaimTime(claims["auth_time"]); !authTime.IsZero() {
		ctx.AuthTime = authTime
	} else if iat := unixClaimTime(claims["iat"]); !iat.IsZero() {
		ctx.AuthTime = iat
	}
	return normalizeOIDCAuthContext(time.Now(), ctx)
}

func unixClaimTime(value interface{}) time.Time {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return time.Unix(int64(v), 0)
		}
	case int64:
		if v > 0 {
			return time.Unix(v, 0)
		}
	case int:
		if v > 0 {
			return time.Unix(int64(v), 0)
		}
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil && parsed > 0 {
			return time.Unix(parsed, 0)
		}
	}
	return time.Time{}
}

func authContextTooOld(ctx OIDCAuthContext, req *AuthorizeRequest) bool {
	maxAge, ok := maxAgeDuration(req)
	if !ok {
		return false
	}
	if ctx.AuthTime.IsZero() {
		return true
	}
	return time.Now().Unix()-ctx.AuthTime.Unix() > int64(maxAge/time.Second)
}

func buildConsentURL(req *AuthorizeRequest, client *model.OAuthClient, decision *UserTenantAuthorizationDecision) string {
	uiBaseURL := strings.TrimRight(config.Get().UI.BaseURL, "/")
	consentPath := "/oauth-consent"
	base := consentPath
	if uiBaseURL != "" {
		base = uiBaseURL + consentPath
	}

	q := url.Values{}
	q.Set("client_id", req.ClientID)
	q.Set("redirect_uri", req.RedirectURI)
	if req.Scope != "" {
		q.Set("scope", req.Scope)
	}
	if req.State != "" {
		q.Set("state", req.State)
	}
	if req.CodeChallenge != "" {
		q.Set("code_challenge", req.CodeChallenge)
	}
	if req.CodeChallengeMethod != "" {
		q.Set("code_challenge_method", req.CodeChallengeMethod)
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
	if req.Claims != "" {
		q.Set("claims", req.Claims)
	}
	if req.ACRValues != "" {
		q.Set("acr_values", req.ACRValues)
	}
	if client != nil {
		appTenantID := oauthServerService.resolveClientTenantID(client)
		if appTenantID > 0 {
			q.Set("app_tenant_id", strconv.FormatUint(uint64(appTenantID), 10))
		}
	}
	if decision != nil {
		if decision.JoinRequired {
			q.Set("current_user_join_required", "true")
		}
		if decision.TenantID > 0 {
			q.Set("decision_tenant_id", strconv.FormatUint(uint64(decision.TenantID), 10))
		}
	}
	if client != nil {
		// Prefer app display fields when available.
		if strings.TrimSpace(client.App.Name) != "" {
			q.Set("client_name", client.App.Name)
		}
		if strings.TrimSpace(client.App.Description) != "" {
			q.Set("client_description", client.App.Description)
		}
		if strings.TrimSpace(client.App.PrivacyPolicyURL) != "" {
			q.Set("privacy_policy_url", client.App.PrivacyPolicyURL)
		}
		if strings.TrimSpace(client.App.TermsOfServiceURL) != "" {
			q.Set("terms_of_service_url", client.App.TermsOfServiceURL)
		}
		if client.App.IsVerified {
			q.Set("is_verified", "true")
		}
	}

	return base + "?" + q.Encode()
}

// ConsentHandler 处理用户授权同意
// POST /oauth/consent
func ConsentHandler(c *fiber.Ctx) error {
	// 检查用户是否已登录
	userIDVal := c.Locals("userID")
	authContext := OIDCAuthContext{}
	if userIDVal == nil {
		if uid, ctx, ok := tryUserIDFromAccessTokenCookie(c); ok {
			c.Locals("userID", uid)
			userIDVal = uid
			authContext = ctx
		}
	}
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID := userIDVal.(uint)

	// 解析表单数据
	clientID := c.FormValue("client_id")
	redirectURI := c.FormValue("redirect_uri")
	state := c.FormValue("state")
	scope := c.FormValue("scope")
	codeChallenge := c.FormValue("code_challenge")
	codeChallengeMethod := c.FormValue("code_challenge_method")
	nonce := c.FormValue("nonce")
	prompt := c.FormValue("prompt")
	maxAge := c.FormValue("max_age")
	loginHint := c.FormValue("login_hint")
	claims := c.FormValue("claims")
	acrValues := c.FormValue("acr_values")
	selectedAccessToken := strings.TrimSpace(c.FormValue("selected_access_token"))
	joinTenant := strings.EqualFold(strings.TrimSpace(c.FormValue("join_tenant")), "true") || c.FormValue("join_tenant") == "1"
	action := c.FormValue("action") // "allow" 或 "deny"

	if action != "allow" {
		// 用户拒绝授权
		return redirectWithErrorIfAllowed(c, clientID, redirectURI, "access_denied", state)
	}

	if selectedAccessToken != "" {
		selectedUserID, selectedContext, ok := parseOIDCSessionFromJWT(selectedAccessToken)
		if !ok || selectedUserID == 0 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_selected_session",
				"error_description": "Selected account session is invalid or expired",
			})
		}
		userID = selectedUserID
		authContext = selectedContext
	} else if _, cookieContext, ok := tryUserIDFromAccessTokenCookie(c); ok {
		authContext = cookieContext
	}

	// 构建授权请求
	req := &AuthorizeRequest{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		ResponseType:        "code",
		Scope:               scope,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Nonce:               nonce,
		Prompt:              prompt,
		MaxAge:              maxAge,
		LoginHint:           loginHint,
		Claims:              claims,
		ACRValues:           acrValues,
	}

	// 验证请求
	client, err := oauthServerService.ValidateAuthorizeRequest(req)
	if err != nil {
		return redirectWithErrorIfAllowed(c, clientID, redirectURI, err.Error(), state)
	}

	decision, err := oauthServerService.EvaluateUserTenantAuthorization(userID, client)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":             "tenant_context_error",
			"error_description": err.Error(),
		})
	}

	if !decision.Allowed {
		if decision.JoinRequired {
			if !joinTenant {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error":             "join_confirmation_required",
					"error_description": "Global account must confirm tenant join before authorization",
				})
			}
			if err := oauthServerService.EnsureUserTenantIdentity(userID, decision.TenantID, model.TenantRoleMember); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error":             "identity_join_failed",
					"error_description": "Failed to join tenant identity",
				})
			}
		} else {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":             "tenant_mismatch",
				"error_description": "User does not belong to the tenant of this application",
			})
		}
	}

	// 生成授权码
	code, err := oauthServerService.GenerateAuthorizationCode(userID, req, client, authContext)
	if err != nil {
		return redirectWithError(c, redirectURI, "server_error", state)
	}

	// 记录审计日志
	aduit.LogAudit(userID, "OAuth2授权", "oauth_client", clientID, c.IP(), c.Get("User-Agent"))

	// 重定向回客户端
	return redirectWithCode(c, redirectURI, code, state)
}

// TokenHandler 处理OAuth2令牌请求
// POST /oauth/token
func TokenHandler(c *fiber.Ctx) error {
	setOAuthTokenResponseHeaders(c)
	grantType := c.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		return handleAuthorizationCodeGrant(c)
	case "refresh_token":
		return handleRefreshTokenGrant(c)
	case "urn:ietf:params:oauth:grant-type:token-exchange":
		return handleTokenExchangeGrant(c)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "unsupported_grant_type",
			"error_description": "Grant type not supported",
		})
	}
}

func setOAuthTokenResponseHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")
}

// handleAuthorizationCodeGrant 处理授权码授权
func handleAuthorizationCodeGrant(c *fiber.Ctx) error {
	// 解析令牌请求
	req := &TokenRequest{
		GrantType:    c.FormValue("grant_type"),
		Code:         c.FormValue("code"),
		RedirectURI:  c.FormValue("redirect_uri"),
		ClientID:     c.FormValue("client_id"),
		CodeVerifier: c.FormValue("code_verifier"),
	}

	client, err := authenticateOAuthClientForEndpoint(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":             "invalid_client",
			"error_description": "Client authentication failed",
		})
	}
	clientID := client.ClientID
	clientSecret := clientSecretForLegacyValidation(c, client)
	req.ClientAuthenticated = true

	// 交换令牌
	tokenResponse, err := oauthServerService.ExchangeCodeForToken(req, clientID, clientSecret)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             err.Error(),
			"error_description": "Authorization code exchange failed",
		})
	}

	return c.JSON(tokenResponse)
}

// handleRefreshTokenGrant 处理刷新令牌授权
func handleRefreshTokenGrant(c *fiber.Ctx) error {
	refreshToken := c.FormValue("refresh_token")

	client, err := authenticateOAuthClientForEndpoint(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":             "invalid_client",
			"error_description": "Client authentication failed",
		})
	}
	clientID := client.ClientID
	clientSecret := clientSecretForLegacyValidation(c, client)

	// 刷新令牌
	tokenResponse, err := oauthServerService.RefreshAccessToken(refreshToken, clientID, clientSecret, true)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             err.Error(),
			"error_description": "Refresh token failed",
		})
	}

	return c.JSON(tokenResponse)
}

// UserInfoHandler 处理用户信息请求（OpenID Connect）
// GET /oauth/userinfo
func UserInfoHandler(c *fiber.Ctx) error {
	// 从Authorization头获取访问令牌
	authHeader := c.Get("Authorization")
	token := ""
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	} else if c.Method() == fiber.MethodPost {
		token = strings.TrimSpace(c.FormValue("access_token"))
	}
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":             "invalid_token",
			"error_description": "Missing or invalid access token",
		})
	}

	// 获取用户信息
	userInfo, err := oauthServerService.GetUserInfo(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":             err.Error(),
			"error_description": "Failed to get user info",
		})
	}

	return c.JSON(userInfo)
}

// IntrospectHandler 令牌内省端点
// POST /oauth/introspect
func IntrospectHandler(c *fiber.Ctx) error {
	authenticatedClientID := getAuthenticatedOAuthClientID(c)
	if authenticatedClientID == "" {
		return oauthInvalidClient(c)
	}

	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing token parameter",
		})
	}

	introspection, err := oauthServerService.IntrospectToken(token, authenticatedClientID)
	if err != nil {
		if err.Error() == "access_denied" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":             "access_denied",
				"error_description": "Token does not belong to authenticated client",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "server_error"})
	}

	return c.JSON(introspection)
}

// RevokeHandler 令牌撤销端点
// POST /oauth/revoke
func RevokeHandler(c *fiber.Ctx) error {
	authenticatedClientID := getAuthenticatedOAuthClientID(c)
	if authenticatedClientID == "" {
		return oauthInvalidClient(c)
	}

	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             "invalid_request",
			"error_description": "Missing token parameter",
		})
	}

	tokenClientID, found, err := oauthServerService.ResolveTokenClientID(token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "server_error",
		})
	}
	if !found {
		// RFC7009 recommends treating unknown tokens as a successful no-op.
		return c.SendStatus(fiber.StatusOK)
	}
	if tokenClientID != authenticatedClientID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":             "access_denied",
			"error_description": "Token does not belong to authenticated client",
		})
	}

	// 撤销令牌
	err = oauthServerService.RevokeToken(token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "server_error",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

// 辅助函数

// buildLoginURLWithTenant 构建租户特定的登录URL，包含OAuth2参数
func buildLoginURLWithTenant(c *fiber.Ctx, req *AuthorizeRequest, client *model.OAuthClient) string {
	var tenantCode string
	tenantID := oauthServerService.resolveClientTenantID(client)
	if tenantID > 0 {
		var tenant model.Tenant
		if err := oauthServerService.db.Select("id", "code").First(&tenant, tenantID).Error; err == nil {
			tenantCode = strings.TrimSpace(tenant.Code)
		}
	}

	uiBaseURL := strings.TrimRight(config.Get().UI.BaseURL, "/")

	// 构建租户登录URL：/auth/tenant/{tenant_code}/login
	var loginURL string
	if tenantCode != "" {
		loginURL = uiBaseURL + "/auth/tenant/" + tenantCode + "/login"
	} else {
		// 如果没有租户code，使用平台登录
		loginURL = uiBaseURL + "/login"
	}

	originalURL := buildAuthorizeReturnURL(c, req)

	q := url.Values{}
	q.Set("redirect", originalURL)
	if strings.TrimSpace(req.LoginHint) != "" {
		q.Set("login_hint", strings.TrimSpace(req.LoginHint))
	}
	return loginURL + "?" + q.Encode()
}

// buildLoginURL 构建登录URL，包含OAuth2参数（保留向后兼容性）
func buildLoginURL(c *fiber.Ctx, req *AuthorizeRequest) string {
	loginURL := "/login"
	uiBaseURL := strings.TrimRight(config.Get().UI.BaseURL, "/")
	if uiBaseURL != "" {
		loginURL = uiBaseURL + "/login"
	}

	originalURL := buildAuthorizeReturnURL(c, req)

	q := url.Values{}
	q.Set("redirect", originalURL)
	if strings.TrimSpace(req.LoginHint) != "" {
		q.Set("login_hint", strings.TrimSpace(req.LoginHint))
	}
	return loginURL + "?" + q.Encode()
}

func buildAuthorizeReturnURL(c *fiber.Ctx, req *AuthorizeRequest) string {
	u := "/api/v1/oauth/authorize"
	q, err := url.ParseQuery(c.Context().QueryArgs().String())
	if err != nil {
		q = url.Values{}
	}
	prompts := make([]string, 0)
	for _, prompt := range strings.Fields(req.Prompt) {
		if prompt != "login" {
			prompts = append(prompts, prompt)
		}
	}
	if len(prompts) == 0 {
		q.Del("prompt")
	} else {
		q.Set("prompt", strings.Join(prompts, " "))
	}
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}
	return u
}

// redirectWithError 带错误信息重定向
func redirectWithError(c *fiber.Ctx, redirectURI, errorCode, state string) error {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid_redirect_uri"})
	}

	q := u.Query()
	q.Set("error", errorCode)
	if state != "" {
		q.Set("state", state)
	}

	u.RawQuery = q.Encode()
	return c.Redirect(u.String(), http.StatusFound)
}

// redirectWithErrorIfAllowed redirects only when redirect_uri is registered on the client.
// Per OAuth2 security recommendations, invalid client/redirect_uri errors must not redirect.
func redirectWithErrorIfAllowed(c *fiber.Ctx, clientID, redirectURI, errorCode, state string) error {
	if !isRedirectURIAllowedForClient(clientID, redirectURI) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorCode,
			"error_description": "invalid redirect_uri",
		})
	}
	return redirectWithError(c, redirectURI, errorCode, state)
}

func isRedirectURIAllowedForClient(clientID, redirectURI string) bool {
	clientID = strings.TrimSpace(clientID)
	redirectURI = strings.TrimSpace(redirectURI)
	if clientID == "" || redirectURI == "" {
		return false
	}

	var client model.OAuthClient
	if err := oauthServerService.db.Select("client_id", "redirect_uris").
		Where("client_id = ?", clientID).
		First(&client).Error; err != nil {
		return false
	}

	return client.ValidateRedirectURI(redirectURI)
}

// redirectWithCode 带授权码重定向
func redirectWithCode(c *fiber.Ctx, redirectURI, code, state string) error {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid_redirect_uri"})
	}

	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}

	u.RawQuery = q.Encode()
	return c.Redirect(u.String(), http.StatusFound)
}

// extractClientCredentials 提取客户端凭证（支持Basic Auth和表单参数）
func extractClientCredentials(c *fiber.Ctx) (clientID, clientSecret string) {
	// 优先从 Authorization: Basic 获取（OAuth2 标准方式）
	if basicID, basicSecret, ok := parseBasicAuthCredentials(c.Get("Authorization")); ok {
		return basicID, basicSecret
	}

	// 向后兼容：支持自定义头 client_id/client_secret
	clientID = strings.TrimSpace(c.Get("client_id"))
	clientSecret = c.Get("client_secret")

	if clientID != "" && clientSecret != "" {
		return clientID, clientSecret
	}

	// 尝试从表单参数获取
	clientID = strings.TrimSpace(c.FormValue("client_id"))
	clientSecret = c.FormValue("client_secret")

	return clientID, clientSecret
}

func clientSecretForLegacyValidation(c *fiber.Ctx, client *model.OAuthClient) string {
	if client == nil {
		return ""
	}
	if isJWTClientAuthMethod(client.GetTokenEndpointAuthMethod()) {
		return ""
	}
	_, secret := extractClientCredentials(c)
	return secret
}

func authenticateOAuthClientForEndpoint(c *fiber.Ctx) (*model.OAuthClient, error) {
	assertion := strings.TrimSpace(c.FormValue("client_assertion"))
	assertionType := strings.TrimSpace(c.FormValue("client_assertion_type"))
	if assertion != "" || assertionType != "" {
		return authenticateOAuthClientAssertion(c, assertionType, assertion)
	}

	clientID, clientSecret := extractClientCredentials(c)
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = strings.TrimSpace(c.FormValue("client_id"))
	}
	if clientID == "" {
		return nil, errors.New("invalid_client")
	}

	var client model.OAuthClient
	if err := common.DB().Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		return nil, errors.New("invalid_client")
	}

	switch client.GetTokenEndpointAuthMethod() {
	case model.OAuthTokenEndpointAuthNone:
		if clientSecret != "" {
			return nil, errors.New("invalid_client")
		}
		return &client, nil
	case model.OAuthTokenEndpointAuthClientSecretBasic, model.OAuthTokenEndpointAuthClientSecretPost:
		if clientSecret == "" || !client.VerifyClientSecret(clientSecret) {
			return nil, errors.New("invalid_client")
		}
		return &client, nil
	default:
		return nil, errors.New("invalid_client")
	}
}

const clientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

func authenticateOAuthClientAssertion(c *fiber.Ctx, assertionType string, assertion string) (*model.OAuthClient, error) {
	if assertionType != clientAssertionTypeJWTBearer || assertion == "" {
		return nil, errors.New("invalid_client")
	}

	claims := jwt.MapClaims{}
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	unverified, _, err := parser.ParseUnverified(assertion, claims)
	if err != nil || unverified == nil {
		return nil, errors.New("invalid_client")
	}

	clientID := strings.TrimSpace(claimString(claims["iss"]))
	if clientID == "" {
		clientID = strings.TrimSpace(c.FormValue("client_id"))
	}
	if clientID == "" || claimString(claims["sub"]) != clientID {
		return nil, errors.New("invalid_client")
	}

	var client model.OAuthClient
	if err := common.DB().Where("client_id = ? AND is_active = ?", clientID, true).First(&client).Error; err != nil {
		return nil, errors.New("invalid_client")
	}

	keyFunc, err := clientAssertionKeyFunc(&client)
	if err != nil {
		return nil, err
	}
	validatedClaims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(assertion, validatedClaims, keyFunc, jwt.WithAudience(tokenEndpointAudience()), jwt.WithIssuer(clientID), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid {
		return nil, errors.New("invalid_client")
	}
	if claimString(validatedClaims["sub"]) != clientID {
		return nil, errors.New("invalid_client")
	}
	return &client, nil
}

func tokenEndpointAudience() string {
	return oidcIssuer() + "/oauth/token"
}

func clientAssertionKeyFunc(client *model.OAuthClient) (jwt.Keyfunc, error) {
	switch client.GetTokenEndpointAuthMethod() {
	case model.OAuthTokenEndpointAuthClientSecretJWT:
		return func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			secret, err := utils.DecryptOAuthClientSecret(client.ClientSecretEncrypted)
			if err != nil || secret == "" {
				return nil, errors.New("client_secret_jwt secret unavailable")
			}
			return []byte(secret), nil
		}, nil
	case model.OAuthTokenEndpointAuthPrivateKeyJWT:
		return func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodRS256 {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			kid, _ := token.Header["kid"].(string)
			return clientPublicKeyForAssertion(client, kid)
		}, nil
	default:
		return nil, errors.New("invalid_client")
	}
}

func clientPublicKeyForAssertion(client *model.OAuthClient, kid string) (interface{}, error) {
	jwksText := strings.TrimSpace(client.ClientJWKS)
	if jwksText == "" && strings.TrimSpace(client.ClientJWKSURI) != "" {
		resp, err := http.Get(strings.TrimSpace(client.ClientJWKSURI))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, errors.New("client jwks uri unavailable")
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}
		bytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		jwksText = string(bytes)
	}
	key, err := rsaPublicKeyFromJWKS(jwksText, kid)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func rsaPublicKeyFromJWKS(jwksText string, kid string) (interface{}, error) {
	var payload struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(jwksText), &payload); err != nil {
		return nil, err
	}
	for _, key := range payload.Keys {
		if kid != "" && key["kid"] != "" && key["kid"] != kid {
			continue
		}
		bytes, err := json.Marshal(key)
		if err != nil {
			continue
		}
		return rsaPublicKeyFromJWK(string(bytes))
	}
	return nil, errors.New("matching jwk not found")
}

func parseBasicAuthCredentials(authHeader string) (clientID, clientSecret string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(authHeader), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return "", "", false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
	if err != nil {
		return "", "", false
	}

	clientID, clientSecret, ok = strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", false
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || clientSecret == "" {
		return "", "", false
	}

	return clientID, clientSecret, true
}

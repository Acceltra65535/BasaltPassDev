package oauth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/model"
	"basaltpass-backend/internal/utils"

	"crypto/sha256"
	"github.com/glebarez/sqlite"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func setupOIDCE2EDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret-for-oidc-unit-tests")
	t.Setenv("OIDC_KEY_ENCRYPTION_SECRET", "test-oidc-key-encryption-secret")

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Tenant{}, &model.TenantUser{}, &model.Gender{}, &model.UserProfile{}, &model.App{}, &model.AppUser{}, &model.OAuthClient{}, &model.OAuthAuthorizationCode{}, &model.OAuthAccessToken{}, &model.OAuthRefreshToken{}, &model.OIDCSigningKey{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	common.SetDBForTest(db)
	return db
}

func TestOIDCAuthCodePKCEIssuesIDTokenAndJWKSVerifies(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "OIDC Tenant", Code: "oidc", Status: model.TenantStatusActive}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user := model.User{
		TenantID:      tenant.ID,
		Email:         "oidc@example.com",
		PasswordHash:  "x",
		Nickname:      "Oidc User",
		GivenName:     "Oidc",
		MiddleName:    "Middle",
		FamilyName:    "User",
		Locale:        "en-US",
		Zoneinfo:      "America/Los_Angeles",
		AvatarURL:     "https://rp.example/avatar.png",
		EmailVerified: true,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	app := model.App{TenantID: tenant.ID, Name: "OIDC App", Status: model.AppStatusActive}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	client := model.OAuthClient{AppID: app.ID, ClientID: "oidc-client", ClientSecret: "oidc-secret", RedirectURIs: "https://rp.example/callback", IsActive: true}
	client.HashClientSecret()
	client.SetScopeList([]string{"openid", "profile", "email", "offline_access"})
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}

	svc := NewOAuthServerService()
	verifier := "verifier-123"
	challenge := base64.RawURLEncoding.EncodeToString(sha256sum(verifier))
	authReq := &AuthorizeRequest{ClientID: client.ClientID, RedirectURI: "https://rp.example/callback", ResponseType: "code", Scope: "openid profile email offline_access", CodeChallenge: challenge, CodeChallengeMethod: "S256", Nonce: "nonce-xyz"}
	code, err := svc.GenerateAuthorizationCode(user.ID, authReq, &client)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	resp, err := svc.ExchangeCodeForToken(&TokenRequest{GrantType: "authorization_code", Code: code, RedirectURI: authReq.RedirectURI, CodeVerifier: verifier}, client.ClientID, "oidc-secret")
	if err != nil {
		t.Fatalf("exchange token: %v", err)
	}
	if strings.TrimSpace(resp.IDToken) == "" {
		t.Fatalf("expected id_token for openid scope")
	}
	var signingKey model.OIDCSigningKey
	if err := db.Where("status = ?", model.OIDCSigningKeyStatusActive).First(&signingKey).Error; err != nil {
		t.Fatalf("expected persisted OIDC signing key: %v", err)
	}
	if signingKey.KID == "" || signingKey.PublicJWK == "" {
		t.Fatalf("persisted OIDC signing key missing kid/public_jwk")
	}
	if !strings.HasPrefix(signingKey.PrivateKeyEncrypted, "enc:oidc:v1:") {
		t.Fatalf("expected encrypted private key, got prefix %q", signingKey.PrivateKeyEncrypted)
	}

	jwkPub := fetchRSAPublicKeyFromJWKS(t)
	token, err := jwt.Parse(resp.IDToken, func(token *jwt.Token) (interface{}, error) { return jwkPub, nil })
	if err != nil || !token.Valid {
		t.Fatalf("id_token verify failed by jwks key: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("unexpected claims type")
	}
	if claims["aud"] != client.ClientID || claims["nonce"] != "nonce-xyz" {
		t.Fatalf("unexpected aud/nonce claims: aud=%v nonce=%v", claims["aud"], claims["nonce"])
	}
	if claims["azp"] != client.ClientID {
		t.Fatalf("unexpected azp claim: %v", claims["azp"])
	}
	if claims["email"] != user.Email || claims["email_verified"] != user.EmailVerified {
		t.Fatalf("expected email claims in id_token: email=%v email_verified=%v", claims["email"], claims["email_verified"])
	}
	for _, claim := range []string{"name", "preferred_username", "given_name", "family_name", "middle_name", "picture"} {
		if _, exists := claims[claim]; exists {
			t.Fatalf("code-flow id_token should leave %s to userinfo, claims=%v", claim, claims)
		}
	}
	if claims["acr"] == "" || len(stringSliceClaim(claims["amr"])) == 0 {
		t.Fatalf("expected acr/amr claims: acr=%v amr=%v", claims["acr"], claims["amr"])
	}

	accessIntrospection, err := svc.IntrospectToken(resp.AccessToken, client.ClientID)
	if err != nil {
		t.Fatalf("introspect access token: %v", err)
	}
	if !accessIntrospection.Active || accessIntrospection.TokenType != "access_token" || accessIntrospection.Sub != strconv.FormatUint(uint64(user.ID), 10) || accessIntrospection.Aud != client.ClientID || accessIntrospection.Iss == "" || accessIntrospection.Nbf == 0 {
		t.Fatalf("unexpected access introspection: %+v", accessIntrospection)
	}
	refreshIntrospection, err := svc.IntrospectToken(resp.RefreshToken, client.ClientID)
	if err != nil {
		t.Fatalf("introspect refresh token: %v", err)
	}
	if !refreshIntrospection.Active || refreshIntrospection.TokenType != "refresh_token" || refreshIntrospection.Sub != strconv.FormatUint(uint64(user.ID), 10) {
		t.Fatalf("unexpected refresh introspection: %+v", refreshIntrospection)
	}
	refreshResp, err := svc.RefreshAccessToken(resp.RefreshToken, client.ClientID, "oidc-secret")
	if err != nil {
		t.Fatalf("refresh token grant: %v", err)
	}
	if refreshResp.IDToken == "" || refreshResp.RefreshToken == "" {
		t.Fatalf("expected refreshed id_token and refresh_token: %+v", refreshResp)
	}
	refreshedToken, err := jwt.Parse(refreshResp.IDToken, func(token *jwt.Token) (interface{}, error) { return fetchRSAPublicKeyFromJWKS(t), nil })
	if err != nil || !refreshedToken.Valid {
		t.Fatalf("refreshed id_token verify failed: %v", err)
	}
	refreshedClaims := refreshedToken.Claims.(jwt.MapClaims)
	if _, exists := refreshedClaims["nonce"]; exists {
		t.Fatalf("refresh id_token should not include nonce, got=%v", refreshedClaims["nonce"])
	}
	if refreshedClaims["auth_time"] != claims["auth_time"] {
		t.Fatalf("refresh id_token should preserve auth_time, got=%v want=%v", refreshedClaims["auth_time"], claims["auth_time"])
	}
	if refreshedClaims["email"] != user.Email || refreshedClaims["email_verified"] != user.EmailVerified {
		t.Fatalf("expected refreshed email claims: email=%v email_verified=%v", refreshedClaims["email"], refreshedClaims["email_verified"])
	}
}

func TestUserInfoIncludesProfileScopeAndEssentialNameClaims(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "UserInfo Tenant", Code: "userinfo", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	user := model.User{
		TenantID:      tenant.ID,
		Email:         "userinfo@example.com",
		PasswordHash:  "x",
		Nickname:      "User Info",
		GivenName:     "User",
		MiddleName:    "Middle",
		FamilyName:    "Info",
		Locale:        "en-US",
		Zoneinfo:      "America/Los_Angeles",
		AvatarURL:     "https://rp.example/avatar.png",
		EmailVerified: true,
	}
	_ = db.Create(&user).Error
	gender := model.Gender{Code: "male", Name: "Male", IsActive: true}
	_ = db.Create(&gender).Error
	birthdate := time.Date(1980, 1, 2, 0, 0, 0, 0, time.UTC)
	_ = db.Create(&model.UserProfile{UserID: user.ID, GenderID: &gender.ID, Website: "https://rp.example/userinfo", BirthDate: &birthdate}).Error
	app := model.App{TenantID: tenant.ID, Name: "UserInfo App", Status: model.AppStatusActive}
	_ = db.Create(&app).Error
	client := model.OAuthClient{AppID: app.ID, ClientID: "userinfo-client", ClientSecret: "secret", RedirectURIs: "https://rp.example/callback", IsActive: true}
	client.HashClientSecret()
	client.SetScopeList([]string{"openid", "profile", "email"})
	_ = db.Create(&client).Error
	access := model.OAuthAccessToken{
		Token:     "userinfo-token",
		ClientID:  client.ClientID,
		UserID:    user.ID,
		TenantID:  tenant.ID,
		AppID:     app.ID,
		Scopes:    "openid profile email",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = db.Create(&access).Error

	info, err := NewOAuthServerService().GetUserInfo(access.Token)
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if info.Name == "" || info.GivenName == "" || info.FamilyName == "" {
		t.Fatalf("expected basic profile claims: %+v", info)
	}
	if info.Profile == "" || info.Website == "" || info.Gender != "male" || info.Birthdate != "1980-01-02" {
		t.Fatalf("expected extended profile claims: %+v", info)
	}

	accessOpenIDOnly := model.OAuthAccessToken{
		Token:     "userinfo-token-openid",
		ClientID:  client.ClientID,
		UserID:    user.ID,
		TenantID:  tenant.ID,
		AppID:     app.ID,
		Scopes:    "openid",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = db.Create(&accessOpenIDOnly).Error
	info, err = NewOAuthServerService().GetUserInfo(accessOpenIDOnly.Token)
	if err != nil {
		t.Fatalf("userinfo openid: %v", err)
	}
	if info.Name != "" || info.GivenName != "" || info.Website != "" {
		t.Fatalf("openid-only userinfo should not expose full profile claims: %+v", info)
	}

	accessClaimsName := model.OAuthAccessToken{
		Token:     "userinfo-token-claims-name",
		ClientID:  client.ClientID,
		UserID:    user.ID,
		TenantID:  tenant.ID,
		AppID:     app.ID,
		Scopes:    "openid",
		Claims:    `{"userinfo":{"name":{"essential":true}}}`,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = db.Create(&accessClaimsName).Error
	info, err = NewOAuthServerService().GetUserInfo(accessClaimsName.Token)
	if err != nil {
		t.Fatalf("userinfo claims name: %v", err)
	}
	if info.Name == "" || info.GivenName != "" {
		t.Fatalf("claims parameter should expose only requested name without full profile: %+v", info)
	}
}

func TestAuthorizationCodeDoesNotReturnRefreshTokenWithoutOfflineAccess(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "No Offline Tenant", Code: "no-offline", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	user := model.User{TenantID: tenant.ID, Email: "no-offline@example.com", PasswordHash: "x"}
	_ = db.Create(&user).Error
	app := model.App{TenantID: tenant.ID, Name: "No Offline App", Status: model.AppStatusActive}
	_ = db.Create(&app).Error
	client := model.OAuthClient{AppID: app.ID, ClientID: "no-offline-client", ClientSecret: "secret", RedirectURIs: "https://rp.example/callback", IsActive: true}
	client.HashClientSecret()
	client.SetScopeList([]string{"openid", "profile", "email", "offline_access"})
	_ = db.Create(&client).Error

	svc := NewOAuthServerService()
	verifier := "verifier-no-offline"
	challenge := base64.RawURLEncoding.EncodeToString(sha256sum(verifier))
	authReq := &AuthorizeRequest{ClientID: client.ClientID, RedirectURI: "https://rp.example/callback", ResponseType: "code", Scope: "openid profile email", CodeChallenge: challenge, CodeChallengeMethod: "S256", Nonce: "nonce-no-offline"}
	code, err := svc.GenerateAuthorizationCode(user.ID, authReq, &client)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	resp, err := svc.ExchangeCodeForToken(&TokenRequest{GrantType: "authorization_code", Code: code, RedirectURI: authReq.RedirectURI, CodeVerifier: verifier}, client.ClientID, "secret")
	if err != nil {
		t.Fatalf("exchange token: %v", err)
	}
	if resp.RefreshToken != "" {
		t.Fatalf("refresh_token should require offline_access, got=%q", resp.RefreshToken)
	}
}

func TestPairwiseSubjectIsStablePerSectorAndDifferentFromPublicSub(t *testing.T) {
	db := setupOIDCE2EDB(t)
	user := model.User{Email: "pairwise@example.com", PasswordHash: "x"}
	_ = db.Create(&user).Error
	sector := "https://rp.example/sector.json"
	clientA := model.OAuthClient{ClientID: "pairwise-a", SubjectType: model.OAuthSubjectTypePairwise, SectorIdentifierURI: sector}
	clientB := model.OAuthClient{ClientID: "pairwise-b", SubjectType: model.OAuthSubjectTypePairwise, SectorIdentifierURI: sector}
	clientC := model.OAuthClient{ClientID: "pairwise-c", SubjectType: model.OAuthSubjectTypePairwise}

	subA := oidcSubjectForClient(clientA, user.ID)
	subB := oidcSubjectForClient(clientB, user.ID)
	subC := oidcSubjectForClient(clientC, user.ID)
	publicSub := strconv.FormatUint(uint64(user.ID), 10)

	if subA == publicSub || subC == publicSub {
		t.Fatalf("pairwise sub should not expose public user id")
	}
	if subA != subB {
		t.Fatalf("clients in same sector should share pairwise sub: %s != %s", subA, subB)
	}
	if subA == subC {
		t.Fatalf("different sectors should produce different pairwise subs")
	}
}

func TestTokenEndpointAcceptsClientSecretJWT(t *testing.T) {
	db := setupOIDCE2EDB(t)
	secret := "client-secret-jwt-secret"
	encrypted, err := utils.EncryptOAuthClientSecret(secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	client := model.OAuthClient{
		ClientID:                "client-secret-jwt-client",
		ClientSecret:            "unused-bcrypt-secret",
		ClientSecretEncrypted:   encrypted,
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthClientSecretJWT,
		IsActive:                true,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}

	assertion := signClientAssertion(t, client.ClientID, jwt.SigningMethodHS256, []byte(secret), "")
	app := fiber.New()
	app.Post("/token", func(c *fiber.Ctx) error {
		authenticated, err := authenticateOAuthClientForEndpoint(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"client_id": authenticated.ClientID})
	})
	req := httptest.NewRequest(fiber.MethodPost, "/token", strings.NewReader(url.Values{
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {assertion},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected client_secret_jwt authentication to pass, got %d", resp.StatusCode)
	}
}

func TestTokenEndpointAcceptsPrivateKeyJWT(t *testing.T) {
	db := setupOIDCE2EDB(t)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	jwk, err := generateRSAJWK(&privateKey.PublicKey, "client-key-1")
	if err != nil {
		t.Fatalf("generate jwk: %v", err)
	}
	jwksBytes, _ := json.Marshal(map[string]interface{}{"keys": []interface{}{jwk}})
	client := model.OAuthClient{
		ClientID:                "private-key-jwt-client",
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthPrivateKeyJWT,
		ClientJWKS:              string(jwksBytes),
		IsActive:                true,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}

	assertion := signClientAssertion(t, client.ClientID, jwt.SigningMethodRS256, privateKey, "client-key-1")
	app := fiber.New()
	app.Post("/token", func(c *fiber.Ctx) error {
		authenticated, err := authenticateOAuthClientForEndpoint(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"client_id": authenticated.ClientID})
	})
	req := httptest.NewRequest(fiber.MethodPost, "/token", strings.NewReader(url.Values{
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {assertion},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected private_key_jwt authentication to pass, got %d", resp.StatusCode)
	}
}

func TestOIDCSigningKeyPersistsAndIsReused(t *testing.T) {
	db := setupOIDCE2EDB(t)

	firstKey, firstKID, err := activeSigningKey()
	if err != nil {
		t.Fatalf("first signing key: %v", err)
	}
	secondKey, secondKID, err := activeSigningKey()
	if err != nil {
		t.Fatalf("second signing key: %v", err)
	}
	if firstKID == "" || firstKID != secondKID {
		t.Fatalf("expected signing kid to be reused, first=%q second=%q", firstKID, secondKID)
	}
	if firstKey.PublicKey.N.Cmp(secondKey.PublicKey.N) != 0 {
		t.Fatalf("expected persisted signing key material to be reused")
	}

	var count int64
	if err := db.Model(&model.OIDCSigningKey{}).
		Where("status = ? AND algorithm = ?", model.OIDCSigningKeyStatusActive, "RS256").
		Count(&count).Error; err != nil {
		t.Fatalf("count active signing keys: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one active signing key, got %d", count)
	}
}

func TestOIDCSigningKeyRotationRetiresOldKeyAndPublishesNewKey(t *testing.T) {
	db := setupOIDCE2EDB(t)

	_, firstKID, err := activeSigningKey()
	if err != nil {
		t.Fatalf("first signing key: %v", err)
	}
	rotated, err := RotateOIDCSigningKey()
	if err != nil {
		t.Fatalf("rotate signing key: %v", err)
	}
	if rotated.KID == "" || rotated.KID == firstKID || rotated.Status != model.OIDCSigningKeyStatusActive {
		t.Fatalf("unexpected rotated key: first=%q rotated=%+v", firstKID, rotated)
	}

	var oldKey model.OIDCSigningKey
	if err := db.Where("kid = ?", firstKID).First(&oldKey).Error; err != nil {
		t.Fatalf("load old signing key: %v", err)
	}
	if oldKey.Status != model.OIDCSigningKeyStatusRetired || oldKey.NotAfter == nil {
		t.Fatalf("old key should be retired with retention window: %+v", oldKey)
	}

	jwks, err := publicJWKS()
	if err != nil {
		t.Fatalf("load jwks: %v", err)
	}
	keys := jwks["keys"].([]interface{})
	if len(keys) < 2 {
		t.Fatalf("expected active and retained retired keys in jwks, got %d", len(keys))
	}
}

func TestAuthorizeRequestAllowsCodeFlowWithoutNonce(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "N", Code: "n", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	app := model.App{TenantID: tenant.ID, Name: "A", Status: model.AppStatusActive}
	_ = db.Create(&app).Error
	client := model.OAuthClient{AppID: app.ID, ClientID: "c1", RedirectURIs: "https://rp.example/cb", IsActive: true}
	client.SetScopeList([]string{"openid"})
	_ = db.Create(&client).Error

	svc := NewOAuthServerService()
	_, err := svc.ValidateAuthorizeRequest(&AuthorizeRequest{ClientID: "c1", RedirectURI: "https://rp.example/cb", ResponseType: "code", Scope: "openid"})
	if err != nil {
		t.Fatalf("code flow without nonce should be valid, got %v", err)
	}
}

func TestOIDCDiscoveryMetadataAdvertisesSupportedEndpoints(t *testing.T) {
	app := fiber.New()
	app.Get("/.well-known/openid-configuration", DiscoveryHandler)

	req := httptest.NewRequest(fiber.MethodGet, "/.well-known/openid-configuration", nil)
	req.Host = "issuer.example"
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("discovery request failed: %v", err)
	}
	defer resp.Body.Close()

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}

	issuer := oidcIssuer()
	expected := map[string]string{
		"issuer":                 issuer,
		"revocation_endpoint":    issuer + "/oauth/revoke",
		"introspection_endpoint": issuer + "/oauth/introspect",
		"end_session_endpoint":   issuer + "/end_session",
		"check_session_iframe":   issuer + "/check_session_iframe",
	}
	for key, want := range expected {
		if got := payload[key]; got != want {
			t.Fatalf("unexpected %s: got=%v want=%s", key, got, want)
		}
	}
	if got, ok := payload["claims_parameter_supported"].(bool); !ok || got {
		t.Fatalf("claims_parameter_supported should be explicit false, got=%v", payload["claims_parameter_supported"])
	}
	if got, ok := payload["request_parameter_supported"].(bool); !ok || !got {
		t.Fatalf("request_parameter_supported should be explicit true, got=%v", payload["request_parameter_supported"])
	}
	if _, exists := payload["userinfo_signing_alg_values_supported"]; exists {
		t.Fatalf("userinfo signing algorithms should not be advertised for plain JSON userinfo")
	}
	signingAlgs := stringSliceClaim(payload["token_endpoint_auth_signing_alg_values_supported"])
	if !containsString(signingAlgs, "HS256") || !containsString(signingAlgs, "RS256") {
		t.Fatalf("token endpoint auth signing algorithms should include HS256/RS256, got=%v", signingAlgs)
	}
	authMethods := stringSliceClaim(payload["token_endpoint_auth_methods_supported"])
	if !containsString(authMethods, "none") || !containsString(authMethods, "client_secret_jwt") || !containsString(authMethods, "private_key_jwt") {
		t.Fatalf("token endpoint auth methods should include public and JWT clients, got=%v", authMethods)
	}
	subjectTypes := stringSliceClaim(payload["subject_types_supported"])
	if !containsString(subjectTypes, "public") || !containsString(subjectTypes, "pairwise") {
		t.Fatalf("subject types should include public and pairwise, got=%v", subjectTypes)
	}
	pkceMethods := stringSliceClaim(payload["code_challenge_methods_supported"])
	if !containsString(pkceMethods, "S256") || containsString(pkceMethods, "plain") {
		t.Fatalf("PKCE methods should advertise only S256, got=%v", pkceMethods)
	}
}

func TestEndSessionRedirectsToRegisteredPostLogoutURI(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "Logout Tenant", Code: "logout", Status: model.TenantStatusActive}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user := model.User{TenantID: tenant.ID, Email: "logout@example.com", PasswordHash: "x", Nickname: "Logout User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	appModel := model.App{TenantID: tenant.ID, Name: "Logout App", Status: model.AppStatusActive}
	if err := db.Create(&appModel).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	client := model.OAuthClient{
		AppID:                  appModel.ID,
		ClientID:               "logout-client",
		ClientSecret:           "logout-secret",
		RedirectURIs:           "https://rp.example/callback",
		PostLogoutRedirectURIs: "https://rp.example/logout",
		IsActive:               true,
	}
	client.HashClientSecret()
	client.SetScopeList([]string{"openid"})
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}

	idToken, err := NewOAuthServerService().buildIDToken(client.ClientID, user.ID, "nonce-logout", "openid", time.Now())
	if err != nil {
		t.Fatalf("build id token: %v", err)
	}

	fiberApp := fiber.New()
	fiberApp.Get("/end_session", EndSessionHandler)
	q := url.Values{}
	q.Set("id_token_hint", idToken)
	q.Set("post_logout_redirect_uri", "https://rp.example/logout")
	q.Set("state", "logout-state")
	req := httptest.NewRequest(fiber.MethodGet, "/end_session?"+q.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "session-token"})
	resp, err := fiberApp.Test(req, -1)
	if err != nil {
		t.Fatalf("end session request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://rp.example/logout?state=logout-state" {
		t.Fatalf("unexpected logout redirect: %s", got)
	}
	if cookies := resp.Header.Values("Set-Cookie"); len(cookies) == 0 || !strings.Contains(strings.Join(cookies, "\n"), "access_token=;") {
		t.Fatalf("expected hosted auth cookies to be cleared, got %v", cookies)
	}
}

func TestEndSessionRejectsUnregisteredPostLogoutURI(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "Logout Tenant 2", Code: "logout2", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	user := model.User{TenantID: tenant.ID, Email: "logout2@example.com", PasswordHash: "x"}
	_ = db.Create(&user).Error
	appModel := model.App{TenantID: tenant.ID, Name: "Logout App 2", Status: model.AppStatusActive}
	_ = db.Create(&appModel).Error
	client := model.OAuthClient{
		AppID:                  appModel.ID,
		ClientID:               "logout-client-2",
		ClientSecret:           "logout-secret",
		RedirectURIs:           "https://rp.example/callback",
		PostLogoutRedirectURIs: "https://rp.example/logout",
		IsActive:               true,
	}
	client.HashClientSecret()
	_ = db.Create(&client).Error

	idToken, err := NewOAuthServerService().buildIDToken(client.ClientID, user.ID, "", "openid", time.Now())
	if err != nil {
		t.Fatalf("build id token: %v", err)
	}

	fiberApp := fiber.New()
	fiberApp.Get("/end_session", EndSessionHandler)
	q := url.Values{}
	q.Set("id_token_hint", idToken)
	q.Set("post_logout_redirect_uri", "https://evil.example/logout")
	resp, err := fiberApp.Test(httptest.NewRequest(fiber.MethodGet, "/end_session?"+q.Encode(), nil), -1)
	if err != nil {
		t.Fatalf("end session request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected bad request for unregistered post logout URI, got %d", resp.StatusCode)
	}
}

func TestEndSessionAcceptsIDTokenSignedByRetainedRetiredKey(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "Logout Rotation Tenant", Code: "logout-rotation", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	user := model.User{TenantID: tenant.ID, Email: "logout-rotation@example.com", PasswordHash: "x"}
	_ = db.Create(&user).Error
	appModel := model.App{TenantID: tenant.ID, Name: "Logout Rotation App", Status: model.AppStatusActive}
	_ = db.Create(&appModel).Error
	client := model.OAuthClient{
		AppID:                  appModel.ID,
		ClientID:               "logout-rotation-client",
		ClientSecret:           "logout-rotation-secret",
		RedirectURIs:           "https://rp.example/callback",
		PostLogoutRedirectURIs: "https://rp.example/logout",
		IsActive:               true,
	}
	client.HashClientSecret()
	_ = db.Create(&client).Error

	idToken, err := NewOAuthServerService().buildIDToken(client.ClientID, user.ID, "", "openid", time.Now())
	if err != nil {
		t.Fatalf("build id token: %v", err)
	}
	if _, err := RotateOIDCSigningKey(); err != nil {
		t.Fatalf("rotate signing key: %v", err)
	}

	fiberApp := fiber.New()
	fiberApp.Get("/end_session", EndSessionHandler)
	q := url.Values{}
	q.Set("id_token_hint", idToken)
	q.Set("post_logout_redirect_uri", "https://rp.example/logout")
	resp, err := fiberApp.Test(httptest.NewRequest(fiber.MethodGet, "/end_session?"+q.Encode(), nil), -1)
	if err != nil {
		t.Fatalf("end session request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("expected redirect with retained retired key, got %d", resp.StatusCode)
	}
}

func TestPublicClientUsesNoneAuthWithS256PKCE(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "Public Tenant", Code: "public", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	user := model.User{TenantID: tenant.ID, Email: "public@example.com", PasswordHash: "x"}
	_ = db.Create(&user).Error
	appModel := model.App{TenantID: tenant.ID, Name: "Public App", Status: model.AppStatusActive}
	_ = db.Create(&appModel).Error
	client := model.OAuthClient{
		AppID:                   appModel.ID,
		ClientID:                "public-client",
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthNone,
		RedirectURIs:            "https://rp.example/callback",
		IsActive:                true,
	}
	client.SetScopeList([]string{"openid"})
	_ = db.Create(&client).Error

	svc := NewOAuthServerService()
	verifier := "public-verifier-123"
	challenge := base64.RawURLEncoding.EncodeToString(sha256sum(verifier))
	authReq := &AuthorizeRequest{
		ClientID:            client.ClientID,
		RedirectURI:         "https://rp.example/callback",
		ResponseType:        "code",
		Scope:               "openid",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		Nonce:               "public-nonce",
	}
	if _, err := svc.ValidateAuthorizeRequest(authReq); err != nil {
		t.Fatalf("public authorize should allow S256 PKCE: %v", err)
	}
	code, err := svc.GenerateAuthorizationCode(user.ID, authReq, &client)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	resp, err := svc.ExchangeCodeForToken(&TokenRequest{GrantType: "authorization_code", Code: code, RedirectURI: authReq.RedirectURI, CodeVerifier: verifier}, client.ClientID, "")
	if err != nil {
		t.Fatalf("public client token exchange: %v", err)
	}
	if resp.IDToken == "" {
		t.Fatalf("expected id token for public OIDC client")
	}
}

func TestPublicClientRejectsMissingS256PKCE(t *testing.T) {
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "Public Tenant 2", Code: "public2", Status: model.TenantStatusActive}
	_ = db.Create(&tenant).Error
	appModel := model.App{TenantID: tenant.ID, Name: "Public App 2", Status: model.AppStatusActive}
	_ = db.Create(&appModel).Error
	client := model.OAuthClient{
		AppID:                   appModel.ID,
		ClientID:                "public-client-2",
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthNone,
		RedirectURIs:            "https://rp.example/callback",
		IsActive:                true,
	}
	client.SetScopeList([]string{"openid"})
	_ = db.Create(&client).Error

	_, err := NewOAuthServerService().ValidateAuthorizeRequest(&AuthorizeRequest{
		ClientID:     client.ClientID,
		RedirectURI:  "https://rp.example/callback",
		ResponseType: "code",
		Scope:        "openid",
		Nonce:        "public-nonce",
	})
	if err == nil || err.Error() != "invalid_request" {
		t.Fatalf("expected public client without S256 PKCE to be rejected, got %v", err)
	}
}

func fetchRSAPublicKeyFromJWKS(t *testing.T) *rsa.PublicKey {
	t.Helper()
	app := fiber.New()
	app.Get("/oauth/jwks", JWKSHandler)
	req := httptest.NewRequest(fiber.MethodGet, "/oauth/jwks", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("jwks request failed: %v", err)
	}
	defer resp.Body.Close()
	var payload struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode jwks: %v", err)
	}
	if len(payload.Keys) == 0 {
		t.Fatalf("no keys in jwks")
	}
	nBytes, _ := base64.RawURLEncoding.DecodeString(payload.Keys[0]["n"])
	eBytes, _ := base64.RawURLEncoding.DecodeString(payload.Keys[0]["e"])
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(new(big.Int).SetBytes(eBytes).Int64())}
}

func sha256sum(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func stringSliceClaim(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func signClientAssertion(t *testing.T, clientID string, method jwt.SigningMethod, key interface{}, kid string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(method, jwt.MapClaims{
		"iss": clientID,
		"sub": clientID,
		"aud": tokenEndpointAudience(),
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"jti": clientID + "-assertion",
	})
	if kid != "" {
		token.Header["kid"] = kid
	}
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign client assertion: %v", err)
	}
	return signed
}

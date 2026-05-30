package oauth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"basaltpass-backend/internal/model"

	"github.com/gofiber/fiber/v2"
)

func seedOneTapPublicClient(t *testing.T) (model.User, model.OAuthClient) {
	t.Helper()
	db := setupOIDCE2EDB(t)
	tenant := model.Tenant{Name: "OneTap Tenant", Code: "onetap", Status: model.TenantStatusActive}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user := model.User{TenantID: tenant.ID, Email: "onetap@example.com", PasswordHash: "x", EmailVerified: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	app := model.App{TenantID: tenant.ID, Name: "OneTap App", Status: model.AppStatusActive}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	client := model.OAuthClient{
		AppID:                   app.ID,
		ClientID:                "onetap-public-client",
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthNone,
		RedirectURIs:            "https://rp.example/callback",
		IsActive:                true,
		CreatedBy:               user.ID,
	}
	client.SetScopeList([]string{"openid", "profile", "email"})
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.AppUser{
		AppID:             app.ID,
		UserID:            user.ID,
		FirstAuthorizedAt: time.Now(),
		LastAuthorizedAt:  time.Now(),
		Scopes:            "openid profile email",
		Status:            model.AppUserStatusActive,
	}).Error; err != nil {
		t.Fatalf("create app user: %v", err)
	}
	oauthServerService = NewOAuthServerService()
	return user, client
}

func oneTapTestApp(userID uint) *fiber.App {
	app := fiber.New()
	app.Post("/oauth/one-tap/login", func(c *fiber.Ctx) error {
		c.Locals("userID", userID)
		return OneTapLoginHandler(c)
	})
	app.Get("/oauth/silent-auth", func(c *fiber.Ctx) error {
		c.Locals("userID", userID)
		return SilentAuthHandler(c)
	})
	return app
}

func TestOneTapLoginPublicClientIssuesCodeExchangeableWithPKCE(t *testing.T) {
	user, client := seedOneTapPublicClient(t)
	app := oneTapTestApp(user.ID)
	verifier := "onetap-verifier-123"
	challenge := base64.RawURLEncoding.EncodeToString(sha256sum(verifier))

	body, _ := json.Marshal(map[string]string{
		"client_id":             client.ClientID,
		"redirect_uri":          "https://rp.example/callback",
		"response_type":         "code",
		"scope":                 "openid profile email",
		"state":                 "onetap-state",
		"nonce":                 "onetap-nonce",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	})
	req := httptest.NewRequest(fiber.MethodPost, "/oauth/one-tap/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("one tap request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var payload OneTapAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode one tap response: %v", err)
	}
	if !payload.Success || payload.Code == "" || payload.State != "onetap-state" {
		t.Fatalf("unexpected one tap response: %+v", payload)
	}

	tokenResp, err := NewOAuthServerService().ExchangeCodeForToken(&TokenRequest{
		GrantType:    "authorization_code",
		Code:         payload.Code,
		RedirectURI:  "https://rp.example/callback",
		CodeVerifier: verifier,
	}, client.ClientID, "")
	if err != nil {
		t.Fatalf("exchange one tap code: %v", err)
	}
	if tokenResp.AccessToken == "" || tokenResp.IDToken == "" {
		t.Fatalf("expected access token and id token, got %+v", tokenResp)
	}
}

func TestOneTapRejectsDirectIDTokenResponseType(t *testing.T) {
	user, client := seedOneTapPublicClient(t)
	app := oneTapTestApp(user.ID)

	body, _ := json.Marshal(map[string]string{
		"client_id":     client.ClientID,
		"redirect_uri":  "https://rp.example/callback",
		"response_type": "id_token",
	})
	req := httptest.NewRequest(fiber.MethodPost, "/oauth/one-tap/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("one tap request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var payload OneTapAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode one tap response: %v", err)
	}
	if payload.Error != "unsupported_response_type" {
		t.Fatalf("expected unsupported_response_type, got %+v", payload)
	}
}

func TestSilentAuthPublicClientRendersCodeWithPKCE(t *testing.T) {
	user, client := seedOneTapPublicClient(t)
	app := oneTapTestApp(user.ID)
	verifier := "silent-verifier-123"
	challenge := base64.RawURLEncoding.EncodeToString(sha256sum(verifier))

	q := url.Values{}
	q.Set("client_id", client.ClientID)
	q.Set("redirect_uri", "https://rp.example/callback")
	q.Set("prompt", "none")
	q.Set("scope", "openid profile email")
	q.Set("state", "silent-state")
	q.Set("nonce", "silent-nonce")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/oauth/silent-auth?"+q.Encode(), nil), -1)
	if err != nil {
		t.Fatalf("silent auth request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	htmlBytes, _ := io.ReadAll(resp.Body)
	html := string(htmlBytes)
	if !strings.Contains(html, "success: true") || !strings.Contains(html, "silent-state") || !strings.Contains(html, "code") {
		t.Fatalf("expected silent auth page with code and state, got %s", html)
	}
}

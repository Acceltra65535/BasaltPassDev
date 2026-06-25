package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"basaltpass-backend/internal/common"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func createJWTForTest(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(common.MustJWTSecret())
	if err != nil {
		t.Fatalf("failed to sign jwt: %v", err)
	}
	return signed
}

func doRequestAndDecode(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, map[string]interface{}) {
	t.Helper()

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	_ = resp.Body.Close()

	if len(body) == 0 {
		return resp, nil
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return resp, nil
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		t.Fatalf("decode json failed: %v body=%s", err, string(body))
	}

	return resp, payload
}

func TestJWTMiddleware_MissingToken(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, body := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if body["code"] != "auth_missing_token" {
		t.Fatalf("expected code auth_missing_token, got %#v", body["code"])
	}
}

func TestJWTMiddleware_InvalidToken(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, body := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if body["code"] != "auth_invalid_token" {
		t.Fatalf("expected code auth_invalid_token, got %#v", body["code"])
	}
}

func TestJWTMiddleware_ValidTokenSetsLocals(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"userID":   c.Locals("userID"),
			"tenantID": c.Locals("tenantID"),
			"scope":    c.Locals("scope"),
		})
	})

	token := createJWTForTest(t, jwt.MapClaims{
		"sub": float64(42),
		"tid": float64(7),
		"scp": "tenant",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, body := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["userID"].(float64) != 42 {
		t.Fatalf("expected userID=42, got %#v", body["userID"])
	}
	if body["tenantID"].(float64) != 7 {
		t.Fatalf("expected tenantID=7, got %#v", body["tenantID"])
	}
	if body["scope"] != "tenant" {
		t.Fatalf("expected scope=tenant, got %#v", body["scope"])
	}
}

func TestJWTMiddleware_RejectsRefreshToken(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	token := createJWTForTest(t, jwt.MapClaims{
		"sub": float64(42),
		"tid": float64(7),
		"scp": "tenant",
		"typ": "refresh",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, body := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if body["code"] != "auth_invalid_token" {
		t.Fatalf("expected code auth_invalid_token, got %#v", body["code"])
	}
}

func TestJWTMiddleware_AcceptsAccessTokenCookieForSafeRequest(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	token := createJWTForTest(t, jwt.MapClaims{
		"sub": float64(42),
		"typ": "access",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	resp, _ := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJWTMiddleware_RequiresCSRFForCookieUnsafeRequest(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	token := createJWTForTest(t, jwt.MapClaims{
		"sub": float64(42),
		"typ": "access",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	resp, body := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	if body["code"] != "csrf_invalid" {
		t.Fatalf("expected code csrf_invalid, got %#v", body["code"])
	}
}

func TestJWTMiddleware_AcceptsCookieUnsafeRequestWithCSRF(t *testing.T) {
	app := fiber.New()
	app.Use(JWTMiddleware())
	app.Post("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	token := createJWTForTest(t, jwt.MapClaims{
		"sub": float64(42),
		"typ": "access",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "csrf-test"})
	req.Header.Set("X-CSRF-Token", "csrf-test")
	resp, _ := doRequestAndDecode(t, app, req)

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

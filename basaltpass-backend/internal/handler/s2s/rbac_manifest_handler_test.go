package s2s

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestManifestSubmissionRejectsQueryCredentialSource(t *testing.T) {
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error {
		c.Locals("s2s_auth_source", "query")
		c.Locals("s2s_tenant_id", uint(1))
		c.Locals("s2s_app_id", uint(2))
		c.Locals("s2s_client_id", "client")
		return SubmitRBACManifestHandler(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected query credentials to be rejected with 401, got %d", resp.StatusCode)
	}
}

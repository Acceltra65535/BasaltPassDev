package oauth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

const oauthClientIDLocalKey = "oauth_client_id"

// OAuthClientAuthMiddleware authenticates OAuth clients for endpoints that must
// not be publicly callable (e.g. introspect / revoke).
func OAuthClientAuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		client, err := authenticateOAuthClientForEndpoint(c)
		if err != nil {
			return oauthInvalidClient(c)
		}

		c.Locals(oauthClientIDLocalKey, client.ClientID)
		return c.Next()
	}
}

func getAuthenticatedOAuthClientID(c *fiber.Ctx) string {
	clientID, _ := c.Locals(oauthClientIDLocalKey).(string)
	return strings.TrimSpace(clientID)
}

func oauthInvalidClient(c *fiber.Ctx) error {
	c.Set("WWW-Authenticate", `Basic realm="oauth2", error="invalid_client"`)
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error":             "invalid_client",
		"error_description": "Client authentication failed",
	})
}

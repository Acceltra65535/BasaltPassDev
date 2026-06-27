package authn

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/middleware/transport"
	serviceauth "basaltpass-backend/internal/service/auth"
	"basaltpass-backend/internal/utils"
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

var errInvalidJWTClaims = errors.New("invalid jwt claims")

func ExtractBearerToken(c *fiber.Ctx) string {
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
}

func ExtractAccessToken(c *fiber.Ctx) (string, bool) {
	if token := ExtractBearerToken(c); token != "" {
		return token, false
	}

	scope := normalizeCookieScope(c.Get("X-Auth-Scope"))
	cookieNames := []string{"access_token"}
	if scope != "user" {
		cookieNames = []string{"access_token_" + scope, "access_token"}
	}
	for _, name := range cookieNames {
		if token := strings.TrimSpace(c.Cookies(name)); token != "" {
			return token, true
		}
	}
	return "", false
}

func normalizeCookieScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "tenant":
		return "tenant"
	case "admin":
		return "admin"
	default:
		return "user"
	}
}

func requiresCSRF(method string) bool {
	switch method {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return false
	default:
		return true
	}
}

func validateCSRFFromCookie(c *fiber.Ctx) bool {
	scope := normalizeCookieScope(c.Get("X-Auth-Scope"))
	cookieNames := []string{"csrf_token"}
	if scope != "user" {
		cookieNames = []string{"csrf_token_" + scope, "csrf_token"}
	}
	var csrfCookie string
	for _, name := range cookieNames {
		csrfCookie = strings.TrimSpace(c.Cookies(name))
		if csrfCookie != "" {
			break
		}
	}
	csrfHeader := strings.TrimSpace(c.Get("X-CSRF-Token"))
	if csrfHeader == "" {
		csrfHeader = strings.TrimSpace(c.Get("X-XSRF-TOKEN"))
	}
	return csrfCookie != "" &&
		csrfHeader != "" &&
		subtle.ConstantTimeCompare([]byte(csrfCookie), []byte(csrfHeader)) == 1
}

func ParseJWTToken(tokenStr string, ignoreClaimsValidation bool) (*jwt.Token, jwt.MapClaims, error) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, nil, jwt.ErrTokenMalformed
	}

	parserOpts := []jwt.ParserOption{}
	if ignoreClaimsValidation {
		parserOpts = append(parserOpts, jwt.WithoutClaimsValidation())
	}

	parser := jwt.NewParser(parserOpts...)
	token, err := parser.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		secret, secretErr := common.JWTSecret()
		if secretErr != nil {
			return nil, secretErr
		}
		return secret, nil
	})
	if err != nil {
		return nil, nil, err
	}
	if token == nil {
		return nil, nil, jwt.ErrTokenUnverifiable
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return token, nil, errInvalidJWTClaims
	}

	return token, claims, nil
}

func JWTMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenStr, fromCookie := ExtractAccessToken(c)
		if tokenStr == "" {
			return transport.APIErrorResponse(c, fiber.StatusUnauthorized, "auth_missing_token", "[Basalt Auth] missing token")
		}
		if fromCookie && requiresCSRF(c.Method()) && !validateCSRFFromCookie(c) {
			return transport.APIErrorResponse(c, fiber.StatusForbidden, "csrf_invalid", "[Basalt Auth] invalid csrf token")
		}

		token, claims, err := ParseJWTToken(tokenStr, false)
		if err != nil || !token.Valid {
			return transport.APIErrorResponse(c, fiber.StatusUnauthorized, "auth_invalid_token", "[Basalt Auth] invalid token")
		}
		if claims == nil {
			return transport.APIErrorResponse(c, fiber.StatusUnauthorized, "auth_invalid_claims", "[Basalt Auth] invalid claims")
		}
		if err := serviceauth.ValidateAccessTokenType(claims); err != nil {
			return transport.APIErrorResponse(c, fiber.StatusUnauthorized, "auth_invalid_token", "[Basalt Auth] invalid token")
		}

		if userID, exists := claims["sub"]; exists {
			if parsed, parseErr := utils.UintFromAny(userID); parseErr == nil {
				c.Locals("userID", parsed)
			}
		}

		if tenantID, exists := claims["tid"]; exists {
			if parsed, parseErr := utils.UintFromAny(tenantID); parseErr == nil {
				c.Locals("tenantID", parsed)
			}
		}

		if scope, exists := claims["scp"]; exists {
			if scopeStr, ok := scope.(string); ok {
				c.Locals("scope", scopeStr)
			}
		}

		c.Locals("user", token)
		return c.Next()
	}
}

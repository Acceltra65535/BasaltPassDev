package routes

import (
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRegisterUserRoutesTeamInvitationPaths(t *testing.T) {
	app := fiber.New()
	v1 := app.Group("/api/v1")

	RegisterUserRoutes(v1)

	expected := map[string]bool{
		"POST /api/v1/teams/:id/invitations":           false,
		"GET /api/v1/teams/:id/invitations":            false,
		"DELETE /api/v1/teams/:id/invitations/:inv_id": false,
		"PUT /api/v1/invitations/:id/accept":           false,
		"PUT /api/v1/invitations/:id/reject":           false,
	}

	unexpected := map[string]bool{
		"POST /api/v1/teams:id/invitations":           false,
		"GET /api/v1/teams:id/invitations":            false,
		"DELETE /api/v1/teams:id/invitations/:inv_id": false,
		"PUT /api/v1/invitations:id/accept":           false,
		"PUT /api/v1/invitations:id/reject":           false,
	}

	for _, route := range app.GetRoutes(true) {
		key := route.Method + " " + route.Path
		if _, ok := expected[key]; ok {
			expected[key] = true
		}
		if _, ok := unexpected[key]; ok {
			unexpected[key] = true
		}
	}

	for route, found := range expected {
		if !found {
			t.Fatalf("expected route %q to be registered", route)
		}
	}
	for route, found := range unexpected {
		if found {
			t.Fatalf("malformed route %q should not be registered", route)
		}
	}
}

package ops

import (
	"basaltpass-backend/internal/common"
	"basaltpass-backend/internal/middleware/transport"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

var (
	dbReadyMu       sync.Mutex
	dbReadyCached   bool
	dbReadyChecked  time.Time
	dbReadyInterval = time.Second
)

func isDBReadyCached() bool {
	dbReadyMu.Lock()
	defer dbReadyMu.Unlock()

	if time.Since(dbReadyChecked) < dbReadyInterval {
		return dbReadyCached
	}

	dbReadyChecked = time.Now()
	dbReadyCached = common.DBReady()
	return dbReadyCached
}

// DBGuardMiddleware blocks requests when the database is unavailable.
func DBGuardMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		if strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/api/v1/health") {
			return c.Next()
		}

		if !isDBReadyCached() {
			return transport.APIErrorResponse(c, fiber.StatusServiceUnavailable, "database_unavailable", "database temporarily unavailable")
		}

		return c.Next()
	}
}

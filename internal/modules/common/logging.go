package common

import (
	"encoding/json"
	"fmt"

	"go-boilerplate/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// LoggingMiddleware assigns a UUID trace ID to every request, stores it in
// c.Locals("trace_id"), and logs the URL, query params, route params, and body.
// Mirrors logging.ts — drop this in place of (or alongside) Fiber's built-in logger.
func LoggingMiddleware(c *fiber.Ctx) error {
	traceId := uuid.New().String()
	c.Locals("trace_id", traceId)

	body := map[string]any{}
	if len(c.Body()) > 0 {
		json.Unmarshal(c.Body(), &body) //nolint:errcheck — best-effort
	}

	utils.LogInfo(
		"LoggingMiddleware", "LoggingMiddleware", traceId,
		fmt.Sprintf(`[%s] URL: "%s" Queries: %v Params: %v Body: %v`,
			c.Method(),
			c.OriginalURL(),
			c.Queries(),
			c.AllParams(),
			body,
		),
	)

	return c.Next()
}

// GetTraceID retrieves the request-scoped trace ID set by LoggingMiddleware.
// Returns an empty string if the middleware was not applied.
func GetTraceID(c *fiber.Ctx) string {
	id, _ := c.Locals("trace_id").(string)
	return id
}

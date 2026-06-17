package common

import (
	"strings"

	"go-boilerplate/utils"

	"github.com/gofiber/fiber/v2"
)

const RoleSuperAdmin = "SUPERADMIN"

// PermissionMiddleware enforces Casbin RBAC on every route it wraps.
//
// Flow (mirrors permission.ts):
//  1. Static-token bypass.
//  2. Skip entirely when SKIP_PERMISSION=true.
//  3. SUPERADMIN role bypasses policy check.
//  4. Delegates to utils.CheckPermissions(roles, path, method).
//
// Must be placed after GlobalAuthMiddleware so c.Locals("auth_user") is set.
func PermissionMiddleware(c *fiber.Ctx) error {
	// Static-token bypass
	if st := utils.Cfg.StaticToken; st != "" {
		parts := strings.SplitN(c.Get("Authorization"), " ", 2)
		if len(parts) == 2 && parts[1] == st {
			return c.Next()
		}
	}

	// Global skip (e.g. local dev)
	if utils.Cfg.SkipPermission {
		return c.Next()
	}

	user, ok := c.Locals("auth_user").(AuthUser)
	if !ok || len(user.ObaRole) == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(authErrorResponse("Unauthorized"))
	}

	// Superadmin bypass
	for _, r := range user.ObaRole {
		if r == RoleSuperAdmin {
			return c.Next()
		}
	}

	method := string(c.Method())
	path := c.Path()

	allowed, err := utils.CheckPermissions(c.Context(), user.ObaRole, path, method)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(authErrorResponse("Unauthorized"))
	}
	if !allowed {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"type":       "APP",
				"code":       "FORBIDDEN",
				"statusCode": fiber.StatusForbidden,
				"message":    "Anda tidak punya akses untuk melakukan aksi ini",
			},
		})
	}

	return c.Next()
}

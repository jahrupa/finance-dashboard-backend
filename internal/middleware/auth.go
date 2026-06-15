package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// DevBypass — used only when DEV_JWT_BYPASS=true
func DevBypass() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("userID", "dev-user")
		c.Locals("userEmail", "dev@test.com")
		c.Locals("userRole", "admin")
		c.Locals("userName", "Developer")
		c.Locals("userDepartment", "")
		c.Locals("pageAccess", []string{})
		c.Locals("crudAccess", map[string]interface{}{})
		return c.Next()
	}
}

// JWTAuth validates the Bearer token AND extracts all claims into c.Locals.
// This is the root fix for Issue 2 (Guest User on refresh) and Issue 1
// (department-based filtering) — previously the middleware validated the token
// but never set c.Locals, so userID was always empty string.
func JWTAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "Missing Authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid Authorization format. Use: Bearer <token>")
		}

		tokenStr := parts[1]

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			// Enforce HS256 — reject tokens signed with a different algorithm
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "Unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid or expired token")
		}

		// ── Extract claims and store in c.Locals ─────────────────────
		// These are now available to every handler via c.Locals("userID") etc.
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid token claims")
		}

		// "sub" holds the MongoDB ObjectID hex string (set in AuthHandler.generateJWT)
		userID, _ := claims["sub"].(string)
		userEmail, _ := claims["email"].(string)
		userRole, _ := claims["role"].(string)
		userName, _ := claims["name"].(string)
		userDepartment, _ := claims["department"].(string)

		// page_access stored as []interface{} in JWT
		var pageAccess []string
		if raw, ok := claims["page_access"].([]interface{}); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					pageAccess = append(pageAccess, s)
				}
			}
		}

		// crud_access stored as map[string]interface{} in JWT
		var crudAccess map[string]interface{}
		if raw, ok := claims["crud_access"].(map[string]interface{}); ok {
			crudAccess = raw
		}

		c.Locals("userID", userID)
		c.Locals("userEmail", userEmail)
		c.Locals("userRole", userRole)
		c.Locals("userName", userName)
		c.Locals("userDepartment", userDepartment)
		c.Locals("pageAccess", pageAccess)
		c.Locals("crudAccess", crudAccess)

		return c.Next()
	}
}

// ── Helper: read a local as string (safe, no panic) ──────────
func GetLocalString(c *fiber.Ctx, key string) string {
	v, _ := c.Locals(key).(string)
	return v
}

// ── Helper: check if role is admin/super_admin ────────────────
func IsAdmin(c *fiber.Ctx) bool {
	role := GetLocalString(c, "userRole")
	return role == "admin" || role == "super_admin"
}

// ── CRUDCheck middleware factory ─────────────────────────────
// Usage in main.go:
//
//	inv.Post("/", middleware.CRUDCheck("Invoice Submission", "create"), invoiceH.Create)
//
// For admin/super_admin roles → always allowed (bypass CRUD check).
// For all others → check crud_access[page][operation] == true.
func CRUDCheck(page, operation string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Admins bypass all CRUD checks
		if IsAdmin(c) {
			return c.Next()
		}

		// Verify the user has page access first
		pageAccess, _ := c.Locals("pageAccess").([]string)
		hasPage := false
		for _, p := range pageAccess {
			if p == page {
				hasPage = true
				break
			}
		}
		if !hasPage {
			return fiber.NewError(fiber.StatusForbidden,
				"Access denied: you do not have access to page '"+page+"'")
		}

		// Check specific CRUD operation
		crudAccess, _ := c.Locals("crudAccess").(map[string]interface{})
		if crudAccess == nil {
			return fiber.NewError(fiber.StatusForbidden, "Access denied: no permissions configured")
		}

		pagePerms, ok := crudAccess[page].(map[string]interface{})
		if !ok {
			return fiber.NewError(fiber.StatusForbidden,
				"Access denied: no permissions for page '"+page+"'")
		}

		allowed, _ := pagePerms[operation].(bool)
		if !allowed {
			return fiber.NewError(fiber.StatusForbidden,
				"Access denied: '"+operation+"' permission not granted for '"+page+"'")
		}

		return c.Next()
	}
}

// ErrorHandler — global Fiber error handler
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   message,
	})
}

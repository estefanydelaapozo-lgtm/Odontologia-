package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"sigo-dental/backend/auth"
	"sigo-dental/backend/repository"
)

func RequireAuth(secret []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		if header == "" {
			return c.Status(401).JSON(fiber.Map{"message": "Inicia sesión para continuar"})
		}
		token, err := jwt.ParseWithClaims(header, &auth.Claims{}, func(t *jwt.Token) (interface{}, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("algoritmo inválido")
			}
			return secret, nil
		})
		if err != nil || !token.Valid {
			return c.Status(401).JSON(fiber.Map{"message": "Sesión inválida o vencida"})
		}
		claims, ok := token.Claims.(*auth.Claims)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"message": "Sesión inválida"})
		}
		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		c.Locals("name", claims.Name)
		c.Locals("mfa", claims.MFA)
		// Sliding 15-minute access token: each authenticated API request renews the idle window.
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(15 * time.Minute))
		claims.IssuedAt = jwt.NewNumericDate(time.Now())
		if renewed, signErr := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret); signErr == nil {
			c.Set("X-Session-Token", renewed)
		}
		return c.Next()
	}
}

func AuditRequests(repo repository.DBRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		userID, _ := c.Locals("user_id").(int64)
		if userID != 0 {
			action := "actualizar"
			if c.Method() == fiber.MethodGet {
				action = "consultar"
			}
			entityID := c.Path()
			if resourceID := c.Params("id"); resourceID != "" {
				entityID += ":" + resourceID
			}
			_, _ = repo.Exec(`INSERT INTO audit_log(user_id,action,entity,entity_id,ip_address) VALUES($1,$2,'api',$3,$4)`, userID, action+":"+c.Method(), entityID, c.IP())
		}
		return err
	}
}

func RequireRoles(roles ...string) fiber.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if _, ok := allowed[role]; !ok {
			return c.Status(403).JSON(fiber.Map{"message": "No tienes permiso para esta operación"})
		}
		return c.Next()
	}
}

func RequireMFA() fiber.Handler {
	return func(c *fiber.Ctx) error {
		verified, _ := c.Locals("mfa").(bool)
		if !verified {
			return c.Status(428).JSON(fiber.Map{"message": "Activa la autenticación multifactor antes de acceder a expedientes o finanzas"})
		}
		return c.Next()
	}
}

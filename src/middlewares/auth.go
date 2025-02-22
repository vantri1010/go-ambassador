package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt"
	"strconv"
	"strings"
	"time"
)

const SecretKey = "secret"

type ClaimWithScope struct {
	jwt.StandardClaims
	Scope string
}

func IsAuthenticated(c *fiber.Ctx) error {
	cookie := c.Cookies("jwt")

	token, err := jwt.ParseWithClaims(cookie, &ClaimWithScope{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(SecretKey), nil
	})

	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthenticated"})
	}

	payload := token.Claims.(*ClaimWithScope)
	isAmbassador := strings.Contains(c.Path(), "/api/ambassador")

	// Catch the event when user change api link
	if isAmbassador != (payload.Scope == "ambassador") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized"})
	}

	return c.Next()
}

func GenerateJWT(id uint, scope string) (string, error) {
	payload := ClaimWithScope{}
	payload.Scope = scope
	payload.Subject = strconv.Itoa(int(id))
	payload.ExpiresAt = time.Now().Add(time.Hour * 24).Unix()

	return jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString([]byte(SecretKey))
}

func GetUserId(c *fiber.Ctx) (uint, error) {
	cookie := c.Cookies("jwt")
	token, err := jwt.ParseWithClaims(cookie, &ClaimWithScope{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(SecretKey), nil
	})
	if err != nil || !token.Valid {
		return 0, err
	}

	claims := token.Claims.(*ClaimWithScope)
	id, _ := strconv.Atoi(claims.Subject)

	return uint(id), nil
}

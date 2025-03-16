package controllers

import (
	"ambassador/src/database"
	"ambassador/src/middlewares"
	"ambassador/src/models"
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"log"
	"strings"
	"time"
)

func Register(c *fiber.Ctx) error {
	var data map[string]string

	// Parse the request body
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate password and password confirmation
	if data["password"] != data["password_confirm"] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Passwords do not match",
		})
	}

	// Create the user
	user := models.User{
		FirstName:    data["first_name"],
		LastName:     data["last_name"],
		Email:        data["email"],
		IsAmbassador: strings.Contains(c.Path(), "/api/ambassador"),
	}

	// Set the user's password
	user.SetPassword(data["password"])

	// Save the user to the database
	if err := database.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to create user",
		})
	}

	// Clear the ambassadors cache
	ctx := context.Background()
	cacheKey := "ambassadors_with_revenue"
	if err := database.Cache.Del(ctx, cacheKey).Err(); err != nil {
		log.Printf("Failed to clear cache: %v", err)
		// Do not return an error; continue to serve the response
	}

	// Return the created user
	return c.Status(fiber.StatusCreated).JSON(user)
}

// Login handles user login and JWT token generation.
func Login(c *fiber.Ctx) error {
	var data map[string]string

	// Parse the request body
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Fetch the user from the database
	var user models.User
	if err := database.DB.Where("email = ?", data["email"]).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid Credentials",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch user",
		})
	}

	// Compare the provided password with the stored hash
	if err := user.ComparePassword(data["password"]); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid Credentials",
		})
	}

	// Determine the scope based on the request path
	isAmbassador := strings.Contains(c.Path(), "/api/ambassador")
	scope := "admin"

	if isAmbassador {
		scope = "ambassador"
	} else if user.IsAmbassador { // Prevent ambassador users from logging in as admin
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized non-admin user",
		})
	}

	// Generate a JWT token
	token, err := middlewares.GenerateJWT(user.Id, scope)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to generate token",
		})
	}

	// Set the JWT token in a cookie
	cookie := fiber.Cookie{
		Name:     "jwt",
		Value:    token,
		Expires:  time.Now().Add(time.Hour * 24),
		HTTPOnly: true,
	}

	c.Cookie(&cookie)

	return c.JSON(fiber.Map{
		"message": "Success",
	})
}

func User(c *fiber.Ctx) error {
	// Get the user ID from the middleware
	id, err := middlewares.GetUserId(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	// Fetch the user from the database
	var user models.User
	if err := database.DB.Where("id = ?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "User not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch user",
		})
	}

	// Check if the request is from the ambassador endpoint
	if strings.Contains(c.Path(), "/api/ambassador") {
		// Fetch orders and calculate revenue
		revenue, err := calculateRevenueForUser(user.Id, database.DB)
		if err != nil {
			log.Printf("Failed to calculate ambassador revenue: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to calculate revenue",
			})
		}

		// Create an ambassador and set the revenue
		ambassador := models.Ambassador(user)
		ambassador.Revenue = &revenue
		return c.JSON(ambassador)
	}

	// Return the user
	return c.JSON(user)
}

// calculateRevenueForUser calculates the revenue for a specific user
func calculateRevenueForUser(userID uint, db *gorm.DB) (float64, error) {
	var orders []models.Order

	// Fetch all completed orders and order items for the user
	if err := db.Preload("OrderItems").Where("user_id = ? AND complete = ?", userID, true).Find(&orders).Error; err != nil {
		log.Printf("Failed to fetch orders for user %d: %v", userID, err)
		return 0, err
	}

	// Calculate revenue
	var revenue float64 = 0.0
	for _, order := range orders {
		for _, orderItem := range order.OrderItems {
			revenue += orderItem.AmbassadorRevenue
		}
	}

	return revenue, nil
}

// Logout by remove cookie. remove cookie by set it expired one hour
func Logout(c *fiber.Ctx) error {
	// Clear the JWT cookie by setting it to expire in the past
	cookie := fiber.Cookie{
		Name:     "jwt",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
	}

	c.Cookie(&cookie)

	return c.JSON(fiber.Map{
		"message": "success",
	})
}

func UpdateInfo(c *fiber.Ctx) error {
	var data map[string]string

	// Parse the request body
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Get the user ID from the middleware
	id, err := middlewares.GetUserId(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	// Validate required fields
	if data["first_name"] == "" || data["last_name"] == "" || data["email"] == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "First name, last name, and email are required",
		})
	}

	// Update user information in the database
	result := database.DB.Model(&models.User{}).Where("id = ?", id).Updates(models.User{
		FirstName: data["first_name"],
		LastName:  data["last_name"],
		Email:     data["email"],
	})

	// Check for database errors
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update user information",
		})
	}

	// Check if the user was found and updated
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "User not found",
		})
	}

	// Clear the ambassadors cache
	ctx := context.Background()
	cacheKey := "ambassadors_with_revenue"
	if err := database.Cache.Del(ctx, cacheKey).Err(); err != nil {
		log.Printf("Failed to clear cache: %v", err)
		// Do not return an error; continue to serve the response
	}

	// Return a success message
	return c.JSON(fiber.Map{
		"message": "User information updated successfully",
	})
}

func UpdatePassword(c *fiber.Ctx) error {
	var data map[string]string

	// Parse the request body
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate password and password confirmation
	if data["password"] != data["password_confirm"] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Passwords do not match",
		})
	}

	// Get the user ID from the middleware
	id, err := middlewares.GetUserId(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	user := models.User{}
	user.Id = id
	user.SetPassword(data["password"])

	// Update the password in the database
	result := database.DB.Model(user).Where("id = ?", id).Update("password", user.Password)
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update password",
		})
	}

	// Check if the password was updated
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "User not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Password updated successfully",
	})
}

package controllers

import (
	"ambassador/src/database"
	"ambassador/src/middlewares"
	"ambassador/src/models"
	"github.com/gofiber/fiber/v2"
	"strings"
	"time"
)

func Register(c *fiber.Ctx) error {
	var data map[string]string

	if err := c.BodyParser(&data); err != nil {
		return err
	}

	if data["password"] != data["password_confirm"] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "passwords do not match",
		})
	}

	user := models.User{
		FirstName:    data["first_name"],
		LastName:     data["last_name"],
		Email:        data["email"],
		IsAmbassador: strings.Contains(c.Path(), "/api/ambassador"),
	}
	user.SetPassword(data["password"])

	database.DB.Create(&user)

	return c.JSON(user)
}

func Login(c *fiber.Ctx) error {
	var data map[string]string

	if err := c.BodyParser(&data); err != nil {
		return err
	}

	user := models.User{}
	database.DB.Where("email = ?", data["email"]).First(&user)
	if user.Id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid Credentials",
		})
	}

	if err := user.ComparePassword(data["password"]); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid Credentials",
		})
	}

	isAmbassador := strings.Contains(c.Path(), "/api/ambassador")
	scope := "admin"

	if isAmbassador {
		scope = "ambassador"
	} else if user.IsAmbassador { // When ambassador user try to login to admin link
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "unauthorized non-admin user"})
	}

	token, err := middlewares.GenerateJWT(user.Id, scope)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Invalid Credentials",
		})
	}

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

// User Send cookie get user
func User(c *fiber.Ctx) error {
	id, _ := middlewares.GetUserId(c)

	var user models.User
	database.DB.Model(&models.User{}).Where("id = ?", id).First(&user)

	if strings.Contains(c.Path(), "/api/ambassador") {
		ambassador := models.Ambassador(user)
		ambassador.CalculateRevenue(database.DB)
		return c.JSON(ambassador)
	}

	return c.JSON(user)
}

// Logout by remove cookie. remove cookie by set it expired one hour
func Logout(c *fiber.Ctx) error {
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
	//user.Id = id
	//user.SetPassword(data["password"])

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

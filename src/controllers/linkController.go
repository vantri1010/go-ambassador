package controllers

import (
	"ambassador/src/database"
	"ambassador/src/middlewares"
	"ambassador/src/models"
	"errors"
	"github.com/go-faker/faker/v4"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"strconv"
)

func Link(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	// Struct to hold query results
	type LinkResult struct {
		ID         uint    `json:"id"`
		Code       string  `json:"code"`
		OrderCount int64   `json:"order_count"`
		Total      float64 `json:"total"`
	}

	var results []LinkResult

	// GORM Query (Using COUNT DISTINCT for orders)
	err = database.DB.
		Table("links AS l").
		Select("l.id, l.code, COUNT(DISTINCT o.id) AS order_count, COALESCE(SUM(oi.price * oi.quantity), 0) AS total").
		Joins("LEFT JOIN orders o ON l.code = o.code AND o.complete = ?", true).
		Joins("LEFT JOIN order_items oi ON o.id = oi.order_id").
		Where("l.user_id = ?", id).
		Group("l.id, l.code").
		Scan(&results).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch links",
		})
	}

	return c.JSON(results)
}

// CreateLinkRequest defines the request body for creating a link.
type CreateLinkRequest struct {
	Products []int `json:"products" validate:"required,min=1"`
}

// CreateLink creates a new link for the user.
func CreateLink(c *fiber.Ctx) error {
	var request CreateLinkRequest

	// Parse and validate the request body
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate the products array
	if len(request.Products) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "At least one product is required",
		})
	}

	// Get the user ID from the middleware
	id, err := middlewares.GetUserId(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	// Create the link
	link := models.Link{
		UserId: id,
		Code:   faker.Username(),
	}

	// Fetch and associate products with the link
	for _, productId := range request.Products {
		product := models.Product{}
		if err := database.DB.First(&product, productId).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid product ID",
			})
		}
		link.Products = append(link.Products, product)
	}

	// Save the link to the database
	if err := database.DB.Create(&link).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to create link",
		})
	}

	return c.JSON(link)
}

// Stats fetches statistics for all links of the user.
func Stats(c *fiber.Ctx) error {
	id, err := middlewares.GetUserId(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	// Fetch all links for the user
	var links []models.Link
	if err := database.DB.Where("user_id = ?", id).Find(&links).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch links",
		})
	}

	// Collect all link codes
	linkCodes := make([]string, len(links))
	for i, link := range links {
		linkCodes[i] = link.Code
	}

	// Fetch all orders for the link codes in one query
	var orders []models.Order
	if err := database.DB.Preload("OrderItems").Where("code IN (?) AND complete = ?", linkCodes, true).Find(&orders).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch orders",
		})
	}

	// Group orders by link code
	ordersByCode := make(map[string][]models.Order)
	for _, order := range orders {
		ordersByCode[order.Code] = append(ordersByCode[order.Code], order)
	}

	// Prepare the result
	result := make([]fiber.Map, 0, len(links))
	for _, link := range links {
		orders := ordersByCode[link.Code]
		revenue := 0.0
		for _, order := range orders {
			revenue += order.GetTotal()
		}

		result = append(result, fiber.Map{
			"code":    link.Code,
			"count":   len(orders),
			"revenue": revenue,
		})
	}

	return c.JSON(result)
}

// GetLink fetches a link by its code.
func GetLink(c *fiber.Ctx) error {
	code := c.Params("code")

	// Fetch the link with the given code and preload User and Products
	var link models.Link
	result := database.DB.Preload("User").Preload("Products").Where("code = ?", code).First(&link)

	// Check if the link was found
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Link not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch link",
		})
	}

	return c.JSON(link)
}

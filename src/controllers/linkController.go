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
	id, _ := strconv.Atoi(c.Params("id"))
	var links []models.Link
	database.DB.Where("user_id = ?", id).Find(&links)

	for i, link := range links {
		var orders []models.Order
		database.DB.Where("code = ? and complete = true", link.Code).Find(&orders)
		links[i].Orders = orders
	}

	return c.JSON(links)
}

type CreateLinkRequest struct {
	Products []int
}

func CreateLink(c *fiber.Ctx) error {
	var request CreateLinkRequest

	if err := c.BodyParser(&request); err != nil {
		return err
	}

	id, _ := middlewares.GetUserId(c)
	link := models.Link{
		UserId: id,
		Code:   faker.Username(),
	}

	for _, productId := range request.Products {
		product := models.Product{}
		product.Id = uint(productId)
		link.Products = append(link.Products, product)
	}

	database.DB.Create(&link)

	return c.JSON(link)
}

func Stats(c *fiber.Ctx) error {
	id, _ := middlewares.GetUserId(c)

	// Fetch all links for the user
	var links []models.Link
	if err := database.DB.Where("user_id = ?", id).Find(&links).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch links",
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
			"error": "Failed to fetch orders",
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

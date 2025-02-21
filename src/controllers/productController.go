package controllers

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"github.com/gofiber/fiber/v2"
	"strconv"
)

func Products(c *fiber.Ctx) error {
	var products []models.Product
	database.DB.Find(&products)

	return c.JSON(&products)
}

func CreateProduct(c *fiber.Ctx) error {
	var product models.Product
	if err := c.BodyParser(&product); err != nil {
		return err
	}

	database.DB.Create(&product)

	return c.JSON(&product)
}

func GetProduct(c *fiber.Ctx) error {
	var product models.Product
	id, _ := strconv.Atoi(c.Params("id"))
	product.Id = uint(id)

	database.DB.Find(&product)

	return c.JSON(product)
}

func DeleteProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))

	database.DB.Delete(&models.Product{}, id)

	return c.JSON(fiber.Map{
		"message": "Product deleted",
	})
}

func UpdateProduct(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var product models.Product
	if err := c.BodyParser(&product); err != nil {
		return err
	}

	database.DB.Model(&product).Where("id = ?", id).Updates(product)

	return c.JSON(product)
}

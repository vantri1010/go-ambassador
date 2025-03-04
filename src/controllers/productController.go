package controllers

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"context"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"sort"
	"strconv"
	"strings"
	"time"
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
	go database.ClearCache("products_frontend", "products_backend")

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
	go database.ClearCache("products_frontend", "products_backend")

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
	go database.ClearCache("products_frontend", "products_backend")

	return c.JSON(product)
}

func ProductsFrontend(c *fiber.Ctx) error {
	var products []models.Product
	var ctx = context.Background()

	result, err := database.Cache.Get(ctx, "products_frontend").Result()
	if err != nil {
		database.DB.Find(&products)

		bytes, err := json.Marshal(products)
		if err != nil {
			panic(err)
		}

		if errKey := database.Cache.Set(ctx, "products_frontend", bytes, 30*time.Minute).Err(); errKey != nil {
			panic(errKey)
		}
	} else {
		json.Unmarshal([]byte(result), &products)
	}

	return c.JSON(products)
}

func ProductsBackend(c *fiber.Ctx) error {
	var products []models.Product
	var ctx = context.Background()

	result, err := database.Cache.Get(ctx, "products_backend").Result()
	if err != nil {
		database.DB.Find(&products)

		bytes, err := json.Marshal(products)
		if err != nil {
			panic(err)
		}

		database.Cache.Set(ctx, "products_backend", bytes, 30*time.Minute)
	} else {
		json.Unmarshal([]byte(result), &products)
	}

	var searchedProducts []models.Product

	if s := c.Query("s"); s != "" {
		lower := strings.ToLower(s)
		for _, product := range products {
			if strings.Contains(strings.ToLower(product.Title), lower) || strings.Contains(strings.ToLower(product.Description), lower) {
				searchedProducts = append(searchedProducts, product)
			}
		}
	} else {
		searchedProducts = products
	}

	if sortParam := c.Query("sort"); sortParam != "" {
		sortLower := strings.ToLower(sortParam)
		if sortLower == "asc" {
			sort.Slice(searchedProducts, func(i, j int) bool {
				return searchedProducts[i].Price < searchedProducts[j].Price
			})
		} else if sortLower == "desc" {
			sort.Slice(searchedProducts, func(i, j int) bool {
				return searchedProducts[i].Price > searchedProducts[j].Price
			})
		}
	}

	var total = len(searchedProducts)
	page, _ := strconv.Atoi(c.Query("page", "1"))
	itemsPerPage := 9
	if page <= 0 || page > (total/itemsPerPage+1) {
		page = 1
	}

	var data []models.Product

	// Check if items remain less than items per page that should return a.k.a the last page
	if total <= page*itemsPerPage && total >= (page-1)*itemsPerPage {
		data = searchedProducts[(page-1)*itemsPerPage : total]
	} else if total >= page*itemsPerPage {
		data = searchedProducts[(page-1)*itemsPerPage : page*itemsPerPage]
	} else {
		data = []models.Product{}
	}

	return c.JSON(fiber.Map{
		"data": data,

		"meta": fiber.Map{
			"total":     total,
			"page":      page,
			"last_page": total/itemsPerPage + 1,
		},
	})
}

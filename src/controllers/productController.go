package controllers

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"context"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Products returns all products from the database.
func Products(c *fiber.Ctx) error {
	var products []models.Product
	if err := database.DB.Find(&products).Error; err != nil {
		log.Printf("Failed to fetch products: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch products",
		})
	}
	return c.JSON(products)
}

// CreateProduct creates a new product in the database.
func CreateProduct(c *fiber.Ctx) error {
	var product models.Product

	// Parse the request body
	if err := c.BodyParser(&product); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate required fields
	if product.Title == "" || product.Description == "" || product.Image == "" || product.Price <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Title, description, image, and price are required, and price must be greater than 0",
		})
	}

	// Create the product in the database
	if err := database.DB.Create(&product).Error; err != nil {
		log.Printf("Failed to create product: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to create product",
		})
	}

	// Clear the cache asynchronously
	go database.ClearCache("products_frontend", "products_backend")

	// Return the created product
	return c.Status(fiber.StatusCreated).JSON(product)
}

// GetProduct retrieves a single product by ID.
func GetProduct(c *fiber.Ctx) error {
	// Parse the product ID from the URL parameter
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	// Fetch the product from the database
	var product models.Product
	if err := database.DB.First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		log.Printf("Failed to fetch product: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch product",
		})
	}

	// Return the product
	return c.JSON(product)
}

// DeleteProduct deletes a product by ID.
func DeleteProduct(c *fiber.Ctx) error {
	// Parse the product ID from the URL parameter
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	// Check if the product exists before deleting
	var product models.Product
	if err := database.DB.First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		log.Printf("Failed to fetch product: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch product",
		})
	}

	// Delete the product from the database
	if err := database.DB.Delete(&product).Error; err != nil {
		log.Printf("Failed to delete product: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to delete product",
		})
	}

	// Clear the cache asynchronously
	go database.ClearCache("products_frontend", "products_backend")

	// Return a success message
	return c.JSON(fiber.Map{
		"message": "Product deleted successfully",
	})
}

// UpdateProduct updates an existing product by ID.
func UpdateProduct(c *fiber.Ctx) error {
	// Parse the product ID from the URL parameter
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	// Parse the request body
	var product models.Product
	if err := c.BodyParser(&product); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate required fields
	if product.Title == "" || product.Description == "" || product.Image == "" || product.Price <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Title, description, image, and price are required, and price must be greater than 0",
		})
	}

	// Check if the product exists before updating
	var existingProduct models.Product
	if err := database.DB.First(&existingProduct, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		log.Printf("Failed to fetch product: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch product",
		})
	}

	// Update the product in the database
	result := database.DB.Model(&models.Product{}).Where("id = ?", id).Updates(product)
	if result.Error != nil {
		log.Printf("Failed to update product: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update product",
		})
	}

	// Check if the product was updated
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Product not found",
		})
	}

	// Clear the cache asynchronously
	go database.ClearCache("products_frontend", "products_backend")

	// Return the updated product
	return c.JSON(product)
}

/*
The ProductsFrontend function is designed to fetch a list of products from a
cache (e.g., Redis). If the data is not found in the cache, it fetches the
products from the database, stores them in the cache, and returns the products.
*/
func ProductsFrontend(c *fiber.Ctx) error {
	var products []models.Product
	ctx := context.Background()
	cacheKey := "products_frontend"

	// Try to fetch products from the cache
	result, err := database.Cache.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit: Deserialize the cached data
		if err := json.Unmarshal([]byte(result), &products); err != nil {
			log.Printf("Failed to unmarshal cached products: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to process cached data",
			})
		}
		return c.JSON(products)
	}

	// Cache miss: Fetch products from the database
	if err := database.DB.Find(&products).Error; err != nil {
		log.Printf("Failed to fetch products from database: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch products",
		})
	}

	// Serialize products to JSON
	bytes, err := json.Marshal(products)
	if err != nil {
		log.Printf("Failed to marshal products: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to process products",
		})
	}

	// Store products in the cache
	if err := database.Cache.Set(ctx, cacheKey, bytes, 30*time.Minute).Err(); err != nil {
		log.Printf("Failed to update cache: %v", err)
		// Do not return an error; continue to serve the response
	}

	// Return the products
	return c.JSON(products)
}

// ProductsBackend fetches products with search, sort, and pagination.
func ProductsBackend(c *fiber.Ctx) error {
	var products []models.Product
	var ctx = context.Background()

	// Try to fetch products from the cache
	result, err := database.Cache.Get(ctx, "products_backend").Result()
	if err != nil {
		// Cache miss: Fetch products from the database
		if err := database.DB.Find(&products).Error; err != nil {
			log.Printf("Failed to fetch products from database: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to fetch products",
			})
		}

		// Serialize products to JSON
		bytes, err := json.Marshal(products)
		if err != nil {
			log.Printf("Failed to marshal products: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to process products",
			})
		}

		// Store products in the cache
		if err := database.Cache.Set(ctx, "products_backend", bytes, 30*time.Minute).Err(); err != nil {
			log.Printf("Failed to update cache: %v", err)
			// Do not return an error; continue to serve the response
		}
	} else {
		// Cache hit: Deserialize the cached data
		if err := json.Unmarshal([]byte(result), &products); err != nil {
			log.Printf("Failed to unmarshal cached products: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to process cached data",
			})
		}
	}

	// Filter products based on search query
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

	// Sort products based on sort query
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

	// Paginate products
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

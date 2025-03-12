package controllers

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"context"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

func Ambassadors(c *fiber.Ctx) error {
	var users []models.User
	ctx := context.Background()
	cacheKey := "ambassadors_with_revenue"

	// Try to fetch ambassadors from the cache
	result, err := database.Cache.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit: Deserialize the cached data
		if err := json.Unmarshal([]byte(result), &users); err != nil {
			log.Printf("Failed to unmarshal cached ambassadors: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to process cached data",
			})
		}
		return c.JSON(users)
	}

	// Cache miss: Fetch and calculate revenue
	if err := fetchAndCalculateRevenue(&users); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch ambassadors",
		})
	}

	// Serialize ambassadors to JSON
	bytes, err := json.Marshal(users)
	if err != nil {
		log.Printf("Failed to marshal ambassadors: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to process ambassadors",
		})
	}

	// Store ambassadors in the cache
	if err := database.Cache.Set(ctx, cacheKey, bytes, 30*time.Minute).Err(); err != nil {
		log.Printf("Failed to update cache: %v", err)
		// Do not return an error; continue to serve the response
	}

	// Return the ambassadors
	return c.JSON(users)
}

func fetchAndCalculateRevenue(users *[]models.User) error {
	// Fetch all ambassadors
	if err := database.DB.Where("is_ambassador = ?", true).Find(users).Error; err != nil {
		return err
	}

	// Fetch all completed orders and order items for ambassadors
	var orders []models.Order
	if err := database.DB.Preload("OrderItems").Where("user_id IN (SELECT id FROM users WHERE is_ambassador = ?)", true).Find(&orders).Error; err != nil {
		return err
	}

	// Group orders by ambassador ID
	orderMap := make(map[uint][]models.Order)
	for _, order := range orders {
		orderMap[order.UserId] = append(orderMap[order.UserId], order)
	}

	// Calculate revenue for each ambassador
	for i, user := range *users {
		ambassador := models.Ambassador(user)
		if orders, ok := orderMap[ambassador.Id]; ok {
			revenue := 0.0
			for _, order := range orders {
				for _, orderItem := range order.OrderItems {
					revenue += orderItem.AmbassadorRevenue
				}
			}
			ambassador.Revenue = &revenue
		} else {
			revenue := 0.0
			ambassador.Revenue = &revenue
		}
		(*users)[i] = models.User(ambassador)
	}

	return nil
}

func Rankings(c *fiber.Ctx) error {
	rankings, err := database.Cache.ZRevRangeByScoreWithScores(context.Background(), "rankings", &redis.ZRangeBy{
		Min: "-inf",
		Max: "+inf",
	}).Result()

	if err != nil {
		return err
	}

	result := make(map[string]float64)

	for _, ranking := range rankings {
		result[ranking.Member.(string)] = ranking.Score
	}

	return c.JSON(result)
}

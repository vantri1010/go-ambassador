package main

import (
	"ambassador/src/database"
	"ambassador/src/routes"
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Initialize database, Redis, and cache
	database.Connect()
	database.AutoMigrate()
	database.SetupRedis()
	database.SetupCacheChannel()

	// Create a new Fiber app
	app := fiber.New()

	// Configure CORS middleware
	app.Use(cors.New(cors.Config{
		AllowCredentials: true,
		AllowOrigins:     "http://localhost:3000, http://localhost:4000, http://localhost:5000",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
	}))

	// Set up routes
	routes.Setup(app)

	// Start the server in a goroutine
	go func() {
		if err := app.Listen(":8000"); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Create a channel to listen for termination signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM) // Listen for SIGINT and SIGTERM
	<-quit                                             // Block until a signal is received

	// Log the shutdown event
	log.Println("Shutting down server...")

	// Create a context with a timeout for graceful shutdown
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown the Fiber server
	if err := app.Shutdown(); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}

	// Close the database connection
	sqlDB, err := database.DB.DB()
	if err != nil {
		log.Printf("Failed to get underlying SQL DB object: %v", err)
	} else {
		if err := sqlDB.Close(); err != nil {
			log.Printf("Failed to close database connection: %v", err)
		}
	}

	// Close the Redis connection
	database.CloseRedis()

	// Log successful shutdown
	log.Println("Server gracefully stopped")
}

package main

import (
	"ambassador/src/database"
	"ambassador/src/routes"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"log"

	"github.com/gofiber/fiber/v2"
)

func main() {
	database.Connect()
	database.AutoMigrate()
	database.SetupRedis()
	database.SetupCacheChannel()

	app := fiber.New()
	app.Use(cors.New(cors.Config{
		AllowCredentials: true,
		AllowOrigins:     "http://localhost:3000, http://localhost:4000, http://localhost:5000",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
	}))
	routes.Setup(app)

	log.Fatal(app.Listen(":8000"))
}

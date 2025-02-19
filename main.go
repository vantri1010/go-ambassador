package main

import (
	"ambassador/src/database"
	"log"

	"github.com/gofiber/fiber/v2"
)

func main() {
	database.Connect()
	database.AutoMigrate()

	app := fiber.New()

	// Define a route for the GET method on the root path '/'
	app.Get("/", func(c *fiber.Ctx) error {
		// Send a string response to the client
		return c.SendString("Hello, World 👋! change here")
	})

	// Start the server on port 3000
	log.Fatal(app.Listen(":8000"))
}

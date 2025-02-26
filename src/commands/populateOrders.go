package main

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"github.com/go-faker/faker/v4"
	"math/rand"
)

func main() {
	database.Connect()

	for i := 0; i < 30; i++ {
		var orderItems []models.OrderItem

		for j := 0; j < rand.Intn(5); j++ {
			price := float64(rand.Intn(90) + 10)
			qty := uint(rand.Intn(5))

			orderItems = append(orderItems, models.OrderItem{
				ProductTitle:      faker.Word(),
				Price:             price,
				Quantity:          qty,
				AdminRevenue:      0.9 * price * float64(qty),
				AmbassadorRevenue: 0.1 * price * float64(qty),
			})
		}

		// Generate a random number between 31 and 64
		min := 31
		max := 64
		randomNum := uint(min + rand.Intn(max-min+1))

		database.DB.Create(&models.Order{
			UserId:          randomNum,
			Code:            faker.Username(),
			AmbassadorEmail: faker.Email(),
			FirstName:       faker.FirstName(),
			LastName:        faker.LastName(),
			Email:           faker.Email(),
			Complete:        true,
			OrderItems:      orderItems,
		})
	}
}

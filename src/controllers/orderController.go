package controllers

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"context"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"gorm.io/gorm"
	"log"
	"net/smtp"
	"os"
)

func Orders(c *fiber.Ctx) error {
	var orders []models.Order

	database.DB.Preload("OrderItems").Find(&orders)

	for i, order := range orders {
		orders[i].Name = order.FullName()
		orders[i].Total = order.GetTotal()
	}

	return c.JSON(orders)
}

type CreateOrderRequest struct {
	FirstName string           `json:"first_name"`
	LastName  string           `json:"last_name"`
	Email     string           `json:"email"`
	Address   string           `json:"address"`
	Country   string           `json:"country"`
	City      string           `json:"city"`
	Zip       string           `json:"zip"`
	Code      string           `json:"code"`
	Products  []map[string]int `json:"products"`
}

func CreateOrder(c *fiber.Ctx) error {
	var request CreateOrderRequest

	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Input validation
	if len(request.FirstName) == 0 || len(request.LastName) == 0 || len(request.Email) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "First name, last name, and email are required",
		})
	}

	// Validate product quantities
	for _, product := range request.Products {
		if product["quantity"] < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Quantity for each product must be at least 1",
			})
		}
	}

	link := models.Link{}

	database.DB.Preload("User").Where("code = ?", request.Code).First(&link)

	if link.Id == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid link!",
		})
	}

	order := models.Order{
		Code:            link.Code,
		UserId:          link.UserId,
		AmbassadorEmail: link.User.Email,
		FirstName:       request.FirstName,
		LastName:        request.LastName,
		Email:           request.Email,
		Address:         request.Address,
		Country:         request.Country,
		City:            request.City,
		Zip:             request.Zip,
	}

	tx := database.DB.Begin()

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	var lineItems []*stripe.CheckoutSessionLineItemParams

	for _, requestProduct := range request.Products {
		product := models.Product{}
		product.Id = uint(requestProduct["product_id"])
		database.DB.First(&product)

		total := product.Price * float64(requestProduct["quantity"])

		item := models.OrderItem{
			OrderId:           order.Id,
			ProductTitle:      product.Title,
			Price:             product.Price,
			Quantity:          uint(requestProduct["quantity"]),
			AmbassadorRevenue: 0.1 * total,
			AdminRevenue:      0.9 * total,
		}

		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": err.Error(),
			})
		}

		lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("usd"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(product.Title),
					Description: stripe.String(product.Description),
					Images:      []*string{stripe.String(product.Image)},
				},
				UnitAmount: stripe.Int64(100 * int64(product.Price)), // Price in cents
			},
			Quantity: stripe.Int64(int64(requestProduct["quantity"])),
		})
	}

	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	params := stripe.CheckoutSessionParams{
		SuccessURL:         stripe.String("http://localhost:5000/success?source={CHECKOUT_SESSION_ID}"),
		CancelURL:          stripe.String("http://localhost:5000/error"),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems:          lineItems,
		Mode:               stripe.String("payment"),
	}

	source, err := session.New(&params)

	if err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	order.TransactionId = source.ID

	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	tx.Commit()

	return c.JSON(source)
}

func CompleteOrder(c *fiber.Ctx) error {
	var data map[string]string

	// Parse the request body
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate the "source" field
	source := data["source"]
	if source == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Source is required",
		})
	}

	// Fetch the order with the given transaction ID
	var order models.Order
	if err := database.DB.Preload("OrderItems").Where("transaction_id = ?", source).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Order not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch order",
		})
	}

	// Mark the order as complete
	order.Complete = true
	if err := database.DB.Save(&order).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update order",
		})
	}

	// Calculate ambassador and admin revenue
	ambassadorRevenue := 0.0
	adminRevenue := 0.0
	for _, item := range order.OrderItems {
		ambassadorRevenue += item.AmbassadorRevenue
		adminRevenue += item.AdminRevenue
	}

	// Fetch the user associated with the order
	var user models.User
	if err := database.DB.First(&user, "id = ?", order.UserId).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch user",
		})
	}

	// Update rankings in Redis
	if err := database.Cache.ZIncrBy(context.Background(), "rankings", ambassadorRevenue, user.Name()).Err(); err != nil {
		log.Printf("Failed to update rankings in Redis: %v", err)
	}

	// Send emails asynchronously
	go func(order models.Order, ambassadorRevenue, adminRevenue float64) {
		ambassadorMessage := []byte(fmt.Sprintf("You earned $%f from the link #%s", ambassadorRevenue, order.Code))
		if err := smtp.SendMail("host.docker.internal:1025", nil, "no-reply@email.com", []string{order.AmbassadorEmail}, ambassadorMessage); err != nil {
			log.Printf("Failed to send email to ambassador: %v", err)
		}

		adminMessage := []byte(fmt.Sprintf("Order #%d with a total of $%f has been completed", order.Id, adminRevenue))
		if err := smtp.SendMail("host.docker.internal:1025", nil, "no-reply@email.com", []string{"admin@admin.com"}, adminMessage); err != nil {
			log.Printf("Failed to send email to admin: %v", err)
		}
	}(order, ambassadorRevenue, adminRevenue)

	return c.JSON(fiber.Map{
		"message": "success",
	})
}

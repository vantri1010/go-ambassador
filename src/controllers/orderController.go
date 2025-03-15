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

// Orders fetches all orders with their order items and calculates totals.
func Orders(c *fiber.Ctx) error {
	var orders []models.Order

	// Fetch all orders with their order items
	if err := database.DB.Preload("OrderItems").Find(&orders).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch orders",
		})
	}

	// Calculate the total for each order
	for i, order := range orders {
		orders[i].Name = order.FullName()
		orders[i].Total = order.GetTotal()
	}

	return c.JSON(orders)
}

// CreateOrderRequest defines the request body for creating an order.
type CreateOrderRequest struct {
	FirstName string           `json:"first_name" validate:"required"`
	LastName  string           `json:"last_name" validate:"required"`
	Email     string           `json:"email" validate:"required,email"`
	Address   string           `json:"address" validate:"required"`
	Country   string           `json:"country" validate:"required"`
	City      string           `json:"city" validate:"required"`
	Zip       string           `json:"zip" validate:"required"`
	Code      string           `json:"code" validate:"required"`
	Products  []map[string]int `json:"products" validate:"required,min=1"`
}

// CreateOrder handles the creation of a new order and Stripe checkout session.
func CreateOrder(c *fiber.Ctx) error {
	var request CreateOrderRequest

	// Parse and validate the request body
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validate required fields
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

	// Fetch the link associated with the order
	link := models.Link{}
	if err := database.DB.Preload("User").Where("code = ?", request.Code).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid link!",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch link",
		})
	}

	// Create the order
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

	// Start a database transaction
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Save the order to the database
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	// Prepare line items for Stripe checkout
	var lineItems []*stripe.CheckoutSessionLineItemParams

	for _, requestProduct := range request.Products {
		product := models.Product{}
		if err := database.DB.First(&product, requestProduct["product_id"]).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid product ID",
			})
		}

		total := product.Price * float64(requestProduct["quantity"])

		item := models.OrderItem{
			OrderId:           order.Id,
			ProductTitle:      product.Title,
			Price:             product.Price,
			Quantity:          uint(requestProduct["quantity"]),
			AmbassadorRevenue: 0.1 * total,
			AdminRevenue:      0.9 * total,
		}

		// Save the order item to the database
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

	// Set the Stripe secret key
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	// Create a Stripe checkout session
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

	// Update the order with the Stripe transaction ID
	order.TransactionId = source.ID
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	// Commit the transaction
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

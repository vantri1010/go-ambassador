package models

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	Model
	FirstName    string   `json:"first_name"`
	LastName     string   `json:"last_name"`
	Email        string   `json:"email" gorm:"unique"`
	Password     []byte   `json:"-"`
	IsAmbassador bool     `json:"-"`
	Revenue      *float64 `json:"revenue,omitempty" gorm:"-"`
}

func (user *User) SetPassword(password string) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	user.Password = hashedPassword
}

type Admin User

func (admin *Admin) CalculateRevenue(db *gorm.DB) {
	var orders []Order
	db.Preload("OrdersItems").Find(&orders, &Order{
		UserId:   admin.Id,
		Complete: true,
	})

	var rev float64 = 0
	for _, order := range orders {
		for _, item := range order.OrderItems {
			rev += item.AmbassadorRevenue
		}
	}

	admin.Revenue = &rev
}

type Ambassador User

func (ambassador *Ambassador) CalculateRevenue(db *gorm.DB) {
	var orders []Order
	db.Preload("OrdersItems").Find(&orders, &Order{
		UserId:   ambassador.Id,
		Complete: true,
	})

	var rev float64 = 0
	for _, order := range orders {
		for _, item := range order.OrderItems {
			rev += item.AmbassadorRevenue
		}
	}

	ambassador.Revenue = &rev
}

package database

import (
	"ambassador/src/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	var err error

	if DB, err = gorm.Open(mysql.Open("root:root@tcp(db:3306)/ambassador?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{}); err != nil {
		DB, err = gorm.Open(mysql.Open("root:root@tcp(localhost:3306)/ambassador?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	}
}

func AutoMigrate() {
	err := DB.AutoMigrate(models.User{}, models.Product{}, models.Link{}, models.Order{}, models.OrderItem{})
	if err != nil {
		return
	}
}

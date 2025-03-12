package database

import (
	"ambassador/src/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"time"
)

var DB *gorm.DB

func Connect() {
	var err error

	// Try to connect to the database using the first connection string
	connectionString := "root:root@tcp(db:3306)/ambassador?charset=utf8mb4&parseTime=True&loc=Local"
	DB, err = gorm.Open(mysql.Open(connectionString), &gorm.Config{})
	if err != nil {
		// If the first connection fails, try the fallback connection string
		log.Printf("Failed to connect to database at 'db:3306': %v. Trying fallback...", err)
		fallbackConnectionString := "root:root@tcp(localhost:3306)/ambassador?charset=utf8mb4&parseTime=True&loc=Local"
		DB, err = gorm.Open(mysql.Open(fallbackConnectionString), &gorm.Config{})
		if err != nil {
			log.Fatalf("Failed to connect to database at 'localhost:3306': %v", err)
		}
	}

	// Get the underlying SQL DB object to configure the connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying SQL DB object: %v", err)
	}

	// Configure the connection pool
	sqlDB.SetMaxIdleConns(10)           // Maximum number of idle connections
	sqlDB.SetMaxOpenConns(100)          // Maximum number of open connections
	sqlDB.SetConnMaxLifetime(time.Hour) // Maximum connection lifetime

	log.Println("Successfully connected to the database and configured the connection pool.")
}

func AutoMigrate() {
	err := DB.AutoMigrate(models.User{}, models.Product{}, models.Link{}, models.Order{}, models.OrderItem{})
	if err != nil {
		return
	}
}

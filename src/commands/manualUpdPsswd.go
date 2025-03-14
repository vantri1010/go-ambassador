package main

import (
	"ambassador/src/database"
	"ambassador/src/models"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	database.Connect()
	user := models.User{}
	password := "a"

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	database.DB.Model(user).Where("id <= 2").Update("password", hashedPassword)
}

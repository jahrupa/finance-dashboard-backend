package database

import (
	"context"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourorg/ap-backend/internal/models"
)

// SeedDefaultAdmin ensures a default admin user exists.

func SeedDefaultAdmin(db *DB, name, email, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	email = strings.ToLower(strings.TrimSpace(email))

	// Already seeded? — skip without touching the existing record.
	var existing models.User
	err := db.Users.FindOne(ctx, bson.M{"email": email}).Decode(&existing)
	if err == nil {
		log.Printf("ℹ️  Default admin already exists: %s", email)
		return nil
	}
	if err != mongo.ErrNoDocuments {
		return err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	now := time.Now()
	admin := models.User{
		Name:      name,
		Email:     email,
		Password:  string(hashed),
		Role:      "admin",
		IsActive:  true,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if _, err := db.Users.InsertOne(ctx, admin); err != nil {
		return err
	}

	log.Printf("✅ Default admin user created: %s", email)
	return nil
}

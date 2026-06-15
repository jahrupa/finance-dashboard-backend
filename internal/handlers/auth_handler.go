package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourorg/ap-backend/internal/database"
	"github.com/yourorg/ap-backend/internal/models"
)

type AuthHandler struct {
	db        *database.DB
	jwtSecret string
}

func NewAuthHandler(db *database.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret}
}

// POST /api/v1/auth/register ------------ not in uses
// func (h *AuthHandler) Register(c *fiber.Ctx) error {
// 	var req models.RegisterRequest
// 	if err := c.BodyParser(&req); err != nil {
// 		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
// 	}

// 	req.Name = strings.TrimSpace(req.Name)
// 	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

// 	if req.Name == "" || req.Email == "" || req.Password == "" || req.Role == "" {
// 		return fiber.NewError(fiber.StatusBadRequest, "name, email, password and role are required")
// 	}

// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()

// 	// Check duplicate email
// 	var existing models.User
// 	err := h.db.Users.FindOne(ctx, bson.M{"email": req.Email}).Decode(&existing)
// 	if err == nil {
// 		return fiber.NewError(fiber.StatusConflict, "Email already registered")
// 	}
// 	if err != mongo.ErrNoDocuments {
// 		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
// 	}

// 	hashed, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

// 	user := models.User{
// 		ID:         primitive.NewObjectID(),
// 		Name:       req.Name,
// 		Email:      req.Email,
// 		Password:   string(hashed),
// 		Role:       req.Role,
// 		Department: req.Department,
// 		// Phone:      req.Phone,
// 		// GSTNo:      req.GSTNo,
// 		// Address:    req.Address,
// 		IsActive:  true,
// 		CreatedAt: time.Now(),
// 		UpdatedAt: time.Now(),
// 	}

// 	if _, err = h.db.Users.InsertOne(ctx, user); err != nil {
// 		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
// 	}

// 	user.Password = ""
// 	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
// 		"success": true,
// 		"user":    user,
// 	})
// }

// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email and password are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err := h.db.Users.FindOne(ctx, bson.M{"email": req.Email}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid email or password")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}

	if !user.IsActive {
		return fiber.NewError(fiber.StatusForbidden, "Account is deactivated. Contact your administrator.")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid email or password")
	}

	// ── Generate JWT with ALL user fields needed by middleware ──
	token, err := h.generateJWT(user)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to generate token")
	}

	user.Password = ""
	return c.JSON(fiber.Map{
		"success": true,
		"token":   token,
		"user":    user,
	})
}

// generateJWT embeds department, page_access, crud_access into the token
// so the JWTAuth middleware can populate c.Locals without a DB round-trip.
func (h *AuthHandler) generateJWT(user models.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":         user.ID.Hex(),
		"email":       user.Email,
		"name":        user.Name,
		"role":        user.Role,
		"department":  user.Department,
		"page_access": user.PageAccess, // []string  — used for page-level access checks
		"crud_access": user.CrudAccess, // map[page]CrudOperation — used for CRUD checks
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.jwtSecret))
}

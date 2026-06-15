package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourorg/ap-backend/internal/database"
	"github.com/yourorg/ap-backend/internal/models"
)

type UserHandler struct{ db *database.DB }

func NewUserHandler(db *database.DB) *UserHandler { return &UserHandler{db: db} }

// GET /users  (admin only)
func (h *UserHandler) GetAll(c *fiber.Ctx) error {
	role := c.Query("role")
	dept := c.Query("department")
	page := c.QueryInt("page", 1)
	lmt := c.QueryInt("limit", 50)

	query := bson.M{}
	if role != "" {
		query["role"] = role
	}
	if dept != "" {
		query["department"] = dept
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Users.CountDocuments(ctx, query)
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}}).
		SetSkip(int64((page - 1) * lmt)).SetLimit(int64(lmt)).
		SetProjection(bson.M{"password": 0}) // never return password

	cursor, err := h.db.Users.Find(ctx, query, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch users")
	}
	defer cursor.Close(ctx)

	var users []models.User
	cursor.All(ctx, &users)
	if users == nil {
		users = []models.User{}
	}

	return c.JSON(fiber.Map{
		"success": true, "data": users,
		"meta": fiber.Map{"total": total, "page": page, "limit": lmt},
	})
}

// GET /users/me  — current logged-in user profile
func (h *UserHandler) GetMe(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(string)
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err = h.db.Users.FindOne(ctx, bson.M{"_id": oid},
		options.FindOne().SetProjection(bson.M{"password": 0})).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}

	return c.JSON(fiber.Map{"success": true, "user": user, "data": user})
}

// GET /users/:id
func (h *UserHandler) GetByID(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var user models.User
	err = h.db.Users.FindOne(ctx, bson.M{"_id": oid},
		options.FindOne().SetProjection(bson.M{"password": 0})).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	return c.JSON(fiber.Map{"success": true, "data": user})
}

// PUT /users/:id  (admin only)
func (h *UserHandler) Update(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	setFields := bson.M{"updated_at": time.Now()}
	if req.Name != "" {
		setFields["name"] = req.Name
	}
	if req.Role != "" {
		setFields["role"] = req.Role
	}
	if req.Department != "" {
		setFields["department"] = req.Department
	}
	if req.Phone != "" {
		setFields["phone"] = req.Phone
	}
	if req.GSTNo != "" {
		setFields["gst_no"] = req.GSTNo
	}
	if req.Address != "" {
		setFields["address"] = req.Address
	}
	if req.Status != "" {
		setFields["status"] = req.Status
	}
	if req.IsActive != nil {
		setFields["is_active"] = *req.IsActive
	}
	// Issue 3: persist page-level and CRUD permissions
	if req.PageAccess != nil {
		setFields["page_access"] = req.PageAccess
	}
	if req.CrudAccess != nil {
		setFields["crud_access"] = req.CrudAccess
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := h.db.Users.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": setFields})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update user")
	}
	if r.MatchedCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}

	var updated models.User
	h.db.Users.FindOne(ctx, bson.M{"_id": oid}, options.FindOne().SetProjection(bson.M{"password": 0})).Decode(&updated)
	return c.JSON(fiber.Map{"success": true, "message": "User updated successfully", "data": updated})
}

// PUT /users/me/password  — change own password
func (h *UserHandler) ChangePassword(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(string)
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid token")
	}

	var req models.ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		return fiber.NewError(fiber.StatusBadRequest, "oldPassword and newPassword are required")
	}
	if len(req.NewPassword) < 6 {
		return fiber.NewError(fiber.StatusBadRequest, "newPassword must be at least 6 characters")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	if err := h.db.Users.FindOne(ctx, bson.M{"_id": oid}).Decode(&user); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Old password is incorrect")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to hash password")
	}

	h.db.Users.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"password": string(hashed), "updated_at": time.Now()}})
	return c.JSON(fiber.Map{"success": true, "message": "Password changed successfully"})
}

// POST /users  OR  POST /user-access/create-user-access
// Admin creates a user with full profile + page/CRUD permissions in one call.
func (h *UserHandler) Create(c *fiber.Ctx) error {
	var req models.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Name == "" || req.Email == "" || req.Password == "" || req.Role == "" || req.Department == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name, email, password, role and department are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Duplicate check
	var dup models.User
	if err := h.db.Users.FindOne(ctx, bson.M{"email": req.Email}).Decode(&dup); err == nil {
		return fiber.NewError(fiber.StatusConflict, "A user with this email already exists")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to hash password")
	}

	now := time.Now()
	user := models.User{
		ID:         primitive.NewObjectID(),
		Name:       req.Name,
		Email:      req.Email,
		Password:   string(hashed),
		Role:       req.Role,
		Department: req.Department,
		Phone:      req.Phone,
		GSTNo:      req.GSTNo,
		Address:    req.Address,
		PageAccess: req.PageAccess,
		CrudAccess: req.CrudAccess,
		Status:     req.Status,
		IsActive:   true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if user.Status == "" {
		user.Status = "active"
	}

	if _, err := h.db.Users.InsertOne(ctx, user); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create user")
	}

	user.Password = ""
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"message": "User created successfully",
		"data":    user,
	})
}

// DELETE /users/:id  — soft delete (admin only)
func (h *UserHandler) Delete(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := h.db.Users.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"is_active": false, "updated_at": time.Now()}})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to deactivate user")
	}
	if r.MatchedCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "User deactivated successfully"})
}

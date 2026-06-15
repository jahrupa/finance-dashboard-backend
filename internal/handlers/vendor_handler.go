package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/yourorg/ap-backend/internal/database"
	"github.com/yourorg/ap-backend/internal/models"
)

type VendorHandler struct{ db *database.DB }

func NewVendorHandler(db *database.DB) *VendorHandler { return &VendorHandler{db: db} }

// GET /vendors
func (h *VendorHandler) GetAll(c *fiber.Ctx) error {
	search := c.Query("search")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)

	query := bson.M{"is_active": true}
	if search != "" {
		query["name"] = bson.M{"$regex": search, "$options": "i"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Vendors.CountDocuments(ctx, query)
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}}).
		SetSkip(int64((page - 1) * limit)).SetLimit(int64(limit))

	cursor, err := h.db.Vendors.Find(ctx, query, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch vendors")
	}
	defer cursor.Close(ctx)

	var vendors []models.Vendor
	cursor.All(ctx, &vendors)
	if vendors == nil {
		vendors = []models.Vendor{}
	}

	return c.JSON(fiber.Map{
		"success": true, "data": vendors,
		"meta": fiber.Map{"total": total, "page": page, "limit": limit},
	})
}

// GET /vendors/:id
func (h *VendorHandler) GetByID(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid vendor ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var vendor models.Vendor
	if err := h.db.Vendors.FindOne(ctx, bson.M{"_id": oid}).Decode(&vendor); err != nil {
		if err == mongo.ErrNoDocuments {
			return fiber.NewError(fiber.StatusNotFound, "Vendor not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	return c.JSON(fiber.Map{"success": true, "data": vendor})
}

// POST /vendors
func (h *VendorHandler) Create(c *fiber.Ctx) error {
	var req models.CreateVendorRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dup models.Vendor
	if err := h.db.Vendors.FindOne(ctx, bson.M{"name": req.Name}).Decode(&dup); err == nil {
		return fiber.NewError(fiber.StatusConflict, "Vendor with this name already exists")
	}

	now := time.Now()
	vendor := models.Vendor{
		ID: primitive.NewObjectID(), Name: req.Name, Email: req.Email,
		Phone: req.Phone, GSTNo: req.GSTNo, Address: req.Address,
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := h.db.Vendors.InsertOne(ctx, vendor); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create vendor")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "Vendor created successfully", "data": vendor})
}

// PUT /vendors/:id
func (h *VendorHandler) Update(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid vendor ID")
	}
	var req models.UpdateVendorRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	setFields := bson.M{"updated_at": time.Now()}
	if req.Name != "" {
		setFields["name"] = req.Name
	}
	if req.Email != "" {
		setFields["email"] = req.Email
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := h.db.Vendors.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": setFields})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update vendor")
	}
	if r.MatchedCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Vendor not found")
	}

	var updated models.Vendor
	h.db.Vendors.FindOne(ctx, bson.M{"_id": oid}).Decode(&updated)
	return c.JSON(fiber.Map{"success": true, "message": "Vendor updated successfully", "data": updated})
}

// DELETE /vendors/:id  (soft delete)
func (h *VendorHandler) Delete(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid vendor ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := h.db.Vendors.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"is_active": false, "updated_at": time.Now()}})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete vendor")
	}
	if r.MatchedCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Vendor not found")
	}
	return c.JSON(fiber.Map{"success": true, "message": "Vendor deactivated successfully"})
}

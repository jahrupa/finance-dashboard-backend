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

type ActivityLogHandler struct{ db *database.DB }

func NewActivityLogHandler(db *database.DB) *ActivityLogHandler {
	return &ActivityLogHandler{db: db}
}

// GET /activity-logs
// Query params: userName, action, department, fromDate, toDate, search, page, limit
func (h *ActivityLogHandler) GetAll(c *fiber.Ctx) error {
	var f models.ActivityLogFilter
	if err := c.QueryParser(&f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid query parameters")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	query := bson.M{}

	if f.UserName != "" {
		query["performed_by"] = bson.M{"$regex": f.UserName, "$options": "i"}
	}
	if f.Action != "" {
		query["action"] = f.Action
	}
	if f.Department != "" {
		query["department"] = f.Department
	}
	if f.Search != "" {
		query["$or"] = bson.A{
			bson.M{"invoice_no": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"vendor": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"performed_by": bson.M{"$regex": f.Search, "$options": "i"}},
		}
	}
	// Date range
	if f.FromDate != "" || f.ToDate != "" {
		tsFilter := bson.M{}
		if f.FromDate != "" {
			if t, err := time.Parse("2006-01-02", f.FromDate); err == nil {
				tsFilter["$gte"] = t
			}
		}
		if f.ToDate != "" {
			if t, err := time.Parse("2006-01-02", f.ToDate); err == nil {
				// include full day
				tsFilter["$lte"] = t.Add(24*time.Hour - time.Second)
			}
		}
		if len(tsFilter) > 0 {
			query["timestamp"] = tsFilter
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.AuditTrail.CountDocuments(ctx, query)

	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetSkip((f.Page - 1) * f.Limit).
		SetLimit(f.Limit)

	cursor, err := h.db.AuditTrail.Find(ctx, query, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch activity logs")
	}
	defer cursor.Close(ctx)

	var entries []models.AuditEntry
	cursor.All(ctx, &entries)
	if entries == nil {
		entries = []models.AuditEntry{}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    entries,
		"meta": fiber.Map{
			"total": total,
			"page":  f.Page,
			"limit": f.Limit,
			"pages": (total + f.Limit - 1) / f.Limit,
		},
	})
}

// GET /activity-logs/stats
// Returns per-user action counts + action-type breakdown — used for the
// "Who did what" summary panel.
func (h *ActivityLogHandler) GetStats(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ── Per-user aggregation ─────────────────────────────────
	userPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "user", Value: "$performed_by"},
				{Key: "role", Value: "$role"},
			}},
			{Key: "totalActions", Value: bson.M{"$sum": 1}},
			{Key: "lastActive", Value: bson.M{"$max": "$timestamp"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "totalActions", Value: -1}}}},
		{{Key: "$limit", Value: int64(20)}},
	}

	cursor, err := h.db.AuditTrail.Aggregate(ctx, userPipeline)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to aggregate user stats")
	}
	defer cursor.Close(ctx)

	type userRow struct {
		ID struct {
			User string `bson:"user"`
			Role string `bson:"role"`
		} `bson:"_id"`
		TotalActions int64     `bson:"totalActions"`
		LastActive   time.Time `bson:"lastActive"`
	}

	var userRows []userRow
	cursor.All(ctx, &userRows)

	// ── Per-user action breakdown ────────────────────────────
	// For each top user fetch their action breakdown
	type userStat struct {
		UserName        string           `json:"userName"`
		Role            string           `json:"role"`
		TotalActions    int64            `json:"totalActions"`
		ActionBreakdown map[string]int64 `json:"actionBreakdown"`
		LastActive      time.Time        `json:"lastActive"`
	}

	var stats []userStat
	for _, row := range userRows {
		bdPipeline := mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"performed_by": row.ID.User}}},
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: "$action"},
				{Key: "count", Value: bson.M{"$sum": 1}},
			}}},
		}
		bdCursor, err := h.db.AuditTrail.Aggregate(ctx, bdPipeline)
		breakdown := map[string]int64{}
		if err == nil {
			type bdRow struct {
				ID    string `bson:"_id"`
				Count int64  `bson:"count"`
			}
			var rows []bdRow
			bdCursor.All(ctx, &rows)
			bdCursor.Close(ctx)
			for _, r := range rows {
				breakdown[r.ID] = r.Count
			}
		}

		stats = append(stats, userStat{
			UserName:        row.ID.User,
			Role:            row.ID.Role,
			TotalActions:    row.TotalActions,
			ActionBreakdown: breakdown,
			LastActive:      row.LastActive,
		})
	}
	if stats == nil {
		stats = []userStat{}
	}

	// ── Overall action-type totals ───────────────────────────
	actionPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$action"},
			{Key: "count", Value: bson.M{"$sum": 1}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}
	aCursor, err := h.db.AuditTrail.Aggregate(ctx, actionPipeline)
	actionTotals := map[string]int64{}
	if err == nil {
		type aRow struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		var aRows []aRow
		aCursor.All(ctx, &aRows)
		aCursor.Close(ctx)
		for _, r := range aRows {
			actionTotals[r.ID] = r.Count
		}
	}

	// ── Total count ─────────────────────────────────────────
	total, _ := h.db.AuditTrail.CountDocuments(ctx, bson.M{})

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"userStats":    stats,
			"actionTotals": actionTotals,
			"totalEntries": total,
		},
	})
}

// GET /activity-logs/invoice/:id
// All activity for a specific invoice, oldest-first (full timeline)
func (h *ActivityLogHandler) GetByInvoice(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid invoice ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})
	cursor, err := h.db.AuditTrail.Find(ctx, bson.M{"invoice_id": oid}, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch invoice activity")
	}
	defer cursor.Close(ctx)

	var entries []models.AuditEntry
	cursor.All(ctx, &entries)
	if entries == nil {
		entries = []models.AuditEntry{}
	}

	return c.JSON(fiber.Map{"success": true, "data": entries})
}

// GET /activity-logs/user/:name
// All activity by a specific user, newest-first
func (h *ActivityLogHandler) GetByUser(c *fiber.Ctx) error {
	userName := c.Params("name")
	if userName == "" {
		return fiber.NewError(fiber.StatusBadRequest, "User name is required")
	}

	page := int64(c.QueryInt("page", 1))
	limit := int64(c.QueryInt("limit", 50))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := bson.M{"performed_by": bson.M{"$regex": "^" + userName + "$", "$options": "i"}}
	total, _ := h.db.AuditTrail.CountDocuments(ctx, query)

	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetSkip((page - 1) * limit).
		SetLimit(limit)

	cursor, err := h.db.AuditTrail.Find(ctx, query, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch user activity")
	}
	defer cursor.Close(ctx)

	var entries []models.AuditEntry
	cursor.All(ctx, &entries)
	if entries == nil {
		entries = []models.AuditEntry{}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    entries,
		"meta":    fiber.Map{"total": total, "page": page, "limit": limit},
	})
}

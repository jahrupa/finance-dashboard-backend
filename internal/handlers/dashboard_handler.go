package handlers

import (
	"context"
	"math"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/yourorg/ap-backend/internal/database"
	"github.com/yourorg/ap-backend/internal/models"
)

type DashboardHandler struct{ db *database.DB }

func NewDashboardHandler(db *database.DB) *DashboardHandler { return &DashboardHandler{db: db} }

// scopeFilter returns a bson.M base filter scoped to the user's department.
// Admin → sees all. Non-admin with dept → sees their dept. No dept → sees nothing.
func scopeFilter(c *fiber.Ctx) bson.M {
	userRole, _ := c.Locals("userRole").(string)
	userDepartment, _ := c.Locals("userDepartment").(string)
	isAdmin := userRole == "admin" || userRole == "super_admin"
	if isAdmin {
		return bson.M{}
	}
	if userDepartment != "" {
		return bson.M{"department": userDepartment}
	}
	// Non-admin with no department → return impossible filter (empty result set)
	return bson.M{"_id": bson.M{"$exists": false}}
}

// GET /dashboard/kpis
func (h *DashboardHandler) GetKPIs(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matchFilter := scopeFilter(c)

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "totalInvoices", Value: bson.M{"$sum": 1}},
			{Key: "totalValue", Value: bson.M{"$sum": "$amount"}},
			{Key: "pendingReview", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Pending Review"}}, 1, 0}}}},
			{Key: "onHold", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "On Hold"}}, 1, 0}}}},
			{Key: "rejected", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Rejected"}}, 1, 0}}}},
			{Key: "paidCount", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Paid"}}, 1, 0}}}},
			{Key: "paidValue", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Paid"}}, "$amount", 0}}}},
			{Key: "hodPending", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "HOD Approval"}}, 1, 0}}}},
			{Key: "paymentPending", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Payment Approval"}}, 1, 0}}}},
			{Key: "readyForPayment", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Ready for Payment"}}, 1, 0}}}},
			{Key: "awaitingApproval", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$in": bson.A{"$status", bson.A{"HOD Approval", "Payment Approval", "Ready for Payment"}}}, 1, 0}}}},
		}}},
	}

	cursor, err := h.db.Invoices.Aggregate(ctx, pipeline)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to aggregate KPIs")
	}
	defer cursor.Close(ctx)

	var results []bson.M
	cursor.All(ctx, &results)

	kpi := models.KPIResponse{}
	if len(results) > 0 {
		r := results[0]
		kpi.TotalInvoices = toInt64(r["totalInvoices"])
		kpi.TotalValue = toFloat64(r["totalValue"])
		kpi.PendingReview = toInt64(r["pendingReview"])
		kpi.OnHold = toInt64(r["onHold"])
		kpi.Rejected = toInt64(r["rejected"])
		kpi.PaidCount = toInt64(r["paidCount"])
		kpi.PaidValue = toFloat64(r["paidValue"])
		kpi.AwaitingApproval = toInt64(r["awaitingApproval"])
		kpi.HODPending = toInt64(r["hodPending"])
		kpi.PaymentPending = toInt64(r["paymentPending"])
		kpi.ReadyForPayment = toInt64(r["readyForPayment"])
	}
	kpi.AvgCycleDays = h.getAvgCycleDays(ctx, matchFilter)

	return c.JSON(fiber.Map{"success": true, "data": kpi})
}

// GET /dashboard/aging
func (h *DashboardHandler) GetAgingAnalysis(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matchFilter := scopeFilter(c)
	matchFilter["status"] = bson.M{"$nin": bson.A{"Paid", "Rejected"}}

	cursor, err := h.db.Invoices.Find(ctx, matchFilter)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch invoices")
	}
	defer cursor.Close(ctx)

	now := time.Now()
	buckets := map[string]int64{"0\u20133 Days": 0, "4\u20137 Days": 0, "8\u201315 Days": 0, "15+ Days": 0}
	for cursor.Next(ctx) {
		var inv models.Invoice
		if err := cursor.Decode(&inv); err != nil {
			continue
		}
		days := int(now.Sub(inv.DateOfReceipt).Hours() / 24)
		switch {
		case days <= 3:
			buckets["0\u20133 Days"]++
		case days <= 7:
			buckets["4\u20137 Days"]++
		case days <= 15:
			buckets["8\u201315 Days"]++
		default:
			buckets["15+ Days"]++
		}
	}
	return c.JSON(fiber.Map{"success": true, "data": []models.AgingBucket{
		{Bucket: "0\u20133 Days", Count: buckets["0\u20133 Days"]},
		{Bucket: "4\u20137 Days", Count: buckets["4\u20137 Days"]},
		{Bucket: "8\u201315 Days", Count: buckets["8\u201315 Days"]},
		{Bucket: "15+ Days", Count: buckets["15+ Days"]},
	}})
}

// GET /dashboard/department-stats
func (h *DashboardHandler) GetDepartmentStats(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matchFilter := scopeFilter(c)

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$department"},
			{Key: "total", Value: bson.M{"$sum": 1}},
			{Key: "pending", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$not": bson.A{bson.M{"$in": bson.A{"$status", bson.A{"Paid", "Rejected"}}}}}, 1, 0}}}},
			{Key: "paid", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Paid"}}, 1, 0}}}},
			{Key: "rejected", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Rejected"}}, 1, 0}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "pending", Value: -1}}}},
	}

	cursor, err := h.db.Invoices.Aggregate(ctx, pipeline)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to aggregate")
	}
	defer cursor.Close(ctx)

	var stats []models.DeptStat
	for cursor.Next(ctx) {
		var r bson.M
		cursor.Decode(&r)
		dept, _ := r["_id"].(string)
		stats = append(stats, models.DeptStat{
			Department: dept, Total: toInt64(r["total"]), Pending: toInt64(r["pending"]),
			Paid: toInt64(r["paid"]), Rejected: toInt64(r["rejected"]),
		})
	}
	if stats == nil {
		stats = []models.DeptStat{}
	}
	return c.JSON(fiber.Map{"success": true, "data": stats})
}

// GET /dashboard/monthly-trends?months=6
func (h *DashboardHandler) GetMonthlyTrends(c *fiber.Ctx) error {
	months := c.QueryInt("months", 6)
	if months <= 0 || months > 24 {
		months = 6
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fromDate := time.Now().AddDate(0, -months, 0)
	matchFilter := scopeFilter(c)
	matchFilter["date_of_receipt"] = bson.M{"$gte": fromDate}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.M{"$dateToString": bson.M{"format": "%Y-%m", "date": "$date_of_receipt"}}},
			{Key: "count", Value: bson.M{"$sum": 1}},
			{Key: "amount", Value: bson.M{"$sum": "$amount"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	cursor, err := h.db.Invoices.Aggregate(ctx, pipeline)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to aggregate")
	}
	defer cursor.Close(ctx)

	var trends []models.MonthlyTrend
	for cursor.Next(ctx) {
		var r bson.M
		cursor.Decode(&r)
		month, _ := r["_id"].(string)
		trends = append(trends, models.MonthlyTrend{
			Month: month, Count: toInt64(r["count"]), Amount: toFloat64(r["amount"]),
		})
	}
	if trends == nil {
		trends = []models.MonthlyTrend{}
	}
	return c.JSON(fiber.Map{"success": true, "data": trends})
}

// GET /dashboard/top-vendors?limit=10
func (h *DashboardHandler) GetTopVendors(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 10)
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	matchFilter := scopeFilter(c)

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$vendor"},
			{Key: "count", Value: bson.M{"$sum": 1}},
			{Key: "amount", Value: bson.M{"$sum": "$amount"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "amount", Value: -1}}}},
		{{Key: "$limit", Value: int64(limit)}},
	}

	cursor, err := h.db.Invoices.Aggregate(ctx, pipeline)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to aggregate")
	}
	defer cursor.Close(ctx)

	var vendors []models.VendorStat
	for cursor.Next(ctx) {
		var r bson.M
		cursor.Decode(&r)
		name, _ := r["_id"].(string)
		vendors = append(vendors, models.VendorStat{
			Vendor: name, Count: toInt64(r["count"]), Amount: toFloat64(r["amount"]),
		})
	}
	if vendors == nil {
		vendors = []models.VendorStat{}
	}
	return c.JSON(fiber.Map{"success": true, "data": vendors})
}

// GET /dashboard/summary  — single endpoint with everything
func (h *DashboardHandler) GetSummary(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	matchFilter := scopeFilter(c)

	// Run KPI pipeline
	kpiPipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "totalInvoices", Value: bson.M{"$sum": 1}},
			{Key: "totalValue", Value: bson.M{"$sum": "$amount"}},
			{Key: "pendingReview", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Pending Review"}}, 1, 0}}}},
			{Key: "onHold", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "On Hold"}}, 1, 0}}}},
			{Key: "rejected", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Rejected"}}, 1, 0}}}},
			{Key: "paidCount", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Paid"}}, 1, 0}}}},
			{Key: "paidValue", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Paid"}}, "$amount", 0}}}},
			{Key: "hodPending", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "HOD Approval"}}, 1, 0}}}},
			{Key: "paymentPending", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Payment Approval"}}, 1, 0}}}},
			{Key: "readyForPayment", Value: bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "Ready for Payment"}}, 1, 0}}}},
		}}},
	}
	cursor, _ := h.db.Invoices.Aggregate(ctx, kpiPipeline)
	var kpiResults []bson.M
	cursor.All(ctx, &kpiResults)
	cursor.Close(ctx)

	kpi := models.KPIResponse{}
	if len(kpiResults) > 0 {
		r := kpiResults[0]
		kpi.TotalInvoices = toInt64(r["totalInvoices"])
		kpi.TotalValue = toFloat64(r["totalValue"])
		kpi.PendingReview = toInt64(r["pendingReview"])
		kpi.OnHold = toInt64(r["onHold"])
		kpi.Rejected = toInt64(r["rejected"])
		kpi.PaidCount = toInt64(r["paidCount"])
		kpi.PaidValue = toFloat64(r["paidValue"])
		kpi.HODPending = toInt64(r["hodPending"])
		kpi.PaymentPending = toInt64(r["paymentPending"])
		kpi.ReadyForPayment = toInt64(r["readyForPayment"])
		kpi.AwaitingApproval = kpi.HODPending + kpi.PaymentPending + kpi.ReadyForPayment
	}
	kpi.AvgCycleDays = h.getAvgCycleDays(ctx, matchFilter)

	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"kpis": kpi}})
}

func (h *DashboardHandler) getAvgCycleDays(ctx context.Context, deptFilter bson.M) float64 {
	matchFilter := bson.M{"status": "Paid", "payment_date": bson.M{"$exists": true}}
	for k, v := range deptFilter {
		matchFilter[k] = v
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$project", Value: bson.M{"cycleDays": bson.M{"$divide": bson.A{
			bson.M{"$subtract": bson.A{"$payment_date", "$date_of_receipt"}},
			1000 * 60 * 60 * 24,
		}}}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "avg": bson.M{"$avg": "$cycleDays"}}}},
	}
	cursor, err := h.db.Invoices.Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	defer cursor.Close(ctx)
	var results []bson.M
	cursor.All(ctx, &results)
	if len(results) == 0 {
		return 0
	}
	return math.Round(toFloat64(results[0]["avg"])*10) / 10
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int32:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	}
	return 0
}

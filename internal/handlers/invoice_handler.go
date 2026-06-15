// this is new file don't remove existing code only fix 365 days ago logic and uploadBasePath DownloadAllDocumentsZip logic also add FinanceEditInvoice logic
package handlers

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/yourorg/ap-backend/internal/database"
	"github.com/yourorg/ap-backend/internal/models"
)

type InvoiceHandler struct{ db *database.DB }

func NewInvoiceHandler(db *database.DB) *InvoiceHandler { return &InvoiceHandler{db: db} }

// ════════════════════════════════════════════════════════════════
//  BASIC CRUD
// ════════════════════════════════════════════════════════════════

// GET /invoices
func (h *InvoiceHandler) GetAll(c *fiber.Ctx) error {
	f := new(models.InvoiceFilter)
	if err := c.QueryParser(f); err != nil {
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
	if f.Status != "" {
		query["status"] = f.Status
	}
	if f.Department != "" {
		query["department"] = f.Department
	}
	if f.Search != "" {
		query["$or"] = bson.A{
			bson.M{"vendor": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"invoice_no": bson.M{"$regex": f.Search, "$options": "i"}},
		}
	}
	if f.FromDate != "" || f.ToDate != "" {
		dateFilter := bson.M{}
		if f.FromDate != "" {
			if t, err := time.Parse("2006-01-02", f.FromDate); err == nil {
				dateFilter["$gte"] = t
			}
		}
		if f.ToDate != "" {
			if t, err := time.Parse("2006-01-02", f.ToDate); err == nil {
				dateFilter["$lte"] = t.Add(24*time.Hour - time.Second)
			}
		}
		if len(dateFilter) > 0 {
			query["date_of_receipt"] = dateFilter
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Invoices.CountDocuments(ctx, query)
	opts := options.Find().
		SetSort(bson.D{{Key: "date_of_receipt", Value: -1}}).
		SetSkip((f.Page - 1) * f.Limit).
		SetLimit(f.Limit)

	cursor, err := h.db.Invoices.Find(ctx, query, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch invoices")
	}
	defer cursor.Close(ctx)

	var invoices []models.Invoice
	cursor.All(ctx, &invoices)
	if invoices == nil {
		invoices = []models.Invoice{}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    invoices,
		"meta":    fiber.Map{"total": total, "page": f.Page, "limit": f.Limit, "pages": (total + f.Limit - 1) / f.Limit},
	})
}

// GET /invoices/:id
func (h *InvoiceHandler) GetByID(c *fiber.Ctx) error {
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"success": true, "data": inv})
}

// POST /invoices
func (h *InvoiceHandler) Create(c *fiber.Ctx) error {
	var req models.CreateInvoiceRequest

	if err := c.BodyParser(&req); err != nil {
		// Multipart form requests may not parse with BodyParser
		req.InvoiceNo = c.FormValue("invoiceNo")
		req.Vendor = c.FormValue("vendor")
		req.VendorEmail = c.FormValue("vendorEmail") // ← was req.vendorEmail
		req.InvoiceDate = c.FormValue("invoiceDate")
		req.Department = c.FormValue("department")
		req.UploadedBy = c.FormValue("uploadedBy")
		req.DueDate = c.FormValue("dueDate")
		req.TaxDetails = c.FormValue("taxDetails")
		req.Remarks = c.FormValue("remarks")
		req.DocumentURL = c.FormValue("documentUrl")
		if amount := c.FormValue("amount"); amount != "" {
			fmt.Sscanf(amount, "%f", &req.Amount)
		}
	}

	if req.InvoiceNo == "" || req.Vendor == "" || req.InvoiceDate == "" ||
		req.Department == "" || req.UploadedBy == "" || req.Amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invoiceNo, vendor, invoiceDate, department, uploadedBy, amount are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dup models.Invoice
	err := h.db.Invoices.FindOne(ctx, bson.M{"invoice_no": req.InvoiceNo, "vendor": req.Vendor}).Decode(&dup)
	if err == nil {
		return fiber.NewError(fiber.StatusConflict, "Invoice with same number and vendor already exists")
	}
	if err != mongo.ErrNoDocuments {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}

	now := time.Now()
	inv := models.Invoice{
		ID: primitive.NewObjectID(), InvoiceNo: req.InvoiceNo, Vendor: req.Vendor,
		InvoiceDate: req.InvoiceDate, DateOfReceipt: now, Department: req.Department,
		UploadedBy: req.UploadedBy, Amount: req.Amount, DueDate: req.DueDate,
		TaxDetails: req.TaxDetails, Remarks: req.Remarks, DocumentURL: req.DocumentURL,
		Status: "Pending Review", CreatedAt: now, UpdatedAt: now,
	}

	savedDocURLs, err := h.saveInvoiceFiles(c, inv.ID)
	if err != nil {
		return err
	}
	if len(savedDocURLs) > 0 {
		inv.Documents = savedDocURLs
		inv.DocumentURL = savedDocURLs[0]
	}

	if _, err := h.db.Invoices.InsertOne(ctx, inv); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create invoice")
	}
	h.addAudit(inv.ID, "Invoice Submitted", c, "", "Pending Review", req.Remarks)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "Invoice submitted successfully", "data": inv})
}

// PUT /invoices/:id  — update editable fields (only when Pending Review or On Hold)
func (h *InvoiceHandler) Update(c *fiber.Ctx) error {

	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	editableStatuses := map[string]bool{
		"Pending Review": true,
		"On Hold":        true,
	}

	if !editableStatuses[inv.Status] {
		return fiber.NewError(fiber.StatusBadRequest,
			"Invoice can only be edited when status is Pending Review or On Hold")
	}

	// ─────────────────────────────────────────────
	// 2️⃣ Parse form data
	// ─────────────────────────────────────────────
	var req models.UpdateInvoiceRequest

	if err := c.BodyParser(&req); err != nil {
		req.Vendor = c.FormValue("vendor")
		req.VendorEmail = c.FormValue("vendorEmail") // ← was req.vendorEmail
		req.InvoiceDate = c.FormValue("invoiceDate")
		req.Department = c.FormValue("department")
		req.DueDate = c.FormValue("dueDate")
		req.TaxDetails = c.FormValue("taxDetails")
		req.Remarks = c.FormValue("remarks")

		if amount := c.FormValue("amount"); amount != "" {
			fmt.Sscanf(amount, "%f", &req.Amount)
		}
	}

	// ─────────────────────────────────────────────
	// 3️⃣ Handle Existing Document Logic
	// ─────────────────────────────────────────────

	// Documents user wants to keep/remove
	var keepDocs, removeDocs []string
	if form, err := c.MultipartForm(); err == nil && form != nil {
		keepDocs = form.Value["keepDocuments"]
		removeDocs = form.Value["removeDocuments"]
	}

	// Create folder path
	folderPath := filepath.Join(uploadBasePath, "invoices", inv.ID.Hex())

	// 🔥 Physically delete removed files
	for _, fileName := range removeDocs {
		fullPath := filepath.Join(folderPath, fileName)
		_ = os.Remove(fullPath) // ignore error if not exists
	}

	// Start with kept documents
	finalDocs := make([]string, 0)
	finalDocs = append(finalDocs, keepDocs...)

	// ─────────────────────────────────────────────
	// 4️⃣ Handle New File Uploads
	// ─────────────────────────────────────────────
	newUploadedFiles, err := h.saveInvoiceFiles(c, inv.ID)
	if err != nil {
		return err
	}

	if len(newUploadedFiles) > 0 {
		finalDocs = append(finalDocs, newUploadedFiles...)
	}

	// ─────────────────────────────────────────────
	// 5️⃣ Prepare Update Fields
	// ─────────────────────────────────────────────
	setFields := bson.M{
		"updated_at": time.Now(),
		"documents":  finalDocs,
	}

	if len(finalDocs) > 0 {
		setFields["document_url"] = finalDocs[0]
	} else {
		setFields["document_url"] = ""
	}

	if req.Vendor != "" {
		setFields["vendor"] = req.Vendor
	}
	if req.VendorEmail != "" {
		setFields["vendor_email"] = req.VendorEmail // ← was req.vendorEmail
	}
	if req.InvoiceDate != "" {
		setFields["invoice_date"] = req.InvoiceDate
	}
	if req.Department != "" {
		setFields["department"] = req.Department
	}
	if req.Amount > 0 {
		setFields["amount"] = req.Amount
	}
	if req.DueDate != "" {
		setFields["due_date"] = req.DueDate
	}
	if req.TaxDetails != "" {
		setFields["tax_details"] = req.TaxDetails
	}
	if req.Remarks != "" {
		setFields["remarks"] = req.Remarks
	}

	// ─────────────────────────────────────────────
	// 6️⃣ Update Database
	// ─────────────────────────────────────────────
	if err := h.updateInvoice(inv.ID, bson.M{"$set": setFields}); err != nil {
		return err
	}

	// ─────────────────────────────────────────────
	// 7️⃣ Audit Log
	// ─────────────────────────────────────────────
	h.addAudit(inv.ID, "Invoice Updated", c, inv.Status, inv.Status, "Invoice updated with file changes")

	updated, _ := h.findByID(c.Params("id"))

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Invoice updated successfully",
		"data":    updated,
	})
}

// DELETE /invoices/:id  — soft delete (only Rejected invoices)
func (h *InvoiceHandler) Delete(c *fiber.Ctx) error {
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	if inv.Status != "Rejected" {
		return fiber.NewError(fiber.StatusBadRequest, "Only Rejected invoices can be deleted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = h.db.Invoices.DeleteOne(ctx, bson.M{"_id": inv.ID})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete invoice")
	}
	h.addAudit(inv.ID, "Invoice Deleted", c, inv.Status, "Deleted", "")
	return c.JSON(fiber.Map{"success": true, "message": "Invoice deleted successfully"})
}

// ════════════════════════════════════════════════════════════════
//  FINANCE SECTION
// ════════════════════════════════════════════════════════════════

// GET /invoices/finance  — all invoices relevant to finance team
func (h *InvoiceHandler) GetFinanceInvoices(c *fiber.Ctx) error {
	f := new(models.FinanceFilter)
	if err := c.QueryParser(f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid query parameters")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := bson.M{}
	if f.FinanceStatus != "" {
		query["finance_status"] = f.FinanceStatus
	} else {
		// Default: show invoices finance team needs to act on
		query["status"] = bson.M{"$in": bson.A{"Pending Review", "On Hold", "Rejected", "HOD Approval", "Payment Approval", "Ready for Payment", "Paid"}}
	}
	if f.Department != "" {
		query["department"] = f.Department
	}
	if f.Search != "" {
		query["$or"] = bson.A{
			bson.M{"vendor": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"invoice_no": bson.M{"$regex": f.Search, "$options": "i"}},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Invoices.CountDocuments(ctx, query)
	opts := options.Find().SetSort(bson.D{{Key: "date_of_receipt", Value: -1}}).
		SetSkip((f.Page - 1) * f.Limit).SetLimit(f.Limit)

	cursor, err := h.db.Invoices.Find(ctx, query, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch invoices")
	}
	defer cursor.Close(ctx)

	var invoices []models.Invoice
	cursor.All(ctx, &invoices)
	if invoices == nil {
		invoices = []models.Invoice{}
	}

	// Summary counts
	pendingCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"status": "Pending Review"})
	holdCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"finance_status": "Hold"})
	acceptedCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"finance_status": "Accepted"})
	rejectedCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"finance_status": "Rejected"})

	return c.JSON(fiber.Map{
		"success": true,
		"data":    invoices,
		"summary": fiber.Map{
			"pendingReview": pendingCount,
			"onHold":        holdCount,
			"accepted":      acceptedCount,
			"rejected":      rejectedCount,
		},
		"meta": fiber.Map{"total": total, "page": f.Page, "limit": f.Limit, "pages": (total + f.Limit - 1) / f.Limit},
	})
}
func (h *InvoiceHandler) FinanceAction(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid invoice ID")
	}

	var req struct {
		Action      string `json:"action"`
		GLCode      string `json:"glCode"`
		ExpenseHead string `json:"expenseHead"`
		Remarks     string `json:"remarks"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	update := bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	switch req.Action {

	case "Accept":
		update["$set"].(bson.M)["status"] = "HOD Approval"
		update["$set"].(bson.M)["finance_status"] = "Accepted"
		update["$set"].(bson.M)["gl_code"] = req.GLCode
		update["$set"].(bson.M)["expense_head"] = req.ExpenseHead

		// 🔥 reset HOD if previously processed
		update["$unset"] = bson.M{
			"hod_status": "",
		}

	case "Reject":
		update["$set"].(bson.M)["status"] = "Rejected"
		update["$set"].(bson.M)["finance_status"] = "Rejected"
		update["$set"].(bson.M)["remarks"] = req.Remarks

		update["$unset"] = bson.M{
			"hod_status": "",
		}

	case "Hold":
		update["$set"].(bson.M)["status"] = "On Hold"
		update["$set"].(bson.M)["finance_status"] = "Hold"
		update["$set"].(bson.M)["remarks"] = req.Remarks

		update["$unset"] = bson.M{
			"hod_status": "",
		}

	case "Pending":
		update["$set"].(bson.M)["status"] = "Pending Review"
		update["$set"].(bson.M)["finance_status"] = "Pending"

		update["$unset"] = bson.M{
			"hod_status": "",
		}

	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid action")
	}

	_, err = h.db.Invoices.UpdateByID(context.Background(), id, update)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update invoice")
	}

	return c.JSON(fiber.Map{
		"message": "Finance action updated successfully",
	})
}

// PATCH /invoices/:id/finance/accept
//
//	func (h *InvoiceHandler) FinanceAccept(c *fiber.Ctx) error {
//		var req models.FinanceActionRequest
//		if err := c.BodyParser(&req); err != nil {
//			return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
//		}
//		if req.GLCode == "" || req.ExpenseHead == "" {
//			return fiber.NewError(fiber.StatusBadRequest, "glCode and expenseHead are required for acceptance")
//		}
//		inv, err := h.findByID(c.Params("id"))
//		if err != nil {
//			return err
//		}
//		if inv.Status != "Pending Review" {
//			return fiber.NewError(fiber.StatusBadRequest, "Invoice must be in Pending Review status")
//		}
//		old := inv.Status
//		h.updateInvoice(inv.ID, bson.M{"$set": bson.M{
//			"status": "HOD Approval", "finance_status": "Accepted",
//			"gl_code": req.GLCode, "expense_head": req.ExpenseHead,
//			"tax_details": req.TaxDetails, "remarks": req.Remarks, "updated_at": time.Now(),
//		}})
//		h.addAudit(inv.ID, "Finance Accepted", c, old, "HOD Approval", req.Remarks)
//		return c.JSON(fiber.Map{"success": true, "message": "Invoice accepted by Finance, forwarded to HOD"})
//	}
func (h *InvoiceHandler) FinanceAccept(c *fiber.Ctx) error {
	var req models.FinanceActionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if req.GLCode == "" || req.ExpenseHead == "" {
		return fiber.NewError(fiber.StatusBadRequest, "glCode and expenseHead are required for acceptance")
	}

	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	// Allow modification unless fully processed
	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}

	old := inv.Status

	h.updateInvoice(inv.ID, bson.M{
		"$set": bson.M{
			"status":                  "HOD Approval",
			"finance_status":          "Accepted",
			"gl_code":                 req.GLCode,
			"expense_head":            req.ExpenseHead,
			"tax_details":             req.TaxDetails,
			"remarks":                 req.Remarks,
			"updated_at":              time.Now(),
			"hod_status":              "",
			"payment_approval_status": "",
		},
	})

	h.addAudit(inv.ID, "Finance Accepted", c, old, "HOD Approval", req.Remarks)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Invoice accepted by Finance",
	})
}

// PATCH /invoices/:id/finance/reject
func (h *InvoiceHandler) FinanceReject(c *fiber.Ctx) error {
	var req models.FinanceActionRequest
	c.BodyParser(&req)
	if req.Remarks == "" {
		return fiber.NewError(fiber.StatusBadRequest, "remarks are required for rejection")
	}
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	// if inv.Status != "Pending Review" {
	// 	return fiber.NewError(fiber.StatusBadRequest, "Invoice must be in Pending Review status")
	// }
	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}
	if inv.Status == "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Your invoice is already ready for payment")
	}
	if inv.Status == "HOD Approval" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is currently under HOD review")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "Rejected", "finance_status": "Rejected", "hod_status": "",
		"payment_approval_status": "", "remarks": req.Remarks, "updated_at": time.Now()}})
	h.addAudit(inv.ID, "Finance Rejected", c, old, "Rejected", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice rejected by Finance"})
}

// PATCH /invoices/:id/finance/hold
func (h *InvoiceHandler) FinanceHold(c *fiber.Ctx) error {
	var req models.FinanceActionRequest
	c.BodyParser(&req)
	if req.Remarks == "" {
		return fiber.NewError(fiber.StatusBadRequest, "remarks are required for hold")
	}
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	// if inv.Status != "Pending Review" {
	// 	return fiber.NewError(fiber.StatusBadRequest, "Invoice must be in Pending Review status")
	// }
	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}
	if inv.Status == "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Your invoice is already ready for payment")
	}
	if inv.Status == "HOD Approval" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is currently under HOD review")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "On Hold", "finance_status": "Hold", "hod_status": "",
		"payment_approval_status": "", "remarks": req.Remarks, "updated_at": time.Now()}})
	h.addAudit(inv.ID, "Finance Held", c, old, "On Hold", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice placed on hold"})
}

// PATCH /invoices/:id/finance/pending
func (h *InvoiceHandler) FinancePending(c *fiber.Ctx) error {
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}
	if inv.Status == "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Your invoice is already ready for payment")
	}
	if inv.Status == "HOD Approval" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is currently under HOD review")
	}
	old := inv.Status

	h.updateInvoice(inv.ID, bson.M{
		"$set": bson.M{
			"status":         "Pending Review",
			"finance_status": "Pending",
			"updated_at":     time.Now(),
		},
	})

	h.addAudit(inv.ID, "Finance moved to Pending", c, old, "Pending Review", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Invoice moved back to Pending Review",
	})
}

// PATCH /invoices/:id/finance/edit
// Finance/Accounts can correct basic invoice fields before approving/rejecting.
// Allowed fields: invoiceNo, invoiceDate, taxDetails, remarks.
// Every change is recorded in the audit trail.
func (h *InvoiceHandler) FinanceEditInvoice(c *fiber.Ctx) error {
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	// Only allow editing while the invoice is in Finance's hands
	editableStatuses := map[string]bool{
		"Pending Review": true,
		"On Hold":        true,
	}
	if !editableStatuses[inv.Status] {
		return fiber.NewError(fiber.StatusBadRequest,
			"Finance can only edit invoices that are Pending Review or On Hold")
	}

	var req struct {
		InvoiceNo   string  `json:"invoiceNo"`
		InvoiceDate string  `json:"invoiceDate"`
		TaxDetails  string  `json:"taxDetails"` // Now a category, not a rate
		Remarks     string  `json:"remarks"`
		Amount      float64 `json:"amount"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	setFields := bson.M{"updated_at": time.Now()}
	changes := []string{}

	if req.InvoiceNo != "" && req.InvoiceNo != inv.InvoiceNo {
		// Check the new invoiceNo isn't a duplicate for this vendor
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		var dup models.Invoice
		dupErr := h.db.Invoices.FindOne(ctx2, bson.M{
			"invoice_no": req.InvoiceNo,
			"vendor":     inv.Vendor,
			"_id":        bson.M{"$ne": inv.ID},
		}).Decode(&dup)
		if dupErr == nil {
			return fiber.NewError(fiber.StatusConflict,
				fmt.Sprintf("Invoice No '%s' already exists for vendor '%s'", req.InvoiceNo, inv.Vendor))
		}
		setFields["invoice_no"] = req.InvoiceNo
		changes = append(changes, fmt.Sprintf("Invoice No: %s → %s", inv.InvoiceNo, req.InvoiceNo))
	}
	if req.InvoiceDate != "" && req.InvoiceDate != inv.InvoiceDate {
		setFields["invoice_date"] = req.InvoiceDate
		changes = append(changes, fmt.Sprintf("Invoice Date: %s → %s", inv.InvoiceDate, req.InvoiceDate))
	}
	if req.TaxDetails != "" && req.TaxDetails != inv.TaxDetails {
		setFields["tax_details"] = req.TaxDetails
		changes = append(changes, fmt.Sprintf("Tax Type: %s → %s", inv.TaxDetails, req.TaxDetails))
	}
	if req.Remarks != "" {
		setFields["remarks"] = req.Remarks
	}
	if req.Amount > 0 && req.Amount != inv.Amount {
		setFields["amount"] = req.Amount
		changes = append(changes, fmt.Sprintf("Amount: %.2f → %.2f", inv.Amount, req.Amount))
	}

	if len(setFields) == 1 { // only updated_at, nothing else changed
		return c.JSON(fiber.Map{"success": true, "message": "No changes detected"})
	}

	if err := h.updateInvoice(inv.ID, bson.M{"$set": setFields}); err != nil {
		return err
	}

	auditNote := "Finance correction: " + strings.Join(changes, "; ")
	if req.Remarks != "" {
		auditNote += " | Remarks: " + req.Remarks
	}
	h.addAudit(inv.ID, "Finance Edited Invoice", c, inv.Status, inv.Status, auditNote)

	updated, _ := h.findByID(c.Params("id"))
	return c.JSON(fiber.Map{
		"success": true,
		"message": "Invoice details updated by Finance",
		"data":    updated,
		"changes": changes,
	})
}

// ════════════════════════════════════════════════════════════════
//  HOD SECTION
// ════════════════════════════════════════════════════════════════

// GET /invoices/hod
func (h *InvoiceHandler) GetHODInvoices(c *fiber.Ctx) error {
	f := new(models.HODFilter)
	if err := c.QueryParser(f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid query")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := bson.M{}
	if f.HODStatus != "" {
		query["hod_status"] = f.HODStatus
	} else {
		query["status"] = bson.M{"$in": bson.A{"Pending Review", "On Hold", "Rejected", "HOD Approval", "Payment Approval", "Ready for Payment", "Paid"}}
	}
	if f.Department != "" {
		query["department"] = f.Department
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Invoices.CountDocuments(ctx, query)
	opts := options.Find().SetSort(bson.D{{Key: "date_of_receipt", Value: 1}}).
		SetSkip((f.Page - 1) * f.Limit).SetLimit(f.Limit)

	cursor, _ := h.db.Invoices.Find(ctx, query, opts)
	defer cursor.Close(ctx)

	var invoices []models.Invoice
	cursor.All(ctx, &invoices)
	if invoices == nil {
		invoices = []models.Invoice{}
	}

	pendingCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"status": "HOD Approval"})
	approvedCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"hod_status": "Approved"})
	rejectedCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"hod_status": "Rejected"})
	sentBackCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"hod_status": "Sent Back"})

	return c.JSON(fiber.Map{
		"success": true, "data": invoices,
		"summary": fiber.Map{"pending": pendingCount, "approved": approvedCount, "rejected": rejectedCount, "sentBack": sentBackCount},
		"meta":    fiber.Map{"total": total, "page": f.Page, "limit": f.Limit, "pages": (total + f.Limit - 1) / f.Limit},
	})
}

// PATCH /invoices/:id/hod/approve
func (h *InvoiceHandler) HODApprove(c *fiber.Ctx) error {
	var req models.HODActionRequest
	c.BodyParser(&req)
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	// if inv.Status != "HOD Approval" {
	// 	return fiber.NewError(fiber.StatusBadRequest, "Invoice is not awaiting HOD approval")
	// }

	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}
	if inv.Status == "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Your invoice is already ready for payment")
	}

	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "Payment Approval", "hod_status": "Approved", "remarks": req.Remarks, "updated_at": time.Now()}})
	h.addAudit(inv.ID, "HOD Approved", c, old, "Payment Approval", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice approved by HOD"})
}

// PATCH /invoices/:id/hod/reject
func (h *InvoiceHandler) HODReject(c *fiber.Ctx) error {
	var req models.HODActionRequest
	c.BodyParser(&req)
	if req.Remarks == "" {
		return fiber.NewError(fiber.StatusBadRequest, "remarks are required")
	}
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	// if inv.Status != "HOD Approval" {
	// 	return fiber.NewError(fiber.StatusBadRequest, "Invoice is not awaiting HOD approval")
	// }
	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}
	if inv.Status == "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Your invoice is already ready for payment")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "Rejected", "hod_status": "Rejected", "remarks": req.Remarks, "updated_at": time.Now()}})
	h.addAudit(inv.ID, "HOD Rejected", c, old, "Rejected", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice rejected by HOD"})
}

// PATCH /invoices/:id/hod/send-back
func (h *InvoiceHandler) HODSendBack(c *fiber.Ctx) error {
	var req models.HODActionRequest
	c.BodyParser(&req)
	if req.Remarks == "" {
		return fiber.NewError(fiber.StatusBadRequest, "remarks are required")
	}
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	// if inv.Status != "HOD Approval" {
	// 	return fiber.NewError(fiber.StatusBadRequest, "Invoice is not awaiting HOD approval")
	// }
	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}
	if inv.Status == "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Your invoice is already ready for payment")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{
		"status": "Pending Review", "hod_status": "Sent Back",
		"finance_status": nil, "remarks": req.Remarks, "updated_at": time.Now(),
	}})
	h.addAudit(inv.ID, "HOD Sent Back", c, old, "Pending Review", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice sent back to Finance"})
}

// ════════════════════════════════════════════════════════════════
//  PAYMENT APPROVAL SECTION
// ════════════════════════════════════════════════════════════════

// GET /invoices/payment-approval
func (h *InvoiceHandler) GetPaymentApprovalInvoices(c *fiber.Ctx) error {
	f := new(models.PaymentApprovalFilter)
	if err := c.QueryParser(f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid query")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := bson.M{}
	if f.PaymentApprovalStatus != "" {
		query["payment_approval_status"] = f.PaymentApprovalStatus
	} else {
		query["status"] = bson.M{"$in": bson.A{"Pending Review", "On Hold", "Rejected", "HOD Approval", "Payment Approval", "Ready for Payment", "Paid"}}
	}
	if f.Department != "" {
		query["department"] = f.Department
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Invoices.CountDocuments(ctx, query)
	opts := options.Find().SetSort(bson.D{{Key: "due_date", Value: 1}, {Key: "date_of_receipt", Value: 1}}).
		SetSkip((f.Page - 1) * f.Limit).SetLimit(f.Limit)

	cursor, _ := h.db.Invoices.Find(ctx, query, opts)
	defer cursor.Close(ctx)

	var invoices []models.Invoice
	cursor.All(ctx, &invoices)
	if invoices == nil {
		invoices = []models.Invoice{}
	}

	pendingCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"status": "Payment Approval"})
	approvedCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"payment_approval_status": "Approved"})
	heldCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"payment_approval_status": "Hold"})
	now := time.Now()
	overdueCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{
		"status":   "Payment Approval",
		"due_date": bson.M{"$lt": now.Format("2006-01-02"), "$ne": ""},
	})

	return c.JSON(fiber.Map{
		"success": true, "data": invoices,
		"summary": fiber.Map{"pending": pendingCount, "approved": approvedCount, "onHold": heldCount, "overdue": overdueCount},
		"meta":    fiber.Map{"total": total, "page": f.Page, "limit": f.Limit, "pages": (total + f.Limit - 1) / f.Limit},
	})
}

// PATCH /invoices/:id/payment-approval/approve
func (h *InvoiceHandler) PaymentApprovalApprove(c *fiber.Ctx) error {
	var req models.PaymentApprovalRequest
	c.BodyParser(&req)
	if req.PaymentMode == "" {
		return fiber.NewError(fiber.StatusBadRequest, "paymentMode is required")
	}
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	if inv.Status != "Payment Approval" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is not awaiting payment approval")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{
		"status": "Ready for Payment", "payment_approval_status": "Approved",
		"payment_mode": req.PaymentMode, "priority": req.Priority,
		"remarks": req.Remarks, "updated_at": time.Now(),
	}})
	h.addAudit(inv.ID, "Payment Approved", c, old, "Ready for Payment", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice approved for payment"})
}

// PATCH /invoices/:id/payment-approval/reject
func (h *InvoiceHandler) PaymentApprovalReject(c *fiber.Ctx) error {
	var req models.PaymentRejectHoldRequest
	c.BodyParser(&req)
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	if inv.Status != "Payment Approval" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is not awaiting payment approval")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "Rejected", "payment_approval_status": "Rejected", "remarks": req.Remarks, "updated_at": time.Now()}})
	h.addAudit(inv.ID, "Payment Rejected", c, old, "Rejected", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice rejected at payment stage"})
}

// PATCH /invoices/:id/payment-approval/hold
func (h *InvoiceHandler) PaymentApprovalHold(c *fiber.Ctx) error {
	var req models.PaymentRejectHoldRequest
	c.BodyParser(&req)
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	if inv.Status != "Payment Approval" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is not awaiting payment approval")
	}
	old := inv.Status
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "On Hold", "payment_approval_status": "Pending Review", "remarks": req.Remarks, "updated_at": time.Now()}})
	h.addAudit(inv.ID, "Payment Held", c, old, "On Hold", req.Remarks)
	return c.JSON(fiber.Map{"success": true, "message": "Invoice held at payment stage"})
}

// PATCH /invoices/:id/payment-approval/send-back
func (h *InvoiceHandler) PaymentApprovalSendBack(c *fiber.Ctx) error {

	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	// Prevent modification
	if inv.Status == "Paid" || inv.Status == "Closed" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice cannot be modified")
	}

	// if inv.Status == "Ready for Payment" {
	// 	return fiber.NewError(fiber.StatusBadRequest, "Invoice is already ready for payment")
	// }

	oldStatus := inv.Status
	newStatus := "Pending Review" // 👈 change as per your workflow

	// Update invoice
	err = h.updateInvoice(inv.ID, bson.M{
		"$set": bson.M{
			"status":     newStatus,
			"updated_at": time.Now(),
		},
	})
	if err != nil {
		return err
	}
	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{"status": "Payment Approval", "payment_approval_status": "", "updated_at": time.Now()}})

	// Add audit log (assuming signature)
	h.addAudit(inv.ID, "Payment Sent Back", c, oldStatus, newStatus, "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Invoice sent back successfully",
	})
}

// ════════════════════════════════════════════════════════════════
//  PAYMENT PROCESSING SECTION
// ════════════════════════════════════════════════════════════════

// GET /invoices/payment-processing
func (h *InvoiceHandler) GetPaymentProcessingInvoices(c *fiber.Ctx) error {
	f := new(models.PaymentProcessingFilter)
	if err := c.QueryParser(f); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid query")
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}

	query := bson.M{}
	if f.PaymentStatus == "Processed" {
		query["payment_status"] = "Processed"
	} else {
		query["status"] = "Ready for Payment"
	}
	if f.Department != "" {
		query["department"] = f.Department
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := h.db.Invoices.CountDocuments(ctx, query)
	opts := options.Find().SetSort(bson.D{{Key: "due_date", Value: 1}, {Key: "priority", Value: -1}}).
		SetSkip((f.Page - 1) * f.Limit).SetLimit(f.Limit)

	cursor, _ := h.db.Invoices.Find(ctx, query, opts)
	defer cursor.Close(ctx)

	var invoices []models.Invoice
	cursor.All(ctx, &invoices)
	if invoices == nil {
		invoices = []models.Invoice{}
	}

	readyCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"status": "Ready for Payment"})
	processedCount, _ := h.db.Invoices.CountDocuments(ctx, bson.M{"payment_status": "Processed"})

	// total amount ready
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"status": "Ready for Payment"}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}}},
	}
	var amtResult []bson.M
	h.db.Invoices.Aggregate(ctx, pipeline) //nolint
	cur, _ := h.db.Invoices.Aggregate(ctx, pipeline)
	cur.All(ctx, &amtResult)
	pendingAmount := 0.0
	if len(amtResult) > 0 {
		pendingAmount, _ = amtResult[0]["total"].(float64)
	}

	return c.JSON(fiber.Map{
		"success": true, "data": invoices,
		"summary": fiber.Map{"readyToProcess": readyCount, "processed": processedCount, "pendingAmount": pendingAmount},
		"meta":    fiber.Map{"total": total, "page": f.Page, "limit": f.Limit, "pages": (total + f.Limit - 1) / f.Limit},
	})
}

// PATCH /invoices/:id/process-payment
// func (h *InvoiceHandler) ProcessPayment(c *fiber.Ctx) error {
// 	var req models.ProcessPaymentRequest
// 	c.BodyParser(&req)
// 	if req.BankRef == "" {
// 		return fiber.NewError(fiber.StatusBadRequest, "bankRef is required")
// 	}
// 	inv, err := h.findByID(c.Params("id"))
// 	if err != nil {
// 		return err
// 	}
// 	if inv.Status != "Ready for Payment" {
// 		return fiber.NewError(fiber.StatusBadRequest, "Invoice is not in Ready for Payment status")
// 	}
// 	now := time.Now()
// 	old := inv.Status
// 	h.updateInvoice(inv.ID, bson.M{"$set": bson.M{
// 		"status": "Paid", "payment_status": "Processed",
// 		"payment_date": now, "bank_ref": req.BankRef, "updated_at": now,
// 	}})
// 	h.addAudit(inv.ID, "Payment Processed", c, old, "Paid", "Bank Ref: "+req.BankRef)
// 	return c.JSON(fiber.Map{"success": true, "message": "Payment processed successfully", "bankRef": req.BankRef})
// }
// PATCH /invoices/:id/process-payment

// PATCH /invoices/:id/process-payment
func (h *InvoiceHandler) ProcessPayment(c *fiber.Ctx) error {
	var req models.ProcessPaymentRequest
	c.BodyParser(&req)
	if req.BankRef == "" {
		return fiber.NewError(fiber.StatusBadRequest, "bankRef is required")
	}
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}
	if inv.Status != "Ready for Payment" {
		return fiber.NewError(fiber.StatusBadRequest, "Invoice is not in Ready for Payment status")
	}

	now := time.Now()
	old := inv.Status

	if err := h.updateInvoice(inv.ID, bson.M{"$set": bson.M{
		"status":         "Paid",
		"payment_status": "Processed",
		"payment_date":   now,
		"bank_ref":       req.BankRef,
		"updated_at":     now,
	}}); err != nil {
		return err
	}

	h.addAudit(inv.ID, "Payment Processed", c, old, "Paid", "Bank Ref: "+req.BankRef)

	// ── Email notification to vendor (non-blocking goroutine) ─────
	// JWT middleware sets userEmail & userName in c.Locals (see auth.go).
	// e.g. processedByEmail = "rupa@karmamgmt.com"
	//      vendorEmail      = "prajakta@karmamgmt.com"  (stored on invoice)
	// Mail is sent FROM SMTP_FROM, Reply-To = rupa's email.
	processedByName, _ := c.Locals("userName").(string)
	processedByEmail, _ := c.Locals("userEmail").(string)

	if inv.VendorEmail != "" {
		go func() {
			emailErr := SendPaymentProcessedEmail(PaymentEmailData{
				VendorEmail:      inv.VendorEmail,
				VendorName:       inv.Vendor,
				InvoiceNo:        inv.InvoiceNo,
				Amount:           inv.Amount,
				BankRef:          req.BankRef,
				PaymentMode:      inv.PaymentMode,
				PaymentDate:      now.Format("02 Jan 2006"),
				ProcessedByName:  processedByName,
				ProcessedByEmail: processedByEmail,
			})
			if emailErr != nil {
				// Payment is already saved — email failure should not affect the response
				fmt.Printf("[EMAIL ERROR] vendor=%s invoice=%s err=%v\n",
					inv.VendorEmail, inv.InvoiceNo, emailErr)
			}
		}()
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Payment processed successfully",
		"bankRef": req.BankRef,
	})
}

// ════════════════════════════════════════════════════════════════
//  AUDIT TRAIL
// ════════════════════════════════════════════════════════════════

// GET /invoices/:id/audit-trail
func (h *InvoiceHandler) GetAuditTrail(c *fiber.Ctx) error {
	oid, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid invoice ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := h.db.AuditTrail.Find(ctx, bson.M{"invoice_id": oid},
		options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch audit trail")
	}
	defer cursor.Close(ctx)
	var entries []models.AuditEntry
	cursor.All(ctx, &entries)
	if entries == nil {
		entries = []models.AuditEntry{}
	}
	return c.JSON(fiber.Map{"success": true, "data": entries})
}

// ════════════════════════════════════════════════════════════════
//  PRIVATE HELPERS
// ════════════════════════════════════════════════════════════════

func (h *InvoiceHandler) findByID(idStr string) (*models.Invoice, error) {
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Invalid invoice ID format")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var inv models.Invoice
	err = h.db.Invoices.FindOne(ctx, bson.M{"_id": oid}).Decode(&inv)
	if err == mongo.ErrNoDocuments {
		return nil, fiber.NewError(fiber.StatusNotFound, "Invoice not found")
	}
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}
	return &inv, nil
}

func (h *InvoiceHandler) updateInvoice(id primitive.ObjectID, update bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := h.db.Invoices.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update invoice")
	}
	if r.MatchedCount == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Invoice not found")
	}
	return nil
}

// addAudit writes a full activity log entry.
// It fetches a fresh invoice snapshot so invoice_no, vendor, department, amount
// are always recorded — even for actions that change those fields.
func (h *InvoiceHandler) addAudit(invoiceID primitive.ObjectID, action string, c *fiber.Ctx, old, newS, remarks string) {
	userID, _ := c.Locals("userID").(string)
	userName, _ := c.Locals("userName").(string)
	userRole, _ := c.Locals("userRole").(string)
	if userName == "" {
		userName = "System"
	}

	// Fetch invoice snapshot (best-effort; don't block on failure)
	var invoiceNo, vendor, department string
	var amount float64
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var snap models.Invoice
	if err := h.db.Invoices.FindOne(ctx, bson.M{"_id": invoiceID}).Decode(&snap); err == nil {
		invoiceNo = snap.InvoiceNo
		vendor = snap.Vendor
		department = snap.Department
		amount = snap.Amount
	}

	entry := models.AuditEntry{
		ID:          primitive.NewObjectID(),
		InvoiceID:   invoiceID,
		InvoiceNo:   invoiceNo,
		Vendor:      vendor,
		Department:  department,
		Amount:      amount,
		UserID:      userID,
		PerformedBy: userName,
		Role:        userRole,
		Action:      action,
		OldStatus:   old,
		NewStatus:   newS,
		Remarks:     remarks,
		Timestamp:   time.Now(),
	}
	h.db.AuditTrail.InsertOne(ctx, entry) //nolint:errcheck
}

const (
	uploadBasePath = "uploads"
	maxFileSize    = 10 * 1024 * 1024 // 10MB
)

func (h *InvoiceHandler) saveInvoiceFiles(c *fiber.Ctx, invoiceID primitive.ObjectID) ([]string, error) {
	form, err := c.MultipartForm()
	if err != nil || form.File == nil {
		return nil, nil
	}
	files := form.File["documents"]
	if len(files) == 0 {
		return nil, nil
	}
	folderPath := filepath.Join(uploadBasePath, "invoices", invoiceID.Hex())
	if err := os.MkdirAll(folderPath, os.ModePerm); err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "Failed to create upload directory")
	}
	var savedFiles []string
	for _, file := range files {
		if file.Size > maxFileSize {
			return nil, fiber.NewError(fiber.StatusBadRequest, "File size exceeds 10MB limit")
		}
		ext := strings.ToLower(filepath.Ext(file.Filename))
		allowedExt := map[string]bool{
			".pdf": true, ".jpg": true, ".jpeg": true,
			".png": true, ".xlsx": true, ".eml": true, ".msg": true,
		}
		if !allowedExt[ext] {
			return nil, fiber.NewError(fiber.StatusBadRequest, "Unsupported file type")
		}
		safeName := sanitizeFileName(file.Filename)
		fileName := fmt.Sprintf("%d_%s", time.Now().Unix(), safeName)
		fullPath := filepath.Join(folderPath, fileName)
		if err := c.SaveFile(file, fullPath); err != nil {
			return nil, fiber.NewError(fiber.StatusInternalServerError, "Failed to save file")
		}
		savedFiles = append(savedFiles, fileName)
	}
	return savedFiles, nil
}

func (h *InvoiceHandler) DownloadDocument(c *fiber.Ctx) error {

	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	// If using :filename
	fileName := filepath.Base(c.Params("filename"))

	// If using wildcard (*), use this instead:
	// fileName := filepath.Base(c.Params("*"))

	// Validate file belongs to invoice
	allowed := false
	for _, doc := range inv.Documents {
		if doc == fileName {
			allowed = true
			break
		}
	}

	if !allowed {
		return fiber.NewError(fiber.StatusForbidden, "File not associated with invoice")
	}

	filePath := filepath.Join(uploadBasePath, "invoices", inv.ID.Hex(), fileName)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fiber.NewError(fiber.StatusNotFound, "File not found")
	}

	return c.Download(filePath)
}
func sanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = filepath.Base(name) // prevent path traversal
	return name
}

// GET /invoices/:id/documents/zip
func (h *InvoiceHandler) DownloadAllDocumentsZip(c *fiber.Ctx) error {
	inv, err := h.findByID(c.Params("id"))
	if err != nil {
		return err
	}

	if len(inv.Documents) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "No documents found")
	}

	folderPath := filepath.Join(uploadBasePath, "invoices", inv.ID.Hex())

	zipFileName := fmt.Sprintf("invoice_%s_documents.zip", inv.InvoiceNo)

	// Set headers BEFORE writing body
	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFileName))
	c.Set("Content-Transfer-Encoding", "binary")

	// Create zip writer
	zipWriter := zip.NewWriter(c.Response().BodyWriter())
	defer zipWriter.Close()

	for _, fileName := range inv.Documents {

		// Prevent path traversal attack
		safeFileName := filepath.Base(fileName)

		fullPath := filepath.Join(folderPath, safeFileName)

		// Check file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		file, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		// Create file inside zip
		w, err := zipWriter.Create(safeFileName)
		if err != nil {
			file.Close()
			continue
		}

		// Copy file content
		_, err = io.Copy(w, file)
		file.Close()

		if err != nil {
			continue
		}
	}

	// Important: explicitly close to flush zip
	if err := zipWriter.Close(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create zip file")
	}

	return nil
}

//	func sanitizeFileName(name string) string {
//		name = strings.ReplaceAll(name, " ", "_")
//		name = filepath.Base(name)
//		return strings.Map(func(r rune) rune {
//			if r == '_' || r == '-' || r == '.' || r == '+' || r == '(' || r == ')' {
//				return r
//			}
//			if r >= '0' && r <= '9' {
//				return r
//			}
//			if r >= 'A' && r <= 'Z' {
//				return r
//			}
//			if r >= 'a' && r <= 'z' {
//				return r
//			}
//			return -1
//		}, name)
//	}
//
// GET /invoices/test-email
// Sirf development mein use karo — production mein remove kar dena
// func (h *InvoiceHandler) TestEmail(c *fiber.Ctx) error {
// 	err := SendPaymentProcessedEmail(PaymentEmailData{
// 		VendorEmail:      "jharupa100@gmail.com", // ← vendor ka email
// 		VendorName:       "Rupa Test Vendor",
// 		InvoiceNo:        "TEST-001",
// 		Amount:           9999.00,
// 		BankRef:          "HDFC20260603TEST",
// 		PaymentMode:      "NEFT",
// 		PaymentDate:      time.Now().Format("02 Jan 2006"),
// 		ProcessedByName:  "Rupa",
// 		ProcessedByEmail: "rupa@karmamgmt.com", // ← logged-in user
// 	})
// 	if err != nil {
// 		// Error ka poora message return karo
// 		return c.Status(500).JSON(fiber.Map{
// 			"success": false,
// 			"error":   err.Error(), // ← yahan exact error dikhega
// 		})
// 	}
// 	return c.JSON(fiber.Map{
// 		"success": true,
// 		"message": "Email sent successfully!",
// 	})
// }

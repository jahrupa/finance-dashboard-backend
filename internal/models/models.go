package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── Invoice ──────────────────────────────────────────────────
type Invoice struct {
	ID                    primitive.ObjectID `bson:"_id,omitempty"          json:"id"`
	InvoiceNo             string             `bson:"invoice_no"             json:"invoiceNo"`
	Vendor                string             `bson:"vendor"                 json:"vendor"`
	VendorEmail           string             `bson:"vendor_email" json:"vendorEmail"`
	InvoiceDate           string             `bson:"invoice_date"           json:"invoiceDate"`
	DateOfReceipt         time.Time          `bson:"date_of_receipt"        json:"dateOfReceipt"`
	Department            string             `bson:"department"             json:"department"`
	UploadedBy            string             `bson:"uploaded_by"            json:"uploadedBy"`
	Amount                float64            `bson:"amount"                 json:"amount"`
	DueDate               string             `bson:"due_date,omitempty"     json:"dueDate,omitempty"`
	TaxDetails            string             `bson:"tax_details,omitempty"  json:"taxDetails,omitempty"`
	Remarks               string             `bson:"remarks,omitempty"      json:"remarks,omitempty"`
	Documents             []string           `bson:"documents,omitempty"    json:"documents"`
	DocumentURL           string             `bson:"document_url,omitempty" json:"documentUrl,omitempty"`
	Status                string             `bson:"status"                             json:"status"`
	FinanceStatus         *string            `bson:"finance_status,omitempty"           json:"financeStatus,omitempty"`
	HODStatus             *string            `bson:"hod_status,omitempty"               json:"hodStatus,omitempty"`
	PaymentApprovalStatus string             `bson:"payment_approval_status,omitempty"  json:"paymentApprovalStatus,omitempty"`
	PaymentStatus         string             `bson:"payment_status,omitempty"           json:"paymentStatus,omitempty"`
	GLCode                string             `bson:"gl_code,omitempty"      json:"glCode,omitempty"`
	ExpenseHead           string             `bson:"expense_head,omitempty" json:"expenseHead,omitempty"`
	PaymentMode           string             `bson:"payment_mode,omitempty" json:"paymentMode,omitempty"`
	Priority              string             `bson:"priority,omitempty"     json:"priority,omitempty"`
	PaymentDate           *time.Time         `bson:"payment_date,omitempty" json:"paymentDate,omitempty"`
	BankRef               string             `bson:"bank_ref,omitempty"     json:"bankRef,omitempty"`
	CreatedAt             time.Time          `bson:"created_at"             json:"createdAt"`
	UpdatedAt             time.Time          `bson:"updated_at"             json:"updatedAt"`
	IsDeleted             bool               `bson:"is_deleted,omitempty"   json:"isDeleted,omitempty"`
	DeletedAt             *time.Time         `bson:"deleted_at,omitempty"   json:"deletedAt,omitempty"`
	DeletedBy             string             `bson:"deleted_by,omitempty"   json:"deletedBy,omitempty"`
}

type CreateInvoiceRequest struct {
	InvoiceNo   string  `json:"invoiceNo"`
	Vendor      string  `json:"vendor"`
	VendorEmail string  `json:"vendorEmail"`
	InvoiceDate string  `json:"invoiceDate"`
	Department  string  `json:"department"`
	UploadedBy  string  `json:"uploadedBy"`
	Amount      float64 `json:"amount"`
	DueDate     string  `json:"dueDate,omitempty"`
	TaxDetails  string  `json:"taxDetails,omitempty"`
	Remarks     string  `json:"remarks,omitempty"`
	DocumentURL string  `json:"documentUrl,omitempty"`
}

type UpdateInvoiceRequest struct {
	Vendor      string  `json:"vendor,omitempty"`
	VendorEmail string  `json:"vendorEmail,omitempty"`
	InvoiceDate string  `json:"invoiceDate,omitempty"`
	Department  string  `json:"department,omitempty"`
	Amount      float64 `json:"amount,omitempty"`
	DueDate     string  `json:"dueDate,omitempty"`
	TaxDetails  string  `json:"taxDetails,omitempty"`
	Remarks     string  `json:"remarks,omitempty"`
	DocumentURL string  `json:"documentUrl,omitempty"`
}

type InvoiceFilter struct {
	Status     string `query:"status"`
	Department string `query:"department"`
	Search     string `query:"search"`
	FromDate   string `query:"fromDate"`
	ToDate     string `query:"toDate"`
	Page       int64  `query:"page"`
	Limit      int64  `query:"limit"`
}

// ── Finance ───────────────────────────────────────────────────
type FinanceActionRequest struct {
	GLCode      string `json:"glCode"`
	ExpenseHead string `json:"expenseHead"`
	TaxDetails  string `json:"taxDetails,omitempty"`
	Remarks     string `json:"remarks"`
}
type FinanceFilter struct {
	FinanceStatus string `query:"financeStatus"`
	Department    string `query:"department"`
	Search        string `query:"search"`
	Page          int64  `query:"page"`
	Limit         int64  `query:"limit"`
}
type FinanceEditRequest struct {
	InvoiceNo   string  `json:"invoiceNo,omitempty"`
	InvoiceDate string  `json:"invoiceDate,omitempty"`
	Amount      float64 `json:"amount,omitempty"`
	TaxDetails  string  `json:"taxDetails,omitempty"`
	Remarks     string  `json:"remarks,omitempty"`
}

// ── HOD ───────────────────────────────────────────────────────
type HODActionRequest struct {
	Remarks string `json:"remarks"`
}
type HODFilter struct {
	HODStatus  string `query:"hodStatus"`
	Department string `query:"department"`
	Page       int64  `query:"page"`
	Limit      int64  `query:"limit"`
}

// ── Payment ───────────────────────────────────────────────────
type PaymentApprovalRequest struct {
	PaymentMode string `json:"paymentMode"`
	Priority    string `json:"priority"`
	Remarks     string `json:"remarks,omitempty"`
}
type PaymentRejectHoldRequest struct {
	Remarks string `json:"remarks,omitempty"`
}
type ProcessPaymentRequest struct {
	BankRef string `json:"bankRef"`
}
type PaymentApprovalFilter struct {
	PaymentApprovalStatus string `query:"paymentApprovalStatus"`
	Department            string `query:"department"`
	Page                  int64  `query:"page"`
	Limit                 int64  `query:"limit"`
}
type PaymentProcessingFilter struct {
	PaymentStatus string `query:"paymentStatus"`
	Department    string `query:"department"`
	Page          int64  `query:"page"`
	Limit         int64  `query:"limit"`
}

// ── Audit Trail ───────────────────────────────────────────────
type AuditEntry struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"      json:"id"`
	InvoiceID primitive.ObjectID `bson:"invoice_id"         json:"invoiceId"`
	// Denormalised snapshot so activity log needs no join
	InvoiceNo  string  `bson:"invoice_no"         json:"invoiceNo"`
	Vendor     string  `bson:"vendor"             json:"vendor"`
	Department string  `bson:"department"         json:"department"`
	Amount     float64 `bson:"amount"             json:"amount"`
	// Who did it
	UserID      string `bson:"user_id"            json:"userId"`
	PerformedBy string `bson:"performed_by"       json:"performedBy"`
	Role        string `bson:"role"               json:"role"`
	// What happened
	Action    string    `bson:"action"             json:"action"`
	OldStatus string    `bson:"old_status"         json:"oldStatus"`
	NewStatus string    `bson:"new_status"         json:"newStatus"`
	Remarks   string    `bson:"remarks,omitempty"  json:"remarks,omitempty"`
	Timestamp time.Time `bson:"timestamp"          json:"timestamp"`
}

// ActivityLogFilter — used by GET /activity-logs
type ActivityLogFilter struct {
	UserName   string `query:"userName"`
	Action     string `query:"action"`
	Department string `query:"department"`
	FromDate   string `query:"fromDate"`
	ToDate     string `query:"toDate"`
	Search     string `query:"search"` // searches invoice_no or vendor
	Page       int64  `query:"page"`
	Limit      int64  `query:"limit"`
}

// UserActivityStat — per-user summary for activity log
type UserActivityStat struct {
	UserName        string           `json:"userName"`
	Role            string           `json:"role"`
	TotalActions    int64            `json:"totalActions"`
	ActionBreakdown map[string]int64 `json:"actionBreakdown"`
	LastActive      time.Time        `json:"lastActive"`
}

// ── CrudOperation (shared) ───────────────────────────────────
type CrudOperation struct {
	Create bool `bson:"create" json:"create"`
	Read   bool `bson:"read"   json:"read"`
	Update bool `bson:"update" json:"update"`
	Delete bool `bson:"delete" json:"delete"`
}

// ── Vendor ────────────────────────────────────────────────────
type Vendor struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"         json:"id"`
	Name      string             `bson:"name"                  json:"name"`
	Email     string             `bson:"email,omitempty"       json:"email,omitempty"`
	Phone     string             `bson:"phone,omitempty"       json:"phone,omitempty"`
	GSTNo     string             `bson:"gst_no,omitempty"      json:"gstNo,omitempty"`
	Address   string             `bson:"address,omitempty"     json:"address,omitempty"`
	IsActive  bool               `bson:"is_active"             json:"isActive"`
	CreatedAt time.Time          `bson:"created_at"            json:"createdAt"`
	UpdatedAt time.Time          `bson:"updated_at"            json:"updatedAt"`
}
type CreateVendorRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	GSTNo   string `json:"gstNo,omitempty"`
	Address string `json:"address,omitempty"`
}
type UpdateVendorRequest struct {
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	GSTNo   string `json:"gstNo,omitempty"`
	Address string `json:"address,omitempty"`
}

// ═══════════════════════════════════════════════════════════════
//  MERGED USER MODEL
//  Combines: User (auth) + UserAccess (permissions)
//  Single collection: "users"
// ═══════════════════════════════════════════════════════════════

type User struct {
	ID primitive.ObjectID `bson:"_id,omitempty"        json:"id"`

	// ── Identity (from old User) ─────────────────────────────
	Name       string `bson:"name"                 json:"name"`
	Email      string `bson:"email"                json:"email"`
	Password   string `bson:"password"             json:"-"`
	Role       string `bson:"role"                 json:"role"`
	Department string `bson:"department,omitempty" json:"department,omitempty"`
	IsActive   bool   `bson:"is_active"            json:"isActive"`

	// ── Contact / Vendor info (from old Vendor) ──────────────
	Phone   string `bson:"phone,omitempty"      json:"phone,omitempty"`
	GSTNo   string `bson:"gst_no,omitempty"     json:"gstNo,omitempty"`
	Address string `bson:"address,omitempty"    json:"address,omitempty"`

	// ── Access Control (from old UserAccess) ─────────────────
	PageAccess []string                 `bson:"page_access,omitempty" json:"pageAccess,omitempty"`
	CrudAccess map[string]CrudOperation `bson:"crud_access,omitempty" json:"crudAccess,omitempty"`
	Status     string                   `bson:"status,omitempty"      json:"status,omitempty"`

	// ── Timestamps ───────────────────────────────────────────
	CreatedAt time.Time `bson:"created_at" json:"createdAt"`
	UpdatedAt time.Time `bson:"updated_at" json:"updatedAt"`
}

// ── Auth Requests ─────────────────────────────────────────────
type RegisterRequest struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Role       string `json:"role"`
	Department string `json:"department,omitempty"`
}
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// ── User CRUD Requests ────────────────────────────────────────
// CreateUserRequest — used by admin to create a full user record
// (replaces both CreateUserAccessRequest and CreateVendorRequest)
type CreateUserRequest struct {
	// Identity
	Name       string `json:"name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Role       string `json:"role"`
	Department string `json:"department,omitempty"`

	// Contact
	Phone   string `json:"phone,omitempty"`
	GSTNo   string `json:"gstNo,omitempty"`
	Address string `json:"address,omitempty"`

	// Access Control
	PageAccess []string                 `json:"pageAccess,omitempty"`
	CrudAccess map[string]CrudOperation `json:"crudAccess,omitempty"`
	Status     string                   `json:"status,omitempty"`
}

// UpdateUserRequest — partial update (all optional)
type UpdateUserRequest struct {
	Name       string `json:"name,omitempty"`
	Role       string `json:"role,omitempty"`
	Department string `json:"department,omitempty"`

	Phone   string `json:"phone,omitempty"`
	GSTNo   string `json:"gstNo,omitempty"`
	Address string `json:"address,omitempty"`

	PageAccess []string                 `json:"pageAccess,omitempty"`
	CrudAccess map[string]CrudOperation `json:"crudAccess,omitempty"`
	Status     string                   `json:"status,omitempty"`
	IsActive   *bool                    `json:"isActive,omitempty"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type UserFilter struct {
	Role       string `query:"role"`
	Department string `query:"department"`
	Status     string `query:"status"`
	Search     string `query:"search"`
	Page       int64  `query:"page"`
	Limit      int64  `query:"limit"`
}

// ── Dashboard ─────────────────────────────────────────────────
type KPIResponse struct {
	TotalInvoices    int64   `json:"totalInvoices"`
	TotalValue       float64 `json:"totalValue"`
	PendingReview    int64   `json:"pendingReview"`
	OnHold           int64   `json:"onHold"`
	Rejected         int64   `json:"rejected"`
	PaidCount        int64   `json:"paidCount"`
	PaidValue        float64 `json:"paidValue"`
	AwaitingApproval int64   `json:"awaitingApproval"`
	AvgCycleDays     float64 `json:"avgCycleDays"`
	ReadyForPayment  int64   `json:"readyForPayment"`
	HODPending       int64   `json:"hodPending"`
	PaymentPending   int64   `json:"paymentPending"`
}
type AgingBucket struct {
	Bucket string `json:"bucket"`
	Count  int64  `json:"count"`
}
type DeptStat struct {
	Department string `json:"department"`
	Total      int64  `json:"total"`
	Pending    int64  `json:"pending"`
	Paid       int64  `json:"paid"`
	Rejected   int64  `json:"rejected"`
}
type MonthlyTrend struct {
	Month  string  `json:"month"`
	Count  int64   `json:"count"`
	Amount float64 `json:"amount"`
}
type VendorStat struct {
	Vendor string  `json:"vendor"`
	Count  int64   `json:"count"`
	Amount float64 `json:"amount"`
}

package main

import (
	"log"

	"github.com/yourorg/ap-backend/internal/config"
	"github.com/yourorg/ap-backend/internal/database"
	"github.com/yourorg/ap-backend/internal/handlers"
	"github.com/yourorg/ap-backend/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer database.Disconnect()
	log.Printf("✅ Connected to MongoDB: %s", cfg.DBName)

	app := fiber.New(fiber.Config{
		AppName:      "AP Dashboard API v2.0",
		ErrorHandler: middleware.ErrorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
	}))
	log.Println(cfg.AllowedOrigins)
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     cfg.AllowMethods,
		AllowHeaders:     cfg.AllowHeaders,
		ExposeHeaders:    cfg.ExposeHeaders,
		AllowCredentials: cfg.AllowCredentials,
	}))

	// ── Handlers ────────────────────────────────────────────
	invoiceH := handlers.NewInvoiceHandler(db)
	authH := handlers.NewAuthHandler(db, cfg.JWTSecret)
	dashboardH := handlers.NewDashboardHandler(db)
	vendorH := handlers.NewVendorHandler(db)
	userH := handlers.NewUserHandler(db)
	activityH := handlers.NewActivityLogHandler(db)

	// ── Routes ──────────────────────────────────────────────
	api := app.Group("/api/v1")

	// ── Health ──────────────────────────────────────────────
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "message": "AP Dashboard API v2.0 running", "version": "2.0"})
	})

	// ── Auth (public) ────────────────────────────────────────
	auth := api.Group("/auth")
	auth.Post("/login", authH.Login)
	// NOTE: /register is intentionally disabled.
	// Users are created by admins via POST /api/v1/users with full permission assignment.

	// ── TEMPORARY TEST EMAIL — delete before production ──────
	// api.Get("/test-email", invoiceH.TestEmail)

	// ── Protected ───────────────────────────────────────────
	// JWTAuth now extracts ALL claims (userID, role, dept, page_access,
	// crud_access) into c.Locals — this is the fix for Issues 1 & 2.
	var protected fiber.Router
	if cfg.DevJWTBypass {
		protected = api.Group("", middleware.DevBypass())
	} else {
		protected = api.Group("", middleware.JWTAuth(cfg.JWTSecret))
	}

	// ── Dashboard ────────────────────────────────────────────
	// Issue 4: CRUDCheck("KPI Dashboard", "read") — must have read access
	dash := protected.Group("/dashboard")
	dash.Get("/kpis", middleware.CRUDCheck("KPI Dashboard", "read"), dashboardH.GetKPIs)
	dash.Get("/aging", middleware.CRUDCheck("KPI Dashboard", "read"), dashboardH.GetAgingAnalysis)
	dash.Get("/department-stats", middleware.CRUDCheck("KPI Dashboard", "read"), dashboardH.GetDepartmentStats)
	dash.Get("/monthly-trends", middleware.CRUDCheck("KPI Dashboard", "read"), dashboardH.GetMonthlyTrends)
	dash.Get("/top-vendors", middleware.CRUDCheck("KPI Dashboard", "read"), dashboardH.GetTopVendors)
	dash.Get("/summary", middleware.CRUDCheck("KPI Dashboard", "read"), dashboardH.GetSummary)

	// ── Invoices ─────────────────────────────────────────────
	// Issue 4: Each action checked against "Invoice Submission" page permissions.
	// read   → list, view, audit trail, download
	// create → new invoice
	// update → edit, workflow actions (finance/hod/payment)
	// delete → delete
	inv := protected.Group("/invoices")

	inv.Get("/", middleware.CRUDCheck("Invoice Submission", "read"), invoiceH.GetAll)
	inv.Post("/", middleware.CRUDCheck("Invoice Submission", "create"), invoiceH.Create)
	inv.Get("/:id/documents/download-zip", middleware.CRUDCheck("Invoice Submission", "read"), invoiceH.DownloadAllDocumentsZip)
	inv.Get("/:id/documents/:filename", middleware.CRUDCheck("Invoice Submission", "read"), invoiceH.DownloadDocument)
	inv.Get("/:id", middleware.CRUDCheck("Invoice Submission", "read"), invoiceH.GetByID)
	inv.Put("/:id", middleware.CRUDCheck("Invoice Submission", "update"), invoiceH.Update)
	inv.Delete("/:id", middleware.CRUDCheck("Invoice Submission", "delete"), invoiceH.Delete)

	// Section-specific GET
	inv.Get("/section/finance", middleware.CRUDCheck("Finance Review", "read"), invoiceH.GetFinanceInvoices)
	inv.Get("/section/hod", middleware.CRUDCheck("HOD Approval", "read"), invoiceH.GetHODInvoices)
	inv.Get("/section/payment-approval", middleware.CRUDCheck("Payment Approval", "read"), invoiceH.GetPaymentApprovalInvoices)
	inv.Get("/section/payment-processing", middleware.CRUDCheck("Payment Processing", "read"), invoiceH.GetPaymentProcessingInvoices)

	// Finance workflow
	inv.Patch("/:id/finance/accept", middleware.CRUDCheck("Finance Review", "update"), invoiceH.FinanceAccept)
	inv.Patch("/:id/finance/reject", middleware.CRUDCheck("Finance Review", "update"), invoiceH.FinanceReject)
	inv.Patch("/:id/finance/hold", middleware.CRUDCheck("Finance Review", "update"), invoiceH.FinanceHold)
	inv.Patch("/:id/finance/pending", middleware.CRUDCheck("Finance Review", "update"), invoiceH.FinancePending)
	inv.Patch("/:id/finance/edit", middleware.CRUDCheck("Finance Review", "update"), invoiceH.FinanceEditInvoice)

	// HOD workflow
	inv.Patch("/:id/hod/approve", middleware.CRUDCheck("HOD Approval", "update"), invoiceH.HODApprove)
	inv.Patch("/:id/hod/reject", middleware.CRUDCheck("HOD Approval", "update"), invoiceH.HODReject)
	inv.Patch("/:id/hod/send-back", middleware.CRUDCheck("HOD Approval", "update"), invoiceH.HODSendBack)

	// Payment approval workflow
	inv.Patch("/:id/payment-approval/approve", middleware.CRUDCheck("Payment Approval", "update"), invoiceH.PaymentApprovalApprove)
	inv.Patch("/:id/payment-approval/reject", middleware.CRUDCheck("Payment Approval", "update"), invoiceH.PaymentApprovalReject)
	inv.Patch("/:id/payment-approval/hold", middleware.CRUDCheck("Payment Approval", "update"), invoiceH.PaymentApprovalHold)
	inv.Patch("/:id/payment-approval/send-back", middleware.CRUDCheck("Payment Approval", "update"), invoiceH.PaymentApprovalSendBack)

	// Payment processing
	inv.Patch("/:id/process-payment", middleware.CRUDCheck("Payment Processing", "update"), invoiceH.ProcessPayment)

	// Audit trail
	inv.Get("/:id/audit-trail", middleware.CRUDCheck("Invoice Submission", "read"), invoiceH.GetAuditTrail)

	// ── Vendors ──────────────────────────────────────────────
	vend := protected.Group("/vendors")
	vend.Get("/", vendorH.GetAll)
	vend.Post("/", vendorH.Create)
	vend.Get("/:id", vendorH.GetByID)
	vend.Put("/:id", vendorH.Update)
	vend.Delete("/:id", vendorH.Delete)

	// ── Users ────────────────────────────────────────────────
	users := protected.Group("/users")
	users.Get("/", userH.GetAll)
	users.Get("/me", userH.GetMe)
	users.Put("/me/password", userH.ChangePassword)
	users.Get("/:id", userH.GetByID)
	users.Post("/", userH.Create)
	users.Put("/:id", userH.Update)
	users.Delete("/:id", userH.Delete)

	// ── Activity Logs ────────────────────────────────────────
	activity := protected.Group("/activity-logs")
	activity.Get("/", activityH.GetAll)
	activity.Get("/stats", activityH.GetStats)
	activity.Get("/invoice/:id", activityH.GetByInvoice)
	activity.Get("/user/:name", activityH.GetByUser)

	log.Printf("🚀 Server starting on port %s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}

package database

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client

// DB holds references to all collections.
// NOTE: UserAccess and Vendors collections have been REMOVED.
//
//	All user, access-control, and vendor-contact data now lives in "users".
type DB struct {
	Invoices   *mongo.Collection
	Users      *mongo.Collection // merged: identity + access control + contact info
	AuditTrail *mongo.Collection
	Vendors    *mongo.Collection
}

func Connect(uri, dbName string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := c.Ping(ctx, nil); err != nil {
		return nil, err
	}

	client = c
	database := c.Database(dbName)
	db := &DB{
		Invoices:   database.Collection("invoices"),
		Users:      database.Collection("users"),
		AuditTrail: database.Collection("audit_trail"),
		Vendors:    database.Collection("vendors"),
	}
	if err := createIndexes(db); err != nil {
		log.Printf("Warning: could not create indexes: %v", err)
	}
	return db, nil
}

func Disconnect() {
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client.Disconnect(ctx)
	}
}

func createIndexes(db *DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ── Invoice indexes ──────────────────────────────────────
	invIdx := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "invoice_no", Value: 1}, {Key: "vendor", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("unique_invoice_no_vendor"),
		},
		{Keys: bson.D{{Key: "status", Value: 1}}, Options: options.Index().SetName("idx_status")},
		{Keys: bson.D{{Key: "department", Value: 1}}, Options: options.Index().SetName("idx_department")},
		{Keys: bson.D{{Key: "date_of_receipt", Value: -1}}, Options: options.Index().SetName("idx_date_of_receipt")},
		{Keys: bson.D{{Key: "finance_status", Value: 1}}, Options: options.Index().SetName("idx_finance_status")},
		{Keys: bson.D{{Key: "hod_status", Value: 1}}, Options: options.Index().SetName("idx_hod_status")},
		{Keys: bson.D{{Key: "payment_approval_status", Value: 1}}, Options: options.Index().SetName("idx_payment_approval_status")},
		{Keys: bson.D{{Key: "due_date", Value: 1}}, Options: options.Index().SetName("idx_due_date")},
		{Keys: bson.D{{Key: "uploaded_by", Value: 1}}, Options: options.Index().SetName("idx_uploaded_by")},
	}
	if _, err := db.Invoices.Indexes().CreateMany(ctx, invIdx); err != nil {
		return err
	}

	// ── User indexes (merged collection) ─────────────────────
	// email is unique — used for login and lookup
	// role + department support filtered queries
	userIdx := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("unique_email"),
		},
		{Keys: bson.D{{Key: "role", Value: 1}}, Options: options.Index().SetName("idx_role")},
		{Keys: bson.D{{Key: "department", Value: 1}}, Options: options.Index().SetName("idx_user_dept")},
		{Keys: bson.D{{Key: "is_active", Value: 1}}, Options: options.Index().SetName("idx_is_active")},
		{Keys: bson.D{{Key: "gst_no", Value: 1}}, Options: options.Index().SetSparse(true).SetName("idx_gst_no")},
	}
	if _, err := db.Users.Indexes().CreateMany(ctx, userIdx); err != nil {
		return err
	}

	// ── AuditTrail indexes ───────────────────────────────────
	auditIdx := []mongo.IndexModel{
		{Keys: bson.D{{Key: "invoice_id", Value: 1}}, Options: options.Index().SetName("idx_audit_invoice_id")},
		{Keys: bson.D{{Key: "performed_by", Value: 1}}, Options: options.Index().SetName("idx_audit_performed_by")},
		{Keys: bson.D{{Key: "timestamp", Value: -1}}, Options: options.Index().SetName("idx_audit_timestamp")},
	}
	if _, err := db.AuditTrail.Indexes().CreateMany(ctx, auditIdx); err != nil {
		return err
	}

	// ── Vendor indexes ───────────────────────────────────────
	vendorIdx := []mongo.IndexModel{
		{Keys: bson.D{{Key: "vendor_code", Value: 1}}, Options: options.Index().SetUnique(true).SetName("unique_vendor_code")},
		{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetName("idx_vendor_name")},
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true).SetName("unique_vendor_email")},
	}
	if _, err := db.Vendors.Indexes().CreateMany(ctx, vendorIdx); err != nil {
		return err
	}

	log.Println("✅ MongoDB indexes created (merged users collection)")
	return nil
}

package handlers

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
)

// PaymentEmailData holds all the fields needed to build the payment email.
type PaymentEmailData struct {
	// To vendor
	VendorEmail string
	VendorName  string

	// Invoice details
	InvoiceNo   string
	Amount      float64
	BankRef     string
	PaymentMode string
	PaymentDate string // formatted string e.g. "02 Jan 2006"

	// Logged-in user (sender context)
	ProcessedByName  string
	ProcessedByEmail string // used as Reply-To
}

// SendPaymentProcessedEmail sends a payment confirmation email to the vendor.
//
// Works with Microsoft 365 / Outlook (smtp.office365.com:587 — STARTTLS).
// Also works with Gmail (smtp.gmail.com:587) using an App Password.
//
// Required .env variables:
//
//	SMTP_HOST=smtp.office365.com
//	SMTP_PORT=587
//	SMTP_USER=accounts@karmamgmt.com   ← mailbox used to authenticate
//	SMTP_PASS=your-outlook-password
//	SMTP_FROM=accounts@karmamgmt.com   ← "From" address shown to vendor
func SendPaymentProcessedEmail(d PaymentEmailData) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromAddr := os.Getenv("SMTP_FROM")

	// Sensible defaults for Microsoft 365
	if smtpHost == "" {
		smtpHost = "smtp.office365.com"
	}
	if smtpPort == "" {
		smtpPort = "587"
	}

	if smtpUser == "" || smtpPass == "" || fromAddr == "" {
		return fmt.Errorf("SMTP_USER, SMTP_PASS and SMTP_FROM must be set in .env")
	}

	addr := smtpHost + ":" + smtpPort

	subject := fmt.Sprintf("Payment Processed - Invoice %s", d.InvoiceNo)
	body := buildPaymentEmailBody(d)

	// Build raw MIME message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", d.ProcessedByName, fromAddr))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", d.VendorEmail))
	// Reply-To → logged-in user (e.g. rupa@karmamgmt.com)
	// Vendor replies land directly in Rupa's inbox
	if d.ProcessedByEmail != "" {
		msg.WriteString(fmt.Sprintf("Reply-To: %s <%s>\r\n", d.ProcessedByName, d.ProcessedByEmail))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	// ── STARTTLS dial (required by Office 365 on port 587) ────────
	// net/smtp.SendMail() also does STARTTLS but some Office 365 tenants
	// need the TLS config to carry the correct ServerName — so we dial
	// manually to be explicit.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("SMTP dial failed: %w", err)
	}

	client, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		return fmt.Errorf("SMTP client error: %w", err)
	}
	defer client.Close()

	// Upgrade to TLS (STARTTLS)
	tlsConfig := &tls.Config{ServerName: smtpHost}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS failed: %w", err)
	}

	// Authenticate
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed (check SMTP_USER / SMTP_PASS): %w", err)
	}

	// Envelope
	if err := client.Mail(fromAddr); err != nil {
		return fmt.Errorf("SMTP MAIL FROM error: %w", err)
	}
	if err := client.Rcpt(d.VendorEmail); err != nil {
		return fmt.Errorf("SMTP RCPT TO error: %w", err)
	}

	// Body
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA error: %w", err)
	}
	if _, err := fmt.Fprint(wc, msg.String()); err != nil {
		return fmt.Errorf("SMTP write error: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("SMTP close error: %w", err)
	}

	return client.Quit()
}

func buildPaymentEmailBody(d PaymentEmailData) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:Arial,sans-serif;color:#333;max-width:600px;margin:0 auto;padding:24px">

  <h2 style="color:#16a34a">✅ Payment Processed</h2>

  <p>Dear <strong>%s</strong>,</p>

  <p>
    We are pleased to inform you that your payment has been successfully processed.
    Please find the details below.
  </p>

  <table style="width:100%%;border-collapse:collapse;margin:16px 0">
    <tr style="background:#f0fdf4">
      <td style="padding:10px 14px;border:1px solid #bbf7d0;font-weight:600">Invoice No</td>
      <td style="padding:10px 14px;border:1px solid #bbf7d0">%s</td>
    </tr>
    <tr>
      <td style="padding:10px 14px;border:1px solid #e5e7eb;font-weight:600">Amount Paid</td>
      <td style="padding:10px 14px;border:1px solid #e5e7eb;color:#16a34a;font-weight:700">₹%.2f</td>
    </tr>
    <tr style="background:#f9fafb">
      <td style="padding:10px 14px;border:1px solid #e5e7eb;font-weight:600">Bank Reference</td>
      <td style="padding:10px 14px;border:1px solid #e5e7eb;font-family:monospace">%s</td>
    </tr>
    <tr>
      <td style="padding:10px 14px;border:1px solid #e5e7eb;font-weight:600">Payment Mode</td>
      <td style="padding:10px 14px;border:1px solid #e5e7eb">%s</td>
    </tr>
    <tr style="background:#f9fafb">
      <td style="padding:10px 14px;border:1px solid #e5e7eb;font-weight:600">Payment Date</td>
      <td style="padding:10px 14px;border:1px solid #e5e7eb">%s</td>
    </tr>
    <tr>
      <td style="padding:10px 14px;border:1px solid #e5e7eb;font-weight:600">Processed By</td>
      <td style="padding:10px 14px;border:1px solid #e5e7eb">%s</td>
    </tr>
  </table>

  <p style="font-size:13px;color:#6b7280">
    If you have any questions regarding this payment, please reply to this email
    and our accounts team will assist you.
  </p>

  <hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0"/>
  <p style="font-size:12px;color:#9ca3af">
    This is an automated notification from the Accounts Payable system.<br/>
    Please do not reply directly to the sender address — use Reply-To instead.
  </p>

</body>
</html>`,
		d.VendorName,
		d.InvoiceNo,
		d.Amount,
		d.BankRef,
		d.PaymentMode,
		d.PaymentDate,
		d.ProcessedByName,
	)
}

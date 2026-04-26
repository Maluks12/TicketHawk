package services

import (
	"bytes"
	"events-db/internal/config"
	"events-db/internal/models"
	"fmt"
	"html/template"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type EmailService struct {
	client *sendgrid.Client
	cfg    *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{
		client: sendgrid.NewSendClient(cfg.SendGridAPIKey),
		cfg: 	cfg,
	}
}

func (s *EmailService) SendBookingConfirmation(booking *models.Booking, event *models.Event, user *models.User) error {
	subject := fmt.Sprintf("Booking Confirmed: %s", event.Title)

	htmlContent, err := s.renderBookingConfirmation(booking, event, user)
	if err != nil {
		return fmt.Errorf("failed to render email: %w", err)
	}

	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(user.GetFullName(), user.Email)

	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

	response, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	if response.StatusCode >= 400 {
		return fmt.Errorf("email API error: %d", response.StatusCode)
	}
	return nil
}

func (s *EmailService) renderBookingConfirmation(booking *models.Booking, event *models.Event, user *models.User) (string, error) {
	tmpl := `
		<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #4CAF50; color: white; padding: 20px; text-align: center; }
        .content { padding: 20px; background: #f9f9f9; }
        .booking-details { background: white; padding: 15px; margin: 15px 0; border-radius: 5px; }
        .qr-code { text-align: center; margin: 20px 0; }
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #666; }
        .button { display: inline-block; padding: 10px 20px; background: #4CAF50; color: white; text-decoration: none; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎉 Booking Confirmed!</h1>
        </div>
        
        <div class="content">
            <p>Hi {{.UserName}},</p>
            
            <p>Your booking has been confirmed! Here are your details:</p>
            
            <div class="booking-details">
                <h2>{{.EventTitle}}</h2>
                <p><strong>Booking Number:</strong> {{.BookingNumber}}</p>
                <p><strong>Date & Time:</strong> {{.EventDate}}</p>
                <p><strong>Venue:</strong> {{.VenueName}}</p>
                <p><strong>Address:</strong> {{.Address}}</p>
                <p><strong>Tickets:</strong> {{.TicketQuantity}}x @ {{.Currency}}{{.UnitPrice}}</p>
                <p><strong>Total Paid:</strong> {{.Currency}}{{.TotalAmount}}</p>
            </div>
            
            <div class="qr-code">
                <h3>Your Ticket QR Code</h3>
                <img src="{{.QRCodeURL}}" alt="QR Code" style="max-width: 200px; display: block; margin: 0 auto;">
                <p><em>Present this QR code at the event entrance</em></p>
            </div>
            
            <p style="text-align: center;">
                <a href="{{.BookingURL}}" class="button">View Booking Details</a>
            </p>
            
            <h3>Important Information:</h3>
            <ul>
                <li>Please arrive 15 minutes before the event starts</li>
                <li>Bring a valid ID for verification</li>
                <li>This QR code is unique and cannot be duplicated</li>
                <li>Refunds available until {{.RefundDeadline}}</li>
            </ul>
            
            {{if .SpecialRequirements}}
            <p><strong>Special Requirements:</strong> {{.SpecialRequirements}}</p>
            {{end}}
        </div>
        
        <div class="footer">
            <p>Questions? Contact us at support@yourplatform.com</p>
            <p>© 2024 Your Platform. All rights reserved.</p>
        </div>
    </div>
</body>
</html>
	`

	t, err := template.New("confirmation").Parse(tmpl)
	if err != nil {
		return "", err
	}

	data := map[string]interface{}{
        "UserName":            user.GetFullName(),
        "EventTitle":          event.Title,
        "BookingNumber":       booking.BookingNumber,
        "EventDate":           event.StartTime.Format("Monday, January 2, 2006 at 3:04 PM"),
        "VenueName":           event.VenueName,
        "Address":             fmt.Sprintf("%s, %s", event.Address, event.City),
        "TicketQuantity":      booking.TicketQuantity,
        "UnitPrice":           fmt.Sprintf("%.2f", booking.UnitPrice),
        "TotalAmount":         fmt.Sprintf("%.2f", booking.TotalAmount),
        "Currency":            booking.Currency,
        "QRCodeURL":           booking.QRCodeURLOrEmpty(),
        "BookingURL":          fmt.Sprintf("%s/bookings/%s", s.cfg.FrontendURL, booking.ID),
        "RefundDeadline":      event.StartTime.Add(-24 * time.Hour).Format("January 2, 2006"),
        "SpecialRequirements": booking.SpecialRequirements,
    }

	 var buf bytes.Buffer
    if err := t.Execute(&buf, data); err != nil {
        return "", err
    }

    return buf.String(), nil
}

func (s *EmailService) SendRefundConfirmation(booking *models.Booking, event *models.Event, user *models.User) error {
    subject := fmt.Sprintf("Refund Processed: %s", event.Title)

    htmlContent := fmt.Sprintf(`
        <html>
        <body style="font-family: Arial, sans-serif;">
            <h1>Refund Processed</h1>
            <p>Hi %s,</p>
            <p>Your refund for booking <strong>%s</strong> has been processed.</p>
            <p><strong>Amount refunded:</strong> %s%.2f</p>
            <p>The refund will appear in your account within 5-10 business days.</p>
            <hr>
            <p><strong>Event:</strong> %s</p>
            <p><strong>Original booking date:</strong> %s</p>
            <p>We're sorry you couldn't make it. Hope to see you at future events!</p>
        </body>
        </html>
    `, user.GetFullName(), booking.BookingNumber, booking.Currency, 
       *booking.RefundedAmount, event.Title, booking.CreatedAt.Format("January 2, 2006"))

    from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
    to := mail.NewEmail(user.GetFullName(), user.Email)

    message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

    response, err := s.client.Send(message)
    if err != nil {
        return fmt.Errorf("failed to send refund email: %w", err)
    }

    if response.StatusCode >= 400 {
        return fmt.Errorf("email API error: %d", response.StatusCode)
    }

    return nil
}

func (s *EmailService) SendEventCancellation(event *models.Event, user *models.User, booking *models.Booking) error {
    subject := fmt.Sprintf("Event Cancelled: %s", event.Title)

    htmlContent := fmt.Sprintf(`
        <html>
        <body style="font-family: Arial, sans-serif;">
            <h1 style="color: #d32f2f;">Event Cancelled</h1>
            <p>Hi %s,</p>
            <p>We regret to inform you that the following event has been cancelled:</p>
            <h2>%s</h2>
            <p><strong>Date:</strong> %s</p>
            <p><strong>Venue:</strong> %s</p>
            <p><strong>Reason:</strong> %s</p>
            <hr>
            <p>A full refund of <strong>%s%.2f</strong> has been initiated and will be processed within 5-10 business days.</p>
            <p>We apologize for any inconvenience this may cause.</p>
        </body>
        </html>
    `, user.GetFullName(), event.Title, event.StartTime.Format("January 2, 2006"),
       event.VenueName, *event.CancellationReason, booking.Currency, booking.TotalAmount)

    from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
    to := mail.NewEmail(user.GetFullName(), user.Email)

    message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

    resp, err := s.client.Send(message)
    if err != nil {
        return fmt.Errorf("failed to send event cancellation email: %w", err)
    }
    if resp.StatusCode >= 400 {
        return fmt.Errorf("event cancellation email API error: %d", resp.StatusCode)
    }
    return nil
}

func (s *EmailService) SendWaitlistNotification(event *models.Event, user *models.User) error {
    subject := fmt.Sprintf("Tickets Available: %s", event.Title)

    htmlContent := fmt.Sprintf(`
        <html>
        <body style="font-family: Arial, sans-serif;">
            <h1 style="color: #4CAF50;">Good News! Tickets Are Available</h1>
            <p>Hi %s,</p>
            <p>Tickets are now available for an event on your waitlist:</p>
            <h2>%s</h2>
            <p><strong>Date:</strong> %s</p>
            <p><strong>Price:</strong> %s%.2f per ticket</p>
            <p><strong>Act fast!</strong> These tickets are available on a first-come, first-served basis.</p>
            <p style="text-align: center; margin: 30px 0;">
                <a href="%s/events/%s" style="background: #4CAF50; color: white; padding: 12px 30px; text-decoration: none; border-radius: 5px; display: inline-block;">Book Now</a>
            </p>
            <p>This notification expires in 24 hours.</p>
        </body>
        </html>
    `, user.GetFullName(), event.Title, event.StartTime.Format("January 2, 2006"),
       event.Currency, event.Price, s.cfg.FrontendURL, event.ID)

    from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
    to := mail.NewEmail(user.GetFullName(), user.Email)

    message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

    resp, err := s.client.Send(message)
    if err != nil {
        return fmt.Errorf("failed to send waitlist notification email: %w", err)
    }
    if resp.StatusCode >= 400 {
        return fmt.Errorf("waitlist notification email API error: %d", resp.StatusCode)
    }
    return nil
}

func (s *EmailService) SendPasswordReset(email string, resetToken string) error {
    resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.cfg.FrontendURL, resetToken)

    subject := "Password Reset Request"
    htmlContent := fmt.Sprintf(`
        <html>
        <body style="font-family: Arial, sans-serif;">
            <h1>Password Reset Request</h1>
            <p>You requested a password reset. Click the link below to reset your password:</p>
            <p style="text-align: center; margin: 30px 0;">
                <a href="%s" style="background: #2196F3; color: white; padding: 12px 30px; text-decoration: none; border-radius: 5px; display: inline-block;">Reset Password</a>
            </p>
            <p>This link expires in 30 minutes.</p>
            <p>If you didn't request this, please ignore this email.</p>
        </body>
        </html>
    `, resetURL)

    from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
    to := mail.NewEmail("", email)

    message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

    resp, err := s.client.Send(message)
    if err != nil {
        return fmt.Errorf("failed to send password reset email: %w", err)
    }
    if resp.StatusCode >= 400 {
        return fmt.Errorf("password reset email API error: %d", resp.StatusCode)
    }
    return nil
}

// SendOrganizerApproval - Notify user they're approved as organizer
func (s *EmailService) SendOrganizerApproval(user *models.User, organizer *models.Organizer) error {
    subject := "Congratulations! You're Approved as an Organizer"

    htmlContent := fmt.Sprintf(`
        <html>
        <body style="font-family: Arial, sans-serif;">
            <h1 style="color: #4CAF50;">🎉 Congratulations!</h1>
            <p>Hi %s,</p>
            <p>Great news! Your organizer application has been approved.</p>
            <h2>%s</h2>
            <p>You can now create and manage events on our platform.</p>
            <p style="text-align: center; margin: 30px 0;">
                <a href="%s/organizer/dashboard" style="background: #4CAF50; color: white; padding: 12px 30px; text-decoration: none; border-radius: 5px; display: inline-block;">Go to Organizer Dashboard</a>
            </p>
            <p>Next steps:</p>
            <ul>
                <li>Complete your organizer profile</li>
                <li>Set up your Stripe account for payouts</li>
                <li>Create your first event</li>
            </ul>
            <p>Welcome to the community!</p>
        </body>
        </html>
    `, user.GetFullName(), organizer.BusinessName, s.cfg.FrontendURL)

    from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
    to := mail.NewEmail(user.GetFullName(), user.Email)

    message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

    resp, err := s.client.Send(message)
    if err != nil {
        return fmt.Errorf("failed to send organizer approval email: %w", err)
    }
    if resp.StatusCode >= 400 {
        return fmt.Errorf("organizer approval email API error: %d", resp.StatusCode)
    }
    return nil
}

// SendOrganizerRejection - Notify user their application was rejected
func (s *EmailService) SendOrganizerRejection(user *models.User, reason string) error {
    subject := "Organizer Application Update"

    htmlContent := fmt.Sprintf(`
        <html>
        <body style="font-family: Arial, sans-serif;">
            <h1>Organizer Application Update</h1>
            <p>Hi %s,</p>
            <p>Thank you for your interest in becoming an organizer on our platform.</p>
            <p>After careful review, we're unable to approve your application at this time.</p>
            <p><strong>Reason:</strong> %s</p>
            <p>You're welcome to apply again in the future. If you have questions, please contact us at support@yourplatform.com.</p>
        </body>
        </html>
    `, user.GetFullName(), reason)

    from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
    to := mail.NewEmail(user.GetFullName(), user.Email)

    message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

    resp, err := s.client.Send(message)
    if err != nil {
        return fmt.Errorf("failed to send organizer rejection email: %w", err)
    }
    if resp.StatusCode >= 400 {
        return fmt.Errorf("organizer rejection email API error: %d", resp.StatusCode)
    }
    return nil
}

func (s *EmailService) SendTicketAssignmentInvite(
	toEmail string,
	toName string,
	buyerName string,
	eventTitle string,
	claimURL string,
	expiresAt time.Time,
) error {
	subject := fmt.Sprintf("You've been given a ticket to %s", eventTitle)
 
	deadline := expiresAt.Format("Monday, 2 January 2006 at 15:04")
 
	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0; }
    .container { max-width: 600px; margin: 0 auto; padding: 20px; }
    .header { background: #6366f1; color: white; padding: 28px 20px; text-align: center; border-radius: 8px 8px 0 0; }
    .header h1 { margin: 0; font-size: 24px; }
    .content { background: #f9f9f9; padding: 28px; border-radius: 0 0 8px 8px; }
    .event-box { background: white; border-radius: 8px; padding: 16px 20px; margin: 20px 0; border-left: 4px solid #6366f1; }
    .event-box h2 { margin: 0 0 4px; font-size: 18px; color: #0f172a; }
    .event-box p  { margin: 0; color: #64748b; font-size: 14px; }
    .btn-wrap { text-align: center; margin: 28px 0; }
    .btn { display: inline-block; padding: 14px 36px; background: #6366f1; color: white; text-decoration: none; border-radius: 8px; font-size: 16px; font-weight: bold; }
    .deadline { font-size: 13px; color: #ef4444; text-align: center; margin-top: -16px; }
    .footer { text-align: center; padding: 20px; font-size: 12px; color: #94a3b8; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>🎟️ You've got a ticket!</h1>
    </div>
    <div class="content">
      <p>Hi %s,</p>
      <p><strong>%s</strong> has assigned you a ticket to:</p>
      <div class="event-box">
        <h2>%s</h2>
      </div>
      <p>Click the button below to claim your ticket. You'll be asked to confirm your name and email address.</p>
      <div class="btn-wrap">
        <a href="%s" class="btn">Claim My Ticket</a>
      </div>
      <p class="deadline">⏳ This link expires on %s</p>
      <p style="font-size:13px; color:#64748b;">
        If you weren't expecting this, you can safely ignore this email — your email address will not be stored.
      </p>
    </div>
    <div class="footer">If the button doesn't work, copy this link: %s</div>
  </div>
</body>
</html>
`, toName, buyerName, eventTitle, claimURL, deadline, claimURL)
 
	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(toName, toEmail)
	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)
 
	resp, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("send assignment invite: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("assignment invite email API error: %d", resp.StatusCode)
	}
	return nil
}
 
// SendTicketClaimConfirmation emails the claimant their QR code after they
// successfully claim a ticket. This mirrors the existing booking confirmation
// email but is scoped to a single assignment slot.
func (s *EmailService) SendTicketClaimConfirmation(
	toEmail string,
	toName string,
	eventTitle string,
	eventDate string,
	venueName string,
	bookingNumber string,
	slotNumber int,
	qrCodeURL string,
) error {
	subject := fmt.Sprintf("Your ticket for %s is confirmed", eventTitle)
 
	qrBlock := ""
	if qrCodeURL != "" {
		qrBlock = fmt.Sprintf(`
      <div style="text-align:center; margin: 24px 0;">
        <h3 style="color:#0f172a;">Your entry QR code</h3>
        <img src="%s" alt="QR Code" style="width:200px; height:200px; display:block; margin:0 auto; border:1px solid #e2e8f0; border-radius:8px; padding:8px;">
        <p style="font-size:13px; color:#64748b; margin-top:8px;">Show this at the event entrance</p>
      </div>`, qrCodeURL)
	}
 
	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
    .container { max-width: 600px; margin: 0 auto; padding: 20px; }
    .header { background: #059669; color: white; padding: 24px 20px; text-align: center; border-radius: 8px 8px 0 0; }
    .content { background: #f9f9f9; padding: 24px; border-radius: 0 0 8px 8px; }
    .detail-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #f1f5f9; font-size: 14px; }
    .detail-row:last-child { border-bottom: none; }
    .label { color: #64748b; }
    .value { font-weight: bold; color: #0f172a; }
    .footer { text-align: center; padding: 20px; font-size: 12px; color: #94a3b8; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>✅ Ticket Confirmed!</h1>
    </div>
    <div class="content">
      <p>Hi %s, your ticket has been confirmed.</p>
      <div style="background:white; border-radius:8px; padding:16px; margin:16px 0;">
        <h2 style="margin:0 0 16px; color:#0f172a;">%s</h2>
        <div class="detail-row"><span class="label">Date</span><span class="value">%s</span></div>
        <div class="detail-row"><span class="label">Venue</span><span class="value">%s</span></div>
        <div class="detail-row"><span class="label">Booking</span><span class="value">%s</span></div>
        <div class="detail-row"><span class="label">Ticket</span><span class="value">Slot %d</span></div>
      </div>
      %s
      <p style="font-size:13px; color:#64748b;">Please arrive 15 minutes before the event starts.</p>
    </div>
    <div class="footer">Keep this email safe — your QR code is your entry ticket.</div>
  </div>
</body>
</html>
`, toName, eventTitle, eventDate, venueName, bookingNumber, slotNumber, qrBlock)
 
	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(toName, toEmail)
	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)
 
	resp, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("send claim confirmation: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("claim confirmation email API error: %d", resp.StatusCode)
	}
	return nil
}

func (s *EmailService) SendTicketInvalidationNotice(
	toEmail string,
	toName string,
	eventTitle string,
	eventDate string,
	bookingNumber string,
) error {
	subject := fmt.Sprintf("Important: your ticket for %s is no longer valid", eventTitle)
 
	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0; }
    .container { max-width: 600px; margin: 0 auto; padding: 20px; }
    .header { background: #dc2626; color: white; padding: 24px 20px; text-align: center; border-radius: 8px 8px 0 0; }
    .header h1 { margin: 0; font-size: 22px; }
    .content { background: #f9f9f9; padding: 24px; border-radius: 0 0 8px 8px; }
    .event-box {
      background: white;
      border-radius: 8px;
      padding: 16px 20px;
      margin: 20px 0;
      border-left: 4px solid #dc2626;
    }
    .event-box h2 { margin: 0 0 4px; font-size: 17px; color: #0f172a; }
    .event-box p  { margin: 4px 0 0; color: #64748b; font-size: 14px; }
    .notice {
      background: #fef2f2;
      border: 1px solid #fecaca;
      border-radius: 8px;
      padding: 14px 16px;
      font-size: 14px;
      color: #991b1b;
      margin: 16px 0;
    }
    .footer { text-align: center; padding: 20px; font-size: 12px; color: #94a3b8; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>Your ticket has been cancelled</h1>
    </div>
    <div class="content">
      <p>Hi %s,</p>
      <p>We're sorry to let you know that the ticket assigned to you for the following event is no longer valid.</p>
 
      <div class="event-box">
        <h2>%s</h2>
        <p>%s &nbsp;·&nbsp; Booking #%s</p>
      </div>
 
      <div class="notice">
        The person who purchased this booking has requested a refund. As a result, all tickets associated with this booking — including yours — have been cancelled.
      </div>
 
      <p>
        <strong>Your QR code will not be accepted at the event entrance.</strong>
        If you believe this is a mistake, please contact the person who sent you the ticket directly.
      </p>
 
      <p style="font-size: 13px; color: #64748b;">
        If you paid anything to the person who assigned this ticket to you (outside of this platform), you will need to seek reimbursement from them directly. Our platform does not facilitate peer-to-peer transactions.
      </p>
    </div>
    <div class="footer">
      This is an automated message. Please do not reply to this email.
    </div>
  </div>
</body>
</html>
`, toName, eventTitle, eventDate, bookingNumber)
 
	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(toName, toEmail)
	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)
 
	resp, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("send invalidation notice: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("invalidation email API error: %d", resp.StatusCode)
	}
	return nil
}

func (s *EmailService) SendBookingConfirmationWithQR(
	booking *models.Booking,
	event *models.Event,
	user *models.User,
	qrCodeURL string,
) error {
	subject := fmt.Sprintf("Booking Confirmed: %s", event.Title)
 
	htmlContent, err := s.renderBookingConfirmationWithQR(booking, event, user, qrCodeURL)
	if err != nil {
		return fmt.Errorf("failed to render email: %w", err)
	}
 
	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(user.GetFullName(), user.Email)
	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)
 
	response, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("email API error: %d", response.StatusCode)
	}
	return nil
}
 
func (s *EmailService) renderBookingConfirmationWithQR(
	booking *models.Booking,
	event *models.Event,
	user *models.User,
	qrCodeURL string,
) (string, error) {
	qrBlock := ""
	if qrCodeURL != "" {
		qrBlock = fmt.Sprintf(`
            <div class="qr-code">
                <h3>Your Ticket QR Code</h3>
                <img src="%s" alt="QR Code" style="max-width: 200px; display: block; margin: 0 auto;">
                <p><em>Present this QR code at the event entrance</em></p>
            </div>`, qrCodeURL)
	}
 
	manageURL := fmt.Sprintf("%s/bookings/%s/assignments", s.cfg.FrontendURL, booking.ID)
 
	multiTicketNote := ""
	if booking.TicketQuantity > 1 {
		multiTicketNote = fmt.Sprintf(`
            <div style="background:#eff6ff; border-left:4px solid #6366f1; padding:12px 16px; margin:16px 0; border-radius:4px;">
                <strong>You have %d tickets.</strong> You can assign the remaining tickets to
                your group from the <a href="%s" style="color:#6366f1;">manage tickets page</a>.
                Each person will receive their own unique QR code.
            </div>`, booking.TicketQuantity, manageURL)
	}
 
	type templateData struct {
		UserName        string
		EventTitle      string
		BookingNumber   string
		EventDate       string
		VenueName       string
		Address         string
		TicketQuantity  int
		Currency        string
		UnitPrice       string 
        TotalAmount     string
		QRBlock         string
		MultiTicketNote string
		BookingURL      string
	}
 
	data := templateData{
		UserName:        user.GetFullName(),
		EventTitle:      event.Title,
		BookingNumber:   booking.BookingNumber,
		EventDate:       event.StartTime.Format("Monday, 2 January 2006 at 15:04"),
		VenueName:       event.VenueName,
		Address:         event.Address,
		TicketQuantity:  booking.TicketQuantity,
		Currency:        booking.Currency,
		UnitPrice:   fmt.Sprintf("%.2f", booking.UnitPrice),
        TotalAmount: fmt.Sprintf("%.2f", booking.TotalAmount),
		QRBlock:         qrBlock,
		MultiTicketNote: multiTicketNote,
		BookingURL:      fmt.Sprintf("%s/bookings/%s", s.cfg.FrontendURL, booking.ID),
	}
 
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #6366f1; color: white; padding: 20px; text-align: center; border-radius: 8px 8px 0 0; }
        .content { padding: 20px; background: #f9f9f9; border-radius: 0 0 8px 8px; }
        .booking-details { background: white; padding: 15px; margin: 15px 0; border-radius: 5px; }
        .qr-code { text-align: center; margin: 20px 0; }
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #666; }
        .button { display: inline-block; padding: 10px 20px; background: #6366f1; color: white; text-decoration: none; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Booking Confirmed!</h1>
        </div>
        <div class="content">
            <p>Hi {{.UserName}},</p>
            <p>Your booking has been confirmed. Here are your details:</p>
            {{.MultiTicketNote}}
            <div class="booking-details">
                <h2>{{.EventTitle}}</h2>
                <p><strong>Booking Number:</strong> {{.BookingNumber}}</p>
                <p><strong>Date &amp; Time:</strong> {{.EventDate}}</p>
                <p><strong>Venue:</strong> {{.VenueName}}</p>
                <p><strong>Address:</strong> {{.Address}}</p>
                <p><strong>Tickets:</strong> {{.TicketQuantity}}x @ {{.Currency}} {{.UnitPrice}}</p>
                <p><strong>Total Paid:</strong> {{.Currency}} {{.TotalAmount}}</p>
            </div>
            {{.QRBlock}}
            <p style="text-align: center;">
                <a href="{{.BookingURL}}" class="button">View Booking</a>
            </p>
            <h3>Important Information:</h3>
            <ul>
                <li>Please arrive 15 minutes before the event starts</li>
                <li>Your QR code is your entry ticket — keep it ready</li>
                <li>Screenshot or download your QR code in case of poor connectivity</li>
            </ul>
        </div>
        <div class="footer">
            <p>If you have any questions, please contact our support team.</p>
        </div>
    </div>
</body>
</html>`
 
	t, err := template.New("confirmation").Parse(tmpl)
	if err != nil {
		return "", err
	}
 
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
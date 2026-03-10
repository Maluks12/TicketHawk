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

    _, err := s.client.Send(message)
    return err
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

    _, err := s.client.Send(message)
    return err
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

    _, err := s.client.Send(message)
    return err
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

    _, err := s.client.Send(message)
    return err
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

    _, err := s.client.Send(message)
    return err
}
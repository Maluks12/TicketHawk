package services

import (
	"bytes"
	"events-db/internal/config"
	"events-db/internal/models"
	"fmt"
	"html/template"
	"net/url"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// EmailAPIError is returned when the email provider (SendGrid) responds with a
// non-2xx status. A 401/403 means the API key is invalid or the sender isn't
// verified — a permanent misconfiguration that retrying cannot fix.
type EmailAPIError struct{ StatusCode int }

func (e *EmailAPIError) Error() string { return fmt.Sprintf("email API error: %d", e.StatusCode) }

// IsPermanent reports a non-retryable auth/permission failure.
func (e *EmailAPIError) IsPermanent() bool { return e.StatusCode == 401 || e.StatusCode == 403 }

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
       event.Currency, event.Price, s.cfg.FrontendURLs[0], event.ID)

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

// SendPasswordReset emails a password-reset link.
//
// NOTE: resetLink must be the COMPLETE URL (e.g. built by the handler as
// "{frontend}/reset-password?token={rawToken}"). Previously this function
// took a raw token and wrapped it in a second URL, which produced
// ".../reset-password?token=https://.../reset-password?token=XYZ" — the
// frontend then submitted that whole URL as the token and the backend's
// uuid4 validation rejected it, so resets always failed.
func (s *EmailService) SendPasswordReset(email string, resetLink string) error {
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
    `, resetLink)

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
    `, user.GetFullName(), organizer.BusinessName, s.cfg.FrontendURLs[0])

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
 
// SendTicketClaimConfirmation emails the claimant after they successfully
// claim a ticket. Instead of embedding a static QR image (which the gate
// scanner — rotating tokens only — could never verify), it links to the
// claimant's live ticket page where the rotating entry QR is rendered.
func (s *EmailService) SendTicketClaimConfirmation(
	toEmail string,
	toName string,
	eventTitle string,
	eventDate string,
	venueName string,
	bookingNumber string,
	slotNumber int,
	ticketURL string,
) error {
	subject := fmt.Sprintf("Your ticket for %s is confirmed", eventTitle)

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
    .btn-wrap { text-align: center; margin: 28px 0 8px; }
    .btn { display: inline-block; padding: 14px 36px; background: #059669; color: white; text-decoration: none; border-radius: 8px; font-size: 16px; font-weight: bold; }
    .footer { text-align: center; padding: 20px; font-size: 12px; color: #94a3b8; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>&#9989; Ticket Confirmed!</h1>
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
      <div class="btn-wrap">
        <a href="%s" class="btn">View My Ticket</a>
      </div>
      <p style="font-size:13px; color:#64748b; text-align:center;">
        Open this link at the gate — it shows your live entry QR code, which
        refreshes automatically for security. Screenshots and printouts won't scan.
      </p>
      <p style="font-size:13px; color:#64748b;">Please arrive 15 minutes before the event starts.</p>
    </div>
    <div class="footer">If the button doesn't work, copy this link: %s</div>
  </div>
</body>
</html>
`, toName, eventTitle, eventDate, venueName, bookingNumber, slotNumber, ticketURL, ticketURL)

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

// SendBookingConfirmation emails the buyer their booking summary.
//
// NOTE: QR codes are intentionally NOT included in this email. Entry is via
// the rotating QR shown in the TicketHawk app / web booking page, which
// static emailed images can't replicate (and screenshots of static QRs were
// a fraud/scan-failure vector). The email instead deep-links to the booking
// page, including guest credentials (?bn=&email=) when there's no account.
func (s *EmailService) SendBookingConfirmation(
	booking *models.Booking,
	event *models.Event,
	user *models.User,
) error {
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
		return &EmailAPIError{StatusCode: response.StatusCode}
	}
	return nil
}

func (s *EmailService) renderBookingConfirmation(
	booking *models.Booking,
	event *models.Event,
	user *models.User,
) (string, error) {
	// Web route is /booking/:id (singular). Guests (no account) need their
	// booking number + email as query params to authenticate on that page.
	bookingURL := fmt.Sprintf("%s/booking/%s", s.cfg.FrontendURLs[0], booking.ID)
	if booking.UserID == nil {
		bookingURL = fmt.Sprintf("%s/booking/%s?bn=%s&email=%s",
			s.cfg.FrontendURLs[0], booking.ID,
			url.QueryEscape(booking.BookingNumber),
			url.QueryEscape(user.Email))
	}

	manageURL := bookingURL

	multiTicketNote := ""
	if booking.TicketQuantity > 1 {
		multiTicketNote = fmt.Sprintf(`
            <div style="background:#eff6ff; border-left:4px solid #6366f1; padding:12px 16px; margin:16px 0; border-radius:4px;">
                <strong>You have %d tickets.</strong> You can assign the remaining tickets to
                your group from the <a href="%s" style="color:#6366f1;">manage tickets page</a>.
                Each person will receive their own invitation email.
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
		UnitPrice:       fmt.Sprintf("%.2f", booking.UnitPrice),
		TotalAmount:     fmt.Sprintf("%.2f", booking.TotalAmount),
		MultiTicketNote: multiTicketNote,
		BookingURL:      bookingURL,
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
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #666; }
        .button { display: inline-block; padding: 12px 28px; background: #6366f1; color: white; text-decoration: none; border-radius: 5px; font-weight: bold; }
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
            <p style="text-align: center;">
                <a href="{{.BookingURL}}" class="button">View My Tickets</a>
            </p>
            <h3>Important Information:</h3>
            <ul>
                <li>Please arrive 15 minutes before the event starts</li>
                <li>Your entry QR code is shown live on your ticket in the TicketHawk app or on the booking page above &mdash; open it at the gate</li>
                <li>QR codes refresh automatically for security, so screenshots and printouts won't scan</li>
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

// SendOrganizerHandover — sent when an admin hands an event over to an
// organiser (Path 4). Covers both a fresh invite (account was just created;
// they must SET a password) and an existing organiser (link doubles as a
// reset in case they've forgotten theirs). The link reuses the standard
// password-reset flow and is valid for 7 days.
func (s *EmailService) SendOrganizerHandover(email, businessName, eventTitle, resetLink, dashboardURL string, newAccount bool) error {
	subject := fmt.Sprintf("Your event %q is live on TicketHawk", eventTitle)

	intro := fmt.Sprintf(`<p>Good news — your event <strong>%s</strong> has been set up on TicketHawk and ownership has been handed over to you.</p>`, eventTitle)
	action := fmt.Sprintf(`<p>To manage it, sign in to the organiser dashboard. If you've forgotten your password you can reset it with the button below.</p>
			<p style="text-align: center; margin: 30px 0;">
				<a href="%s" style="background: #ff2d55; color: white; padding: 12px 30px; text-decoration: none; border-radius: 8px; display: inline-block; font-weight: bold;">Reset Password</a>
			</p>`, resetLink)
	if newAccount {
		action = fmt.Sprintf(`<p>An organiser account has been created for you with this email address. Set your password to activate it and access your dashboard:</p>
			<p style="text-align: center; margin: 30px 0;">
				<a href="%s" style="background: #ff2d55; color: white; padding: 12px 30px; text-decoration: none; border-radius: 8px; display: inline-block; font-weight: bold;">Set Your Password</a>
			</p>`, resetLink)
	}

	htmlContent := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; color: #1a1a1a;">
			<h1 style="letter-spacing: -0.5px;">Welcome to TicketHawk</h1>
			%s
			%s
			<p>From your dashboard you can view your event, edit its details, track ticket sales, and manage door staff.</p>
			<p style="color: #666; font-size: 13px;">Dashboard: <a href="%s">%s</a><br>
			This link expires in 7 days. If you weren't expecting this email, please ignore it or contact support.</p>
		</body>
		</html>
	`, intro, action, dashboardURL, dashboardURL)

	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(businessName, email)

	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

	resp, err := s.client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send organizer handover email: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("organizer handover email API error: %d", resp.StatusCode)
	}
	return nil
}
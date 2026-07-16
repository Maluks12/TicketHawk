package services

import (
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

// IsPermanent reports whether the failure is a non-retryable auth/config
// problem — 401/403 mean the SendGrid API key is invalid or the sender
// identity isn't verified, which no amount of retrying can fix. The booking
// worker uses this to skip queueing retries for such failures.
func (e *EmailAPIError) IsPermanent() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

type EmailService struct {
	client *sendgrid.Client
	cfg    *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{
		client: sendgrid.NewSendClient(cfg.SendGridAPIKey),
		cfg:    cfg,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Brand layout
//
// Every email shares one look: a black TicketHawk header band (logo +
// wordmark, "HAWK" in the brand pink #FF2D55), a white content card on a
// light-grey page, and a consistent footer. Layout is table-based and all
// styles are inline, because email clients (Outlook, Gmail apps) ignore most
// <style> blocks and modern CSS. Deliberately: NO emojis, NO QR codes — entry
// QR codes are live/rotating and only exist in the app and web ticket pages.
// ═══════════════════════════════════════════════════════════════════════════

const (
	brandPink  = "#FF2D55"
	brandDark  = "#0a0a0a"
	pageBg     = "#f4f4f5"
	cardBg     = "#ffffff"
	textDark   = "#111114"
	textMuted  = "#6b7280"
	borderCol  = "#e5e7eb"
	dangerBg   = "#fef2f2"
	dangerBord = "#fecaca"
	dangerText = "#991b1b"
)

// esc HTML-escapes user-supplied strings (names, event titles, free-text
// reasons) before they are interpolated into email markup.
func esc(s string) string { return template.HTMLEscapeString(s) }

// logoURL points at the brand mark served by the web app, so emails always
// show whatever logo is currently deployed.
func (s *EmailService) logoURL() string {
	return s.cfg.FrontendURLs[0] + "/brand-logo.png"
}

// brandEmail wraps body HTML in the shared TicketHawk chrome.
func (s *EmailService) brandEmail(title string, bodyHTML string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
</head>
<body style="margin:0; padding:0; background:%s; -webkit-text-size-adjust:100%%;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:%s; padding:24px 12px;">
    <tr>
      <td align="center">
        <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px; width:100%%;">

          <!-- Header band -->
          <tr>
            <td style="background:%s; border-radius:14px 14px 0 0; padding:22px 28px;" align="center">
              <img src="%s" alt="TicketHawk" width="44" height="44" style="vertical-align:middle; border:0; margin-right:10px;">
              <span style="font-family:Arial,Helvetica,sans-serif; font-size:22px; font-weight:900; letter-spacing:1px; color:#ffffff; vertical-align:middle;">TICKET<span style="color:%s;">HAWK</span></span>
            </td>
          </tr>

          <!-- Content card -->
          <tr>
            <td style="background:%s; border:1px solid %s; border-top:none; border-radius:0 0 14px 14px; padding:32px 28px; font-family:Arial,Helvetica,sans-serif; font-size:15px; line-height:1.6; color:%s;">
%s
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td align="center" style="padding:20px 28px; font-family:Arial,Helvetica,sans-serif; font-size:12px; line-height:1.6; color:%s;">
              &copy; %d TicketHawk. All rights reserved.<br>
              This is an automated message from TicketHawk.
            </td>
          </tr>

        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		esc(title), pageBg, pageBg,
		brandDark, s.logoURL(), brandPink,
		cardBg, borderCol, textDark,
		bodyHTML,
		textMuted, time.Now().Year(),
	)
}

// heading renders the card's title line.
func heading(text string) string {
	return fmt.Sprintf(`<h1 style="margin:0 0 16px; font-size:22px; line-height:1.3; color:%s; letter-spacing:-0.3px;">%s</h1>`, textDark, esc(text))
}

// button renders the primary call-to-action.
func button(href, label string) string {
	return fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:26px 0 10px;">
  <a href="%s" style="display:inline-block; background:%s; color:#ffffff; text-decoration:none; font-family:Arial,Helvetica,sans-serif; font-size:15px; font-weight:bold; padding:14px 36px; border-radius:10px;">%s</a>
</td></tr></table>`, href, brandPink, esc(label))
}

// linkFallback renders the "if the button doesn't work" line.
func linkFallback(href string) string {
	return fmt.Sprintf(`<p style="margin:14px 0 0; font-size:12px; color:%s; word-break:break-all;">If the button doesn't work, copy this link into your browser:<br><a href="%s" style="color:%s;">%s</a></p>`, textMuted, href, brandPink, href)
}

// detailRow renders a label/value line inside a detail box.
func detailRow(label, value string) string {
	return fmt.Sprintf(`
<tr>
  <td style="padding:8px 0; font-size:13px; color:%s; border-bottom:1px solid #f3f4f6;">%s</td>
  <td align="right" style="padding:8px 0; font-size:13px; font-weight:bold; color:%s; border-bottom:1px solid #f3f4f6;">%s</td>
</tr>`, textMuted, esc(label), textDark, esc(value))
}

// detailBox wraps detailRow lines in a bordered panel, optionally titled.
func detailBox(title string, rowsHTML string) string {
	titleHTML := ""
	if title != "" {
		titleHTML = fmt.Sprintf(`<h2 style="margin:0 0 12px; font-size:17px; color:%s;">%s</h2>`, textDark, esc(title))
	}
	return fmt.Sprintf(`
<div style="background:#fafafa; border:1px solid %s; border-radius:10px; padding:18px 20px; margin:18px 0;">
  %s
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">%s</table>
</div>`, borderCol, titleHTML, rowsHTML)
}

// notice renders a red attention panel (cancellations, invalidations).
func notice(html string) string {
	return fmt.Sprintf(`<div style="background:%s; border:1px solid %s; border-radius:10px; padding:14px 16px; margin:16px 0; font-size:14px; color:%s;">%s</div>`, dangerBg, dangerBord, dangerText, html)
}

// muted renders small secondary copy.
func muted(text string) string {
	return fmt.Sprintf(`<p style="margin:14px 0 0; font-size:13px; color:%s;">%s</p>`, textMuted, esc(text))
}

// sendHTML dispatches the email and returns the provider status code.
func (s *EmailService) sendHTML(toName, toEmail, subject, htmlContent string) (int, error) {
	from := mail.NewEmail(s.cfg.EmailFromName, s.cfg.EmailFromAddress)
	to := mail.NewEmail(toName, toEmail)
	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)

	resp, err := s.client.Send(message)
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Transactional emails
// ═══════════════════════════════════════════════════════════════════════════

func (s *EmailService) SendRefundConfirmation(booking *models.Booking, event *models.Event, user *models.User) error {
	subject := fmt.Sprintf("Refund Processed: %s", event.Title)

	refunded := booking.TotalAmount
	if booking.RefundedAmount != nil {
		refunded = *booking.RefundedAmount
	}

	body := heading("Refund processed") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">Your refund for booking <strong>%s</strong> has been processed.</p>`,
			esc(user.GetFullName()), esc(booking.BookingNumber)) +
		detailBox(event.Title,
			detailRow("Amount refunded", fmt.Sprintf("%s %.2f", booking.Currency, refunded))+
				detailRow("Original booking date", booking.CreatedAt.Format("2 January 2006")),
		) +
		`<p style="margin:0 0 14px;">The refund will reflect in your account within 5&ndash;10 business days.</p>` +
		muted("We're sorry you couldn't make it. We hope to see you at future events.")

	status, err := s.sendHTML(user.GetFullName(), user.Email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send refund email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("email API error: %d", status)
	}
	return nil
}

func (s *EmailService) SendEventCancellation(event *models.Event, user *models.User, booking *models.Booking) error {
	subject := fmt.Sprintf("Event Cancelled: %s", event.Title)

	reason := "Not specified"
	if event.CancellationReason != nil && *event.CancellationReason != "" {
		reason = *event.CancellationReason
	}

	body := heading("Event cancelled") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">We're sorry to let you know that the following event has been cancelled:</p>`,
			esc(user.GetFullName())) +
		detailBox(event.Title,
			detailRow("Date", event.StartTime.Format("Monday, 2 January 2006"))+
				detailRow("Venue", event.VenueName)+
				detailRow("Reason", reason),
		) +
		notice(fmt.Sprintf(`A full refund of <strong>%s %.2f</strong> has been initiated and will be processed within 5&ndash;10 business days.`, esc(booking.Currency), booking.TotalAmount)) +
		muted("We apologise for any inconvenience this may cause.")

	status, err := s.sendHTML(user.GetFullName(), user.Email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send event cancellation email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("event cancellation email API error: %d", status)
	}
	return nil
}

func (s *EmailService) SendWaitlistNotification(event *models.Event, user *models.User) error {
	subject := fmt.Sprintf("Tickets Available: %s", event.Title)

	eventURL := fmt.Sprintf("%s/events/%s", s.cfg.FrontendURLs[0], event.ID)

	body := heading("Tickets are available") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">Tickets have just become available for an event on your waitlist:</p>`,
			esc(user.GetFullName())) +
		detailBox(event.Title,
			detailRow("Date", event.StartTime.Format("Monday, 2 January 2006"))+
				detailRow("Price", fmt.Sprintf("%s %.2f per ticket", event.Currency, event.Price)),
		) +
		`<p style="margin:0;">Tickets are first come, first served &mdash; this notification expires in 24 hours.</p>` +
		button(eventURL, "Book Now") +
		linkFallback(eventURL)

	status, err := s.sendHTML(user.GetFullName(), user.Email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send waitlist notification email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("waitlist notification email API error: %d", status)
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
	subject := "Reset your TicketHawk password"

	body := heading("Password reset") +
		`<p style="margin:0 0 14px;">You requested a password reset for your TicketHawk account. Click the button below to choose a new password.</p>` +
		button(resetLink, "Reset Password") +
		linkFallback(resetLink) +
		muted("This link expires in 30 minutes. If you didn't request this, you can safely ignore this email — your password will not change.")

	status, err := s.sendHTML("", email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("password reset email API error: %d", status)
	}
	return nil
}

// SendOrganizerApproval - Notify user they're approved as organizer
func (s *EmailService) SendOrganizerApproval(user *models.User, organizer *models.Organizer) error {
	subject := "You're approved as a TicketHawk organiser"

	dashURL := s.cfg.FrontendURLs[0] + "/organizer/dashboard"

	body := heading("Congratulations — you're approved") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">Great news: your organiser application for <strong>%s</strong> has been approved. You can now create and manage events on TicketHawk.</p>`,
			esc(user.GetFullName()), esc(organizer.BusinessName)) +
		button(dashURL, "Go to Organiser Dashboard") +
		`<p style="margin:20px 0 6px;"><strong>Next steps:</strong></p>
<p style="margin:0 0 4px;">1. Complete your organiser profile</p>
<p style="margin:0 0 4px;">2. Set up your payout details</p>
<p style="margin:0 0 14px;">3. Create your first event</p>` +
		muted("Welcome to the community.")

	status, err := s.sendHTML(user.GetFullName(), user.Email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send organizer approval email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("organizer approval email API error: %d", status)
	}
	return nil
}

// SendOrganizerRejection - Notify user their application was rejected
func (s *EmailService) SendOrganizerRejection(user *models.User, reason string) error {
	subject := "Your organiser application"

	body := heading("Organiser application update") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">Thank you for your interest in becoming an organiser on TicketHawk. After careful review, we're unable to approve your application at this time.</p>`,
			esc(user.GetFullName())) +
		detailBox("", detailRow("Reason", reason)) +
		muted("You're welcome to apply again in the future. If you have questions, please contact our support team.")

	status, err := s.sendHTML(user.GetFullName(), user.Email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send organizer rejection email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("organizer rejection email API error: %d", status)
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

	body := heading("You've got a ticket") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;"><strong>%s</strong> has assigned you a ticket to:</p>`,
			esc(toName), esc(buyerName)) +
		detailBox(eventTitle, "") +
		`<p style="margin:0;">Click the button below to claim your ticket. You'll be asked to confirm your name and the email address this invitation was sent to.</p>` +
		button(claimURL, "Claim My Ticket") +
		fmt.Sprintf(`<p style="margin:6px 0 0; text-align:center; font-size:13px; color:%s;">This link expires on %s.</p>`, brandPink, esc(deadline)) +
		linkFallback(claimURL) +
		muted("If you weren't expecting this, you can safely ignore this email and the ticket will remain unclaimed.")

	status, err := s.sendHTML(toName, toEmail, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("send assignment invite: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("assignment invite email API error: %d", status)
	}
	return nil
}

// SendTicketClaimConfirmation emails the claimant after they successfully
// claim a ticket. It links to the claimant's live ticket page where the
// rotating entry QR is rendered — no QR image is embedded in the email.
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

	body := heading("Ticket confirmed") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s, your ticket has been confirmed.</p>`, esc(toName)) +
		detailBox(eventTitle,
			detailRow("Date", eventDate)+
				detailRow("Venue", venueName)+
				detailRow("Booking", bookingNumber)+
				detailRow("Ticket", fmt.Sprintf("Slot %d", slotNumber)),
		) +
		button(ticketURL, "View My Ticket") +
		`<p style="margin:6px 0 0; text-align:center; font-size:13px; color:`+textMuted+`;">Open this link at the gate &mdash; it shows your live entry code, which refreshes automatically for security. Screenshots and printouts won't scan.</p>` +
		linkFallback(ticketURL) +
		muted("Please arrive 15 minutes before the event starts.")

	status, err := s.sendHTML(toName, toEmail, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("send claim confirmation: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("claim confirmation email API error: %d", status)
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

	body := heading("Your ticket has been cancelled") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">We're sorry to let you know that the ticket assigned to you for the following event is no longer valid.</p>`,
			esc(toName)) +
		detailBox(eventTitle,
			detailRow("Date", eventDate)+
				detailRow("Booking", "#"+bookingNumber),
		) +
		notice(`The person who purchased this booking has requested a refund. As a result, all tickets associated with this booking &mdash; including yours &mdash; have been cancelled.`) +
		`<p style="margin:0 0 14px;"><strong>This ticket will not be accepted at the event entrance.</strong> If you believe this is a mistake, please contact the person who sent you the ticket directly.</p>` +
		muted("If you paid anything to the person who assigned this ticket to you (outside of TicketHawk), you will need to seek reimbursement from them directly. TicketHawk does not facilitate peer-to-peer transactions.")

	status, err := s.sendHTML(toName, toEmail, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("send invalidation notice: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("invalidation email API error: %d", status)
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

	status, err := s.sendHTML(user.GetFullName(), user.Email, subject, htmlContent)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	if status >= 400 {
		return &EmailAPIError{StatusCode: status}
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

	multiTicketNote := ""
	if booking.TicketQuantity > 1 {
		multiTicketNote = fmt.Sprintf(`
<div style="background:#fff1f4; border-left:4px solid %s; border-radius:6px; padding:12px 16px; margin:0 0 16px; font-size:14px;">
  <strong>You have %d tickets.</strong> You can assign the remaining tickets to your group from the <a href="%s" style="color:%s; font-weight:bold;">manage tickets page</a>. Each person will receive their own invitation email.
</div>`, brandPink, booking.TicketQuantity, bookingURL, brandPink)
	}

	body := heading("Booking confirmed") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Hi %s,</p>
<p style="margin:0 0 14px;">Your booking has been confirmed. Here are your details:</p>`,
			esc(user.GetFullName())) +
		multiTicketNote +
		detailBox(event.Title,
			detailRow("Booking number", booking.BookingNumber)+
				detailRow("Date & time", event.StartTime.Format("Monday, 2 January 2006 at 15:04"))+
				detailRow("Venue", event.VenueName)+
				detailRow("Address", event.Address)+
				detailRow("Tickets", fmt.Sprintf("%d x %s %.2f", booking.TicketQuantity, booking.Currency, booking.UnitPrice))+
				detailRow("Total paid", fmt.Sprintf("%s %.2f", booking.Currency, booking.TotalAmount)),
		) +
		button(bookingURL, "View My Tickets") +
		linkFallback(bookingURL) +
		`<p style="margin:20px 0 6px;"><strong>Important information:</strong></p>
<p style="margin:0 0 4px; font-size:14px;">Please arrive 15 minutes before the event starts.</p>
<p style="margin:0 0 4px; font-size:14px;">Your entry code is shown live on your ticket in the TicketHawk app or on the booking page above &mdash; open it at the gate.</p>
<p style="margin:0 0 14px; font-size:14px;">Entry codes refresh automatically for security, so screenshots and printouts won't scan.</p>` +
		muted("If you have any questions, please contact our support team.")

	return s.brandEmail("Booking Confirmed", body), nil
}

// SendOrganizerHandover notifies a business that an admin-created event has
// been handed over to them (and gives new accounts a password-set link, or
// existing accounts a reset in case they've forgotten theirs). The link
// reuses the standard password-reset flow and is valid for 7 days.
func (s *EmailService) SendOrganizerHandover(email, businessName, eventTitle, resetLink, dashboardURL string, newAccount bool) error {
	subject := fmt.Sprintf("Your event %q is live on TicketHawk", eventTitle)

	action := `<p style="margin:0 0 14px;">To manage it, sign in to the organiser dashboard. If you've forgotten your password you can reset it with the button below.</p>` +
		button(resetLink, "Reset Password")
	if newAccount {
		action = `<p style="margin:0 0 14px;">An organiser account has been created for you with this email address. Set your password to activate it and access your dashboard:</p>` +
			button(resetLink, "Set Your Password")
	}

	body := heading("Welcome to TicketHawk") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">Good news &mdash; your event <strong>%s</strong> has been set up on TicketHawk and ownership has been handed over to you.</p>`, esc(eventTitle)) +
		action +
		linkFallback(resetLink) +
		`<p style="margin:20px 0 14px;">From your dashboard you can view your event, edit its details, track ticket sales, and manage door staff.</p>` +
		fmt.Sprintf(`<p style="margin:0 0 4px; font-size:13px; color:%s;">Dashboard: <a href="%s" style="color:%s;">%s</a></p>`, textMuted, dashboardURL, brandPink, dashboardURL) +
		muted("This link expires in 7 days. If you weren't expecting this email, please ignore it or contact support.")

	status, err := s.sendHTML(businessName, email, subject, s.brandEmail(subject, body))
	if err != nil {
		return fmt.Errorf("failed to send organizer handover email: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("organizer handover email API error: %d", status)
	}
	return nil
}
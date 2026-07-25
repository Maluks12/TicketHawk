package services

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"events-db/internal/config"
	"events-db/internal/models"
	"fmt"
	"html/template"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"net/url"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// brandLogoFS is the TicketHawk mark, embedded directly into the compiled
// binary. Emails previously linked to the logo via a URL built from
// FrontendURLs[0], which meant the image only rendered when that host was
// publicly reachable (it silently broke, e.g., against a local/dev URL).
// Embedding the file and attaching it inline (see sendHTML) removes that
// dependency entirely — the logo always shows, in every environment and
// every mail client, including ones that block remote image loading.
//
//go:embed assets/brand-logo.png
var brandLogoFS embed.FS

// brandLogoCID is the Content-ID the logo is attached under; the header
// image references it via "cid:<brandLogoCID>".
const brandLogoCID = "brand-logo"

// EmailAPIError is returned when the email provider (Amazon SES) rejects a
// message for a reason that retrying cannot fix. sendHTML maps such failures
// to StatusCode 403 (see isPermanentSESError) so the booking worker can skip
// queueing pointless retries. The numeric StatusCode is retained purely for
// backwards compatibility with the worker's IsPermanent() check — SES itself
// does not return HTTP-style status codes to callers.
type EmailAPIError struct{ StatusCode int }

func (e *EmailAPIError) Error() string { return fmt.Sprintf("email API error: %d", e.StatusCode) }

// IsPermanent reports whether the failure is a non-retryable problem — a 403
// here means SES rejected the message for a permanent reason (unverified
// sender identity, suspended/paused account, malformed message), which no
// amount of retrying can fix. The booking worker uses this to skip queueing
// retries for such failures.
func (e *EmailAPIError) IsPermanent() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

type EmailService struct {
	cfg *config.Config

	// The SES client is built lazily on first send and cached. Building it
	// requires a context (LoadDefaultConfig) and can fail, but the existing
	// NewEmailService signature returns no error — so we defer construction
	// to getClient() rather than change every caller. Guarded by clientMu.
	clientMu sync.Mutex
	client   *sesv2.Client
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// getClient lazily builds and caches the SES v2 client. Credentials come from
// explicit SES_* keys when set (config.SESAccessKeyID / SESSecretAccessKey);
// otherwise the AWS default chain is used — on EC2 this is the instance role,
// which is the recommended way to grant SES access in production (no static
// secrets on disk). Region is config.AWSRegion (default af-south-1).
func (s *EmailService) getClient(ctx context.Context) (*sesv2.Client, error) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	if s.client != nil {
		return s.client, nil
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(s.cfg.AWSRegion),
	}
	if s.cfg.SESAccessKeyID != "" && s.cfg.SESSecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				s.cfg.SESAccessKeyID, s.cfg.SESSecretAccessKey, "",
			),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for SES: %w", err)
	}
	s.client = sesv2.NewFromConfig(awsCfg)
	return s.client, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Brand layout
//
// Every email shares one look: the TicketHawk logo + wordmark ("HAWK" in the
// brand pink #FF2D55) sit at the top of a single content card, on a light-grey
// page, with a consistent footer. The header shares the card's surface (no
// separate dark band) so it inverts together with the body in dark-mode mail
// clients (Gmail, Outlook) instead of flipping to a mismatched white strip.
// Layout is table-based and all styles are inline, because email clients
// (Outlook, Gmail apps) ignore most <style> blocks and modern CSS.
// Deliberately: NO emojis, NO QR codes — entry QR codes are live/rotating and
// only exist in the app and web ticket pages.
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

// money formats an amount for display in emails. ZAR (our only currency
// today) is shown with the familiar "R" symbol instead of the ISO code, e.g.
// "R 340.00" rather than "ZAR 340.00". Any other currency code falls back to
// showing the code as-is.
func money(currency string, amount float64) string {
	if currency == "ZAR" {
		return fmt.Sprintf("R %.2f", amount)
	}
	return fmt.Sprintf("%s %.2f", currency, amount)
}

// brandEmail wraps body HTML in the shared TicketHawk chrome.
func (s *EmailService) brandEmail(title string, bodyHTML string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="color-scheme" content="light">
  <meta name="supported-color-schemes" content="light">
  <title>%s</title>
</head>
<body style="margin:0; padding:0; background:%s; -webkit-text-size-adjust:100%%;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" bgcolor="%s" style="background:%s; padding:24px 12px;">
    <tr>
      <td align="center">
        <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px; width:100%%;">

          <!-- Header (shares the card surface — no separate dark band) -->
          <tr>
            <td bgcolor="%s" style="background:%s; border:1px solid #e5e7eb; border-bottom:none; border-radius:14px 14px 0 0; padding:28px 28px 22px;" align="center">
              <img src="cid:%s" alt="TicketHawk" width="88" height="88" style="vertical-align:middle; border:0; margin-right:12px;">
              <span style="font-family:Arial,Helvetica,sans-serif; font-size:26px; font-weight:900; letter-spacing:0.5px; color:#111114; vertical-align:middle; -webkit-text-stroke:1px #111114; text-shadow:0.5px 0 0 currentColor, -0.5px 0 0 currentColor, 0 0.5px 0 currentColor, 0 -0.5px 0 currentColor;">TICKET<span style="color:%s; -webkit-text-stroke:1px %s; text-shadow:0.5px 0 0 currentColor, -0.5px 0 0 currentColor, 0 0.5px 0 currentColor, 0 -0.5px 0 currentColor;">HAWK</span></span>
            </td>
          </tr>

          <!-- Content card -->
          <tr>
            <td bgcolor="%s" style="background:%s; border:1px solid %s; border-top:none; border-radius:0 0 14px 14px; padding:32px 28px; font-family:Arial,Helvetica,sans-serif; font-size:15px; line-height:1.6; color:%s;">
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
		esc(title), pageBg, pageBg, pageBg,
		cardBg, cardBg, brandLogoCID, brandPink, brandPink,
		cardBg, cardBg, borderCol, textDark,
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

// sendHTML dispatches the email through Amazon SES and returns a synthetic
// status code kept for backwards compatibility with callers that check
// `status >= 400` and build EmailAPIError{StatusCode: status}:
//
//	200 → sent successfully
//	403 → permanent SES rejection (unverified sender, suspended/paused
//	      account, malformed message) — see classifySESError. Callers wrap
//	      this as EmailAPIError{403}, whose IsPermanent() is true, so the
//	      booking worker skips a pointless retry.
//	(0, err) → transient failure (throttling, network, quota) — the caller
//	      returns the raw error, which is NOT an EmailAPIError, so the worker
//	      treats it as retryable.
//
// Every email gets the TicketHawk logo attached inline (see brandLogoFS) via a
// multipart/related MIME message referencing it by Content-ID, so the header
// image renders regardless of the recipient's mail client or network
// conditions — it never depends on fetching an external URL.
func (s *EmailService) sendHTML(toName, toEmail, subject, htmlContent string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := s.getClient(ctx)
	if err != nil {
		return 0, err
	}

	raw, err := buildRawMIME(s.cfg.EmailFromName, s.cfg.EmailFromAddress, toName, toEmail, subject, htmlContent)
	if err != nil {
		return 0, fmt.Errorf("failed to build email message: %w", err)
	}

	fromAddr := (&mail.Address{Name: s.cfg.EmailFromName, Address: s.cfg.EmailFromAddress}).String()

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(fromAddr),
		Destination:      &sestypes.Destination{ToAddresses: []string{toEmail}},
		Content: &sestypes.EmailContent{
			Raw: &sestypes.RawMessage{Data: raw},
		},
	}
	// A configuration set enables SES event publishing (bounces, complaints,
	// deliveries) and dedicated-IP / reputation controls. Optional.
	if s.cfg.SESConfigurationSet != "" {
		input.ConfigurationSetName = aws.String(s.cfg.SESConfigurationSet)
	}

	if _, err := client.SendEmail(ctx, input); err != nil {
		if isPermanentSESError(err) {
			return 403, nil
		}
		return 0, err
	}
	return 200, nil
}

// isPermanentSESError reports whether an SES SendEmail failure is one that
// retrying cannot fix. These map to a 403 status so EmailAPIError.IsPermanent()
// returns true. Everything else (throttling, quota, transient network) is left
// as-is so the booking worker retries.
func isPermanentSESError(err error) bool {
	var (
		rejected  *sestypes.MessageRejected
		suspended *sestypes.AccountSuspendedException
		paused    *sestypes.SendingPausedException
		mailFrom  *sestypes.MailFromDomainNotVerifiedException
		badReq    *sestypes.BadRequestException
		notFound  *sestypes.NotFoundException
	)
	return errors.As(err, &rejected) ||
		errors.As(err, &suspended) ||
		errors.As(err, &paused) ||
		errors.As(err, &mailFrom) ||
		errors.As(err, &badReq) ||
		errors.As(err, &notFound)
}

// buildRawMIME assembles a multipart/related message: the HTML body plus the
// TicketHawk logo attached inline and referenced by Content-ID (cid:brand-logo).
// Bodies are base64-encoded with 76-char line wrapping so no MIME line exceeds
// the 998-octet limit (the branded HTML contains some very long lines). The
// subject is RFC 2047-encoded so event titles with any Unicode survive intact.
func buildRawMIME(fromName, fromAddr, toName, toAddr, subject, htmlContent string) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// HTML part.
	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", "base64")
	hp, err := mw.CreatePart(htmlHeader)
	if err != nil {
		return nil, err
	}
	if err := writeWrappedBase64(hp, []byte(htmlContent)); err != nil {
		return nil, err
	}

	// Inline logo part (best-effort: if the embedded asset can't be read we
	// still send the email — the header text renders without the mark).
	if logoBytes, err := brandLogoFS.ReadFile("assets/brand-logo.png"); err == nil {
		imgHeader := textproto.MIMEHeader{}
		imgHeader.Set("Content-Type", "image/png")
		imgHeader.Set("Content-Transfer-Encoding", "base64")
		imgHeader.Set("Content-ID", "<"+brandLogoCID+">")
		imgHeader.Set("Content-Disposition", `inline; filename="brand-logo.png"`)
		ip, err := mw.CreatePart(imgHeader)
		if err != nil {
			return nil, err
		}
		if err := writeWrappedBase64(ip, logoBytes); err != nil {
			return nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}

	to := (&mail.Address{Name: toName, Address: toAddr}).String()
	from := (&mail.Address{Name: fromName, Address: fromAddr}).String()

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/related; boundary=%q\r\n", mw.Boundary())
	msg.WriteString("\r\n")
	msg.Write(body.Bytes())

	return msg.Bytes(), nil
}

// writeWrappedBase64 writes data as base64 with CRLF every 76 characters, as
// required for MIME transfer encoding.
func writeWrappedBase64(w io.Writer, data []byte) error {
	enc := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	for i := 0; i < len(enc); i += lineLen {
		end := i + lineLen
		if end > len(enc) {
			end = len(enc)
		}
		if _, err := io.WriteString(w, enc[i:end]+"\r\n"); err != nil {
			return err
		}
	}
	return nil
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
			detailRow("Amount refunded", money(booking.Currency, refunded))+
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
		notice(fmt.Sprintf(`A full refund of <strong>%s</strong> has been initiated and will be processed within 5&ndash;10 business days.`, esc(money(booking.Currency, booking.TotalAmount)))) +
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
				detailRow("Price", fmt.Sprintf("%s per ticket", money(event.Currency, event.Price))),
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
		`<p style="margin:6px 0 0; text-align:center; font-size:13px; color:` + textMuted + `;">Open this link at the gate &mdash; it shows your live entry code, which refreshes automatically for security. Screenshots and printouts won't scan.</p>` +
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
				detailRow("Tickets", fmt.Sprintf("%d x %s", booking.TicketQuantity, money(booking.Currency, booking.UnitPrice)))+
				detailRow("Total paid", money(booking.Currency, booking.TotalAmount)),
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
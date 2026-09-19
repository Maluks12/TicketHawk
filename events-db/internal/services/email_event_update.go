package services

import (
	"fmt"
	"net/url"
	"strings"

	"events-db/internal/models"
)

// EventUpdateEmail is one ticket holder's copy of an event-change notice.
//
// Everything here comes from the notice row as it was announced (see
// event_update_notices), not from the live event, so the email describes the
// same change the push did.
type EventUpdateEmail struct {
	ToEmail string
	ToName  string

	EventID    string
	EventTitle string
	WhenText   string
	VenueText  string
	Changes    models.EventDetailChanges

	// LinkKind picks the button: "account" and "guest" open the booking,
	// "claimant" opens the ticket that was assigned to them.
	LinkKind      string
	BookingID     string
	BookingNumber string
	AssignmentID  string
}

// SendEventUpdate emails a ticket holder what changed about their event.
//
// Deliberately makes no promise about refunds. Whether a change entitles
// anyone to one is policy, decided elsewhere; this email points at the booking,
// where whatever options apply are shown.
func (s *EmailService) SendEventUpdate(m EventUpdateEmail) error {
	subject := m.Changes.EmailSubject(m.EventTitle)

	greeting := "Hi there,"
	if n := strings.TrimSpace(m.ToName); n != "" {
		greeting = fmt.Sprintf("Hi %s,", n)
	}

	intro := "The organiser has made changes to an event you have tickets for."
	if m.LinkKind == "claimant" {
		intro = "The organiser has made changes to an event you have a ticket for."
	}

	// The two changes that can send someone to the wrong place or the wrong
	// time get a panel above everything else.
	alert := ""
	switch {
	case m.Changes.TimeChanged() && m.Changes.VenueChanged():
		alert = notice(`<strong>The date, time and venue have changed.</strong> Please check the new details before you set off.`)
	case m.Changes.TimeChanged():
		alert = notice(`<strong>The date or time has changed.</strong> Please check the new details below.`)
	case m.Changes.VenueChanged():
		alert = notice(`<strong>The venue has changed.</strong> Please check where to go before you set off.`)
	}

	href, label := s.eventUpdateLink(m)
	cta := ""
	if href != "" {
		cta = button(href, label) + linkFallback(href)
	}

	afterword := `<p style="margin:20px 0 6px; font-size:14px;">Your ticket is still valid for the updated event &mdash; you don't need to do anything to keep it.</p>`
	if m.Changes.TimeChanged() || m.Changes.VenueChanged() {
		if m.LinkKind == "claimant" {
			afterword += muted("If you can no longer make it, let the person who gave you the ticket know.")
		} else {
			afterword += muted("If the change means you can no longer attend, open your booking to see the options available to you.")
		}
	}

	body := heading("Your event has been updated") +
		fmt.Sprintf(`<p style="margin:0 0 14px;">%s</p>
<p style="margin:0 0 14px;">%s</p>`, esc(greeting), esc(intro)) +
		alert +
		eventChangesHTML(m.Changes) +
		detailBox(m.EventTitle,
			detailRow("Date & time", m.WhenText)+
				detailRow("Venue", m.VenueText),
		) +
		cta +
		afterword +
		muted("You're receiving this because you hold a ticket to this event.")

	status, err := s.sendHTML(m.ToName, m.ToEmail, subject, s.brandEmail(subject, body))
	if err != nil {
		// %w keeps *EmailAPIError reachable, so the worker's permanent-failure
		// check (isPermanentEmailErr) still recognises an SES rejection.
		return fmt.Errorf("failed to send event update email: %w", err)
	}
	if status >= 400 {
		return &EmailAPIError{StatusCode: status}
	}
	return nil
}

// eventUpdateLink builds the button for this recipient.
//
// The same routes the existing emails use: /booking/:id, with ?bn=&email= for a
// guest (renderBookingConfirmation), and the claimant's permanent ticket link
// from ClaimTicketHandler, which matches the address case-insensitively.
func (s *EmailService) eventUpdateLink(m EventUpdateEmail) (href, label string) {
	if len(s.cfg.FrontendURLs) == 0 {
		return "", ""
	}
	base := strings.TrimRight(strings.TrimSpace(s.cfg.FrontendURLs[0]), "/")
	if base == "" {
		return "", ""
	}

	switch m.LinkKind {
	case "guest":
		return fmt.Sprintf("%s/booking/%s?bn=%s&email=%s",
			base, m.BookingID, url.QueryEscape(m.BookingNumber), url.QueryEscape(m.ToEmail)), "View my booking"
	case "claimant":
		if m.AssignmentID != "" {
			return fmt.Sprintf("%s/claim-ticket?assignment=%s&email=%s",
				base, m.AssignmentID, url.QueryEscape(m.ToEmail)), "View my ticket"
		}
		// The assignment was removed after the email was queued; the event
		// page still shows the new details.
		return fmt.Sprintf("%s/events/%s", base, m.EventID), "View event"
	default:
		return fmt.Sprintf("%s/booking/%s", base, m.BookingID), "View my booking"
	}
}

// eventChangesHTML renders the "What changed" panel.
//
// Stacked rather than label | value like detailRow: Information values are
// free text up to 600 characters, and a right-aligned bold paragraph is hard
// to read.
func eventChangesHTML(changes models.EventDetailChanges) string {
	if len(changes) == 0 {
		return ""
	}

	var rows strings.Builder
	for _, c := range changes {
		label := esc(c.Label)
		now, before := "", ""

		switch {
		case c.Field == models.DetailFieldDescription:
			now = "Updated &mdash; see the event page for the full description."
		case c.Kind == models.DetailRemoved:
			now = "No longer listed"
			before = c.Old
		case c.Kind == models.DetailAdded:
			label += " (new)"
			now = multiline(c.New)
		default:
			now = multiline(c.New)
			before = c.Old
		}

		beforeHTML := ""
		if strings.TrimSpace(before) != "" {
			beforeHTML = fmt.Sprintf(`
  <div style="margin-top:3px; font-size:12px; color:%s;">Previously: %s</div>`, textMuted, multiline(before))
		}

		fmt.Fprintf(&rows, `
<tr>
  <td style="padding:10px 0; border-bottom:1px solid #f3f4f6;">
  <div style="font-size:12px; color:%s;">%s</div>
  <div style="margin-top:2px; font-size:14px; font-weight:bold; color:%s;">%s</div>%s
  </td>
</tr>`, textMuted, label, textDark, now, beforeHTML)
	}

	return fmt.Sprintf(`
<div style="background:#fafafa; border:1px solid %s; border-radius:10px; padding:18px 20px; margin:18px 0;">
  <h2 style="margin:0 0 6px; font-size:17px; color:%s;">What changed</h2>
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">%s</table>
</div>`, borderCol, textDark, rows.String())
}

// multiline escapes free text and keeps the organiser's line breaks — a
// parking note written as three lines should not arrive as one.
func multiline(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	return strings.ReplaceAll(esc(s), "\n", "<br>")
}
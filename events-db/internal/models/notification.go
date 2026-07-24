package models

import (
	"encoding/json"
	"time"
)

type NotificationType string

const (
	// ── Booking / payment ─────────────────────────────────────────────────────
	NotifBookingConfirmed NotificationType = "booking_confirmed"
	NotifEventCancelled   NotificationType = "event_cancelled"
	NotifRefundProcessed  NotificationType = "refund_processed"

	// ── Event reminders (follow system) ───────────────────────────────────────
	// Both the 7-day and 1-day reminders share this type; they are distinguished
	// by the "days_until" integer in the notification Data payload.
	NotifEventReminder NotificationType = "event_reminder"

	// ── Ticket availability alerts (follow system) ────────────────────────────
	// Sent to all followers when an event crosses the 80 % sold threshold.
	NotifTicketsSellingFast NotificationType = "tickets_selling_fast"
	// Sent to all followers when an event becomes fully sold out.
	NotifEventSoldOut NotificationType = "event_sold_out"

	// ── Organiser KYC ─────────────────────────────────────────────────────────
	// Emitted exactly once, the moment full (tier-2) verification passes —
	// whichever path got them there: an inline VerifyNow full match, an admin
	// approval, or an admin registry re-run. The organiser dashboard renders
	// this as a banner on the Overview screen so approval is visible without
	// the organiser having to go looking for it.
	NotifKYCApproved NotificationType = "kyc_approved"
)

type Notification struct {
	ID        string           `json:"id" db:"id"`
	UserID    string           `json:"user_id" db:"user_id"`
	Type      NotificationType `json:"type" db:"type"`
	Title     string           `json:"title" db:"title"`
	Body      string           `json:"body" db:"body"`
	Data      json.RawMessage  `json:"data,omitempty" db:"data"`
	IsRead    bool             `json:"is_read" db:"is_read"`
	CreatedAt time.Time        `json:"created_at" db:"created_at"`
}
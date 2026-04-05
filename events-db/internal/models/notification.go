package models

import (
	"encoding/json"
	"time"
)

type NotificationType string

const (
	NotifBookingConfirmed NotificationType = "booking_confirmed"
	NotifEventCancelled   NotificationType = "event_cancelled"
	NotifRefundProcessed  NotificationType = "refund_processed"
	NotifEventReminder    NotificationType = "event_reminder"
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
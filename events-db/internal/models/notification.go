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
	ID        string           `json:"id"`
	UserID    string           `json:"user_id"`
	Type      NotificationType `json:"type"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	Data      json.RawMessage  `json:"data,omitempty"`
	IsRead    bool             `json:"is_read"`
	CreatedAt time.Time        `json:"created_at"`
}
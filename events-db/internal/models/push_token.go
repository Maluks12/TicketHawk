package models

import "time"

// PushPlatform is the OS the registered device runs. Web push is deliberately
// absent — the web surfaces (web-desktop, web-mobile, events-organizer) use the
// in-app notification feed, not a service worker.
type PushPlatform string

const (
	PushPlatformIOS     PushPlatform = "ios"
	PushPlatformAndroid PushPlatform = "android"
)

// PushApp distinguishes the two Expo apps that can register a token. They are
// separate installs with separate tokens; an organiser may run both.
type PushApp string

const (
	PushAppConsumer PushApp = "consumer" // com.tickethawk.app
	PushAppStudio   PushApp = "studio"   // com.tickethawk.studio
)

// PushToken is one registered device.
type PushToken struct {
	ID           string       `json:"id"            db:"id"`
	UserID       string       `json:"user_id"       db:"user_id"`
	Token        string       `json:"token"         db:"token"`
	Platform     PushPlatform `json:"platform"      db:"platform"`
	App          PushApp      `json:"app"           db:"app"`
	DeviceName   *string      `json:"device_name"   db:"device_name"`
	FailureCount int          `json:"failure_count" db:"failure_count"`
	LastUsedAt   *time.Time   `json:"last_used_at"  db:"last_used_at"`
	CreatedAt    time.Time    `json:"created_at"    db:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"    db:"updated_at"`
}

// UserPushTarget is the flattened row the push service works with: it only ever
// needs the token itself plus the user it belongs to (for badge counts and for
// logging which account a failed send belonged to).
type UserPushTarget struct {
	UserID   string
	Token    string
	Platform PushPlatform
}

// PushChannel maps a notification type onto an Android notification channel.
//
// Android 8+ requires every notification to declare a channel, and the channel
// — not the app — owns importance, sound and vibration. Putting reminders and
// booking confirmations on separate channels means a user who finds "selling
// fast" alerts noisy can mute just those in system settings instead of turning
// the whole app off. The channel IDs here MUST match the ones created in the
// app's lib/push.ts ensureAndroidChannels().
//
// iOS ignores channelId entirely; it is simply passed through and dropped.
func PushChannel(t NotificationType) string {
	switch t {
	case NotifBookingConfirmed, NotifRefundProcessed, NotifKYCApproved:
		// Transactional: the user paid money or was waiting on an outcome.
		// These should always arrive with sound.
		return "tickets"
	case NotifEventReminder:
		return "reminders"
	case NotifTicketsSellingFast, NotifEventSoldOut, NotifEventCancelled, NotifEventDetailsChanged:
		// A changed date, time or venue belongs with a cancellation: something
		// happened to an event the user holds a ticket for.
		return "alerts"
	default:
		return "default"
	}
}

// PushPriority returns the Expo delivery priority for a notification type.
//
// "high" wakes the device immediately (FCM high priority / APNs priority 10);
// "normal" may be batched by the OS to save battery. Marketing-ish alerts get
// "normal" so they don't punch through Doze mode at 3am for a discount.
func PushPriority(t NotificationType) string {
	switch t {
	case NotifBookingConfirmed, NotifRefundProcessed, NotifEventCancelled, NotifKYCApproved,
		NotifEventDetailsChanged:
		// An event change can be time-critical ("doors now 17:00" on the day),
		// so it must not wait out Doze batching.
		return "high"
	default:
		return "normal"
	}
}
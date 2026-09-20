package repository

import (
	"context"
	"time"

	"events-db/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// HydrateBookingEvent fills in everything a ticket holder needs to know about
// the event on their booking screen, on top of the handful of columns
// GetBookingByID and GetGuestBooking load (title, start, venue, address,
// cover): the end time, the description, the organiser, the Information block
// (doors, parking, age requirement, dress code, ...) and the line-up.
//
// The Information block and line-up came after the booking screens were
// built, so they never reached them: a buyer could read "doors 17:00, park at
// the north gate" on the event page but not on the ticket they carry to the
// door. HydrateEventDetail is what the public event page uses, so both screens
// now describe the event the same way.
//
// Only the booking-detail endpoints call this. GetBookingByID itself stays
// lean — workers, emails and refund paths call it constantly and need none of
// it.
//
// Best effort: if anything here fails the booking is returned exactly as it
// was, so the ticket still loads with fewer event details rather than not at
// all.
//
// No status filter, unlike the public event endpoints: someone holding a
// ticket to an event that has since been taken down, made private or finished
// can still read what they were sold.
func HydrateBookingEvent(pool *pgxpool.Pool, booking *models.Booking) {
	if booking == nil || booking.EventID == nil || *booking.EventID == "" {
		return // merch-only bookings have no event
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		id, organizerID, slug, description string
		endTime                            time.Time
	)
	if err := pool.QueryRow(ctx, `
		SELECT id::text,
		       COALESCE(organizer_id::text, ''),
		       COALESCE(slug, ''),
		       COALESCE(description, ''),
		       end_time
		FROM   events
		WHERE  id = $1
	`, *booking.EventID).Scan(&id, &organizerID, &slug, &description, &endTime); err != nil {
		log.Warn().Err(err).Str("booking_id", booking.ID).
			Msg("booking detail: could not load event details — returning the booking without them")
		return
	}

	if booking.Event == nil {
		booking.Event = &models.Event{}
	}
	ev := booking.Event
	ev.ID = id
	ev.OrganizerID = organizerID
	ev.Slug = slug
	ev.Description = description
	ev.EndTime = endTime

	// Organiser (public profile only), Information block and line-up.
	HydrateEventDetail(pool, ev)
}
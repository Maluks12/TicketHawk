package handlers

import (
	"context"
	"net/http"
	"time"

	"events-db/internal/cache"
	"events-db/internal/middleware"
	"events-db/internal/repository"
	"events-db/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─────────────────────────────────────────────────────────────────────────────
// Event review — full detail, and approval with an optional featuring window
//
// WHY A SEPARATE APPROVE ENDPOINT
//
// The existing AdminApproveEventHandler publishes and nothing else, and the
// console paired it with a second button, "Approve & Add to Home", which set
// is_featured with no end date. Featuring with no end is how a stale event sits
// on the homepage for months after it happened — nobody remembers to unfeature
// anything.
//
// This one takes the featuring decision as part of the approval, with a window
// that has to be bounded. The old handler is left in place; this is additive.
// ─────────────────────────────────────────────────────────────────────────────

// GET /admin/events/:id/review
//
// Everything needed to judge an event, in one call: the event itself, its
// ticket tiers, and who is asking.
//
// The old approvals screen showed a summary — title, venue, date, price — and
// approving on that basis means nobody ever looks at the tiers, which is where
// the mistakes actually are (a "VIP" tier priced at R0, a tier with unlimited
// capacity on a 200-capacity venue).
func GetEventReviewHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventID := c.Param("id")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		var ev struct {
			ID            string     `json:"id"`
			Title         string     `json:"title"`
			Description   *string    `json:"description"`
			Status        string     `json:"status"`
			StartTime     time.Time  `json:"start_time"`
			EndTime       time.Time  `json:"end_time"`
			ExpiresAt     *time.Time `json:"expires_at"`
			Timezone      *string    `json:"timezone"`
			VenueName     *string    `json:"venue_name"`
			Address       *string    `json:"address"`
			City          *string    `json:"city"`
			Price         float64    `json:"price"`
			Currency      string     `json:"currency"`
			MaxCapacity   *int       `json:"max_capacity"`
			MaxPerUser    *int       `json:"max_tickets_per_user"`
			CoverImage    *string    `json:"cover_image_url"`
			BannerImage   *string    `json:"banner_image_url"`
			CategoryID    *int       `json:"category_id"`
			CategoryName  *string    `json:"category_name"`
			OrganiserID   string     `json:"organiser_id"`
			OrganiserName *string    `json:"organiser_name"`
			OrganiserMail *string    `json:"organiser_email"`
			OrganiserKYC  *int       `json:"organiser_kyc_level"`
			IsFeatured    bool       `json:"is_featured"`
			FeaturedFrom  *time.Time `json:"featured_from"`
			FeaturedUntil *time.Time `json:"featured_until"`
			RefundPolicy  *string    `json:"refund_policy"`
			Terms         *string    `json:"terms_and_conditions"`
			CreatedAt     time.Time  `json:"created_at"`
			TicketsSold   int64      `json:"tickets_sold"`
		}

		err := pool.QueryRow(ctx, `
			SELECT e.id::text, e.title, e.description, e.status::text,
			       e.start_time, e.end_time, e.expires_at, e.timezone,
			       e.venue_name, e.address, e.city,
			       e.price, e.currency, e.max_capacity, e.max_tickets_per_user,
			       e.cover_image_url, e.banner_image_url,
			       e.category_id, cat.name,
			       e.organizer_id::text, o.business_name, u.email, o.kyc_level,
			       COALESCE(e.is_featured, FALSE), e.featured_from, e.featured_until,
			       e.refund_policy, e.terms_and_conditions,
			       e.created_at, COALESCE(e.tickets_sold, 0)::bigint
			FROM events e
			LEFT JOIN organizers o  ON o.id = e.organizer_id
			LEFT JOIN users u       ON u.id = o.user_id
			LEFT JOIN categories cat ON cat.id = e.category_id
			WHERE e.id = $1
		`, eventID).Scan(
			&ev.ID, &ev.Title, &ev.Description, &ev.Status,
			&ev.StartTime, &ev.EndTime, &ev.ExpiresAt, &ev.Timezone,
			&ev.VenueName, &ev.Address, &ev.City,
			&ev.Price, &ev.Currency, &ev.MaxCapacity, &ev.MaxPerUser,
			&ev.CoverImage, &ev.BannerImage,
			&ev.CategoryID, &ev.CategoryName,
			&ev.OrganiserID, &ev.OrganiserName, &ev.OrganiserMail, &ev.OrganiserKYC,
			&ev.IsFeatured, &ev.FeaturedFrom, &ev.FeaturedUntil,
			&ev.RefundPolicy, &ev.Terms,
			&ev.CreatedAt, &ev.TicketsSold,
		)
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		if err != nil {
			utils.InternalError(c, err, "could not load event")
			return
		}

		// Ticket tiers — the part the old summary omitted.
		type tier struct {
			Name        string  `json:"name"`
			Price       float64 `json:"price"`
			Capacity    *int    `json:"capacity"`
			Description *string `json:"description"`
			IsVisible   bool    `json:"is_visible"`
			SortOrder   int     `json:"sort_order"`
		}
		tiers := []tier{}
		rows, err := pool.Query(ctx, `
			SELECT name, price, capacity, description,
			       COALESCE(is_visible, TRUE), COALESCE(sort_order, 0)
			FROM ticket_types
			WHERE event_id = $1
			ORDER BY sort_order, name
		`, eventID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var t tier
				if scanErr := rows.Scan(&t.Name, &t.Price, &t.Capacity,
					&t.Description, &t.IsVisible, &t.SortOrder); scanErr == nil {
					tiers = append(tiers, t)
				}
			}
		}
		// A missing ticket_types table or a query error is not fatal here: the
		// reviewer still needs the event. Tiers come back empty and the UI says
		// "flat pricing", which is also what an event with no tiers looks like.

		middleware.AuditAllowed(pool, c, "event.review_open", "event", eventID, nil)

		c.JSON(http.StatusOK, gin.H{"event": ev, "ticket_types": tiers})
	}
}

// PUT /admin/events/:id/approve-with-feature
//
//	{ "feature": true, "featured_from": "2026-08-20T00:00:00Z", "featured_days": 7 }
//
// feature=false is an ordinary approval.
func ApproveEventWithFeatureHandler(pool *pgxpool.Pool, ec *cache.EventCache) gin.HandlerFunc {
	// A featuring window has to end. 90 days is far longer than any real
	// promotion and still short enough that a forgotten one expires by itself —
	// the failure mode being designed against is an event that stays on the
	// homepage after it has happened.
	const maxFeatureDays = 90

	return func(c *gin.Context) {
		eventID := c.Param("id")

		var body struct {
			Feature      bool       `json:"feature"`
			FeaturedFrom *time.Time `json:"featured_from"`
			FeaturedDays int        `json:"featured_days"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		if body.Feature {
			if body.FeaturedDays < 1 || body.FeaturedDays > maxFeatureDays {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "featured_days must be between 1 and 90",
				})
				return
			}
		}

		// Publish first, through the existing repository function, so approval
		// keeps whatever side effects it already has (organiser notification,
		// status transition rules) rather than this endpoint reimplementing
		// them and drifting.
		if err := repository.AdminApproveEvent(pool, eventID); err != nil {
			if err == repository.ErrEventNotPublishable {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Event not found or not in pending_review state",
				})
				return
			}
			utils.InternalError(c, err, "failed to approve event")
			return
		}

		var from, until *time.Time
		if body.Feature {
			start := time.Now()
			if body.FeaturedFrom != nil {
				start = *body.FeaturedFrom
			}
			end := start.AddDate(0, 0, body.FeaturedDays)
			from, until = &start, &end

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			if _, err := pool.Exec(ctx, `
				UPDATE events
				SET is_featured = TRUE, featured_from = $2, featured_until = $3
				WHERE id = $1
			`, eventID, start, end); err != nil {
				// The event IS published at this point. Featuring is the lesser
				// half, so this reports the partial outcome instead of implying
				// the approval failed and inviting a second attempt.
				utils.InternalError(c, err, "event approved, but featuring failed")
				return
			}
		}

		if ec != nil {
			ec.BustOnPublish(context.Background())
		}

		middleware.AuditAllowed(pool, c, "event.approve", "event", eventID, map[string]any{
			"featured":      body.Feature,
			"featured_days": body.FeaturedDays,
		})

		c.JSON(http.StatusOK, gin.H{
			"event_id":       eventID,
			"featured":       body.Feature,
			"featured_from":  from,
			"featured_until": until,
		})
	}
}
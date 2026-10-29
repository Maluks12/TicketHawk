package repository

// Event collections (migration 000096) — integration tests.
//
// These need the REAL schema (events has a dozen NOT NULL columns, enums and
// triggers that a hand-built test table would not reproduce), so they run
// against a database migrated to head and are SKIPPED unless one is named:
//
//	EVENT_COLLECTIONS_DB_URL=postgresql://user:pass@localhost:5433/mig_test \
//	  go test ./internal/repository -run EventCollection
//
// Every row they create is cleaned up, so pointing them at a scratch copy of
// a real database is safe.

import (
	"context"
	"os"
	"testing"
	"time"

	"events-db/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eventCollectionsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("EVENT_COLLECTIONS_DB_URL")
	if url == "" {
		t.Skip("EVENT_COLLECTIONS_DB_URL not set — needs a database migrated to head")
	}
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	return pool
}

type ecFixture struct {
	pool   *pgxpool.Pool
	userA  string
	userB  string
	events []string // userA's events
}

func newECFixture(t *testing.T) *ecFixture {
	pool := eventCollectionsTestPool(t)
	ctx := context.Background()
	f := &ecFixture{pool: pool, userA: uuid.NewString(), userB: uuid.NewString()}

	for _, u := range []string{f.userA, f.userB} {
		_, err := pool.Exec(ctx, `INSERT INTO users (id, email, role) VALUES ($1, $2, 'organizer')`,
			u, u+"@ec-test.invalid")
		require.NoError(t, err)
	}

	var catID int
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM categories ORDER BY id LIMIT 1`).Scan(&catID))

	// Four of userA's events: three live public, one private (never shown).
	mk := func(owner, title, city, status string, daysAhead int, price float64) string {
		id := uuid.NewString()
		start := time.Now().Add(time.Duration(daysAhead) * 24 * time.Hour)
		_, err := pool.Exec(ctx, `
			INSERT INTO events (id, slug, title, description, start_time, end_time, expires_at,
			                    venue_name, address, city, category_id, organizer_id, status, price,
			                    visibility)
			VALUES ($1, $2, $3, 'd', $4, $5, $4, 'Venue', 'Addr', $6, $7, $8, $9::event_status, $10,
			        CASE WHEN $9 = 'published_private' THEN 'private_link' ELSE 'public' END)
		`, id, "ec-"+id[:8], title, start, start.Add(3*time.Hour), city, catID, owner, status, price)
		require.NoError(t, err)
		return id
	}
	f.events = []string{
		mk(f.userA, "Stop 1", "East London", "published", 3, 150),
		mk(f.userA, "Stop 2", "Cape Town", "published", 10, 50),
		mk(f.userA, "Stop 3", "Johannesburg", "published", 20, 100),
		mk(f.userA, "Private stop", "Durban", "published_private", 5, 0),
	}
	// userB's event, for the ownership checks.
	f.events = append(f.events, mk(f.userB, "Not yours", "Pretoria", "published", 4, 80))

	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM events WHERE organizer_id = ANY($1::uuid[])`, []string{f.userA, f.userB})
		pool.Exec(ctx, `DELETE FROM event_collections WHERE organizer_id = ANY($1::uuid[])`, []string{f.userA, f.userB})
		pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{f.userA, f.userB})
		pool.Close()
	})
	return f
}

func TestEventCollectionLifecycle(t *testing.T) {
	f := newECFixture(t)
	ctx := context.Background()
	desc := "  Twelve stops across Southern Africa  "

	col, err := CreateEventCollection(ctx, f.pool, f.userA, models.CreateEventCollectionDTO{
		Title: "  Afrochild Pop-Up Tour ", Description: &desc,
		EventIDs: f.events[:4],
	})
	require.NoError(t, err)
	assert.Equal(t, "Afrochild Pop-Up Tour", col.Title, "title is trimmed")
	assert.Equal(t, "Twelve stops across Southern Africa", *col.Description)
	assert.Regexp(t, `^afrochild-pop-up-tour-[0-9a-f]{8}$`, col.Slug)
	assert.Equal(t, 4, col.EventCount)
	assert.Equal(t, 3, col.LiveEventCount, "private event is a member but not live")
	assert.Len(t, col.EventIDs, 4)

	t.Run("someone else's event is rejected and nothing changes", func(t *testing.T) {
		_, err := SetEventCollectionEvents(ctx, f.pool, f.userA, col.ID, []string{f.events[0], f.events[4]})
		assert.ErrorIs(t, err, ErrEventNotOwned)
		again, err := GetOrganizerEventCollection(ctx, f.pool, f.userA, col.ID)
		require.NoError(t, err)
		assert.Equal(t, 4, again.EventCount, "all-or-nothing: membership untouched")
	})

	t.Run("another organiser cannot see, edit or delete it", func(t *testing.T) {
		_, err := GetOrganizerEventCollection(ctx, f.pool, f.userB, col.ID)
		assert.ErrorIs(t, err, ErrEventCollectionNotFound)
		title := "hijacked"
		_, err = UpdateEventCollection(ctx, f.pool, f.userB, col.ID, models.UpdateEventCollectionDTO{Title: &title})
		assert.ErrorIs(t, err, ErrEventCollectionNotFound)
		_, err = SetEventCollectionEvents(ctx, f.pool, f.userB, col.ID, nil)
		assert.ErrorIs(t, err, ErrEventCollectionNotFound)
		assert.ErrorIs(t, DeleteEventCollection(ctx, f.pool, f.userB, col.ID), ErrEventCollectionNotFound)
		owned, err := EventCollectionOwned(ctx, f.pool, f.userB, col.ID)
		require.NoError(t, err)
		assert.False(t, owned)
	})

	t.Run("feed refs: only live dates count, shared ref, cities in date order", func(t *testing.T) {
		refs, err := CollectionRefsForEvents(ctx, f.pool, f.events)
		require.NoError(t, err)
		require.Contains(t, refs, f.events[0])
		ref := refs[f.events[0]]
		assert.Same(t, ref, refs[f.events[1]], "events of one collection share one ref")
		assert.Equal(t, 3, ref.EventCount)
		assert.Equal(t, 50.0, ref.MinPrice)
		assert.Equal(t, []string{"East London", "Cape Town", "Johannesburg"}, ref.Cities)
		assert.NotContains(t, refs, f.events[4], "not in any collection")
	})

	t.Run("fold keeps the first event per collection, in section order", func(t *testing.T) {
		refs, err := CollectionRefsForEvents(ctx, f.pool, f.events)
		require.NoError(t, err)
		section := []models.Event{
			{ID: f.events[4]}, {ID: f.events[1]}, {ID: f.events[0]}, {ID: f.events[2]},
		}
		got := FoldCollections(section, refs)
		require.Len(t, got, 2)
		assert.Equal(t, f.events[4], got[0].ID)
		assert.Nil(t, got[0].Collection)
		assert.Equal(t, f.events[1], got[1].ID, "first-ranked date stands in for the collection")
		require.NotNil(t, got[1].Collection)
		assert.Equal(t, col.ID, got[1].Collection.ID)
	})

	t.Run("public page lists upcoming public dates soonest first", func(t *testing.T) {
		pub, err := GetPublicEventCollection(ctx, f.pool, col.Slug)
		require.NoError(t, err)
		require.Len(t, pub.Events, 3)
		assert.Equal(t, []string{"Stop 1", "Stop 2", "Stop 3"},
			[]string{pub.Events[0].Title, pub.Events[1].Title, pub.Events[2].Title})
		byID, err := GetPublicEventCollection(ctx, f.pool, col.ID)
		require.NoError(t, err)
		assert.Equal(t, col.Slug, byID.Slug, "resolves by id too")
	})

	t.Run("below two live dates it is not a card", func(t *testing.T) {
		_, err := SetEventCollectionEvents(ctx, f.pool, f.userA, col.ID, []string{f.events[0], f.events[3]})
		require.NoError(t, err)
		refs, err := CollectionRefsForEvents(ctx, f.pool, f.events)
		require.NoError(t, err)
		assert.Empty(t, refs, "one live date + one private = no collection card")
		var released *string
		require.NoError(t, f.pool.QueryRow(ctx,
			`SELECT event_collection_id::text FROM events WHERE id = $1`, f.events[1]).Scan(&released))
		assert.Nil(t, released, "dropped events are released")
	})

	t.Run("moving an event between my collections", func(t *testing.T) {
		other, err := CreateEventCollection(ctx, f.pool, f.userA, models.CreateEventCollectionDTO{
			Title: "Second", EventIDs: []string{f.events[0]},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, other.EventCount)
		first, err := GetOrganizerEventCollection(ctx, f.pool, f.userA, col.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{f.events[3]}, first.EventIDs, "event moved, one collection per event")
	})

	t.Run("delete releases events, keeps them", func(t *testing.T) {
		require.NoError(t, DeleteEventCollection(ctx, f.pool, f.userA, col.ID))
		var n int
		require.NoError(t, f.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM events WHERE id = $1 AND event_collection_id IS NULL`, f.events[3]).Scan(&n))
		assert.Equal(t, 1, n)
		_, err := GetPublicEventCollection(ctx, f.pool, col.Slug)
		assert.ErrorIs(t, err, ErrEventCollectionNotFound)
	})
}

// The real home feed path: SharedHomeSections → runSections → foldHomeSections.
func TestEventCollectionFoldsHomeFeed(t *testing.T) {
	f := newECFixture(t)
	ctx := context.Background()

	col, err := CreateEventCollection(ctx, f.pool, f.userA, models.CreateEventCollectionDTO{
		Title: "Tour", EventIDs: f.events[:3],
	})
	require.NoError(t, err)

	res, err := SharedHomeSections(f.pool)
	require.NoError(t, err)

	var soon *HomeSection
	for i := range res.Sections {
		if res.Sections[i].ID == "happening_soon" {
			soon = &res.Sections[i]
		}
	}
	require.NotNil(t, soon, "happening_soon section present")

	var cards, members int
	for _, e := range soon.Events {
		for _, id := range f.events[:3] {
			if e.ID == id {
				members++
				if e.Collection != nil && e.Collection.ID == col.ID {
					cards++
				}
			}
		}
	}
	assert.Equal(t, 1, members, "three dates fold to one entry")
	assert.Equal(t, 1, cards, "and that entry carries the collection")
	for _, e := range soon.Events {
		if e.ID == f.events[4] {
			assert.Nil(t, e.Collection, "other organisers' events untouched")
		}
	}
}

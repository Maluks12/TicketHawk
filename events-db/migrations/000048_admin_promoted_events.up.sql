-- migrations/000048_admin_promoted_events.up.sql
--
-- Admin-promoted events ("Add to Home").
--
-- Lets an admin place an event on the home feed as if it were a paid ad, without
-- the organiser running (or paying for) a boost campaign. Promoted events are
-- woven into the feed as zero-cost sponsored cards (see GetAdminPromotedForFeed)
-- so they never touch the ad-wallet / daily-spend billing path. Promotion also
-- sets listing_fee_paid = TRUE on the event (done in the repository) so a free,
-- fee-unpaid event still clears the home-feed eligibility gate.

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS admin_promoted    BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS admin_promoted_at TIMESTAMPTZ;

-- Hot-path index for the feed query: published, upcoming, promoted events.
CREATE INDEX IF NOT EXISTS idx_events_admin_promoted
    ON events (start_time)
    WHERE admin_promoted = TRUE AND status = 'published';
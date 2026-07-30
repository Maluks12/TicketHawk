-- migrations/000064_push_tokens.up.sql
--
-- Device registry for out-of-app (push) notifications.
--
-- One row per DEVICE, not per user: a user with a phone and a tablet has two
-- rows, and a single phone shared by two people has one row that moves between
-- them. That second case is why `token` is UNIQUE rather than (user_id, token):
-- when user B signs in on user A's handset, Expo hands the app the SAME push
-- token (it is tied to the app install, not the account). Registering it
-- re-points the existing row at user B, so user A's notifications stop landing
-- on a phone they no longer control. A (user_id, token) unique key would
-- instead create a second row and push A's booking confirmations to B.
--
-- Tokens are removed, never soft-disabled:
--   • logout                        → app calls DELETE /v1/notifications/push-token
--   • notifications toggled off     → same call
--   • Expo replies DeviceNotRegistered (app uninstalled, token rotated)
--                                   → push service deletes the row
-- A dead token that lingers costs a wasted API call on every send, and Expo
-- rate-limits on volume, so pruning is worth doing eagerly.

CREATE TABLE IF NOT EXISTS push_tokens (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Expo push token: "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]".
    -- Validated for shape by the handler before it ever reaches this table.
    token         TEXT        NOT NULL UNIQUE,

    platform      TEXT        NOT NULL CHECK (platform IN ('ios', 'android')),

    -- Which app the token belongs to. The consumer app (com.tickethawk.app)
    -- and Studio (com.tickethawk.studio) are separate installs with separate
    -- tokens, and an organiser may well have both. Kept so a future
    -- organiser-only notification can target one without buzzing the other.
    app           TEXT        NOT NULL DEFAULT 'consumer'
                              CHECK (app IN ('consumer', 'studio')),

    -- Free-text device label ("iPhone 14 Pro") purely for support/debugging —
    -- when someone says "I stopped getting notifications", this is how you tell
    -- which of their three rows is the handset in their hand.
    device_name   TEXT,

    -- Incremented on transient Expo errors, reset on success. Not currently
    -- used to gate sends; it exists so a token that fails persistently without
    -- ever returning DeviceNotRegistered can be spotted and cleaned up.
    failure_count INT         NOT NULL DEFAULT 0,

    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The only read path that matters: "give me every device for these users".
-- The push service resolves a whole batch of recipients in one ANY($1) query,
-- so this index carries the entire send pipeline.
CREATE INDEX IF NOT EXISTS idx_push_tokens_user
    ON push_tokens (user_id);
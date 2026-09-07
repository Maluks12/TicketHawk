-- ── 000086: Cashless wallets, vendors, and the spend ledger ─────────────────
--
-- Phase 3. Money enters the platform's own books for the first time, so a few
-- things here are stricter than they would be for a feature that only moves
-- tickets around.
--
-- BALANCE IS NEVER STORED. It is SUM(amount) over cashless_transactions, an
-- append-only ledger. This is the one rule the whole phase rests on, and it is
-- deliberately NOT how ad_wallets works — that table keeps a mutable `balance`
-- column with a non-negative CHECK, which is fine for advertising credit the
-- organiser tops up and burns down in one place, and wrong here. A cashless
-- wallet is written by many vendor devices at once, reconciled after outages,
-- and disputed weeks later by an attendee who says they were charged twice. A
-- mutable balance answers "how much is left" and destroys the evidence for
-- every other question.
--
-- Snapshots exist for reads (§5 of the architecture doc) but are explicitly not
-- a source of truth: a snapshot is a checkpoint plus a sequence number, and the
-- balance is the snapshot plus the delta after it. Delete every snapshot and
-- nothing is lost but speed.
--
-- WHAT THE CHIP IS ALLOWED TO CARRY. rfid_tags.verification_class was created
-- in 000085 for exactly this moment. A uid_only chip is a serial number any
-- phone can emit; letting one hold spendable value would mean a cloned band
-- drains a stranger's wallet. The constraint below makes that unrepresentable
-- rather than merely discouraged, so no future handler can get it wrong.

-- ── Vendors ──────────────────────────────────────────────────────────────────

CREATE TYPE vendor_status AS ENUM ('active', 'suspended', 'closed');

CREATE TABLE event_vendors (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name        VARCHAR(160) NOT NULL,

    -- Short code printed on the stall and shown on the attendee's statement.
    -- "BAR1" reads better than a UUID on a disputed charge.
    code        VARCHAR(24) NOT NULL,

    contact_name  VARCHAR(160),
    contact_email VARCHAR(255),
    contact_phone VARCHAR(40),

    -- Platform commission on this vendor's takings, as a percentage. Nullable
    -- so an event can run on a flat per-tag fee alone (§6) without implying a
    -- zero commission it never agreed to.
    commission_rate DECIMAL(5,2) CHECK (commission_rate IS NULL OR (commission_rate >= 0 AND commission_rate <= 100)),

    status      vendor_status NOT NULL DEFAULT 'active',
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_vendor_event_code UNIQUE (event_id, code)
);

CREATE INDEX idx_event_vendors_event ON event_vendors (event_id) WHERE status = 'active';

CREATE TRIGGER update_event_vendors_updated_at
    BEFORE UPDATE ON event_vendors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Vendor menus ─────────────────────────────────────────────────────────────
--
-- Separate from `merchandise`, which is pre-ordered online and fulfilled from a
-- collection. A bar menu is priced per event, changes during the night, and is
-- rung up at a stall. Sharing the table would have meant either polluting the
-- merch model with vendor concepts or vice versa.

CREATE TABLE vendor_products (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id   UUID NOT NULL REFERENCES event_vendors(id) ON DELETE CASCADE,
    name        VARCHAR(160) NOT NULL,
    category    VARCHAR(80),
    price       DECIMAL(10,2) NOT NULL CHECK (price >= 0),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vendor_products_vendor ON vendor_products (vendor_id, sort_order)
    WHERE is_active;

CREATE TRIGGER update_vendor_products_updated_at
    BEFORE UPDATE ON vendor_products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── POS devices ──────────────────────────────────────────────────────────────
--
-- Device identity is separate from human identity on purpose. The operator
-- still authenticates with their normal JWT and still has to pass the event's
-- existing staff check, so revoking a person cuts off their access exactly as
-- it does at the gate. The device token answers a different question — WHICH
-- till rang this up — which matters for vendor settlement and for the §5
-- offline caps in Phase 4, and which a human credential cannot answer.
--
-- Only a hash of the token is stored. A leaked database should not hand out
-- working till credentials.

CREATE TYPE pos_device_status AS ENUM ('active', 'suspended', 'retired');

CREATE TABLE vendor_pos_devices (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id      UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    vendor_id     UUID NOT NULL REFERENCES event_vendors(id) ON DELETE CASCADE,
    label         VARCHAR(120) NOT NULL,
    token_hash    VARCHAR(64) NOT NULL UNIQUE,   -- SHA-256 hex of the issued token
    status        pos_device_status NOT NULL DEFAULT 'active',

    -- Phase 4 fields, created now so the offline work is a code change rather
    -- than a migration during a festival build-up. Unused in Phase 3.
    attestation_status VARCHAR(32),
    last_seen_at       TIMESTAMPTZ,

    created_by    UUID NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pos_devices_vendor ON vendor_pos_devices (vendor_id) WHERE status = 'active';
CREATE INDEX idx_pos_devices_event  ON vendor_pos_devices (event_id);

CREATE TRIGGER update_vendor_pos_devices_updated_at
    BEFORE UPDATE ON vendor_pos_devices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Wallets ──────────────────────────────────────────────────────────────────

CREATE TYPE cashless_wallet_status AS ENUM ('active', 'frozen', 'closed');

-- How a wallet has been funded so far. This drives the §5 cash-user rule in
-- Phase 4 — a wallet with no card behind it has nothing to auto-charge if it
-- goes negative, so its offline ceiling is zero. Maintained by trigger below
-- rather than by handlers, because getting it wrong means unrecoverable loss
-- and there is more than one place that credits a wallet.
CREATE TYPE wallet_funding AS ENUM ('none', 'cash', 'card', 'mixed');

CREATE TABLE cashless_wallets (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id  UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,

    -- NULL for a cash walk-up who never created an account. The whole point of
    -- a cashless festival is that somebody can hand over notes at a top-up
    -- booth and start spending, and requiring an account would exclude exactly
    -- the attendees this is meant to include.
    user_id   UUID REFERENCES users(id) ON DELETE SET NULL,

    status    cashless_wallet_status NOT NULL DEFAULT 'active',
    currency  CHAR(3) NOT NULL DEFAULT 'ZAR',
    funding   wallet_funding NOT NULL DEFAULT 'none',

    -- Set when a card has been successfully charged for this wallet, so the
    -- reconciliation path in Phase 4 knows there is something to charge.
    card_on_file BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One wallet per person per event. Partial so multiple anonymous cash
    -- wallets can coexist, which is the normal case at a top-up booth.
    CONSTRAINT uq_wallet_event_user UNIQUE (event_id, user_id)
);

CREATE INDEX idx_cashless_wallets_event ON cashless_wallets (event_id);

CREATE TRIGGER update_cashless_wallets_updated_at
    BEFORE UPDATE ON cashless_wallets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ── Tag → wallet binding ─────────────────────────────────────────────────────
--
-- The constraint below is the reason verification_class exists.

ALTER TABLE rfid_tags
    ADD COLUMN wallet_id UUID REFERENCES cashless_wallets(id) ON DELETE SET NULL;

ALTER TABLE rfid_tags
    ADD CONSTRAINT chk_rfid_wallet_requires_verifiable_chip
    CHECK (wallet_id IS NULL OR verification_class <> 'unverified');

CREATE INDEX idx_rfid_tags_wallet ON rfid_tags (wallet_id) WHERE wallet_id IS NOT NULL;

-- One live band per wallet, mirroring uq_rfid_active_assignment: re-issuing a
-- lost band must deactivate the old one first, so the old chip stops spending
-- before the new one starts.
CREATE UNIQUE INDEX uq_rfid_active_wallet
    ON rfid_tags (wallet_id)
    WHERE wallet_id IS NOT NULL AND status IN ('encoded', 'active');

COMMENT ON COLUMN rfid_tags.wallet_id IS
    'Spending wallet this band carries. Refused for unverified (uid_only) chips '
    'by chk_rfid_wallet_requires_verifiable_chip — a cloneable serial number '
    'must never hold spendable value.';

-- ── The ledger ───────────────────────────────────────────────────────────────

CREATE TYPE cashless_txn_kind AS ENUM (
    'topup',      -- money in: card settled or cash taken at a booth
    'purchase',   -- money out: a vendor sale
    'refund',     -- money in: a purchase reversed at the vendor
    'payout',     -- money out: unspent balance returned to the attendee
    'adjustment'  -- either direction: manual correction, always attributed
);

CREATE TYPE cashless_txn_mode AS ENUM ('online', 'offline');

CREATE TABLE cashless_transactions (
    -- CLIENT-generated, exactly like scan_events.id. A till that loses its
    -- response and retries must not charge twice, and the only reliable way to
    -- guarantee that is to let the client name the transaction.
    id          UUID PRIMARY KEY,

    wallet_id   UUID NOT NULL REFERENCES cashless_wallets(id) ON DELETE RESTRICT,
    event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    kind        cashless_txn_kind NOT NULL,

    -- SIGNED. Credits positive, debits negative, so a balance is a plain
    -- SUM(amount) with no CASE over kind — one place to be wrong instead of
    -- one per query. The sign/kind agreement is enforced below.
    amount      DECIMAL(10,2) NOT NULL CHECK (amount <> 0),

    vendor_id   UUID REFERENCES event_vendors(id) ON DELETE SET NULL,
    device_id   UUID REFERENCES vendor_pos_devices(id) ON DELETE SET NULL,
    tag_uid     VARCHAR(20),

    -- Per-wallet monotonic sequence. Gives every wallet its own ordering that
    -- does not depend on clocks — §5 calls for this precisely so a vendor
    -- cannot backdate a transaction to slip under a cap.
    seq_num     BIGINT NOT NULL CHECK (seq_num > 0),

    -- Hash chain over the wallet's history. Any edit to a past row breaks every
    -- hash after it, which turns silent tampering into a detectable one.
    prev_hash   CHAR(64),
    hash        CHAR(64) NOT NULL,

    mode        cashless_txn_mode NOT NULL DEFAULT 'online',

    -- Free-form line items for a purchase (product names, quantities), kept for
    -- the attendee's statement. Deliberately not normalised: a menu changes
    -- during the night and a receipt must show what was actually sold, not what
    -- the product row says today.
    detail      JSONB NOT NULL DEFAULT '{}',

    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_cashless_wallet_seq UNIQUE (wallet_id, seq_num),

    -- Sign must agree with direction. Without this a purchase written with a
    -- positive amount silently CREDITS the attendee, and the ledger's own sum
    -- would confirm it.
    CONSTRAINT chk_cashless_sign CHECK (
        (kind IN ('topup', 'refund') AND amount > 0) OR
        (kind IN ('purchase', 'payout') AND amount < 0) OR
        (kind = 'adjustment')
    ),

    -- A sale has to say which till rang it up, or vendor settlement is guesswork.
    CONSTRAINT chk_cashless_purchase_has_vendor
        CHECK (kind <> 'purchase' OR vendor_id IS NOT NULL)
);

CREATE INDEX idx_cashless_txn_wallet ON cashless_transactions (wallet_id, seq_num);
CREATE INDEX idx_cashless_txn_event  ON cashless_transactions (event_id, created_at DESC);
CREATE INDEX idx_cashless_txn_vendor ON cashless_transactions (vendor_id, created_at DESC)
    WHERE vendor_id IS NOT NULL;

-- ── Pending top-ups ──────────────────────────────────────────────────────────
--
-- A card top-up is not money until Paystack says so, so it cannot be a ledger
-- row when it starts. This table holds the intent; the webhook turns a
-- completed one into a 'topup' entry. Cash top-ups are recorded here too, with
-- the staff member who took the notes — that attribution is the only audit
-- trail cash has.

CREATE TYPE cashless_topup_method AS ENUM ('card', 'cash');
CREATE TYPE cashless_topup_status AS ENUM ('pending', 'completed', 'failed', 'cancelled');

CREATE TABLE cashless_topups (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id          UUID NOT NULL REFERENCES cashless_wallets(id) ON DELETE CASCADE,
    event_id           UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    amount             DECIMAL(10,2) NOT NULL CHECK (amount > 0),

    -- The service fee disclosed at the time of top-up (§6, transparent fee
    -- disclosure). Stored alongside the amount so a statement can show exactly
    -- what the attendee was told, even after the event's fee changes.
    fee                DECIMAL(10,2) NOT NULL DEFAULT 0.00 CHECK (fee >= 0),

    method             cashless_topup_method NOT NULL,
    status             cashless_topup_status NOT NULL DEFAULT 'pending',
    paystack_reference VARCHAR(255) UNIQUE,
    txn_id             UUID REFERENCES cashless_transactions(id) ON DELETE SET NULL,
    created_by         UUID REFERENCES users(id),   -- staff member, for cash
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at       TIMESTAMPTZ,

    -- Cash is taken by a human and has no gateway reference; card must have one.
    CONSTRAINT chk_topup_reference CHECK (
        (method = 'card' AND paystack_reference IS NOT NULL) OR
        (method = 'cash' AND paystack_reference IS NULL AND created_by IS NOT NULL)
    )
);

CREATE INDEX idx_cashless_topups_wallet ON cashless_topups (wallet_id, created_at DESC);
CREATE INDEX idx_cashless_topups_pending ON cashless_topups (status) WHERE status = 'pending';

-- ── Funding-source maintenance ───────────────────────────────────────────────
--
-- In a trigger rather than in handlers because Phase 4's cash-only offline cap
-- depends on it, an unrecoverable loss depends on that, and more than one code
-- path credits a wallet.

CREATE OR REPLACE FUNCTION cashless_touch_funding() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'completed' AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'completed') THEN
        UPDATE cashless_wallets w
        SET funding = CASE
                WHEN w.funding = 'none' THEN NEW.method::text::wallet_funding
                WHEN w.funding::text = NEW.method::text THEN w.funding
                ELSE 'mixed'
            END,
            card_on_file = w.card_on_file OR (NEW.method = 'card')
        WHERE w.id = NEW.wallet_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cashless_touch_funding
    AFTER INSERT OR UPDATE OF status ON cashless_topups
    FOR EACH ROW EXECUTE FUNCTION cashless_touch_funding();

-- ── Balance snapshots ────────────────────────────────────────────────────────
--
-- A read aid, not truth. Balance is snapshot.balance plus the sum of entries
-- after snapshot.as_of_seq_num. Truncate this table and nothing is lost.

CREATE TABLE wallet_balance_snapshots (
    wallet_id     UUID NOT NULL REFERENCES cashless_wallets(id) ON DELETE CASCADE,
    snapshot_seq  BIGINT NOT NULL,
    balance       DECIMAL(10,2) NOT NULL,
    as_of_seq_num BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (wallet_id, snapshot_seq)
);

CREATE INDEX idx_wallet_snapshots_latest ON wallet_balance_snapshots (wallet_id, as_of_seq_num DESC);

COMMENT ON TABLE wallet_balance_snapshots IS
    'Performance checkpoint only. Never a source of truth — balance is always '
    'derivable from cashless_transactions alone.';
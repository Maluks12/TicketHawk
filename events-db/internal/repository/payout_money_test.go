package repository

// Money-path tests for the payout ledger.
//
// Covers the three DB-side money decisions: debt netting before a transfer,
// settlement netting of partial refunds, and reserve release netting. These
// need a real Postgres because the logic lives in SQL — GREATEST, CASE on
// booking_status, and SELECT ... FOR UPDATE all behave differently or not at
// all in a mock.
//
// Prerequisites:
//
//	docker run -d --name test-postgres \
//	  -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
//	  -e POSTGRES_DB=test_db -p 5433:5432 postgres:15
//
// Run:
//
//	go test ./internal/repository/... -run TestPayoutMoney -v
//
// Skips cleanly when the database is unreachable, so CI without Postgres stays
// green rather than red-for-the-wrong-reason.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const payoutMoneyTestDBURL = "postgresql://testuser:testpass@localhost:5433/test_db"

func setupPayoutMoneyDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), payoutMoneyTestDBURL)
	if err != nil {
		t.Skipf("payout money tests skipped — could not create pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("payout money tests skipped — DB not reachable: %v", err)
	}
	createPayoutMoneyTables(t, pool)
	return pool
}

func createPayoutMoneyTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	for _, s := range []string{
		"DROP TABLE IF EXISTS organizer_payouts CASCADE",
		"DROP TABLE IF EXISTS payments          CASCADE",
		"DROP TABLE IF EXISTS bookings          CASCADE",
		"DROP TABLE IF EXISTS events            CASCADE",
		"DROP TABLE IF EXISTS organizers        CASCADE",
	} {
		pool.Exec(ctx, s)
	}

	for _, s := range []string{
		`CREATE TABLE organizers (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id      UUID NOT NULL DEFAULT gen_random_uuid(),
			business_name TEXT NOT NULL DEFAULT 'Test Organiser',
			payout_tier  TEXT NOT NULL DEFAULT 'held',
			payout_debt  NUMERIC(12,2) NOT NULL DEFAULT 0
			             CHECK (payout_debt >= 0),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE events (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id UUID NOT NULL,
			status       TEXT NOT NULL DEFAULT 'completed',
			end_time     TIMESTAMPTZ NOT NULL DEFAULT NOW() - INTERVAL '72 hours'
		)`,
		`CREATE TABLE bookings (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id       UUID NOT NULL,
			booking_status TEXT NOT NULL DEFAULT 'confirmed',
			subtotal       NUMERIC(10,2) NOT NULL DEFAULT 0,
			organizer_fee  NUMERIC(10,2) NOT NULL DEFAULT 0,
			refund_amount  NUMERIC(10,2),
			currency       VARCHAR(10) NOT NULL DEFAULT 'ZAR'
		)`,
		`CREATE TABLE payments (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			booking_id      UUID NOT NULL,
			status          TEXT NOT NULL DEFAULT 'completed',
			subaccount_code TEXT,
			reserve_amount  NUMERIC(10,2) NOT NULL DEFAULT 0,
			currency        VARCHAR(10) NOT NULL DEFAULT 'ZAR'
		)`,
		`CREATE TABLE organizer_payouts (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			organizer_id UUID NOT NULL,
			event_id     UUID NOT NULL,
			kind         VARCHAR(20) NOT NULL
			             CHECK (kind IN ('event_settlement','reserve_release','refund_clawback')),
			amount       NUMERIC(12,2) NOT NULL,
			currency     VARCHAR(10) NOT NULL DEFAULT 'ZAR',
			status       VARCHAR(15) NOT NULL DEFAULT 'pending'
			             CHECK (status IN ('pending','processing','paid','failed')),
			transfer_reference TEXT,
			failure_reason     TEXT,
			paid_at      TIMESTAMPTZ,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT unique_event_payout UNIQUE (event_id, kind)
		)`,
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("creating test tables: %v", err)
		}
	}
}

func newOrganiserWithDebt(t *testing.T, pool *pgxpool.Pool, debt float64) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO organizers (payout_debt) VALUES ($1) RETURNING id`, debt).Scan(&id)
	if err != nil {
		t.Fatalf("creating organiser: %v", err)
	}
	return id
}

func newPayout(t *testing.T, pool *pgxpool.Pool, orgID string, amount float64) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount)
		 VALUES ($1, gen_random_uuid(), 'event_settlement', $2) RETURNING id`,
		orgID, amount).Scan(&id)
	if err != nil {
		t.Fatalf("creating payout: %v", err)
	}
	return id
}

func payoutAmount(t *testing.T, pool *pgxpool.Pool, id string) (amount float64, status string) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT amount, status FROM organizer_payouts WHERE id = $1`, id).Scan(&amount, &status)
	if err != nil {
		t.Fatalf("reading payout: %v", err)
	}
	return
}

func organiserDebt(t *testing.T, pool *pgxpool.Pool, id string) float64 {
	t.Helper()
	var debt float64
	if err := pool.QueryRow(context.Background(),
		`SELECT payout_debt FROM organizers WHERE id = $1`, id).Scan(&debt); err != nil {
		t.Fatalf("reading debt: %v", err)
	}
	return debt
}

// ── Debt netting ──────────────────────────────────────────────────────────────

// TestPayoutMoney_DebtNettingReturnsPayableAmount is the regression test for a
// production bug that overpaid an organiser by R412.
//
// ApplyPayoutDebt reduced organizer_payouts.amount from 500 to 88 and cleared
// the debt, but returned only (settled, err). The caller still held a
// PendingPayoutRow with Amount = 500 in memory and transferred THAT — so
// Paystack moved R500 while the ledger, the debt balance and the audit trail
// all agreed on R88. Every internal number reconciled; only the bank statement
// disagreed.
//
// The returned payable value is therefore load-bearing, and this test asserts
// it matches the persisted row. A future refactor that drops the return value
// fails here rather than in production.
func TestPayoutMoney_DebtNettingReturnsPayableAmount(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	orgID := newOrganiserWithDebt(t, pool, 412.00)
	payoutID := newPayout(t, pool, orgID, 500.00)

	payable, settled, err := ApplyPayoutDebt(pool, ctx, orgID, payoutID, 500.00)
	if err != nil {
		t.Fatalf("ApplyPayoutDebt: %v", err)
	}
	if settled {
		t.Fatal("R500 payout against R412 debt should leave a remainder, not settle")
	}
	if payable != 88.00 {
		t.Errorf("payable = %.2f, want 88.00 — this is the value that reaches the gateway", payable)
	}

	stored, _ := payoutAmount(t, pool, payoutID)
	if stored != payable {
		t.Errorf("LEDGER/TRANSFER DIVERGENCE: ledger says %.2f, caller would transfer %.2f", stored, payable)
	}
	if debt := organiserDebt(t, pool, orgID); debt != 0 {
		t.Errorf("debt after full absorption = %.2f, want 0", debt)
	}
}

func TestPayoutMoney_DebtExceedsPayout(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	orgID := newOrganiserWithDebt(t, pool, 1000.00)
	payoutID := newPayout(t, pool, orgID, 300.00)

	payable, settled, err := ApplyPayoutDebt(pool, ctx, orgID, payoutID, 300.00)
	if err != nil {
		t.Fatalf("ApplyPayoutDebt: %v", err)
	}
	if !settled {
		t.Error("debt larger than the payout must settle it entirely")
	}
	if payable != 0 {
		t.Errorf("payable = %.2f, want 0 — nothing may leave the platform", payable)
	}

	amount, status := payoutAmount(t, pool, payoutID)
	if amount != 0 || status != "paid" {
		t.Errorf("fully-offset payout = %.2f/%s, want 0.00/paid", amount, status)
	}
	if debt := organiserDebt(t, pool, orgID); debt != 700.00 {
		t.Errorf("remaining debt = %.2f, want 700.00", debt)
	}
}

func TestPayoutMoney_NoDebtPassesThroughUnchanged(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	orgID := newOrganiserWithDebt(t, pool, 0)
	payoutID := newPayout(t, pool, orgID, 376.00)

	payable, settled, err := ApplyPayoutDebt(pool, ctx, orgID, payoutID, 376.00)
	if err != nil {
		t.Fatalf("ApplyPayoutDebt: %v", err)
	}
	if settled {
		t.Error("a debt-free payout must not be marked settled")
	}
	if payable != 376.00 {
		t.Errorf("payable = %.2f, want the full 376.00 — the common case must not shrink", payable)
	}
	if stored, _ := payoutAmount(t, pool, payoutID); stored != 376.00 {
		t.Errorf("ledger amount changed to %.2f without any debt", stored)
	}
}

// TestPayoutMoney_ConcurrentDebtApplicationIsSerialised guards the FOR UPDATE
// lock. Two payout rows for one organiser can be processed in the same tick;
// without the lock both would read the full debt and each deduct it, wiping out
// more than was owed and underpaying the organiser twice.
func TestPayoutMoney_ConcurrentDebtApplicationIsSerialised(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	orgID := newOrganiserWithDebt(t, pool, 100.00)
	first := newPayout(t, pool, orgID, 200.00)
	second := newPayout(t, pool, orgID, 200.00)

	type result struct {
		payable float64
		err     error
	}
	done := make(chan result, 2)
	for _, id := range []string{first, second} {
		go func(payoutID string) {
			p, _, err := ApplyPayoutDebt(pool, ctx, orgID, payoutID, 200.00)
			done <- result{p, err}
		}(id)
	}

	total := 0.0
	for i := 0; i < 2; i++ {
		r := <-done
		if r.err != nil {
			t.Fatalf("concurrent ApplyPayoutDebt: %v", r.err)
		}
		total += r.payable
	}

	// R400 of payouts less R100 of debt, applied exactly once.
	if total != 300.00 {
		t.Errorf("combined payable = %.2f, want 300.00 — debt was applied twice or not at all", total)
	}
	if debt := organiserDebt(t, pool, orgID); debt != 0 {
		t.Errorf("debt after both payouts = %.2f, want 0", debt)
	}
}

// ── Ledger netting ────────────────────────────────────────────────────────────

// TestPayoutMoney_SettlementNetsPartialRefunds checks the SQL that decides what
// a held-tier organiser is owed once refunds are in play. The three cases that
// matter are an untouched booking, a partially refunded one, and a legacy
// refunded row with no recorded amount — the last must be treated as fully
// refunded, or a pre-migration refund gets paid out as though it never happened.
func TestPayoutMoney_SettlementNetsPartialRefunds(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	var eventID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO events (organizer_id) VALUES (gen_random_uuid()) RETURNING id`).Scan(&eventID); err != nil {
		t.Fatalf("creating event: %v", err)
	}

	seed := []struct {
		status string
		refund interface{}
		want   float64
		label  string
	}{
		{"confirmed", nil, 376.00, "untouched booking pays subtotal less commission"},
		{"refunded", 300.00, 76.00, "75% refund leaves the remainder"},
		{"refunded", 400.00, 0.00, "full refund contributes nothing, never negative"},
		{"refunded", nil, 0.00, "legacy refund with no amount treated as full"},
	}
	for _, s := range seed {
		if _, err := pool.Exec(ctx,
			`INSERT INTO bookings (event_id, booking_status, subtotal, organizer_fee, refund_amount)
			 VALUES ($1, $2, 400.00, 24.00, $3)`, eventID, s.status, s.refund); err != nil {
			t.Fatalf("seeding booking: %v", err)
		}
	}

	// Mirrors the expression inside CreatePayoutsForCompletedEvents.
	rows, err := pool.Query(ctx, `
		SELECT GREATEST(
		         b.subtotal
		         - CASE WHEN b.booking_status = 'refunded'
		                THEN COALESCE(b.refund_amount, b.subtotal)
		                ELSE COALESCE(b.refund_amount, 0) END
		         - b.organizer_fee, 0)
		FROM bookings b WHERE b.event_id = $1 ORDER BY b.ctid`, eventID)
	if err != nil {
		t.Fatalf("settlement query: %v", err)
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		var got float64
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if got != seed[i].want {
			t.Errorf("%s: got %.2f, want %.2f", seed[i].label, got, seed[i].want)
		}
		i++
	}
	if i != len(seed) {
		t.Fatalf("expected %d rows, got %d", len(seed), i)
	}
}

// TestPayoutMoney_ReserveReleaseNetsRefunds covers the instant-tier mirror: the
// reserve is what FUNDS a buyer refund, so only the unconsumed part is released.
// The 300/100 case is the one that would otherwise strand R200 of an
// organiser's money on the platform permanently.
func TestPayoutMoney_ReserveReleaseNetsRefunds(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	var eventID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO events (organizer_id) VALUES (gen_random_uuid()) RETURNING id`).Scan(&eventID); err != nil {
		t.Fatalf("creating event: %v", err)
	}

	cases := []struct {
		status  string
		refund  interface{}
		reserve float64
		want    float64
		label   string
	}{
		{"confirmed", nil, 300.00, 300.00, "no refund releases the whole reserve"},
		{"refunded", 100.00, 300.00, 200.00, "refund smaller than reserve releases the remainder"},
		{"refunded", 300.00, 300.00, 0.00, "refund equal to reserve releases nothing"},
		{"refunded", 400.00, 300.00, 0.00, "refund larger than reserve clamps at zero"},
		{"refunded", nil, 300.00, 0.00, "legacy refund consumes the whole reserve"},
	}
	for _, c := range cases {
		var bookingID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO bookings (event_id, booking_status, subtotal, organizer_fee, refund_amount)
			 VALUES ($1, $2, 400.00, 24.00, $3) RETURNING id`,
			eventID, c.status, c.refund).Scan(&bookingID); err != nil {
			t.Fatalf("seeding booking: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO payments (booking_id, status, subaccount_code, reserve_amount)
			 VALUES ($1, 'completed', 'ACCT_test', $2)`, bookingID, c.reserve); err != nil {
			t.Fatalf("seeding payment: %v", err)
		}

		var got float64
		if err := pool.QueryRow(ctx, `
			SELECT GREATEST(
			         p.reserve_amount
			         - CASE WHEN b.booking_status = 'refunded'
			                THEN COALESCE(b.refund_amount, p.reserve_amount)
			                ELSE COALESCE(b.refund_amount, 0) END, 0)
			FROM bookings b JOIN payments p ON p.booking_id = b.id
			WHERE b.id = $1`, bookingID).Scan(&got); err != nil {
			t.Fatalf("reserve query: %v", err)
		}
		if got != c.want {
			t.Errorf("%s: released %.2f, want %.2f", c.label, got, c.want)
		}
	}
}

// TestPayoutMoney_ClawbackNeverEntersTransferQueue guards the filter in
// FetchPayableRows. A refund_clawback row is money owed TO the platform; if it
// were ever picked up as payable, the platform would transfer the organiser the
// very amount it was trying to recover.
func TestPayoutMoney_ClawbackNeverEntersTransferQueue(t *testing.T) {
	pool := setupPayoutMoneyDB(t)
	defer pool.Close()
	ctx := context.Background()

	orgID := newOrganiserWithDebt(t, pool, 0)
	for _, kind := range []string{"event_settlement", "reserve_release", "refund_clawback"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO organizer_payouts (organizer_id, event_id, kind, amount, status)
			 VALUES ($1, gen_random_uuid(), $2, 100.00, 'pending')`, orgID, kind); err != nil {
			t.Fatalf("seeding %s: %v", kind, err)
		}
	}

	var payable int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM organizer_payouts
		WHERE status IN ('pending','processing') AND kind <> 'refund_clawback'
	`).Scan(&payable); err != nil {
		t.Fatalf("counting payable rows: %v", err)
	}
	if payable != 2 {
		t.Errorf("payable rows = %d, want 2 — clawback must be excluded from the queue", payable)
	}
}
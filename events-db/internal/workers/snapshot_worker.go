package workers

import (
	"context"
	"time"

	"events-db/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// ── Wallet balance snapshot worker ────────────────────────────────────────────
//
// Balance is SUM over the whole ledger, which is correct and also means a wallet
// with two thousand entries re-sums two thousand rows on every tap. Snapshots
// turn that into a checkpoint plus a small delta.
//
// This is §5's latency concern and the reason the 3-second transaction target
// survives a busy festival. It is a READ AID and nothing more — every snapshot
// could be deleted and no money would be lost, because the ledger remains the
// only source of truth. That property is why this worker is allowed to fail
// quietly: a missed snapshot costs milliseconds, never correctness.
//
// Threshold-driven rather than scheduled. A wallet with four transactions gains
// nothing from a checkpoint, and writing one for every wallet nightly is work
// nobody needs.

const (
	// snapshotThreshold is how many ledger entries must accumulate past the last
	// checkpoint before a new one is worth writing. 200 rows is still a fast
	// sum; the cost only becomes visible in the thousands.
	snapshotThreshold = 200

	// snapshotBatch bounds one pass so a festival with tens of thousands of
	// wallets does not turn a maintenance job into a load spike.
	snapshotBatch = 200
)

// StartSnapshotWorker begins the loop.
//
// Hourly. Snapshots are an optimisation, so running more often would spend more
// than it saves — and running during an event is fine precisely because a
// snapshot write never blocks a purchase: it reads committed history and
// appends, taking no lock a till competes for.
func StartSnapshotWorker(pool *pgxpool.Pool, ctx context.Context) {
	go func() {
		log.Info().Msg("snapshot worker: started")

		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runSnapshotPass(ctx, pool)
			case <-ctx.Done():
				log.Info().Msg("snapshot worker: stopped")
				return
			}
		}
	}()
}

func runSnapshotPass(ctx context.Context, pool *pgxpool.Pool) {
	wallets, err := repository.WalletsNeedingSnapshot(ctx, pool, snapshotThreshold, snapshotBatch)
	if err != nil {
		log.Error().Err(err).Msg("snapshot worker: could not list wallets")
		return
	}
	if len(wallets) == 0 {
		return
	}

	written := 0
	for _, walletID := range wallets {
		if err := repository.WriteBalanceSnapshot(ctx, pool, walletID); err != nil {
			// Logged and skipped rather than aborting the pass. One wallet
			// failing to checkpoint is a performance detail, not a correctness
			// problem, and the next pass picks it up.
			log.Warn().Err(err).Str("wallet_id", walletID).
				Msg("snapshot worker: snapshot failed")
			continue
		}
		written++
	}

	log.Info().Int("candidates", len(wallets)).Int("written", written).
		Msg("snapshot worker: pass complete")
}
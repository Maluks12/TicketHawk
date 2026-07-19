package services

import (
	"context"
	"fmt"
	"strings"

	"events-db/internal/models"
	"events-db/internal/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// PayoutService owns the two payout paths:
//
//	Onboarding — creates the Paystack subaccount (used by the instant tier's
//	             splits) AND the transfer recipient (used by the held tier's
//	             post-event transfers) from one set of bank details. Both are
//	             created for every organizer regardless of tier, so promotion
//	             later is a single column flip.
//	Processing — pays pending ledger rows from the platform Paystack balance,
//	             idempotently: each row carries a transfer_reference stamped
//	             exactly once; Paystack rejects a duplicate reference, and a
//	             stuck 'processing' row is reconciled via FetchTransfer rather
//	             than re-initiated.
type PayoutService struct {
	pool     *pgxpool.Pool
	paystack *PaystackGateway
}

func NewPayoutService(pool *pgxpool.Pool, paystack *PaystackGateway) *PayoutService {
	return &PayoutService{pool: pool, paystack: paystack}
}

func (s *PayoutService) Gateway() *PaystackGateway { return s.paystack }

// ── Onboarding ────────────────────────────────────────────────────────────────

// SetPayoutDetails creates/updates the organizer's subaccount and transfer
// recipient at Paystack from the submitted bank details, then persists the
// codes. Safe to call again on a bank change: the subaccount is updated in
// place (stable ACCT code) and a fresh recipient is created.
func (s *PayoutService) SetPayoutDetails(ctx context.Context, organizerID string, dto *models.SetPayoutDetailsDTO) (*models.PayoutProfile, error) {
	profile, err := repository.GetPayoutProfileByOrganizerID(s.pool, ctx, organizerID)
	if err != nil {
		return nil, err
	}

	var subCode string
	if profile.HasSubaccount && profile.SubaccountCode != nil {
		sub, uErr := s.paystack.UpdateSubaccount(ctx, *profile.SubaccountCode, profile.BusinessName, dto.BankCode, dto.AccountNumber)
		if uErr != nil {
			return nil, fmt.Errorf("updating subaccount: %w", uErr)
		}
		subCode = sub.SubaccountCode
	} else {
		var email string
		_ = s.pool.QueryRow(ctx, `SELECT business_email FROM organizers WHERE id = $1`, organizerID).Scan(&email)
		sub, cErr := s.paystack.CreateSubaccount(ctx, profile.BusinessName, dto.BankCode, dto.AccountNumber, dto.AccountName, email)
		if cErr != nil {
			return nil, fmt.Errorf("creating subaccount: %w", cErr)
		}
		subCode = sub.SubaccountCode
	}

	recipientCode, err := s.paystack.CreateTransferRecipient(ctx, dto.AccountName, dto.BankCode, dto.AccountNumber)
	if err != nil {
		// The subaccount exists but transfers can't target this account yet.
		// Persist nothing half-way: report the error so the organizer retries.
		return nil, fmt.Errorf("creating transfer recipient: %w", err)
	}

	if err := repository.SavePayoutDetails(s.pool, ctx, organizerID, subCode, recipientCode, dto); err != nil {
		return nil, err
	}
	return repository.GetPayoutProfileByOrganizerID(s.pool, ctx, organizerID)
}

// ── Ledger processing ─────────────────────────────────────────────────────────

// ProcessPendingPayouts pays every payable ledger row once. Called by the
// payout worker on a timer and by the admin retry endpoint. Individual row
// failures are recorded on the row and never abort the batch.
func (s *PayoutService) ProcessPendingPayouts(ctx context.Context) {
	rows, err := repository.FetchPayableRows(s.pool, ctx, 25)
	if err != nil {
		log.Error().Err(err).Msg("payouts: fetching payable rows")
		return
	}

	for _, row := range rows {
		if row.Status == "processing" {
			s.reconcileProcessing(ctx, row)
			continue
		}
		s.payRow(ctx, row)
	}
}

// payRow claims a pending row and initiates the transfer.
func (s *PayoutService) payRow(ctx context.Context, row repository.PendingPayoutRow) {
	if row.RecipientCode == nil || *row.RecipientCode == "" {
		// Organizer has never submitted bank details. Leave the row pending —
		// it becomes payable the moment they onboard. Surfaced in admin.
		log.Warn().Str("payout_id", row.PayoutID).Str("organizer", row.BusinessName).
			Msg("payouts: skipping — organizer has no bank details on file")
		return
	}

	ref := "po_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	claimed, reference, err := repository.ClaimPayoutForProcessing(s.pool, ctx, row.PayoutID, ref)
	if err != nil {
		log.Error().Err(err).Str("payout_id", row.PayoutID).Msg("payouts: claim failed")
		return
	}
	if !claimed {
		return // raced by another tick / instance
	}

	transfer, err := s.paystack.InitiateTransfer(ctx,
		int64(row.Amount*100), *row.RecipientCode, reference,
		fmt.Sprintf("TicketHawk payout — %s", row.BusinessName))
	if err != nil {
		// "Duplicate reference" means a previous attempt actually went
		// through — reconcile instead of failing.
		if strings.Contains(strings.ToLower(err.Error()), "reference") &&
			strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			s.reconcileByReference(ctx, row.PayoutID, reference)
			return
		}
		log.Error().Err(err).Str("payout_id", row.PayoutID).Msg("payouts: transfer failed")
		_ = repository.MarkPayoutFailed(s.pool, ctx, row.PayoutID, err.Error())
		return
	}

	s.applyTransferStatus(ctx, row.PayoutID, transfer)
}

// reconcileProcessing resolves rows stuck in 'processing' — a crash between
// InitiateTransfer and the DB update, or a transfer that was pending at
// Paystack. Fetches the authoritative status by our reference.
func (s *PayoutService) reconcileProcessing(ctx context.Context, row repository.PendingPayoutRow) {
	if row.TransferRef == nil || *row.TransferRef == "" {
		// Claimed but never sent — safe to fail back for retry.
		_ = repository.MarkPayoutFailed(s.pool, ctx, row.PayoutID, "claimed but transfer never initiated")
		return
	}
	s.reconcileByReference(ctx, row.PayoutID, *row.TransferRef)
}

func (s *PayoutService) reconcileByReference(ctx context.Context, payoutID, reference string) {
	transfer, err := s.paystack.FetchTransfer(ctx, reference)
	if err != nil {
		// Unknown at Paystack → the initiate never landed. Fail for retry.
		_ = repository.MarkPayoutFailed(s.pool, ctx, payoutID, "transfer not found at gateway: "+err.Error())
		return
	}
	s.applyTransferStatus(ctx, payoutID, transfer)
}

func (s *PayoutService) applyTransferStatus(ctx context.Context, payoutID string, t *PaystackTransfer) {
	switch strings.ToLower(t.Status) {
	case "success":
		if err := repository.MarkPayoutPaid(s.pool, ctx, payoutID, t.TransferCode); err != nil {
			log.Error().Err(err).Str("payout_id", payoutID).Msg("payouts: transfer OK but DB update failed")
		} else {
			log.Info().Str("payout_id", payoutID).Str("transfer_code", t.TransferCode).
				Msg("payouts: paid")
		}
	case "failed", "reversed":
		_ = repository.MarkPayoutFailed(s.pool, ctx, payoutID, "gateway status: "+t.Status)
	case "otp":
		// Transfers are waiting for a dashboard OTP — "Disable OTP for
		// Transfers" hasn't been switched on. Loudly actionable.
		log.Error().Str("payout_id", payoutID).
			Msg("payouts: transfer requires OTP — enable 'Disable OTP for Transfers' in the Paystack dashboard")
		// Leave as processing; it resolves once OTP is disabled/approved.
	default: // pending / queued — leave as processing, next tick reconciles.
	}
}
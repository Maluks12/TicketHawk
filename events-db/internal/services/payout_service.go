package services

import (
	"context"
	"math"
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
	pool      *pgxpool.Pool
	paystack  *PaystackGateway
	verifynow *VerifyNow // optional; when set, bank details are ownership-checked at entry
	decryptID func([]byte) (string, error) // optional; recovers full SA ID for the bank check
}

func NewPayoutService(pool *pgxpool.Pool, paystack *PaystackGateway) *PayoutService {
	return &PayoutService{pool: pool, paystack: paystack}
}

// WithVerifyNow attaches the VerifyNow client so SetPayoutDetails runs a
// bank-account ownership check while the full account number is in hand.
// decryptID recovers the organizer's full SA ID (stored encrypted by the KYC
// layer) for the check — pass kycService.Decrypt. Optional: without this,
// bank details save exactly as before (unverified).
func (s *PayoutService) WithVerifyNow(vn *VerifyNow, decryptID func([]byte) (string, error)) *PayoutService {
	s.verifynow = vn
	s.decryptID = decryptID
	return s
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

	// Bank-account ownership check — this is the only point we hold the full
	// account number (SavePayoutDetails keeps only last 4). Verify now, store
	// the result, discard the number. Runs best-effort: a provider failure
	// leaves bank_account_verified = false, which the Tier-2 promotion gate
	// treats as "not yet confirmed" rather than blocking payout setup itself.
	if s.verifynow != nil {
		s.runBankVerification(ctx, organizerID, dto)
	}

	return repository.GetPayoutProfileByOrganizerID(s.pool, ctx, organizerID)
}

// runBankVerification confirms the payout account belongs to the organizer.
// It recovers the full SA ID from the KYC-encrypted column (via the injected
// decryptor); if no verified ID is on file it skips the check and marks the
// account unverified, since VerifyNow needs the ID number to match ownership.
func (s *PayoutService) runBankVerification(ctx context.Context, organizerID string, dto *models.SetPayoutDetailsDTO) {
	var encrypted []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT sa_id_encrypted FROM organizers WHERE id = $1`, organizerID).Scan(&encrypted); err != nil || len(encrypted) == 0 || s.decryptID == nil {
		log.Info().Str("organizer_id", organizerID).
			Msg("payouts: no KYC ID on file — bank ownership check skipped, saved unverified")
		_ = repository.SaveBankVerification(s.pool, ctx, organizerID, false, "", nil)
		return
	}
	idNumber, dErr := s.decryptID(encrypted)
	if dErr != nil {
		log.Warn().Err(dErr).Str("organizer_id", organizerID).Msg("payouts: decrypting ID for bank check")
		_ = repository.SaveBankVerification(s.pool, ctx, organizerID, false, "", nil)
		return
	}

	first, last := splitAccountName(dto.AccountName)
	res, vErr := s.verifynow.VerifyBankAccount(ctx, BankAccountCheckInput{
		FirstName:     first,
		Surname:       last,
		IDNumber:      idNumber,
		AccountNumber: dto.AccountNumber,
		BranchCode:    dto.BankCode,
		AccountType:   dto.AccountType,
	})
	if vErr != nil {
		log.Warn().Err(vErr).Str("organizer_id", organizerID).
			Msg("payouts: bank verification call failed — saved unverified")
		_ = repository.SaveBankVerification(s.pool, ctx, organizerID, false, "", nil)
		return
	}
	if sErr := repository.SaveBankVerification(s.pool, ctx, organizerID, res.Verified, res.TransactionID, res.Raw); sErr != nil {
		log.Warn().Err(sErr).Str("organizer_id", organizerID).Msg("payouts: storing bank verification result")
	}
}

// splitAccountName splits an account holder name into (first, surname) for the
// bank check. Best-effort: last token is the surname, the rest first names.
func splitAccountName(full string) (first, surname string) {
	parts := strings.Fields(full)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
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

	// Net off anything this organiser owes the platform before moving money.
	// Debt accrues when a cancellation refund exceeded the reserve held against
	// it — see RefundService.accrueClawback. Paying out in full while a balance
	// is outstanding would mean the platform never recovers it.
	payable, settled, err := repository.ApplyPayoutDebt(s.pool, ctx, row.OrganizerID, row.PayoutID, row.Amount)
	if err != nil {
		log.Error().Err(err).Str("payout_id", row.PayoutID).
			Msg("payouts: could not apply outstanding debt — skipping row this tick")
		return
	}
	if settled {
		// The debt swallowed the entire payout. The row is closed out as paid
		// with a zero transfer; nothing leaves the platform balance.
		log.Info().Str("payout_id", row.PayoutID).Str("organizer", row.BusinessName).
			Msg("payouts: fully absorbed by outstanding debt, no transfer needed")
		return
	}

	// Overwrite the in-memory amount with what the ledger now says. Skipping
	// this transfers the pre-debt figure while the database records the reduced
	// one — the organiser is overpaid and every internal number still reconciles.
	row.Amount = payable

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
		int64(math.Round(row.Amount*100)), *row.RecipientCode, reference,
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

// ── Tier-2 promotion identity check ──────────────────────────────────────────

// PromotionCheckResult reports what the automated Tier-2 checks found so the
// caller (admin handler) can auto-approve or surface the reason for review.
type PromotionCheckResult struct {
	IDChecked       bool   `json:"id_checked"`        // the DHA check actually ran
	IDFullMatch     bool   `json:"id_full_match"`     // DHA name == legal name
	IDMatchType     string `json:"id_match_type"`     // full_match | partial_match | name_mismatch | id_not_found | …
	BankVerified    bool   `json:"bank_verified"`     // stored result from bank-detail entry
	AutoApprove     bool   `json:"auto_approve"`      // both gates green
	Reason          string `json:"reason,omitempty"`  // why not, when AutoApprove is false
}

// RunPromotionChecks runs the paid DHA ID check for a Tier-2 request and reads
// the stored bank-verification flag. It requires a KYC-verified organizer with
// an ID on file. The DHA result is persisted for audit. AutoApprove is true
// only when the ID is a full name match AND the bank account was verified at
// detail entry.
//
// When no VerifyNow client is configured, IDChecked is false and the caller
// falls back to manual admin judgement (unchanged legacy behaviour).
func (s *PayoutService) RunPromotionChecks(ctx context.Context, organizerID string) (*PromotionCheckResult, error) {
	ident, err := repository.GetPromotionIdentity(s.pool, ctx, organizerID)
	if err != nil {
		return nil, err
	}
	res := &PromotionCheckResult{BankVerified: ident.BankAccountVerified}

	// No provider wired → nothing to auto-run; admin decides manually.
	if s.verifynow == nil || s.decryptID == nil {
		res.Reason = "automated ID check not configured — review manually"
		return res, nil
	}
	if ident.KYCStatus != "verified" {
		res.Reason = "organizer is not KYC-verified yet"
		return res, nil
	}
	if len(ident.IDEncrypted) == 0 {
		res.Reason = "no ID number on file for the DHA check"
		return res, nil
	}

	idNumber, dErr := s.decryptID(ident.IDEncrypted)
	if dErr != nil {
		return res, fmt.Errorf("decrypting ID for DHA check: %w", dErr)
	}

	vr, vErr := s.verifynow.Verify(ctx, VerifyIdentityInput{
		FirstName: ident.LegalFirstName,
		LastName:  ident.LegalLastName,
		IDNumber:  idNumber,
	})
	// Persist the raw result regardless of outcome (audit trail).
	if vr != nil {
		_ = repository.SaveDHAIDVerification(s.pool, ctx, organizerID, vr.FullMatch, vr.Raw)
	}
	if vErr != nil {
		res.Reason = "DHA ID check failed: " + vErr.Error()
		return res, nil
	}

	res.IDChecked = true
	res.IDFullMatch = vr.FullMatch
	res.IDMatchType = vr.MatchType

	switch {
	case !res.BankVerified:
		res.Reason = "bank account was not confirmed as belonging to the organizer"
	case !vr.FullMatch:
		res.Reason = "DHA name did not fully match the organizer's legal name (" + vr.MatchType + ")"
	default:
		res.AutoApprove = true
	}
	return res, nil
}
// ApplyTransferWebhook reconciles an asynchronous transfer outcome pushed by
// Paystack against the payout ledger.
//
// InitiateTransfer returns immediately, often with status "pending" or "otp",
// and the real outcome lands minutes or hours later. Before this existed
// nothing consumed transfer.success / transfer.failed, so a transfer that was
// accepted and then reversed by the receiving bank left the payout row stuck in
// 'processing' forever while the organiser was never paid and nobody knew.
//
// Matching is by transfer_reference — the po_… value the platform generated —
// which is what Paystack echoes back in the webhook.
func (s *PayoutService) ApplyTransferWebhook(ctx context.Context, reference, status, reason string) error {
	if reference == "" {
		return nil
	}

	switch status {
	case "success":
		res, err := s.pool.Exec(ctx, `
			UPDATE organizer_payouts
			SET status = 'paid', paid_at = CURRENT_TIMESTAMP,
			    failure_reason = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE transfer_reference = $1 AND status <> 'paid'
		`, reference)
		if err != nil {
			return fmt.Errorf("marking payout paid from webhook: %w", err)
		}
		if res.RowsAffected() > 0 {
			log.Info().Str("reference", reference).Msg("payouts: confirmed paid via webhook")
		}
		return nil

	default:
		// Back to 'pending', not 'failed': FetchPayableRows picks pending rows
		// up again, and payRow reuses the same transfer_reference, so Paystack
		// dedupes rather than double-paying. A genuinely dead recipient will
		// simply keep failing and stay visible in admin.
		if reason == "" {
			reason = "Transfer " + status + " at gateway"
		}
		res, err := s.pool.Exec(ctx, `
			UPDATE organizer_payouts
			SET status = 'pending', failure_reason = $2, updated_at = CURRENT_TIMESTAMP
			WHERE transfer_reference = $1 AND status <> 'paid'
		`, reference, reason)
		if err != nil {
			return fmt.Errorf("marking payout failed from webhook: %w", err)
		}
		if res.RowsAffected() > 0 {
			log.Warn().Str("reference", reference).Str("reason", reason).
				Msg("payouts: transfer failed at gateway, requeued for retry")
		}
		return nil
	}
}
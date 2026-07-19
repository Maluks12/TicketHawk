package services

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Paystack API surface for the two-tier payout system, riding the existing
// PaystackGateway.do() helper so every call gets the circuit breaker, 4xx
// clientError handling, and envelope decoding for free.
//
//   Subaccounts  — instant tier. Created once per organizer; the code is passed
//                  as `subaccount` on transaction initialize and Paystack
//                  settles the organizer directly.
//   Transfers    — held tier + reserve releases. The platform pays organizers
//                  from its Paystack balance after events complete.
//
// NOTE (Transfers): API-initiated transfers require "Disable OTP for
// Transfers" to be turned on in the Paystack dashboard (Settings →
// Preferences), otherwise every transfer waits for a dashboard OTP. Do this
// once on the live account before launch.

// ── Banks ─────────────────────────────────────────────────────────────────────

type PaystackBank struct {
	Name string `json:"name"`
	Code string `json:"code"`
	Slug string `json:"slug"`
}

// bankCache: the ZA bank list changes ~never; cache it in-process for a day so
// the organizer-facing bank picker doesn't hammer Paystack.
var (
	bankCacheMu      sync.Mutex
	bankCacheValue   []PaystackBank
	bankCacheExpires time.Time
)

// ListBanks returns South African settlement banks (currency=ZAR).
func (g *PaystackGateway) ListBanks(ctx context.Context) ([]PaystackBank, error) {
	bankCacheMu.Lock()
	if time.Now().Before(bankCacheExpires) && len(bankCacheValue) > 0 {
		cached := bankCacheValue
		bankCacheMu.Unlock()
		return cached, nil
	}
	bankCacheMu.Unlock()

	var banks []PaystackBank
	if err := g.do(ctx, http.MethodGet, "/bank?currency=ZAR&perPage=100", nil, &banks); err != nil {
		return nil, fmt.Errorf("paystack list banks: %w", err)
	}

	bankCacheMu.Lock()
	bankCacheValue = banks
	bankCacheExpires = time.Now().Add(24 * time.Hour)
	bankCacheMu.Unlock()
	return banks, nil
}

// ── Subaccounts (instant tier) ────────────────────────────────────────────────

type paystackSubaccountRequest struct {
	BusinessName   string `json:"business_name"`
	SettlementBank string `json:"settlement_bank"` // bank code
	AccountNumber  string `json:"account_number"`
	// PercentageCharge is required by the API but is overridden on every
	// transaction by transaction_charge (our flat platform cut + reserve), so
	// its value never determines an actual split. Set to 0 defensively: if a
	// code path ever forgets transaction_charge, the organizer receives the
	// full amount and the platform loses its fee — an error in the safe
	// direction (never over-charging the organizer).
	PercentageCharge    float64 `json:"percentage_charge"`
	PrimaryContactEmail string  `json:"primary_contact_email,omitempty"`
	PrimaryContactName  string  `json:"primary_contact_name,omitempty"`
	Description         string  `json:"description,omitempty"`
}

type PaystackSubaccount struct {
	SubaccountCode string `json:"subaccount_code"`
	BusinessName   string `json:"business_name"`
	AccountNumber  string `json:"account_number"`
	SettlementBank string `json:"settlement_bank"`
	Active         bool   `json:"active"`
}

// CreateSubaccount registers an organizer's bank account with Paystack and
// returns the ACCT_xxx code used to split transactions to them.
// Paystack validates the account number format against the bank; it does NOT
// verify the account holder's name for ZA banks, so we store what the
// organizer typed and surface it for admin review.
func (g *PaystackGateway) CreateSubaccount(ctx context.Context, businessName, bankCode, accountNumber, contactName, contactEmail string) (*PaystackSubaccount, error) {
	var sub PaystackSubaccount
	err := g.do(ctx, http.MethodPost, "/subaccount", paystackSubaccountRequest{
		BusinessName:        businessName,
		SettlementBank:      bankCode,
		AccountNumber:       accountNumber,
		PercentageCharge:    0,
		PrimaryContactEmail: contactEmail,
		PrimaryContactName:  contactName,
		Description:         "TicketHawk organizer settlement account",
	}, &sub)
	if err != nil {
		return nil, fmt.Errorf("paystack create subaccount: %w", err)
	}
	if sub.SubaccountCode == "" {
		return nil, fmt.Errorf("paystack returned empty subaccount_code")
	}
	return &sub, nil
}

// UpdateSubaccount changes the settlement bank details on an existing
// subaccount — used when an organizer switches banks. The ACCT code is stable.
func (g *PaystackGateway) UpdateSubaccount(ctx context.Context, subaccountCode, businessName, bankCode, accountNumber string) (*PaystackSubaccount, error) {
	var sub PaystackSubaccount
	err := g.do(ctx, http.MethodPut, "/subaccount/"+url.PathEscape(subaccountCode), paystackSubaccountRequest{
		BusinessName:     businessName,
		SettlementBank:   bankCode,
		AccountNumber:    accountNumber,
		PercentageCharge: 0,
	}, &sub)
	if err != nil {
		return nil, fmt.Errorf("paystack update subaccount %s: %w", subaccountCode, err)
	}
	return &sub, nil
}

// ── Transfer recipients + transfers (held tier & reserve releases) ────────────

type paystackRecipientRequest struct {
	Type          string `json:"type"` // "basa" = Bank Account South Africa
	Name          string `json:"name"`
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
	Currency      string `json:"currency"`
}

type paystackRecipientData struct {
	RecipientCode string `json:"recipient_code"`
	Active        bool   `json:"active"`
}

// CreateTransferRecipient registers the organizer's bank account as a transfer
// destination and returns the RCP_xxx code. Idempotent on Paystack's side for
// identical details, so calling it again after a bank change is safe.
func (g *PaystackGateway) CreateTransferRecipient(ctx context.Context, name, bankCode, accountNumber string) (string, error) {
	var data paystackRecipientData
	err := g.do(ctx, http.MethodPost, "/transferrecipient", paystackRecipientRequest{
		Type:          "basa",
		Name:          name,
		AccountNumber: accountNumber,
		BankCode:      bankCode,
		Currency:      "ZAR",
	}, &data)
	if err != nil {
		return "", fmt.Errorf("paystack create transfer recipient: %w", err)
	}
	if data.RecipientCode == "" {
		return "", fmt.Errorf("paystack returned empty recipient_code")
	}
	return data.RecipientCode, nil
}

type paystackTransferRequest struct {
	Source    string `json:"source"` // always "balance"
	Amount    int64  `json:"amount"` // minor unit (cents)
	Recipient string `json:"recipient"`
	Reason    string `json:"reason,omitempty"`
	Currency  string `json:"currency"`
	Reference string `json:"reference"` // OUR idempotency key — Paystack rejects duplicates
}

type PaystackTransfer struct {
	TransferCode string `json:"transfer_code"`
	Status       string `json:"status"` // success | pending | otp | failed | reversed
	Reference    string `json:"reference"`
	Amount       int64  `json:"amount"`
}

// InitiateTransfer pays an organizer from the platform's Paystack balance.
// reference is the caller-supplied idempotency key: retrying with the same
// reference can never double-pay — Paystack returns the original transfer or
// a duplicate error, both handled upstream.
func (g *PaystackGateway) InitiateTransfer(ctx context.Context, amountCents int64, recipientCode, reference, reason string) (*PaystackTransfer, error) {
	var t PaystackTransfer
	err := g.do(ctx, http.MethodPost, "/transfer", paystackTransferRequest{
		Source:    "balance",
		Amount:    amountCents,
		Recipient: recipientCode,
		Reason:    reason,
		Currency:  "ZAR",
		Reference: reference,
	}, &t)
	if err != nil {
		return nil, fmt.Errorf("paystack initiate transfer: %w", err)
	}
	return &t, nil
}

// FetchTransfer re-fetches a transfer's authoritative status by our reference.
// Used to reconcile 'processing' payouts (e.g. after a crash mid-transfer).
func (g *PaystackGateway) FetchTransfer(ctx context.Context, reference string) (*PaystackTransfer, error) {
	var t PaystackTransfer
	if err := g.do(ctx, http.MethodGet, "/transfer/verify/"+url.PathEscape(reference), nil, &t); err != nil {
		return nil, fmt.Errorf("paystack fetch transfer: %w", err)
	}
	return &t, nil
}
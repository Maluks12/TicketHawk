package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"events-db/internal/models"

	"github.com/google/uuid"
)

// VerifyNow is a South-Africa-focused verification provider (verifynow.co.za).
// We use two of its checks:
//
//	said_verification         — validates an SA ID against Home Affairs and
//	                            returns the registered name. Used at Tier-2
//	                            promotion; a name match auto-approves the tier.
//	bank-account-verification — confirms a bank account belongs to a named
//	                            person. Used at bank-detail entry, the only
//	                            moment we hold the full account number.
//
// Both are billed per call (credits). Sandbox mode returns mock data at zero
// credits and is used for wiring/tests — flip VERIFYNOW_MODE to "production"
// (and fund the account) to go live.
//
// The DHA ID adapter implements the existing IDVerifier interface so it drops
// into the same slot the Didit adapter used. Bank verification is a separate
// method: it needs bank fields that aren't part of an identity check.
type VerifyNow struct {
	apiKey  string
	baseURL string
	mode    string // "sandbox" | "production"
	client  *http.Client
}

// NewVerifyNow builds the client. mode defaults to "sandbox" when unset so a
// misconfiguration can never silently spend real credits.
func NewVerifyNow(apiKey, mode string) *VerifyNow {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m != "production" {
		m = "sandbox"
	}
	return &VerifyNow{
		apiKey:  apiKey,
		baseURL: "https://www.verifynow.co.za/api/external",
		mode:    m,
		client:  &http.Client{Timeout: 25 * time.Second},
	}
}

func (v *VerifyNow) Name() string { return "verifynow" }
func (v *VerifyNow) Mode() string { return v.mode }

// post issues a JSON request with the api key, an idempotency key (required in
// production, harmless in sandbox), and returns the raw body for the caller to
// parse. Non-2xx is surfaced as an error carrying the status.
func (v *VerifyNow) post(ctx context.Context, path string, payload map[string]any) ([]byte, error) {
	payload["mode"] = v.mode
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("verifynow: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("verifynow: new request: %w", err)
	}
	req.Header.Set("x-api-key", v.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("verifynow: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("verifynow: read body: %w", err)
	}
	if resp.StatusCode == http.StatusPaymentRequired {
		return raw, fmt.Errorf("verifynow: insufficient credits (top up at verifynow.co.za)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("verifynow: http %d: %s", resp.StatusCode, truncate(raw, 200))
	}
	return raw, nil
}

// ── DHA ID check (implements IDVerifier) ─────────────────────────────────────

type vnSAIDResponse struct {
	Success bool `json:"success"`
	Results struct {
		SAID struct {
			Status          string `json:"Status"`
			RealTimeResults struct {
				Status       string `json:"Status"`
				Verification struct {
					Firstnames  string `json:"Firstnames"`
					Lastname    string `json:"Lastname"`
					Dob         string `json:"Dob"`
					Citizenship string `json:"Citizenship"`
				} `json:"Verification"`
				TransactionID string `json:"transaction_id"`
			} `json:"realTimeResults"`
		} `json:"said_verification"`
	} `json:"results"`
}

// Verify runs said_verification and decides the match by comparing the
// Home-Affairs-returned name against the submitted legal name. VerifyNow's
// said_verification returns the DHA name for the ID number, so "full match"
// means the ID is real AND the name the organizer gave us matches the record.
func (v *VerifyNow) Verify(ctx context.Context, in VerifyIdentityInput) (*VerifyIdentityResult, error) {
	raw, err := v.post(ctx, "/verify", map[string]any{
		"reportType": "said_verification",
		"idNumber":   in.IDNumber,
	})
	if err != nil {
		// Persist whatever came back for audit even on transport failure.
		return &VerifyIdentityResult{Conclusive: false, MatchType: "error", Raw: raw}, err
	}

	var parsed vnSAIDResponse
	if jErr := json.Unmarshal(raw, &parsed); jErr != nil {
		return &VerifyIdentityResult{Conclusive: false, MatchType: "unparseable", Raw: raw}, nil
	}

	rt := parsed.Results.SAID.RealTimeResults
	idValid := parsed.Success &&
		strings.EqualFold(parsed.Results.SAID.Status, "Success") &&
		strings.Contains(strings.ToLower(rt.Status), "valid")

	if !idValid {
		// ID didn't resolve at Home Affairs at all.
		return &VerifyIdentityResult{Conclusive: true, FullMatch: false, MatchType: "id_not_found", Raw: raw}, nil
	}

	// DHA-specific name match. Home Affairs commonly returns MORE names than
	// the organizer entered (e.g. DHA "NALEDI LERATO KHUMALO" vs entered
	// "Naledi Khumalo"). For auto-approve we treat the organizer's name as a
	// match when every name they gave appears in the DHA record AND the
	// surname matches exactly — i.e. their name is a subset of the fuller
	// official record. That's the real-world "same person" test; requiring an
	// exact token-set match would send almost everyone to manual review.
	res := &VerifyIdentityResult{Conclusive: true, Raw: raw}
	switch dhaNameMatch(rt.Verification.Firstnames, rt.Verification.Lastname, in.FirstName, in.LastName) {
	case models.NameMatchFull:
		res.FullMatch = true
		res.MatchType = "full_match"
	case models.NameMatchPartial:
		res.MatchType = "partial_match"
	default:
		res.MatchType = "name_mismatch"
	}
	return res, nil
}

// dhaNameMatch grades the organizer's entered name against the fuller DHA
// record. Full: exact surname match + every entered first-name token present
// in the DHA first names (subset, order-independent). Partial: surname matches
// but a first name doesn't line up. None: surname mismatch.
func dhaNameMatch(dhaFirst, dhaLast, gotFirst, gotLast string) models.NameMatchResult {
	norm := func(s string) []string { return strings.Fields(strings.ToUpper(strings.TrimSpace(s))) }
	dhaF, dhaL := norm(dhaFirst), strings.ToUpper(strings.TrimSpace(dhaLast))
	gotF, gotL := norm(gotFirst), strings.ToUpper(strings.TrimSpace(gotLast))

	if dhaL == "" || gotL == "" || dhaL != gotL {
		return models.NameMatchNone
	}
	// Every entered first-name token must appear in the DHA first names.
	inDHA := make(map[string]bool, len(dhaF))
	for _, t := range dhaF {
		inDHA[t] = true
	}
	for _, t := range gotF {
		if !inDHA[t] {
			return models.NameMatchPartial // surname ok, a first name diverges
		}
	}
	if len(gotF) == 0 {
		return models.NameMatchPartial // surname only, no first name to confirm
	}
	return models.NameMatchFull
}

// ── Bank-account ownership check ─────────────────────────────────────────────

// BankAccountCheckInput carries everything VerifyNow needs to confirm an
// account belongs to a named individual.
type BankAccountCheckInput struct {
	FirstName     string
	Surname       string
	IDNumber      string
	AccountNumber string
	BranchCode    string
	AccountType   string // "Savings" | "Current" | "Transmission" | ...
}

// BankAccountResult is the stored outcome — never the account number.
type BankAccountResult struct {
	Verified      bool
	TransactionID string
	Raw           json.RawMessage
}

type vnBankResponse struct {
	Success string `json:"-"`
	Results struct {
		Bank map[string]any `json:"-"`
	} `json:"-"`
}

// VerifyBankAccount confirms account ownership. VerifyNow's response shape for
// this endpoint varies by bank, so we parse defensively: success + an
// account-verified/accountExists-style truthy signal anywhere in the result
// block. Anything ambiguous is treated as unverified (fail closed) and the
// raw response is returned for the admin/audit trail.
func (v *VerifyNow) VerifyBankAccount(ctx context.Context, in BankAccountCheckInput) (*BankAccountResult, error) {
	accType := in.AccountType
	if accType == "" {
		accType = "Savings"
	}
	raw, err := v.post(ctx, "/bank-account-verification", map[string]any{
		"type":              "Individual",
		"firstName":         in.FirstName,
		"surname":           in.Surname,
		"identityNumber":    in.IDNumber,
		"identityType":      "IDNumber",
		"bankAccountNumber": in.AccountNumber,
		"bankBranchCode":    in.BranchCode,
		"bankAccountType":   accType,
	})
	if err != nil {
		return &BankAccountResult{Verified: false, Raw: raw}, err
	}

	// VerifyNow returns a purpose-built verdict at
	// results.identity_and_account_verified (bool), with a field-level
	// breakdown under results.verification_results (nameMatch, identityMatch,
	// accountFound, … each "Yes"/"No"). We trust the boolean as the primary
	// signal; the truthy walker stays as a fallback in case the field is
	// absent for some bank/response variant. Fail closed on ambiguity.
	var envelope struct {
		Success       bool   `json:"success"`
		TransactionID string `json:"requestId"`
		Results       struct {
			IdentityAndAccountVerified *bool          `json:"identity_and_account_verified"`
			VerificationResults        map[string]any `json:"verification_results"`
			Raw                        map[string]any `json:"-"`
		} `json:"results"`
	}
	if jErr := json.Unmarshal(raw, &envelope); jErr != nil {
		return &BankAccountResult{Verified: false, Raw: raw}, nil
	}

	verified := false
	switch {
	case envelope.Results.IdentityAndAccountVerified != nil:
		// Primary path: the provider's own verdict.
		verified = envelope.Success && *envelope.Results.IdentityAndAccountVerified
	default:
		// Fallback: no explicit verdict field — scan the breakdown for a
		// truthy ownership/name-match signal.
		verified = envelope.Success && bankResultLooksVerified(envelope.Results.VerificationResults)
	}

	return &BankAccountResult{
		Verified:      verified,
		TransactionID: envelope.TransactionID,
		Raw:           raw,
	}, nil
}

// bankResultLooksVerified walks the results object looking for a truthy
// account-ownership signal under any of the common key spellings. Conservative
// by design: unknown shape → not verified.
func bankResultLooksVerified(results map[string]any) bool {
	if results == nil {
		return false
	}
	positive := map[string]bool{
		"accountverified": true, "accountexists": true, "verified": true,
		"accountidmatch": true, "idmatch": true, "match": true,
		"accountopen": true, "accountactive": true, "canaccept": true,
	}
	var walk func(v any) bool
	walk = func(v any) bool {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				key := strings.ToLower(strings.ReplaceAll(k, "_", ""))
				if positive[key] && isTruthy(val) {
					return true
				}
				if walk(val) {
					return true
				}
			}
		case []any:
			for _, item := range t {
				if walk(item) {
					return true
				}
			}
		}
		return false
	}
	return walk(results)
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "y" || s == "yes" || s == "true" || s == "matched" || s == "valid" || s == "success" || s == "1"
	case float64:
		return t == 1
	}
	return false
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
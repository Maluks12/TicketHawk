package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// IDVerifier abstracts identity verification against an authoritative source.
// Two implementations ship:
//
//	ManualVerifier — the R0 path. Returns inconclusive; the SA ID checksum
//	                 runs at submission and a human approves in the admin
//	                 KYC queue.
//	DiditVerifier  — Didit's DHA database validation (500 free checks/month,
//	                 then $1.10 per conclusive query). A full match
//	                 auto-verifies the organizer; anything else falls back to
//	                 the manual review queue.
//
// Switching providers later is config, not code: implement this interface
// and swap the factory.
type IDVerifier interface {
	Name() string
	// Verify checks the identity against the provider. A nil error with
	// Conclusive=false means "provider couldn't decide" (route to manual
	// review); an error means the call itself failed (also route to manual).
	Verify(ctx context.Context, in VerifyIdentityInput) (*VerifyIdentityResult, error)
}

type VerifyIdentityInput struct {
	FirstName   string
	LastName    string
	DateOfBirth time.Time
	IDNumber    string
}

type VerifyIdentityResult struct {
	Conclusive bool            // provider returned a definitive answer
	FullMatch  bool            // identity confirmed at the source
	MatchType  string          // provider's normalized verdict (e.g. full_match)
	Raw        json.RawMessage // full provider response, persisted for audit
}

// NewIDVerifier picks the DHA ID-check implementation from config, in order
// of preference: VerifyNow (SA-local, cheaper) when its key is set, then
// Didit, then manual review. VerifyNow needs its mode ("sandbox"/"production")
// too; sandbox costs zero credits.
func NewIDVerifier(verifyNowKey, verifyNowMode, diditAPIKey string) IDVerifier {
	if strings.TrimSpace(verifyNowKey) != "" {
		return NewVerifyNow(verifyNowKey, verifyNowMode)
	}
	if strings.TrimSpace(diditAPIKey) != "" {
		return NewDiditVerifier(diditAPIKey)
	}
	return &ManualVerifier{}
}

// ── Manual ───────────────────────────────────────────────────────────────────

type ManualVerifier struct{}

func (m *ManualVerifier) Name() string { return "manual" }

func (m *ManualVerifier) Verify(ctx context.Context, in VerifyIdentityInput) (*VerifyIdentityResult, error) {
	return &VerifyIdentityResult{Conclusive: false, MatchType: "manual_review"}, nil
}

// ── Didit (DHA registry) ─────────────────────────────────────────────────────

const diditEndpoint = "https://verification.didit.me/v3/database-validation/"

type DiditVerifier struct {
	apiKey string
	client *http.Client
}

func NewDiditVerifier(apiKey string) *DiditVerifier {
	return &DiditVerifier{
		apiKey: apiKey,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (d *DiditVerifier) Name() string { return "didit" }

type diditResponse struct {
	RequestID   string `json:"request_id"`
	Status      string `json:"status"`     // "Approved" on conclusive success
	MatchType   string `json:"match_type"` // e.g. "full_match"
	Validations []struct {
		OutcomeCode string            `json:"outcome_code"` // "MATCH" etc.
		ServiceID   string            `json:"service_id"`
		Validation  map[string]string `json:"validation"` // per-field verdicts
	} `json:"validations"`
}

// Verify calls Didit's database-validation endpoint with the ZA DHA service.
// Multipart form per their API: issuing_state=ZAF, services=zaf_africa_national_id.
func (d *DiditVerifier) Verify(ctx context.Context, in VerifyIdentityInput) (*VerifyIdentityResult, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := map[string]string{
		"issuing_state": "ZAF",
		"services":      "zaf_africa_national_id",
		"first_name":    in.FirstName,
		"last_name":     in.LastName,
		"date_of_birth": in.DateOfBirth.Format("2006-01-02"),
		"national_id":   in.IDNumber,
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("didit: building request: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("didit: building request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, diditEndpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("didit: creating request: %w", err)
	}
	req.Header.Set("x-api-key", d.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("didit: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("didit: reading response: %w", err)
	}

	// Non-2xx (source downtime, pre-source rejection) isn't billed and isn't
	// conclusive — persist what we got and route to manual review.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &VerifyIdentityResult{
			Conclusive: false,
			MatchType:  fmt.Sprintf("http_%d", resp.StatusCode),
			Raw:        json.RawMessage(raw),
		}, nil
	}

	var parsed diditResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return &VerifyIdentityResult{Conclusive: false, MatchType: "unparseable", Raw: json.RawMessage(raw)}, nil
	}

	res := &VerifyIdentityResult{
		Conclusive: strings.EqualFold(parsed.Status, "Approved") && parsed.MatchType != "",
		MatchType:  parsed.MatchType,
		Raw:        json.RawMessage(raw),
	}
	res.FullMatch = res.Conclusive && strings.EqualFold(parsed.MatchType, "full_match")
	return res, nil
}
package services

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ── Play Integrity verifier ───────────────────────────────────────────────────
//
// Decodes and evaluates a Play Integrity token by calling Google's
// decodeIntegrityToken endpoint. Server-side decoding is used rather than local
// decryption because the decryption keys would otherwise have to live in the
// API's environment, and a leaked set lets an attacker mint verdicts — which
// defeats the whole point of asking.
//
// NO NEW MODULE DEPENDENCY. Service-account authentication is implemented here
// (a signed JWT exchanged for an access token) rather than pulling in
// google.golang.org/api. That library brings a large transitive tree into the
// payments trust boundary for roughly a hundred lines of well-specified OAuth,
// and the same reasoning applied to AES-CMAC in Phase 1.
//
// SETUP. In Google Play Console, enable Play Integrity for the Studio app and
// link a Google Cloud project. Create a service account with the Play Integrity
// API enabled, download its JSON key, and set:
//
//	PLAY_INTEGRITY_SA_JSON   path to the service-account JSON key file
//	PLAY_INTEGRITY_PACKAGE   com.tickethawk.studio
//
// The service-account key is a credential that can mint integrity verdicts for
// your app. Treat it like TAG_MASTER_KEY.

const (
	googleTokenURL     = "https://oauth2.googleapis.com/token"
	playIntegrityScope = "https://www.googleapis.com/auth/playintegrity"
	playIntegrityAPI   = "https://playintegrity.googleapis.com/v1"
)

var (
	ErrIntegrityNotConfigured = errors.New("play integrity is not configured")
	ErrIntegrityDecodeFailed  = errors.New("play integrity token could not be decoded")
)

// serviceAccount is the subset of a Google service-account JSON key we need.
type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// PlayIntegrityVerifier calls Google's decodeIntegrityToken endpoint.
type PlayIntegrityVerifier struct {
	sa          serviceAccount
	key         *rsa.PrivateKey
	packageName string
	http        *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewPlayIntegrityVerifier loads the service account and parses its key.
//
// Fails at construction rather than on first use, for the same reason
// NewTagCryptoService does: a misconfiguration should stop the process at boot,
// not surface as a till that cannot enrol on a Friday night.
func NewPlayIntegrityVerifier(saJSONPath, packageName string) (*PlayIntegrityVerifier, error) {
	if saJSONPath == "" || packageName == "" {
		return nil, fmt.Errorf("%w: PLAY_INTEGRITY_SA_JSON and PLAY_INTEGRITY_PACKAGE are both required", ErrIntegrityNotConfigured)
	}
	raw, err := readFileFn(saJSONPath)
	if err != nil {
		return nil, fmt.Errorf("reading Play Integrity service account: %w", err)
	}
	var sa serviceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("parsing service account JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("service account JSON is missing client_email or private_key")
	}

	key, err := parsePKCS8RSA(sa.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("service account private key: %w", err)
	}
	if sa.TokenURI == "" {
		sa.TokenURI = googleTokenURL
	}

	return &PlayIntegrityVerifier{
		sa: sa, key: key, packageName: packageName,
		http: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// integrityResponse is the shape of decodeIntegrityToken's reply.
type integrityResponse struct {
	TokenPayloadExternal struct {
		RequestDetails struct {
			RequestPackageName string `json:"requestPackageName"`
			RequestHash        string `json:"requestHash"`
			TimestampMillis    string `json:"timestampMillis"`
		} `json:"requestDetails"`
		AppIntegrity struct {
			AppRecognitionVerdict string `json:"appRecognitionVerdict"`
			PackageName           string `json:"packageName"`
		} `json:"appIntegrity"`
		DeviceIntegrity struct {
			DeviceRecognitionVerdict []string `json:"deviceRecognitionVerdict"`
		} `json:"deviceIntegrity"`
	} `json:"tokenPayloadExternal"`
}

// Verify decodes a token and normalises Google's verdict strings into claims.
//
// The nonce comes back as requestHash, which is where the device put it. It is
// returned rather than compared here: EvaluateAttestation owns the comparison,
// so the policy decision stays in one pure, tested place.
func (v *PlayIntegrityVerifier) Verify(ctx context.Context, token string) (*IntegrityClaims, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: empty token", ErrIntegrityDecodeFailed)
	}

	accessToken, err := v.accessTokenFor(ctx)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string]string{"integrityToken": token})
	endpoint := fmt.Sprintf("%s/%s:decodeIntegrityToken", playIntegrityAPI, url.PathEscape(v.packageName))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIntegrityDecodeFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: google returned %d", ErrIntegrityDecodeFailed, resp.StatusCode)
	}

	var out integrityResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIntegrityDecodeFailed, err)
	}

	return normaliseIntegrityClaims(&out), nil
}

// normaliseIntegrityClaims turns Google's verdict strings into our claims.
//
// Extracted from Verify so the mapping is unit-testable without a network call.
// It is the part most likely to be wrong, because the verdict vocabulary is a
// list of strings whose absence means failure.
func normaliseIntegrityClaims(out *integrityResponse) *IntegrityClaims {
	p := out.TokenPayloadExternal
	claims := &IntegrityClaims{
		Nonce:         p.RequestDetails.RequestHash,
		PackageName:   p.AppIntegrity.PackageName,
		AppRecognised: p.AppIntegrity.AppRecognitionVerdict == "PLAY_RECOGNIZED",
		IssuedAt:      millisToTime(p.RequestDetails.TimestampMillis),
	}

	// deviceRecognitionVerdict is a LIST, and an EMPTY list is the signal for a
	// device that failed every check — rooted, emulated, or unrecognised. A
	// verifier that merely checked the field was present would read that as a
	// pass, which is the single most dangerous misreading available here.
	for _, verdict := range p.DeviceIntegrity.DeviceRecognitionVerdict {
		switch verdict {
		case "MEETS_DEVICE_INTEGRITY":
			claims.DeviceIntegrity = true
		case "MEETS_BASIC_INTEGRITY":
			claims.BasicIntegrity = true
		case "MEETS_STRONG_INTEGRITY":
			claims.DeviceIntegrity = true
			claims.BasicIntegrity = true
		}
	}
	// MEETS_DEVICE_INTEGRITY implies the basic tier. Google does not always send
	// both, and requiring the pair literally would fail good devices.
	if claims.DeviceIntegrity {
		claims.BasicIntegrity = true
	}

	// Note what is NOT decided here: whether the package matches, whether the
	// nonce is ours, or whether the token is fresh. Those are policy, and they
	// live in EvaluateAttestation so there is exactly one place that decides
	// what a verdict means.
	return claims
}

// ── Service-account OAuth ────────────────────────────────────────────────────

// accessTokenFor returns a cached access token, refreshing when it is close to
// expiry. Cached because a busy enrolment window would otherwise mint one token
// exchange per till, and Google rate-limits that.
func (v *PlayIntegrityVerifier) accessTokenFor(ctx context.Context) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// 60s of slack so a token cannot expire between this check and the call.
	if v.accessToken != "" && time.Now().Add(60*time.Second).Before(v.tokenExpiry) {
		return v.accessToken, nil
	}

	now := time.Now()
	assertion, err := v.signedAssertion(now)
	if err != nil {
		return "", err
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.sa.TokenURI,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("google token exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google token exchange returned %d", resp.StatusCode)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", errors.New("google token exchange returned no access token")
	}

	v.accessToken = tok.AccessToken
	v.tokenExpiry = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	return v.accessToken, nil
}

// signedAssertion builds the RS256 JWT that is exchanged for an access token.
func (v *PlayIntegrityVerifier) signedAssertion(now time.Time) (string, error) {
	header := base64URL([]byte(`{"alg":"RS256","typ":"JWT"}`))

	claims, err := json.Marshal(map[string]any{
		"iss":   v.sa.ClientEmail,
		"scope": playIntegrityScope,
		"aud":   v.sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(), // Google's maximum
	})
	if err != nil {
		return "", err
	}

	signingInput := header + "." + base64URL(claims)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, v.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing service-account assertion: %w", err)
	}
	return signingInput + "." + base64URL(sig), nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func base64URL(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func parsePKCS8RSA(pemStr string) (*rsa.PrivateKey, error) {
	// Service-account JSON escapes newlines; unescape before PEM decoding.
	pemStr = strings.ReplaceAll(pemStr, "\\n", "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("private key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Older keys are PKCS#1.
		if k, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
			return k, nil
		}
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("service account key is not RSA")
	}
	return key, nil
}

func millisToTime(ms string) time.Time {
	if ms == "" {
		return time.Time{}
	}
	var n int64
	if _, err := fmt.Sscanf(ms, "%d", &n); err != nil || n <= 0 {
		// Zero time is treated as stale by EvaluateAttestation, which is the
		// safe reading of "Google did not tell us when this was issued".
		return time.Time{}
	}
	return time.UnixMilli(n)
}

// readFileFn is a seam so tests can supply a service account without touching
// the filesystem.
var readFileFn = defaultReadFile

func defaultReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
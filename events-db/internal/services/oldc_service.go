package services

import (
	"context"
	"crypto/rsa"
	"encoding/base64"

	//"encoding/big"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/bits"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

// ─── Why this file is hand-rolled ────────────────────────────────────────────
//
// The usual answer is github.com/coreos/go-oidc. It is a good library, but it
// pulls in a transitive tree (golang.org/x/oauth2 and friends) into a service
// that handles card payments, and everything it does that we need is ~200 lines
// of standard-library code. Verifying an ID token is: fetch the provider's
// public keys, check the signature, check four claims. There is no OAuth dance
// on the server — the CLIENT does the interactive part and hands us the
// resulting ID token, which is why this is the whole server-side surface.
//
// ─── What an attacker would try, and what stops it ───────────────────────────
//
// 1. Send a token they signed themselves        → signature check against JWKS
// 2. Send a valid Google token issued to a       → audience check; this is the
//    DIFFERENT app ("token substitution")           one people forget, and it
//                                                   lets any app's token in
// 3. Send a token with alg:none                  → explicit alg allow-list
// 4. Replay a token captured earlier             → exp/iat check + nonce
// 5. Claim someone else's email                  → email is never a lookup key
//                                                   unless email_verified
//
// Item 2 is the important one. Google will happily verify a token from some
// other developer's project; it is the `aud` claim that says it was minted for
// US. Skipping it means anyone who can get a Google token from any app can
// impersonate any TicketHawk user by email.

const (
	googleIssuerPrimary = "https://accounts.google.com"
	googleIssuerAlt     = "accounts.google.com" // Google emits both spellings
	googleJWKSURL       = "https://www.googleapis.com/oauth2/v3/certs"

	appleIssuer  = "https://appleid.apple.com"
	appleJWKSURL = "https://appleid.apple.com/auth/keys"
)

// SocialProvider identifies which provider minted an ID token.
type SocialProvider string

const (
	ProviderGoogle SocialProvider = "google"
	ProviderApple  SocialProvider = "apple"
)

// VerifiedIdentity is the trusted subset of an ID token: everything here has
// survived signature and claim verification.
type VerifiedIdentity struct {
	Provider      SocialProvider
	Subject       string // `sub` — stable, per-app, the ONLY safe lookup key
	Email         string
	EmailVerified bool
	FirstName     string // best-effort; Apple never sends it in the token
	LastName      string
	IsPrivateRelay bool
}

// ─── JWKS cache ──────────────────────────────────────────────────────────────
//
// Both providers rotate signing keys. Fetching per sign-in would put a
// synchronous third-party HTTP call in the login path — a Google outage would
// become a TicketHawk outage — so keys are cached and refreshed lazily.
//
// The cache is deliberately NOT invalidated on every miss: an unknown `kid` is
// what a rotation looks like, but it is also what a forged token looks like, so
// an attacker sending random kids could otherwise force unbounded refetches.
// One refresh per minute at most, per provider.

type jwksCache struct {
	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
	url         string
}

var (
	googleKeys = &jwksCache{url: googleJWKSURL, keys: map[string]*rsa.PublicKey{}}
	appleKeys  = &jwksCache{url: appleJWKSURL, keys: map[string]*rsa.PublicKey{}}

	jwksTTL          = 6 * time.Hour
	jwksMinRefetch   = 1 * time.Minute
	jwksHTTPTimeout  = 10 * time.Second
)

type jwkSet struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (c *jwksCache) get(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	fresh := time.Since(c.fetchedAt) < jwksTTL
	c.mu.RUnlock()

	if ok && fresh {
		return key, nil
	}

	if err := c.refresh(ctx); err != nil {
		// A refresh failure with a usable cached key is survivable — serve the
		// stale key rather than locking every user out because Google's CDN
		// blipped. Keys are valid long after they stop being advertised.
		if ok {
			log.Warn().Err(err).Str("jwks", c.url).Msg("JWKS refresh failed, using cached key")
			return key, nil
		}
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("no signing key %q at %s", kid, c.url)
}

func (c *jwksCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	if time.Since(c.lastAttempt) < jwksMinRefetch {
		c.mu.Unlock()
		return errors.New("jwks refresh throttled")
	}
	c.lastAttempt = time.Now()
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, jwksHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks %s returned %d", c.url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var set jwkSet
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("parse jwks: %w", err)
	}

	parsed := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		if e == 0 || bits.Len(uint(e)) > 31 {
			continue
		}
		parsed[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}

	if len(parsed) == 0 {
		return fmt.Errorf("jwks %s contained no usable RSA keys", c.url)
	}

	c.mu.Lock()
	c.keys = parsed
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}

// ─── Verification ────────────────────────────────────────────────────────────

// OIDCVerifier holds the audiences we accept. One process, many client IDs:
// the iOS app, the Android app and the web app each have their own Google
// client ID, and Apple issues tokens audienced to the bundle ID on iOS but to
// the Services ID on web.
type OIDCVerifier struct {
	GoogleAudiences []string
	AppleAudiences  []string
}

func NewOIDCVerifier(googleAudiences, appleAudiences []string) *OIDCVerifier {
	return &OIDCVerifier{
		GoogleAudiences: normaliseList(googleAudiences),
		AppleAudiences:  normaliseList(appleAudiences),
	}
}

func normaliseList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Enabled reports whether a provider is configured. Used so the API can return
// a clear 501 instead of a confusing 401 when the env vars are missing.
func (v *OIDCVerifier) Enabled(p SocialProvider) bool {
	switch p {
	case ProviderGoogle:
		return len(v.GoogleAudiences) > 0
	case ProviderApple:
		return len(v.AppleAudiences) > 0
	}
	return false
}

type oidcClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"` // Google sends bool, Apple sends "true"
	Nonce         string `json:"nonce"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Name          string `json:"name"`
	IsPrivateEmail any   `json:"is_private_email"` // Apple only, also stringly-typed
}

// Verify checks an ID token end to end and returns the trusted identity.
//
// `expectedNonce` may be empty for flows that don't use one, but the CLIENTS
// here always send one: it is what stops a token captured from another context
// being replayed into our sign-in endpoint.
func (v *OIDCVerifier) Verify(ctx context.Context, provider SocialProvider, idToken, expectedNonce string) (*VerifiedIdentity, error) {
	var cache *jwksCache
	var audiences []string
	var issuers []string

	switch provider {
	case ProviderGoogle:
		cache, audiences, issuers = googleKeys, v.GoogleAudiences, []string{googleIssuerPrimary, googleIssuerAlt}
	case ProviderApple:
		cache, audiences, issuers = appleKeys, v.AppleAudiences, []string{appleIssuer}
	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}

	if len(audiences) == 0 {
		return nil, fmt.Errorf("provider %s is not configured", provider)
	}

	claims := &oidcClaims{}
	// jwt.WithValidMethods is the alg allow-list. Without it a token with
	// alg:none, or an HS256 token signed with the RSA public key as the HMAC
	// secret, would parse.
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithLeeway(60*time.Second),
	)

	_, err := parser.ParseWithClaims(idToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("id token has no kid header")
		}
		return cache.get(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("invalid id token: %w", err)
	}

	// ── Issuer ───────────────────────────────────────────────────────────────
	issuerOK := false
	for _, iss := range issuers {
		if claims.Issuer == iss {
			issuerOK = true
			break
		}
	}
	if !issuerOK {
		return nil, fmt.Errorf("unexpected issuer %q", claims.Issuer)
	}

	// ── Audience — the token-substitution defence ────────────────────────────
	audOK := false
	for _, want := range audiences {
		for _, got := range claims.Audience {
			if got == want {
				audOK = true
				break
			}
		}
	}
	if !audOK {
		return nil, fmt.Errorf("id token audience %v is not one of ours", claims.Audience)
	}

	// ── Nonce — replay defence ───────────────────────────────────────────────
	// Compared only when the caller supplied one, but note the asymmetry: if we
	// expect a nonce and the token has none, that is a failure. The reverse is
	// fine (a token carrying a nonce we didn't ask about).
	if expectedNonce != "" && claims.Nonce != expectedNonce {
		return nil, errors.New("id token nonce mismatch")
	}

	if claims.Subject == "" {
		return nil, errors.New("id token has no subject")
	}

	// exp/iat are enforced by the parser above (with 60s leeway for clock skew).

	email := strings.ToLower(strings.TrimSpace(claims.Email))
	identity := &VerifiedIdentity{
		Provider:       provider,
		Subject:        claims.Subject,
		Email:          email,
		EmailVerified:  truthy(claims.EmailVerified),
		FirstName:      strings.TrimSpace(claims.GivenName),
		LastName:       strings.TrimSpace(claims.FamilyName),
		IsPrivateRelay: truthy(claims.IsPrivateEmail) || strings.HasSuffix(email, "@privaterelay.appleid.com"),
	}

	// Google sends `name` but not always given/family. Split as a fallback so
	// the profile isn't blank.
	if identity.FirstName == "" && claims.Name != "" {
		parts := strings.Fields(claims.Name)
		if len(parts) > 0 {
			identity.FirstName = parts[0]
		}
		if len(parts) > 1 {
			identity.LastName = strings.Join(parts[1:], " ")
		}
	}

	return identity, nil
}

// truthy normalises the two providers' disagreement about JSON booleans:
// Google sends `true`, Apple sends `"true"`.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	}
	return false
}
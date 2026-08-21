package models

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ── Link validation ──────────────────────────────────────────────────────────
//
// This file is the entire control that stops an organiser's public page — which
// carries TicketHawk branding and, once indexed, some of our domain authority —
// from becoming a redirect to somewhere it shouldn't go.
//
// The rule is simple and deliberately strict: a link labelled with a platform
// must point at that platform. An "Instagram" button that opens a payment page
// is a phishing primitive, and the label is what the visitor trusts, not the
// URL they never see. `website` is the one platform with no host restriction —
// it is by definition the organiser's own domain — so it carries the other
// checks instead: no credentials, no IP literals, no known shortener.
//
// Everything here runs BEFORE the row is written. The chk_organizer_link_https
// CHECK in migration 000077 is a backstop for writes that bypass this code, not
// the primary control.

var (
	ErrLinkUnknownPlatform = errors.New("unknown platform")
	ErrLinkNotHTTPS        = errors.New("link must start with https://")
	ErrLinkWrongHost       = errors.New("link does not point at that platform")
	ErrLinkMalformed       = errors.New("link is not a valid URL")
	ErrLinkHandleEmpty     = errors.New("handle is empty")
	ErrLinkShortener       = errors.New("link shorteners are not allowed")
	ErrLinkCredentials     = errors.New("link must not contain a username or password")
)

// linkHosts maps each platform to the hosts it may point at.
//
// Regional and legacy domains are included because organisers paste whatever
// their app gave them: instagr.am still appears in older bios, twitter.com
// outnumbers x.com in the wild, and fb.me is what the Facebook share sheet
// produces. Rejecting those would read as a bug, not a policy.
//
// A nil entry means "no host restriction" — only `website` has one.
var linkHosts = map[OrganizerLinkPlatform][]string{
	LinkInstagram: {"instagram.com", "instagr.am"},
	LinkX:         {"x.com", "twitter.com"},
	LinkFacebook:  {"facebook.com", "fb.com", "fb.me", "m.facebook.com"},
	LinkTikTok:    {"tiktok.com", "vm.tiktok.com"},
	LinkYouTube:   {"youtube.com", "youtu.be", "m.youtube.com"},
	LinkLinkedIn:  {"linkedin.com"},
	LinkWhatsApp:  {"wa.me", "whatsapp.com", "chat.whatsapp.com", "api.whatsapp.com"},
	LinkSpotify:   {"spotify.com", "open.spotify.com"},
	LinkWebsite:   nil,
}

// handleTemplates builds a canonical URL from a bare handle. Platforms absent
// from this map (website, whatsapp) require a full URL: there is no such thing
// as a "handle" for someone's own domain, and a WhatsApp link is a phone
// number or an invite code that we should not be guessing at.
var handleTemplates = map[OrganizerLinkPlatform]string{
	LinkInstagram: "https://instagram.com/%s",
	LinkX:         "https://x.com/%s",
	LinkFacebook:  "https://facebook.com/%s",
	LinkTikTok:    "https://tiktok.com/@%s",
	LinkYouTube:   "https://youtube.com/@%s",
	LinkLinkedIn:  "https://linkedin.com/company/%s",
}

// shortenerHosts are refused on `website`, which has no host allow-list to
// catch them. A shortener defeats the entire point of validating a host: the
// destination is decided after we have already approved the link, and can be
// changed afterwards without touching our data.
var shortenerHosts = map[string]bool{
	"bit.ly": true, "tinyurl.com": true, "t.co": true, "goo.gl": true,
	"ow.ly": true, "buff.ly": true, "rebrand.ly": true, "cutt.ly": true,
	"is.gd": true, "shorturl.at": true, "rb.gy": true, "lnkd.in": true,
	"tiny.cc": true, "s.id": true, "linktr.ee": false, // linktr.ee is allowed: it is a legitimate, stable organiser landing page rather than an opaque redirect
}

// ValidateOrganizerLink normalises and checks a single link, returning the URL
// to store.
//
// Accepts either a full https URL or a bare handle, with or without a leading
// '@'. A handle is anything with no scheme and no dot — "wildernessbeach" and
// "@wildernessbeach" are handles, "wildernessbeach.co.za" is a URL missing its
// scheme and is treated as such.
func ValidateOrganizerLink(platform, raw string) (string, error) {
	p := OrganizerLinkPlatform(strings.ToLower(strings.TrimSpace(platform)))
	if !IsValidLinkPlatform(string(p)) {
		return "", fmt.Errorf("%w: %q", ErrLinkUnknownPlatform, platform)
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrLinkHandleEmpty
	}

	// ── Handle form ──────────────────────────────────────────────────────────
	if looksLikeHandle(raw) {
		tmpl, ok := handleTemplates[p]
		if !ok {
			// website and whatsapp: no sensible expansion exists.
			return "", fmt.Errorf("%w: enter the full https:// link for %s", ErrLinkMalformed, p)
		}
		h := strings.TrimPrefix(raw, "@")
		h = strings.Trim(h, "/")
		if h == "" {
			return "", ErrLinkHandleEmpty
		}
		if strings.ContainsAny(h, " \t/?#&=") {
			return "", fmt.Errorf("%w: %q is not a valid handle", ErrLinkMalformed, raw)
		}
		return fmt.Sprintf(tmpl, url.PathEscape(h)), nil
	}

	// ── URL form ─────────────────────────────────────────────────────────────
	//
	// A scheme-less URL ("instagram.com/x") is upgraded to https rather than
	// rejected. That is not the silent rewriting the migration header warns
	// against — adding a missing scheme preserves the destination exactly,
	// whereas upgrading an explicit http:// would change the security
	// properties of a link the organiser deliberately typed.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLinkMalformed, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", ErrLinkNotHTTPS
	}
	if u.User != nil {
		// https://evil.com@instagram.com/… renders as an instagram.com link in
		// some clients while resolving elsewhere. Refuse the whole shape.
		return "", ErrLinkCredentials
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("%w: no host", ErrLinkMalformed)
	}
	host = strings.TrimPrefix(host, "www.")

	// A nil slice means "no host restriction". Note this is NOT the same as the
	// two-value map lookup: `website` is PRESENT in linkHosts with a nil value,
	// so testing the comma-ok bool here would take the wrong branch and reject
	// every website link with "not a website link".
	allowed := linkHosts[p]
	if allowed == nil {
		// `website`: no host allow-list, so apply the generic protections.
		if shortenerHosts[host] {
			return "", fmt.Errorf("%w: %s", ErrLinkShortener, host)
		}
		if !strings.Contains(host, ".") {
			return "", fmt.Errorf("%w: %q is not a public domain", ErrLinkMalformed, host)
		}
		if isIPLiteral(host) {
			return "", fmt.Errorf("%w: raw IP addresses are not allowed", ErrLinkMalformed)
		}
		u.Fragment = ""
		return u.String(), nil
	}

	for _, h := range allowed {
		// Exact match or a subdomain of it. "evilinstagram.com" fails both:
		// it is not equal, and it does not end with ".instagram.com".
		if host == h || strings.HasSuffix(host, "."+h) {
			u.Fragment = ""
			return u.String(), nil
		}
	}

	return "", fmt.Errorf("%w: %s is not a %s link", ErrLinkWrongHost, host, p)
}

// looksLikeHandle reports whether raw is a bare username rather than a URL.
// A dot is the discriminator: handles do not contain one, hostnames always do.
func looksLikeHandle(raw string) bool {
	if strings.Contains(raw, "://") {
		return false
	}
	if strings.HasPrefix(raw, "@") {
		return true
	}
	return !strings.Contains(raw, ".") && !strings.Contains(raw, "/")
}

// isIPLiteral reports whether host is a bare IPv4/IPv6 address. url.Hostname()
// has already stripped the brackets from an IPv6 literal, so a colon is the
// tell for v6 and an all-numeric dotted form for v4.
func isIPLiteral(host string) bool {
	if strings.Contains(host, ":") {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// ValidateOrganizerLinks normalises a whole replacement set, rejecting
// duplicates. Returns links with SortOrder assigned from array position, which
// is the order the organiser arranged them in the form.
func ValidateOrganizerLinks(in []OrganizerLinkDTO) ([]OrganizerLink, error) {
	out := make([]OrganizerLink, 0, len(in))
	seen := make(map[string]bool, len(in))

	for i, dto := range in {
		p := strings.ToLower(strings.TrimSpace(dto.Platform))
		if seen[p] {
			return nil, fmt.Errorf("duplicate platform %q", p)
		}
		normalised, err := ValidateOrganizerLink(p, dto.URL)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		seen[p] = true
		out = append(out, OrganizerLink{
			Platform:  OrganizerLinkPlatform(p),
			URL:       normalised,
			SortOrder: i,
		})
	}
	return out, nil
}
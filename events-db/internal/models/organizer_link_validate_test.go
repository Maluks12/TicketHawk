package models

import "testing"

// The validator is the only thing standing between an organiser's public page
// and a phishing redirect, so it gets a table rather than a smoke test. The
// cases that matter most are the REJECTIONS — anything that renders as an
// Instagram button but resolves elsewhere.

func TestValidateOrganizerLink_Accepts(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		in       string
		want     string
	}{
		{"instagram handle", "instagram", "wildernessbeach", "https://instagram.com/wildernessbeach"},
		{"instagram @handle", "instagram", "@wildernessbeach", "https://instagram.com/wildernessbeach"},
		{"instagram full url", "instagram", "https://instagram.com/wildernessbeach", "https://instagram.com/wildernessbeach"},
		{"instagram www stripped for matching", "instagram", "https://www.instagram.com/wb", "https://www.instagram.com/wb"},
		{"legacy instagr.am", "instagram", "https://instagr.am/wb", "https://instagr.am/wb"},
		{"twitter still accepted for x", "x", "https://twitter.com/wb", "https://twitter.com/wb"},
		{"x handle", "x", "@wb", "https://x.com/wb"},
		{"tiktok handle gets @ prefix", "tiktok", "wb", "https://tiktok.com/@wb"},
		{"youtube handle", "youtube", "wb", "https://youtube.com/@wb"},
		{"youtu.be short form", "youtube", "https://youtu.be/abc123", "https://youtu.be/abc123"},
		{"facebook subdomain", "facebook", "https://m.facebook.com/wb", "https://m.facebook.com/wb"},
		{"whatsapp wa.me", "whatsapp", "https://wa.me/27821234567", "https://wa.me/27821234567"},
		{"spotify open subdomain", "spotify", "https://open.spotify.com/artist/x", "https://open.spotify.com/artist/x"},
		{"website own domain", "website", "https://wildernessbeach.co.za", "https://wildernessbeach.co.za"},
		{"website missing scheme upgraded", "website", "wildernessbeach.co.za", "https://wildernessbeach.co.za"},
		{"website with path", "website", "https://wildernessbeach.co.za/events", "https://wildernessbeach.co.za/events"},
		{"linktree allowed as a landing page", "website", "https://linktr.ee/wb", "https://linktr.ee/wb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateOrganizerLink(tc.platform, tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateOrganizerLink_Rejects(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		in       string
	}{
		// The whole point of the allow-list.
		{"instagram label, foreign host", "instagram", "https://pay-me-now.example/checkout"},
		{"lookalike domain", "instagram", "https://evilinstagram.com/wb"},
		{"suffix without the dot boundary", "instagram", "https://notinstagram.com/wb"},

		// Credential trick: renders as instagram.com in some clients.
		{"userinfo before host", "instagram", "https://instagram.com@evil.example/x"},

		// http is refused outright rather than upgraded.
		{"plain http", "website", "http://wildernessbeach.co.za"},

		// website has no host allow-list, so these are its guards.
		{"shortener on website", "website", "https://bit.ly/xyz"},
		{"raw ipv4", "website", "https://192.168.0.1/"},
		{"ipv6 literal", "website", "https://[::1]/"},
		{"bare hostname, no dot", "website", "https://localhost/"},

		// Handles make no sense for these two.
		{"handle for website", "website", "@wildernessbeach"},
		{"handle for whatsapp", "whatsapp", "@wildernessbeach"},

		{"unknown platform", "threads", "https://threads.net/@wb"},
		{"empty", "instagram", "   "},
		{"handle with a slash", "instagram", "@wb/extra"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateOrganizerLink(tc.platform, tc.in)
			if err == nil {
				t.Fatalf("expected rejection, got %q", got)
			}
		})
	}
}

func TestValidateOrganizerLinks_RejectsDuplicates(t *testing.T) {
	_, err := ValidateOrganizerLinks([]OrganizerLinkDTO{
		{Platform: "instagram", URL: "@a"},
		{Platform: "instagram", URL: "@b"},
	})
	if err == nil {
		t.Fatal("expected duplicate platform to be rejected")
	}
}

func TestValidateOrganizerLinks_AssignsSortOrder(t *testing.T) {
	out, err := ValidateOrganizerLinks([]OrganizerLinkDTO{
		{Platform: "website", URL: "https://a.co.za"},
		{Platform: "instagram", URL: "@b"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 links, got %d", len(out))
	}
	// Order is the order the organiser arranged them, not the canonical
	// platform order — the form is the source of truth.
	if out[0].Platform != LinkWebsite || out[0].SortOrder != 0 {
		t.Errorf("first link wrong: %+v", out[0])
	}
	if out[1].Platform != LinkInstagram || out[1].SortOrder != 1 {
		t.Errorf("second link wrong: %+v", out[1])
	}
}

// CanHavePublicProfile and the SQL gate in GetPublicOrganizerProfile must agree.
// This asserts the Go half; the SQL half is mirrored by hand in the repository
// and this test is the reminder that changing one means changing both.
func TestCanHavePublicProfile(t *testing.T) {
	base := func() *Organizer {
		return &Organizer{
			KYCLevel:             KYCLevelFull,
			PublicProfileEnabled: true,
			Status:               OrganizerActive,
		}
	}

	if !base().CanHavePublicProfile() {
		t.Error("a verified, enabled, active organiser should have a public profile")
	}

	o := base()
	o.KYCLevel = KYCLevelBasic
	if o.CanHavePublicProfile() {
		t.Error("level 1 must not get a public page")
	}

	o = base()
	o.PublicProfileEnabled = false
	if o.CanHavePublicProfile() {
		t.Error("organiser opt-out must be honoured")
	}

	for _, s := range []OrganizerStatus{OrganizerSuspended, OrganizerBanned, OrganizerRejected} {
		o = base()
		o.Status = s
		if o.CanHavePublicProfile() {
			t.Errorf("status %q must not have a public page", s)
		}
	}

	var nilOrg *Organizer
	if nilOrg.CanHavePublicProfile() {
		t.Error("nil receiver must be safe and false")
	}
}
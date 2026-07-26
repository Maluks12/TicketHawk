package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────────────────────
// Service identity at the API root.
//
// ── Why this exists ──────────────────────────────────────────────────────────
//
// api.ticket-hawk.com had no handler on "/", so anything hitting the root got
// Gin's bare 404. URL-categorisation crawlers do exactly that: they fetch the
// root of a hostname and classify whatever comes back. With nothing to read,
// Symantec's WebPulse rated the subdomain "Placeholders" — its bucket for
// parked domains and sites under construction — and filed it under the
// *Security* category group.
//
// That is a materially worse outcome than being unrated. Parked-domain
// categories get blocked aggressively, because parking is a well-known malware
// and typosquat pattern, so a network enforcing that rule would have blocked
// the API the mobile apps depend on. And it fails invisibly: the app would show
// cached bookings from AsyncStorage while every live call quietly failed, so it
// reads to the user as "TicketHawk is broken", not "my network blocked it".
//
// One dispute fixes one vendor. Every other crawler that visits will reach the
// same conclusion from the same evidence, so the durable fix is to give the
// root something to read.
//
// ── Why it content-negotiates ────────────────────────────────────────────────
//
// The two audiences want different things from the same URL:
//
//   • Crawlers, health probes and curl send Accept: *_/_* or application/json
//     and want a small machine-readable document.
//   • A human reviewer assessing a categorisation dispute opens the link in a
//     browser, which sends Accept: text/html. Raw JSON in a browser window is
//     precisely the "no useful value" impression being corrected here.
//
// So browsers get a plain HTML card and everything else gets JSON. Same facts
// either way — this is presentation, not a different answer per client.
//
// ── What it deliberately does not expose ─────────────────────────────────────
//
// No version string, build hash, framework name, dependency list, uptime or
// environment. An unauthenticated root path is the single most-probed URL on
// any host, and every one of those fields is reconnaissance that narrows an
// attacker's search for a known CVE. The response answers "what is this and who
// runs it" and nothing further. Component health stays on /health, which is
// where infrastructure already looks for it.
// ─────────────────────────────────────────────────────────────────────────────

// serviceIdentity is the JSON document served to non-browser clients.
type serviceIdentity struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Website string `json:"website"`
	Contact string `json:"contact"`
	Message string `json:"message"`
}

const (
	identityService = "TicketHawk API"
	identityWebsite = "https://ticket-hawk.com"
	identityContact = "admin@ticket-hawk.com"
	identityMessage = "This host serves the REST API for the TicketHawk event " +
		"ticketing platform. It returns JSON and has no browsable pages. " +
		"Visit ticket-hawk.com to browse events and buy tickets."
)

// prefersHTML reports whether the caller is a browser rather than a program.
//
// Checking that text/html appears before application/json is more reliable than
// sniffing the User-Agent, which crawlers routinely spoof to look like Chrome.
// An empty or wildcard Accept header means a tool, so it falls through to JSON.
func prefersHTML(accept string) bool {
	accept = strings.ToLower(accept)
	if !strings.Contains(accept, "text/html") {
		return false
	}
	jsonAt := strings.Index(accept, "application/json")
	if jsonAt == -1 {
		return true
	}
	return strings.Index(accept, "text/html") < jsonAt
}

// ServiceIdentityHandler answers GET and HEAD on "/".
//
// Register it for HEAD as well as GET: Gin does not derive one from the other,
// and several crawlers issue a HEAD probe before deciding whether to fetch the
// body. A 404 on that probe is enough on its own to look like a dead host.
func ServiceIdentityHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Categorisation databases re-crawl on their own schedule. A short TTL
		// keeps a stale copy from outliving a change here, without inviting a
		// fetch per request.
		c.Header("Cache-Control", "public, max-age=3600")

		if prefersHTML(c.GetHeader("Accept")) {
			c.Header("X-Content-Type-Options", "nosniff")
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(identityHTML))
			return
		}

		c.JSON(http.StatusOK, serviceIdentity{
			Service: identityService,
			Status:  "operational",
			Website: identityWebsite,
			Contact: identityContact,
			Message: identityMessage,
		})
	}
}

// RobotsHandler serves /robots.txt.
//
// Note what this does NOT do: a blanket "Disallow: /" would be the reflex for an
// API, and it would undo the fix above — a crawler that is forbidden from
// reading the root has nothing to categorise on and lands back at "no useful
// value". So the root is explicitly allowed and the API surface below it is not.
// Reviewers get their evidence; nothing else gets walked.
func RobotsHandler() gin.HandlerFunc {
	const robots = `User-agent: *
Allow: /$
Disallow: /v1/
Disallow: /webhooks/
Disallow: /payments/
`
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(robots))
	}
}

// Self-contained: no external CSS, fonts or images, so it renders identically
// inside the sandboxed viewers some review tools use.
const identityHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>TicketHawk API</title>
<meta name="description" content="REST API for the TicketHawk event ticketing platform (South Africa).">
<meta name="robots" content="noindex">
<style>
  :root { color-scheme: light dark; }
  body {
    margin: 0; min-height: 100vh;
    display: flex; align-items: center; justify-content: center;
    background: #0e0e12; color: #f4f4f5;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    line-height: 1.55; padding: 24px;
  }
  main { max-width: 30rem; }
  h1 { font-size: 1.5rem; margin: 0 0 .25rem; letter-spacing: -.02em; }
  .tag { color: #FF2D55; font-size: .75rem; font-weight: 700; letter-spacing: .12em; text-transform: uppercase; }
  p { color: #a1a1aa; font-size: .95rem; margin: .75rem 0 0; }
  a { color: #FF2D55; }
  hr { border: 0; border-top: 1px solid #2e2e35; margin: 1.5rem 0; }
  .meta { font-size: .8rem; color: #71717a; }
</style>
</head>
<body>
<main>
  <p class="tag">TicketHawk</p>
  <h1>API endpoint</h1>
  <p>
    This host serves the REST API for TicketHawk, an event ticketing platform
    operating in South Africa. It returns JSON to the TicketHawk mobile and web
    applications and has no browsable pages of its own.
  </p>
  <p>
    To browse events and buy tickets, visit
    <a href="https://ticket-hawk.com">ticket-hawk.com</a>.
  </p>
  <hr>
  <p class="meta">
    Status: operational &middot; Contact:
    <a href="mailto:admin@ticket-hawk.com">admin@ticket-hawk.com</a>
  </p>
</main>
</body>
</html>
`
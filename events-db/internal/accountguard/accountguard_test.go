package accountguard

import (
	"strings"
	"testing"
	"time"

	"events-db/internal/models"

	"github.com/gin-gonic/gin"
)

// install swaps in a registry holding s, restoring the previous one when the
// test ends. No database: the snapshot is the whole state these tests need.
func install(t *testing.T, s *snapshot) {
	t.Helper()
	r := &registry{}
	r.snap.Store(s)
	prev := current.Load()
	current.Store(r)
	t.Cleanup(func() { current.Store(prev) })
}

func at(t time.Time) *time.Time { return &t }

func TestNoRegistryRestrictsNobody(t *testing.T) {
	prev := current.Load()
	current.Store(nil)
	t.Cleanup(func() { current.Store(prev) })

	if _, locked := LockFor("u1"); locked {
		t.Fatal("a registry that was never started must not lock anyone out")
	}
	if _, blocked := FeatureBlock("u1", models.RestrictBuyTickets); blocked {
		t.Fatal("a registry that was never started must not block anything")
	}
	if PurchaseBlockedForEmail("someone@example.com") {
		t.Fatal("a registry that was never started must not block guest checkout")
	}
}

func TestBanOutranksDisable(t *testing.T) {
	now := time.Now()
	s := emptySnapshot()
	s.byUser["u1"] = []Entry{
		{ID: "d", Kind: models.RestrictDisable, EndsAt: at(now.Add(72 * time.Hour))},
		{ID: "b", Kind: models.RestrictBan},
	}
	lock, locked := lockAt(s, "u1", now)
	if !locked || lock.Kind != models.RestrictBan {
		t.Fatalf("want ban, got %+v locked=%v", lock, locked)
	}
}

func TestLaterDisableWins(t *testing.T) {
	now := time.Now()
	s := emptySnapshot()
	s.byUser["u1"] = []Entry{
		{ID: "short", Kind: models.RestrictDisable, EndsAt: at(now.Add(24 * time.Hour))},
		{ID: "long", Kind: models.RestrictDisable, EndsAt: at(now.Add(96 * time.Hour))},
	}
	lock, _ := lockAt(s, "u1", now)
	if lock.EndsAt == nil || !lock.EndsAt.Equal(now.Add(96*time.Hour)) {
		t.Fatalf("want the later disable, got %+v", lock)
	}
}

func TestDisableLiftsItselfWhenItEnds(t *testing.T) {
	end := time.Now().Add(2 * time.Hour)
	s := emptySnapshot()
	s.byUser["u1"] = []Entry{{ID: "d", Kind: models.RestrictDisable, EndsAt: at(end)}}

	if _, locked := lockAt(s, "u1", end.Add(-time.Second)); !locked {
		t.Fatal("must be locked one second before the end")
	}
	if _, locked := lockAt(s, "u1", end); locked {
		t.Fatal("must be unlocked at the end, with no reload")
	}
}

func TestFeatureBlocksDoNotLockTheAccount(t *testing.T) {
	now := time.Now()
	s := emptySnapshot()
	s.byUser["u1"] = []Entry{
		{ID: "p", Kind: models.RestrictPayouts},
		{ID: "t", Kind: models.RestrictBuyTickets, EndsAt: at(now.Add(time.Hour))},
	}
	if _, locked := lockAt(s, "u1", now); locked {
		t.Fatal("a feature block is not a lockout")
	}
	if _, blocked := featureBlockAt(s, "u1", models.RestrictCreateEvents, now); blocked {
		t.Fatal("an unblocked feature must pass")
	}
	blocks := featureBlocksAt(s, "u1", now)
	// Console order, not insertion order: buy_tickets before payouts.
	if len(blocks) != 2 || blocks[0].Kind != models.RestrictBuyTickets || blocks[1].Kind != models.RestrictPayouts {
		t.Fatalf("want [buy_tickets payouts], got %+v", blocks)
	}
}

func TestIndefiniteFeatureBlockOutranksDated(t *testing.T) {
	now := time.Now()
	s := emptySnapshot()
	s.byUser["u1"] = []Entry{
		{ID: "dated", Kind: models.RestrictPayouts, EndsAt: at(now.Add(48 * time.Hour))},
		{ID: "forever", Kind: models.RestrictPayouts},
	}
	e, _ := featureBlockAt(s, "u1", models.RestrictPayouts, now)
	if e.ID != "forever" {
		t.Fatalf("want the indefinite block, got %q", e.ID)
	}
}

func TestDaysRemainingRoundsUp(t *testing.T) {
	now := time.Now()
	cases := []struct {
		left time.Duration
		want int
	}{
		{72 * time.Hour, 3},
		{72*time.Hour - time.Second, 3},
		{49 * time.Hour, 3},
		{48 * time.Hour, 2},
		{5 * time.Minute, 1},
		{0, 0},
		{-time.Hour, 0},
	}
	for _, c := range cases {
		if got := DaysRemaining(now.Add(c.left), now); got != c.want {
			t.Errorf("DaysRemaining(%v) = %d, want %d", c.left, got, c.want)
		}
	}
}

func TestLockMessages(t *testing.T) {
	now := time.Now()
	ban := Lock{Kind: models.RestrictBan}
	if got := ban.Message(now); got != "Your account has been banned." {
		t.Errorf("ban message = %q", got)
	}
	three := Lock{Kind: models.RestrictDisable, EndsAt: at(now.Add(70 * time.Hour))}
	if got := three.Message(now); got != "Your account has been disabled for 3 days." {
		t.Errorf("3-day message = %q", got)
	}
	one := Lock{Kind: models.RestrictDisable, EndsAt: at(now.Add(10 * time.Minute))}
	if got := one.Message(now); got != "Your account has been disabled for 1 day." {
		t.Errorf("1-day message = %q", got)
	}
}

func TestLockBodyCarriesCodeAndPopupText(t *testing.T) {
	now := time.Now()
	body := Lock{Kind: models.RestrictDisable, EndsAt: at(now.Add(30 * time.Hour))}.Body(now)
	if body["code"] != CodeDisabled {
		t.Fatalf("code = %v", body["code"])
	}
	// `error` must be the popup text itself: it is all an older app shows.
	if body["error"] != "Your account has been disabled for 2 days." {
		t.Fatalf("error = %v", body["error"])
	}
	status, ok := body["account_status"].(gin.H)
	if !ok {
		t.Fatalf("account_status is %T", body["account_status"])
	}
	if status["state"] != "disabled" || status["days_remaining"] != 2 {
		t.Fatalf("status = %+v", status)
	}
}

func TestPurchaseBlockedForEmail(t *testing.T) {
	now := time.Now()
	s := emptySnapshot()
	s.byUser["buyblocked"] = []Entry{{ID: "1", Kind: models.RestrictBuyTickets}}
	s.byUser["banned"] = []Entry{{ID: "2", Kind: models.RestrictBan}}
	s.byUser["lapsed"] = []Entry{{ID: "3", Kind: models.RestrictDisable, EndsAt: at(now.Add(-time.Minute))}}
	s.byEmail["buyer@example.com"] = "buyblocked"
	s.byEmail["banned@example.com"] = "banned"
	s.byEmail["lapsed@example.com"] = "lapsed"
	install(t, s)

	if !PurchaseBlockedForEmail("  Buyer@Example.com ") {
		t.Error("email matching must ignore case and surrounding spaces")
	}
	if !PurchaseBlockedForEmail("banned@example.com") {
		t.Error("a banned account's email must be refused at guest checkout")
	}
	if PurchaseBlockedForEmail("lapsed@example.com") {
		t.Error("a disable that has ended must not block guest checkout")
	}
	if PurchaseBlockedForEmail("nobody@example.com") {
		t.Error("an unrestricted email must pass")
	}
}

func TestFeatureBlockedBodyExplainsTheRefusal(t *testing.T) {
	now := time.Now()
	s := emptySnapshot()
	s.byUser["u1"] = []Entry{
		{ID: "t", Kind: models.RestrictBuyTickets},
		{ID: "b", Kind: models.RestrictBankDetails, EndsAt: at(now.Add(24 * time.Hour))},
	}
	install(t, s)

	blocked, ok := FeatureBlock("u1", models.RestrictBuyTickets)
	if !ok {
		t.Fatal("expected buy_tickets to be blocked")
	}
	body := FeatureBlockedBody("u1", blocked)
	if body["code"] != CodeLimited {
		t.Fatalf("code = %v", body["code"])
	}
	msg, _ := body["error"].(string)
	if !strings.HasPrefix(msg, "Your account has been limited.") || !strings.Contains(msg, "You can't buy tickets.") {
		t.Fatalf("error = %q", msg)
	}
	status, _ := body["account_status"].(gin.H)
	limits, _ := status["limits"].([]gin.H)
	if len(limits) != 2 {
		t.Fatalf("want both blocks listed, got %+v", status["limits"])
	}
}
package intercept

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func domainNet(d string) EngagementNet { return EngagementNet{Kind: "domain", Domain: d} }
func cidrNet(c string) EngagementNet {
	return EngagementNet{Kind: "cidr", Net: netip.MustParsePrefix(c)}
}
func ipNet(ip string) EngagementNet {
	a := netip.MustParseAddr(ip)
	return EngagementNet{Kind: "ip", Net: netip.PrefixFrom(a, a.BitLen())}
}

// fixedNow returns a clock pinned to t.
func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

// active builds an active policy whose window is open at the fixed now.
func active(allow, deny []EngagementNet) *EngagementPolicy {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return &EngagementPolicy{
		EngagementID: 1, Status: "active",
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
		Allow: allow, Deny: deny, Now: fixedNow(now),
	}
}

const (
	webApp   = `{"url":"https://app.example.com/login"}` // domain app.example.com:443
	webOther = `{"url":"https://evil.example.org/"}`     // domain evil.example.org:443
	webSub   = `{"url":"https://secret.example.com/"}`   // domain secret.example.com:443
	nmapIP   = `{"command":"nmap 203.0.113.7"}`          // ip 203.0.113.7
	nmapIP2  = `{"command":"nmap 198.51.100.9"}`         // ip 198.51.100.9
	nmapCIDR = `{"command":"nmap 203.0.113.0/28"}`       // cidr 203.0.113.0/28
	nmapWide = `{"command":"nmap 203.0.0.0/16"}`         // cidr 203.0.0.0/16
	readFile = `{"file_path":"/etc/hosts"}`              // no network target
)

func wantCode(t *testing.T, p *EngagementPolicy, tool, input, code string) {
	t.Helper()
	res := p.CheckEngagement(tool, []byte(input))
	if res.Code != code {
		t.Fatalf("%s %s: code=%q reason=%q, want %q", tool, input, res.Code, res.Reason, code)
	}
}

// The window must be open, the status active, for any call to pass.
func TestEngagementWindow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	base := func() *EngagementPolicy {
		return &EngagementPolicy{EngagementID: 1, Status: "active",
			Allow: []EngagementNet{domainNet("example.com")}, Now: fixedNow(now)}
	}
	// Open: now inside [start, end).
	p := base()
	p.StartsAt, p.EndsAt = now.Add(-time.Hour), now.Add(time.Hour)
	wantCode(t, p, "WebFetch", webApp, "")

	// Not yet started.
	p = base()
	p.StartsAt, p.EndsAt = now.Add(time.Hour), now.Add(2*time.Hour)
	wantCode(t, p, "WebFetch", webApp, EngagementWindowClosed)

	// Expired: now == end is outside [start, end) (half-open).
	p = base()
	p.StartsAt, p.EndsAt = now.Add(-2*time.Hour), now
	wantCode(t, p, "WebFetch", webApp, EngagementWindowClosed)

	// Status != active blocks before the window is even considered.
	for _, st := range []string{"draft", "suspended", "closed", ""} {
		p = base()
		p.Status = st
		p.StartsAt, p.EndsAt = now.Add(-time.Hour), now.Add(time.Hour)
		wantCode(t, p, "WebFetch", webApp, EngagementNotActive)
	}
}

// A call that declares no network target is permitted once the window/status are
// satisfied, even with an empty allow-list (the target gate/budgets still apply).
func TestEngagementNoTargetPassesWindow(t *testing.T) {
	p := active(nil, nil)
	wantCode(t, p, "Read", readFile, "")
	wantCode(t, p, "web_search", `{"query":"example.com login"}`, "")

	// But a declared target against an empty allow-list is out of scope.
	wantCode(t, p, "WebFetch", webApp, EngagementOutOfScope)

	// With the window closed, even a no-target call is blocked.
	closed := active(nil, nil)
	closed.EndsAt = closed.StartsAt // force now outside the window
	wantCode(t, closed, "Read", readFile, EngagementWindowClosed)
}

// A target inside the allow-list (and not on the deny-list) passes; one outside
// is blocked. Domains match by suffix; CIDRs by containment.
func TestEngagementAllowDenyIntersection(t *testing.T) {
	p := active([]EngagementNet{domainNet("example.com"), cidrNet("203.0.113.0/24")},
		[]EngagementNet{domainNet("secret.example.com")})

	wantCode(t, p, "WebFetch", webApp, "") // app.example.com ⊆ example.com
	wantCode(t, p, "Bash", nmapIP, "")     // 203.0.113.7 ∈ 203.0.113.0/24
	wantCode(t, p, "Bash", nmapCIDR, "")   // 203.0.113.0/28 ⊆ 203.0.113.0/24
	// evil.example.org matches no domain allow, but a CIDR allow row is present, so
	// without DNS it cannot be ruled out → ambiguous (fail safe).
	wantCode(t, p, "WebFetch", webOther, EngagementAmbiguous)
	wantCode(t, p, "Bash", nmapIP2, EngagementOutOfScope)    // 198.51.100.9 outside the CIDR (IP is decidable)
	wantCode(t, p, "Bash", nmapWide, EngagementOutOfScope)   // 203.0.0.0/16 not contained in /24
	wantCode(t, p, "WebFetch", webSub, EngagementOutOfScope) // deny secret.example.com wins (deny matched before ambiguity)
}

// Deny always wins over allow, even for an identical entry.
func TestEngagementDenyBeatsAllow(t *testing.T) {
	// Identical allow + deny.
	p := active([]EngagementNet{domainNet("app.example.com")},
		[]EngagementNet{domainNet("app.example.com")})
	wantCode(t, p, "WebFetch", webApp, EngagementOutOfScope)

	// Deny a sub-range of an allowed CIDR: an IP in the denied range is blocked,
	// one only in the wider allow range passes.
	p = active([]EngagementNet{cidrNet("203.0.113.0/24")},
		[]EngagementNet{cidrNet("203.0.113.0/28")})
	wantCode(t, p, "Bash", nmapIP, EngagementOutOfScope)           // .7 ∈ /28 deny
	wantCode(t, p, "Bash", `{"command":"nmap 203.0.113.200"}`, "") // .200 only in /24 allow
	// A declared CIDR target overlapping a deny range is blocked (deny = overlap).
	wantCode(t, p, "Bash", `{"command":"nmap 203.0.113.0/26"}`, EngagementOutOfScope)
}

// A target whose membership cannot be decided without DNS (a domain, when the
// allow-list has ip/cidr rows it might resolve into) is ambiguous => block. A
// domain with only domain allow-rows is definitively out of scope, not ambiguous.
func TestEngagementAmbiguousBlocks(t *testing.T) {
	// Only a CIDR allow row: a domain target could resolve into it → ambiguous.
	p := active([]EngagementNet{cidrNet("10.0.0.0/8")}, nil)
	wantCode(t, p, "WebFetch", webApp, EngagementAmbiguous)

	// A domain allow row present but not matching, plus a CIDR row → still
	// ambiguous (the CIDR row makes the domain undecidable without DNS).
	p = active([]EngagementNet{domainNet("example.net"), cidrNet("10.0.0.0/8")}, nil)
	wantCode(t, p, "WebFetch", webApp, EngagementAmbiguous)

	// Only domain allow rows: a non-matching domain is definitively out of scope.
	p = active([]EngagementNet{domainNet("example.net")}, nil)
	wantCode(t, p, "WebFetch", webApp, EngagementOutOfScope)

	// An IP target is always decidable against ip/cidr rows (and can never match a
	// domain row), so a miss is out of scope, never ambiguous.
	p = active([]EngagementNet{domainNet("example.com")}, nil)
	wantCode(t, p, "Bash", nmapIP, EngagementOutOfScope)

	// An unresolved policy blocks everything as ambiguous (fail safe).
	un := &EngagementPolicy{EngagementID: 9, Unresolved: true}
	wantCode(t, un, "Read", readFile, EngagementAmbiguous)
	wantCode(t, un, "WebFetch", webApp, EngagementAmbiguous)
}

// A nil policy is the engagement_id IS NULL fallback: it passes everything (the
// guard skips the step; CheckEngagement on a nil receiver is a safety net).
func TestEngagementNilPolicyPasses(t *testing.T) {
	var p *EngagementPolicy
	if res := p.CheckEngagement("WebFetch", []byte(webApp)); res.Code != "" {
		t.Fatalf("nil policy blocked: %+v", res)
	}
}

// The model-facing block text carries the current platform-control framing, the
// code, the target and final guidance (no stale "[ARTEX ...]" prefix).
func TestEngagementBlockMessage(t *testing.T) {
	res := active([]EngagementNet{domainNet("example.net")}, nil).CheckEngagement("WebFetch", []byte(webApp))
	msg := EngagementBlockMessage(res)
	if !strings.HasPrefix(msg, "[Barrier platform control, not target defense] "+EngagementOutOfScope+": ") {
		t.Fatalf("missing framing: %q", msg)
	}
	for _, want := range []string{"app.example.com", "authorised scope", "Do not reach it"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "ARTEX") {
		t.Fatalf("stale ARTEX framing: %q", msg)
	}
}

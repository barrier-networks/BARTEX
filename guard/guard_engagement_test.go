package guard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/barrier-networks/bartex/db"
	"github.com/barrier-networks/bartex/intercept"
)

// engagementCtx wraps a context with a TargetScope that carries an engagement
// policy. gate selects whether the target-gate opt-in is on; the engagement gate
// is enforced regardless.
func engagementCtx(taskID string, pol *intercept.EngagementPolicy, gate bool) context.Context {
	ctx := intercept.WithTaskContext(context.Background(), taskID, "worker", nil)
	s := intercept.TargetScope{TaskID: taskID, WaitBound: time.Minute, Engagement: pol}
	if gate {
		s.Enabled = func() bool { return true }
	}
	return intercept.WithTargetScope(ctx, s)
}

// activeEng is an active engagement whose window is open now, allowing the given
// domains.
func activeEng(allowDomains ...string) *intercept.EngagementPolicy {
	now := time.Now()
	var allow []intercept.EngagementNet
	for _, d := range allowDomains {
		allow = append(allow, intercept.EngagementNet{Kind: "domain", Domain: d})
	}
	return &intercept.EngagementPolicy{EngagementID: 1, Status: "active",
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Allow: allow}
}

const engFetchApp = `{"url":"https://app.example.com/login"}`

// The engagement gate blocks a window-closed engagement and an out-of-scope
// target, for both guard flavours. A block returns before the target gate is
// consulted, so it holds whether the target-gate opt-in is off or on; the
// in-scope pass is checked with the gate off to isolate the engagement decision
// (the gate-on interaction is covered by TestEngagementBlocksBeforeTargetGate).
func TestEngagementStepBlocks(t *testing.T) {
	now := time.Now()
	closed := activeEng("app.example.com")
	closed.StartsAt, closed.EndsAt = now.Add(-2*time.Hour), now.Add(-time.Hour) // expired

	inScope := activeEng("app.example.com")    // allows app.example.com
	outScope := activeEng("other.example.net") // app.example.com is out of scope

	for name, mk := range map[string]func() *Guard{
		"full guard":        func() *Guard { return New() },
		"target-only guard": func() *Guard { return NewTargetOnly(nil) },
	} {
		for _, gate := range []bool{false, true} {
			// Window closed → block, regardless of the gate.
			block, msg := pre(mk(), engagementCtx("7", closed, gate), "WebFetch", engFetchApp)
			if !block || !strings.HasPrefix(msg, "[BARTEX platform control, not target defense] "+intercept.EngagementWindowClosed+": ") {
				t.Fatalf("%s gate=%v: window-closed block=%v msg=%q", name, gate, block, msg)
			}
			// Out-of-scope target → block, regardless of the gate.
			block, msg = pre(mk(), engagementCtx("7", outScope, gate), "WebFetch", engFetchApp)
			if !block || !strings.Contains(msg, intercept.EngagementOutOfScope) {
				t.Fatalf("%s gate=%v: out-of-scope block=%v msg=%q", name, gate, block, msg)
			}
		}
		// In-scope, active, open window → passes the engagement step (target gate
		// off so the engagement decision is the only gate in play).
		if block, msg := pre(mk(), engagementCtx("7", inScope, false), "WebFetch", engFetchApp); block {
			t.Fatalf("%s: in-scope call blocked: %s", name, msg)
		}
	}
}

// The engagement gate is evaluated BEFORE the target gate: an out-of-scope target
// is blocked by the engagement step and CheckTargets is never reached.
func TestEngagementBlocksBeforeTargetGate(t *testing.T) {
	g := NewTargetOnly(nil)
	gate := &fakeGate{t: t}
	g.gate = gate
	ctx := engagementCtx("7", activeEng("other.example.net"), true) // app.example.com out of scope
	block, msg := pre(g, ctx, "WebFetch", engFetchApp)
	if !block || !strings.Contains(msg, intercept.EngagementOutOfScope) {
		t.Fatalf("out-of-scope not blocked by the engagement step: block=%v msg=%q", block, msg)
	}
	if gate.checks != 0 || len(gate.asked) != 0 {
		t.Fatalf("the target gate was reached before/after an engagement block: checks=%d requests=%d", gate.checks, len(gate.asked))
	}

	// An in-scope target passes the engagement step and does reach the gate.
	gate.checks = 0
	if block, msg := pre(g, engagementCtx("7", activeEng("app.example.com"), true), "WebFetch", engFetchApp); block {
		t.Fatalf("in-scope call blocked: %s", msg)
	}
	if gate.checks != 1 {
		t.Fatalf("in-scope call did not reach the target gate: checks=%d", gate.checks)
	}
}

// A platform deny rule wins before the engagement step: a deny-matched call is
// blocked with the deny rule's text, not an engagement code, and the engagement
// scope is irrelevant (nothing about the target is evaluated).
func TestDenyRuleWinsBeforeEngagement(t *testing.T) {
	ic := intercept.New(nil)
	ic.SetRulesForTest([]db.InterceptRule{
		{ID: 1, Name: "[built-in] block marker", Enabled: true, Priority: 100,
			MatchTarget: "tool_input", MatchType: "string", Action: "deny",
			Pattern: "DENYMARK", Message: "Destructive command rejected"},
	})
	// Engagement that would block app.example.com as out of scope. The deny rule
	// must win first, so the model never sees an engagement code.
	pol := activeEng("other.example.net")

	// Full guard applies the user/built-in rule; deny wins over the engagement.
	full := NewWithInterceptor(ic)
	block, msg := pre(full, engagementCtx("7", pol, true), "Bash", `{"command":"curl https://app.example.com/ # DENYMARK"}`)
	if !block || msg != systemBlockMessage("Destructive command rejected") || strings.Contains(msg, "ENGAGEMENT_") {
		t.Fatalf("full guard: deny did not win before the engagement: block=%v msg=%q", block, msg)
	}

	// Target-only guard: the built-in deny rule also wins before the engagement.
	to := NewTargetOnly(ic)
	block, msg = pre(to, engagementCtx("7", pol, true), "Bash", `{"command":"curl https://app.example.com/ # DENYMARK"}`)
	if !block || msg != systemBlockMessage("Destructive command rejected") || strings.Contains(msg, "ENGAGEMENT_") {
		t.Fatalf("target-only guard: deny did not win before the engagement: block=%v msg=%q", block, msg)
	}
}

// No regression: a nil engagement policy (engagement_id IS NULL) leaves every
// branch of both guard flavours byte-for-byte unchanged. The engagement step is
// skipped, so an otherwise out-of-scope target still passes when no policy is
// attached.
func TestNilEngagementNoRegression(t *testing.T) {
	calls := []struct{ tool, input string }{
		{"WebFetch", engFetchApp},
		{"Bash", `{"command":"curl https://anything.example.org/"}`},
		{"shell_send", `{"text":"nc 203.0.113.7 4444\n"}`},
		{"mcp__scan__run", `{"target":"203.0.113.7"}`},
	}
	// Contexts with a TargetScope but NO engagement policy, gate off, and the bare
	// context with no scope at all. All must behave as before (pass).
	ctxs := map[string]context.Context{
		"scope, nil engagement, gate off": engagementCtx("7", nil, false),
		"no scope":                        context.Background(),
	}
	for name, mk := range map[string]func() *Guard{
		"full guard (NewWithInterceptor)": func() *Guard { return New() },
		"target-only (NewTargetOnly)":     func() *Guard { return NewTargetOnly(nil) },
	} {
		for cname, ctx := range ctxs {
			g := mk()
			for _, c := range calls {
				if block, msg := pre(g, ctx, c.tool, c.input); block {
					t.Fatalf("%s / %s: %s blocked with no engagement: %s", name, cname, c.tool, msg)
				}
			}
		}
	}

	// A full guard with a real interceptor and a passing command: a matching allow
	// rule keeps the call off the judge fallback (which needs a DB), so the outcome
	// is deterministic; nil engagement must not change the pass-through.
	ic := intercept.New(nil)
	ic.SetRulesForTest([]db.InterceptRule{
		{ID: 1, Name: "allow id", Enabled: true, Priority: 100,
			MatchTarget: "tool_input", MatchType: "string", Action: "allow", Pattern: "id"},
	})
	g := NewWithInterceptor(ic)
	if block, msg := pre(g, engagementCtx("7", nil, false), "Bash", `{"command":"id"}`); block {
		t.Fatalf("NewWithInterceptor with nil engagement blocked an allow-ruled call: %s", msg)
	}
}

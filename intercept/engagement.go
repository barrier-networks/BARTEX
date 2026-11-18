package intercept

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// Engagement / Rules-of-Engagement enforcement. This layer is purely additive
// and can only SUBTRACT reach: CheckEngagement returns only "pass" (an empty
// TargetResult) or "block" (a result carrying a code). It has no "allow" that
// could short-circuit a later check. It is evaluated in the guard hook AFTER the
// deny-rule/judge-deny step and BEFORE the target gate's CheckTargets, gated only
// on a non-nil policy, so platform deny still wins and the target gate / pins /
// budgets still run unchanged afterwards on a pass.
//
// A task with engagement_id IS NULL carries no policy (TargetScope.Engagement ==
// nil) and this step is skipped entirely — byte-for-byte legacy behaviour.

// Engagement block codes, carried in the model-facing block text, framed as
// platform controls (not target-side defences).
const (
	// EngagementNotActive: the engagement status is not 'active' (draft,
	// suspended or closed), so no action is authorised.
	EngagementNotActive = "ENGAGEMENT_NOT_ACTIVE"
	// EngagementWindowClosed: now is outside [starts_at, ends_at).
	EngagementWindowClosed = "ENGAGEMENT_WINDOW_CLOSED"
	// EngagementOutOfScope: a declared target matches a deny row, or matches no
	// allow row (deny beats allow).
	EngagementOutOfScope = "ENGAGEMENT_OUT_OF_SCOPE"
	// EngagementAmbiguous: the target could not be classified against the
	// allow/deny sets, or the policy could not be resolved — fail safe => block.
	EngagementAmbiguous = "ENGAGEMENT_AMBIGUOUS"
)

// EngagementNet is one resolved allow/deny scope entry. Kind selects which field
// is meaningful: a domain/subdomain uses Domain; an ip/cidr uses Net; a
// cloud_account uses CloudAccount (Phase-2 advisory — see CheckEngagement).
type EngagementNet struct {
	Kind         string // domain|subdomain|ip|cidr|cloud_account
	Domain       string // lower-case suffix for domain/subdomain
	Net          netip.Prefix
	CloudAccount string
}

// EngagementPolicy is the resolved RoE boundary for a session, evaluated on every
// gated tool call. It is a read-only snapshot built once per session (so the hot
// path does no DB work). It can only subtract reach.
type EngagementPolicy struct {
	EngagementID int64
	Status       string // "active" => may pass the status check; anything else => block
	StartsAt     time.Time
	EndsAt       time.Time
	Allow        []EngagementNet // allow-list
	Deny         []EngagementNet // explicit exclusions; win over Allow
	IntensityCap string
	// Unresolved marks a policy that could not be built from the database at
	// session start. A task flagged for enforcement whose policy is unresolvable
	// must not run wide: every call is blocked as ambiguous (fail safe). The ONLY
	// "open" outcome is the explicit engagement_id IS NULL fallback (nil policy).
	Unresolved bool
	// Now is an injectable clock; nil defaults to time.Now.
	Now func() time.Time
}

func (p *EngagementPolicy) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// block builds a blocking TargetResult for an engagement decision.
func engagementBlock(code string, t Target, reason string) TargetResult {
	return TargetResult{Code: code, Host: t.Host, Port: t.Port, Reason: reason}
}

// CheckEngagement decides whether one tool call is permitted by the engagement
// window and scope. It reuses extractDeclared (the same target extraction the
// target gate uses) so both gates see identical declared targets. It never
// resolves names, never opens an approval request, and never widens: the only
// outcomes are pass (empty TargetResult) or block.
func (p *EngagementPolicy) CheckEngagement(toolName string, input []byte) TargetResult {
	if p == nil {
		return TargetResult{}
	}
	// Fail safe: a policy that could not be resolved blocks everything.
	if p.Unresolved {
		return TargetResult{Code: EngagementAmbiguous,
			Reason: "the engagement policy could not be resolved at session start"}
	}
	// Status gate: only 'active' may pass.
	if p.Status != "active" {
		return TargetResult{Code: EngagementNotActive,
			Reason: "the engagement status is " + policyStatusLabel(p.Status) + ", not active"}
	}
	// Window gate: now must be inside [starts_at, ends_at).
	now := p.now()
	if now.Before(p.StartsAt) || !now.Before(p.EndsAt) {
		return TargetResult{Code: EngagementWindowClosed,
			Reason: "the current time is outside the authorised window [" +
				p.StartsAt.UTC().Format(time.RFC3339) + ", " + p.EndsAt.UTC().Format(time.RFC3339) + ")"}
	}
	// Scope gate: evaluate every declared network target. A call with no declared
	// network target (a pure local tool) is permitted by the engagement — the
	// window and status are already satisfied and the target gate/budgets still
	// apply afterwards.
	declared := extractDeclared(toolName, input).targets
	for _, t := range declared {
		if code := p.evalTarget(t); code != "" {
			return engagementBlock(code, t, p.targetReason(code, t))
		}
	}
	return TargetResult{}
}

// evalTarget classifies one declared target against the allow/deny sets and
// returns a block code, or "" when the target is authorised. Deny wins over
// allow. A target that cannot be decided (a domain that might resolve into an
// ip/cidr allow row, which Phase 1 does not resolve) is ambiguous => block.
func (p *EngagementPolicy) evalTarget(t Target) string {
	addr, isAddr := targetAddr(t)
	pfx, isPfx := targetPrefix(t)

	// 1. Explicit exclusions win.
	for _, r := range p.Deny {
		if scopeMatches(r, t, addr, isAddr, pfx, isPfx, true) {
			return EngagementOutOfScope
		}
	}
	// 2. Must match an allow row.
	for _, r := range p.Allow {
		if scopeMatches(r, t, addr, isAddr, pfx, isPfx, false) {
			return "" // authorised
		}
	}
	// 3. No allow match. A domain target could still resolve into an ip/cidr allow
	//    row; without DNS (Phase 1 does not resolve) that is undecidable => block
	//    as ambiguous. An IP/CIDR target is fully decidable against ip/cidr rows,
	//    and can never match a domain row, so it is definitively out of scope.
	if t.Kind == TargetDomain && p.hasNetAllow() {
		return EngagementAmbiguous
	}
	return EngagementOutOfScope
}

// hasNetAllow reports whether any allow row is an ip/cidr entry.
func (p *EngagementPolicy) hasNetAllow() bool {
	for _, r := range p.Allow {
		if (r.Kind == "ip" || r.Kind == "cidr") && r.Net.IsValid() {
			return true
		}
	}
	return false
}

// scopeMatches reports whether one scope row matches a declared target. deny
// selects the deny-side semantics for a CIDR target: a deny row matches any
// overlap (conservative), while an allow row requires the declared range to be
// fully contained. Domain matching is a suffix match (as declaredScopeFilter does
// in the coverage SQL); CIDR containment mirrors covTargetCTE's net >>= addr.
// cloud_account rows are Phase-2 advisory and never match here (host/CIDR
// enforcement carries Phase 1).
func scopeMatches(r EngagementNet, t Target, addr netip.Addr, isAddr bool, pfx netip.Prefix, isPfx bool, deny bool) bool {
	switch r.Kind {
	case "domain", "subdomain":
		return t.Kind == TargetDomain && domainSuffixMatch(t.Host, r.Domain)
	case "ip", "cidr":
		if !r.Net.IsValid() {
			return false
		}
		switch {
		case isAddr:
			return r.Net.Contains(addr)
		case isPfx:
			if deny {
				return r.Net.Overlaps(pfx)
			}
			return prefixContains(r.Net, pfx)
		}
		return false
	}
	return false
}

// domainSuffixMatch reports whether host equals suffix or is a sub-domain of it.
// Both are lower-case punycode. Mirrors the coverage SQL:
// ts.domain = d.domain OR right(ts.domain, len+1) = '.'||d.domain.
func domainSuffixMatch(host, suffix string) bool {
	if suffix == "" {
		return false
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	suffix = strings.TrimSuffix(strings.ToLower(suffix), ".")
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// prefixContains reports whether outer fully contains inner (outer is no more
// specific than inner and inner's network address falls inside outer).
func prefixContains(outer, inner netip.Prefix) bool {
	if !outer.IsValid() || !inner.IsValid() {
		return false
	}
	if outer.Addr().Is4() != inner.Addr().Is4() {
		return false
	}
	if outer.Bits() > inner.Bits() {
		return false
	}
	return outer.Contains(inner.Masked().Addr())
}

// targetAddr parses an IP-literal target into a netip.Addr.
func targetAddr(t Target) (netip.Addr, bool) {
	if t.Kind != TargetIP {
		return netip.Addr{}, false
	}
	a, err := netip.ParseAddr(t.Host)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// targetPrefix parses a CIDR target into a netip.Prefix.
func targetPrefix(t Target) (netip.Prefix, bool) {
	if t.Kind != TargetCIDR {
		return netip.Prefix{}, false
	}
	p, err := netip.ParsePrefix(t.Host)
	if err != nil {
		return netip.Prefix{}, false
	}
	return p.Masked(), true
}

func policyStatusLabel(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

// targetReason is the human/model reason stored for an engagement block.
func (p *EngagementPolicy) targetReason(code string, t Target) string {
	switch code {
	case EngagementOutOfScope:
		return "target " + t.String() + " is outside the engagement's authorised scope (or matches an exclusion)"
	case EngagementAmbiguous:
		return "target " + t.String() + " could not be placed inside the engagement's authorised scope"
	default:
		return "target " + t.String()
	}
}

// engagementGuidance tells the model what an engagement block means. Every
// engagement block is final for the call (window closed, out of scope, not
// active and ambiguous are not defeatable obstacles): the agent must not retry or
// reach the target another way.
var engagementGuidance = map[string]string{
	EngagementNotActive: "The engagement is not active (draft, suspended or closed): no testing is authorised. " +
		"Do not retry or reach the target another way; an operator must activate the engagement first.",
	EngagementWindowClosed: "The authorised testing window is not open (not started or already ended): no testing is authorised now. " +
		"Do not retry or reach the target another way; an operator must open the window first.",
	EngagementOutOfScope: "This target is outside the engagement's authorised scope, or on its exclusion list. " +
		"Do not reach it by another route or substitute an address; only authorised in-scope targets may be tested.",
	EngagementAmbiguous: "The platform could not place this target inside the engagement's authorised scope. " +
		"Do not retry or substitute another form of the target; stay within the explicitly authorised scope.",
}

// EngagementBlockMessage is the model-facing text of an engagement block,
// mirroring TargetBlockMessage: the platform-control framing prefix, the block
// code, the target and reason, then the guidance. An engagement block is final,
// so the guidance says not to retry or reach the target another way.
func EngagementBlockMessage(res TargetResult) string {
	reason := res.Reason
	if res.Host != "" && !strings.Contains(reason, res.Host) {
		target := res.Host
		if res.Port != 0 {
			target = net.JoinHostPort(res.Host, strconv.Itoa(res.Port))
		}
		if reason == "" {
			reason = "target " + target
		} else {
			reason = "target " + target + ": " + reason
		}
	}
	return "[BARTEX platform control, not target defense] " + res.Code + ": " + reason + ". " + engagementGuidance[res.Code]
}

package report

import (
	"strings"
	"testing"
	"time"

	"github.com/barrier-networks/bartex/db"
)

func TestClientReportHTML(t *testing.T) {
	fs := []*db.DBFinding{
		{ID: 1, Name: "SQL injection in /login", Severity: "critical", Status: "open",
			Summary: "Unauthenticated SQL injection.", Evidence: "payload: ' OR 1=1 --",
			VerificationStatus: "verified", PoELevel: 3,
			TrafficBindings: []db.FindingTrafficBinding{
				{Role: "proof", Snapshot: db.TrafficEvidenceSnapshot{Method: "POST", URL: "https://app.example.com/login", Status: 200}},
			}},
		{ID: 2, Name: "Verbose error messages", Severity: "low", Status: "open", Summary: "Stack traces leak."},
	}
	meta := ClientReportMeta{Client: "Acme plc", EngagementName: "Acme Web App Q4", Reference: "ROE-2026-014",
		ScopeSummary: "app.example.com", PeriodStart: "1 Nov 2026", PeriodEnd: "14 Nov 2026"}

	html, err := ClientReportHTML(fs, meta, time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ClientReportHTML: %v", err)
	}
	for _, want := range []string{
		"CONFIDENTIAL",     // default classification
		"Acme plc",         // client
		"Acme Web App Q4",  // engagement
		"ROE-2026-014",     // authorisation reference
		"Barrier Networks", // brand (logo aria-label)
		"SQL injection in /login",
		"sev-critical",                  // severity pill class
		"9.1",                           // indicative CVSS for critical
		"(indicative)",                  // labelled indicative
		"payload: &#39; OR 1=1",         // evidence, HTML-escaped
		"https://app.example.com/login", // captured traffic
		"NIST SP 800-115",               // methodology reference
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report HTML missing %q", want)
		}
	}
	// Most-severe first: the critical finding appears before the low one.
	if strings.Index(html, "SQL injection") > strings.Index(html, "Verbose error") {
		t.Errorf("findings not ordered most-severe first")
	}
}

func TestClientReportHTMLEmpty(t *testing.T) {
	html, err := ClientReportHTML(nil, ClientReportMeta{Client: "Acme"}, time.Now())
	if err != nil {
		t.Fatalf("ClientReportHTML empty: %v", err)
	}
	if !strings.Contains(html, "No findings were recorded") {
		t.Errorf("empty report missing no-findings note")
	}
}

package report

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/barrier-networks/bartex/db"
	"github.com/barrier-networks/bartex/report/cvss"
)

// ClientReportMeta is the engagement/document context for a client-ready report.
// All fields are optional; sensible defaults are applied when empty.
type ClientReportMeta struct {
	Client         string // client / company name
	EngagementName string // engagement name, when the task is bound to one
	Title          string // report title
	Classification string // e.g. "CONFIDENTIAL"
	Author         string // preparing consultant / team
	Reference      string // engagement authorisation reference
	PeriodStart    string // test window start (pre-formatted)
	PeriodEnd      string // test window end (pre-formatted)
	ScopeSummary   string // one-line authorised-scope summary
}

func (m ClientReportMeta) withDefaults() ClientReportMeta {
	if m.Title == "" {
		m.Title = "Penetration Test Report"
	}
	if m.Classification == "" {
		m.Classification = "CONFIDENTIAL"
	}
	if m.Client == "" {
		m.Client = "Client"
	}
	if m.Author == "" {
		m.Author = "Barrier Networks"
	}
	return m
}

type reportTrafficRow struct {
	Role   string
	Method string
	URL    string
	Status int
}

type reportFinding struct {
	Index            int
	Title            string
	Severity         string // original analyst severity (upper-cased)
	SeverityClass    string // lower-case, for CSS
	CVSSScore        string // formatted "9.8"
	CVSSSeverity     string
	CVSSVector       string
	CVSSIndicative   bool
	Status           string
	Verification     string
	VerificationNote string
	PoE              int
	Summary          string
	Evidence         string
	Counterevidence  string
	Confidence       string
	Assets           []int64
	Traffic          []reportTrafficRow
}

type severityCount struct {
	Severity string
	Class    string
	Count    int
}

type clientReportData struct {
	Meta            ClientReportMeta
	GeneratedAt     string
	Total           int
	SeverityCounts  []severityCount
	Findings        []reportFinding
	HighestSeverity string
}

var severityOrder = []string{"critical", "high", "medium", "low", "info"}

var clientSevRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}

func normSeverity(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if _, ok := clientSevRank[s]; !ok {
		return "info"
	}
	return s
}

// ClientReportHTML renders a Barrier-branded, UK-style penetration-test report
// as a standalone HTML document. Findings are ordered most-severe first. CVSS
// scores are taken from an analyst-set vector when present on the finding, and
// otherwise derived as an indicative score from the finding's severity (clearly
// labelled as indicative).
func ClientReportHTML(fs []*db.DBFinding, meta ClientReportMeta, generatedAt time.Time) (string, error) {
	meta = meta.withDefaults()

	ordered := append([]*db.DBFinding(nil), fs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return clientSevRank[normSeverity(ordered[i].Severity)] < clientSevRank[normSeverity(ordered[j].Severity)]
	})

	counts := map[string]int{}
	findings := make([]reportFinding, 0, len(ordered))
	for i, f := range ordered {
		sev := normSeverity(f.Severity)
		counts[sev]++
		res := cvss.Resolve(findingCVSSVector(f), f.Severity)
		rf := reportFinding{
			Index:            i + 1,
			Title:            clientFindingTitle(f),
			Severity:         strings.ToUpper(sev),
			SeverityClass:    sev,
			CVSSScore:        fmt.Sprintf("%.1f", res.Score),
			CVSSSeverity:     res.Severity,
			CVSSVector:       res.Vector,
			CVSSIndicative:   res.Indicative,
			Status:           nz(f.Status, "open"),
			Verification:     nz(f.VerificationStatus, "not_assessed"),
			VerificationNote: f.VerificationNote,
			PoE:              f.PoELevel,
			Summary:          strings.TrimSpace(f.Summary),
			Evidence:         strings.TrimSpace(f.Evidence),
			Counterevidence:  strings.TrimSpace(f.Counterevidence),
			Confidence:       strings.TrimSpace(f.ConfidenceRationale),
			Assets:           f.AssetIDs,
		}
		for _, b := range f.TrafficBindings {
			rf.Traffic = append(rf.Traffic, reportTrafficRow{
				Role: b.Role, Method: b.Snapshot.Method, URL: b.Snapshot.URL, Status: b.Snapshot.Status,
			})
		}
		findings = append(findings, rf)
	}

	sevCounts := make([]severityCount, 0, len(severityOrder))
	for _, s := range severityOrder {
		sevCounts = append(sevCounts, severityCount{Severity: strings.ToUpper(s), Class: s, Count: counts[s]})
	}
	highest := "none"
	for _, s := range severityOrder {
		if counts[s] > 0 {
			highest = s
			break
		}
	}

	data := clientReportData{
		Meta:            meta,
		GeneratedAt:     generatedAt.UTC().Format("2 January 2006, 15:04 MST"),
		Total:           len(findings),
		SeverityCounts:  sevCounts,
		Findings:        findings,
		HighestSeverity: highest,
	}

	var b bytes.Buffer
	if err := clientReportTmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func clientFindingTitle(f *db.DBFinding) string {
	if t := strings.TrimSpace(f.Name); t != "" {
		return t
	}
	if t := strings.TrimSpace(f.VulnClass); t != "" {
		return t
	}
	return "Uncategorised finding"
}

// findingCVSSVector returns a finding's analyst-set CVSS vector. The vector
// column is Phase 2; until then this is always empty and scores are indicative.
func findingCVSSVector(f *db.DBFinding) string { return "" }

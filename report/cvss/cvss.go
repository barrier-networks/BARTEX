// Package cvss implements the CVSS v3.1 base-score equation and the mapping
// between a free-text severity and an indicative base score. It has no
// dependencies and is used by the client report to present a numeric score and
// vector alongside each finding.
//
// Reference: FIRST CVSS v3.1 specification, section 7.1 (Base equation).
package cvss

import (
	"math"
	"strings"
)

// Result is a resolved CVSS score for a finding.
type Result struct {
	Score      float64 // 0.0 - 10.0, rounded up to one decimal
	Severity   string  // None | Low | Medium | High | Critical
	Vector     string  // the CVSS v3.1 vector string, when one was parsed
	Indicative bool    // true when the score was derived from the severity band, not a vector
}

// metric weight tables (CVSS v3.1 §7.4).
var (
	weightAV = map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.20}
	weightAC = map[string]float64{"L": 0.77, "H": 0.44}
	weightUI = map[string]float64{"N": 0.85, "R": 0.62}
	weightCI = map[string]float64{"H": 0.56, "L": 0.22, "N": 0.00} // C, I and A share this table
	// PR depends on Scope: unchanged vs changed.
	weightPRUnchanged = map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
	weightPRChanged   = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.50}
)

// Parse reads a CVSS v3.0/3.1 base vector and returns its base score. ok is
// false when the string is not a usable base vector (missing mandatory base
// metrics or unknown values).
func Parse(vector string) (score float64, ok bool) {
	m := map[string]string{}
	for _, part := range strings.Split(strings.TrimSpace(vector), "/") {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		m[strings.ToUpper(strings.TrimSpace(kv[0]))] = strings.ToUpper(strings.TrimSpace(kv[1]))
	}
	av, okAV := weightAV[m["AV"]]
	ac, okAC := weightAC[m["AC"]]
	ui, okUI := weightUI[m["UI"]]
	c, okC := weightCI[m["C"]]
	i, okI := weightCI[m["I"]]
	a, okA := weightCI[m["A"]]
	scope := m["S"]
	if scope != "U" && scope != "C" {
		return 0, false
	}
	prTable := weightPRUnchanged
	if scope == "C" {
		prTable = weightPRChanged
	}
	pr, okPR := prTable[m["PR"]]
	if !(okAV && okAC && okUI && okC && okI && okA && okPR) {
		return 0, false
	}

	iss := 1 - ((1 - c) * (1 - i) * (1 - a))
	var impact float64
	if scope == "C" {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}
	if impact <= 0 {
		return 0, true
	}
	exploitability := 8.22 * av * ac * pr * ui
	var base float64
	if scope == "C" {
		base = roundup(math.Min(1.08*(impact+exploitability), 10))
	} else {
		base = roundup(math.Min(impact+exploitability, 10))
	}
	return base, true
}

// roundup rounds up to the nearest 0.1, per the CVSS v3.1 reference
// implementation (integer arithmetic avoids binary-float surprises).
func roundup(input float64) float64 {
	i := int(math.Round(input * 100000))
	if i%10000 == 0 {
		return float64(i) / 100000
	}
	return (math.Floor(float64(i)/10000) + 1) / 10
}

// SeverityForScore maps a base score to the CVSS v3.1 qualitative rating.
func SeverityForScore(score float64) string {
	switch {
	case score <= 0:
		return "None"
	case score < 4.0:
		return "Low"
	case score < 7.0:
		return "Medium"
	case score < 9.0:
		return "High"
	default:
		return "Critical"
	}
}

// indicativeBand maps a free-text finding severity to a representative base
// score in the middle-upper part of its CVSS band, used when a finding carries
// no analyst-set vector.
var indicativeBand = map[string]float64{
	"critical": 9.1,
	"high":     7.5,
	"medium":   5.3,
	"low":      3.1,
	"info":     0.0,
	"none":     0.0,
}

// IndicativeFromSeverity returns a representative score for a free-text
// severity. Unknown severities map to 0.
func IndicativeFromSeverity(sev string) float64 {
	return indicativeBand[strings.ToLower(strings.TrimSpace(sev))]
}

// Resolve prefers an analyst-set vector; failing that it derives an indicative
// score from the free-text severity. The returned Severity is always the CVSS
// rating for the score (so the report is internally consistent).
func Resolve(vector, severity string) Result {
	if score, ok := Parse(vector); ok {
		return Result{Score: score, Severity: SeverityForScore(score), Vector: strings.TrimSpace(vector)}
	}
	score := IndicativeFromSeverity(severity)
	return Result{Score: score, Severity: SeverityForScore(score), Indicative: true}
}

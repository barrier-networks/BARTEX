package cvss

import (
	"math"
	"testing"
)

func TestParseKnownVectors(t *testing.T) {
	cases := []struct {
		vector string
		want   float64
	}{
		// FIRST CVSS v3.1 reference examples.
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8},  // classic unauthenticated RCE
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", 10.0}, // scope changed -> 10.0
		{"CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:L/A:N", 5.4},  // medium
		{"CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:N/I:N/A:N", 0.0},  // no impact -> 0
	}
	for _, c := range cases {
		got, ok := Parse(c.vector)
		if !ok {
			t.Errorf("Parse(%q) not ok", c.vector)
			continue
		}
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("Parse(%q) = %.1f, want %.1f", c.vector, got, c.want)
		}
	}
}

func TestParseRejectsBadVectors(t *testing.T) {
	for _, v := range []string{"", "not-a-vector", "CVSS:3.1/AV:N", "CVSS:3.1/AV:X/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"} {
		if _, ok := Parse(v); ok {
			t.Errorf("Parse(%q) unexpectedly ok", v)
		}
	}
}

func TestSeverityForScore(t *testing.T) {
	cases := map[float64]string{0: "None", 0.1: "Low", 3.9: "Low", 4.0: "Medium", 6.9: "Medium", 7.0: "High", 8.9: "High", 9.0: "Critical", 10.0: "Critical"}
	for score, want := range cases {
		if got := SeverityForScore(score); got != want {
			t.Errorf("SeverityForScore(%.1f) = %q, want %q", score, got, want)
		}
	}
}

func TestResolvePrefersVectorThenIndicative(t *testing.T) {
	// A valid vector wins and is not marked indicative.
	r := Resolve("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", "low")
	if r.Indicative || math.Abs(r.Score-9.8) > 0.001 || r.Severity != "Critical" {
		t.Fatalf("vector resolve = %+v", r)
	}
	// No vector -> indicative from severity, marked as such.
	r = Resolve("", "high")
	if !r.Indicative || math.Abs(r.Score-7.5) > 0.001 || r.Severity != "High" {
		t.Fatalf("indicative resolve = %+v", r)
	}
	// Unknown severity -> 0 / None.
	if r := Resolve("", "banana"); r.Score != 0 || r.Severity != "None" {
		t.Fatalf("unknown severity resolve = %+v", r)
	}
}

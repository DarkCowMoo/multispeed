package main

import (
	"strings"
	"testing"
)

func TestPaletteOffIsPlain(t *testing.T) {
	var p palette
	if got := p.bad(p.bold("x")); got != "x" {
		t.Errorf("colors off should leave text alone, got %q", got)
	}
	if got := (palette{on: true}).good("ok"); got != "\x1b[32mok\x1b[0m" {
		t.Errorf("good = %q", got)
	}
}

func TestBar(t *testing.T) {
	var p palette
	tests := []struct {
		v, maxV float64
		want    string
	}{
		{100, 100, "████"},
		{50, 100, "██░░"},
		{0, 100, "░░░░"},
		{10, 0, "░░░░"},
		{200, 100, "████"},
	}
	for _, tt := range tests {
		if got := bar(p, tt.v, tt.maxV, 4); got != tt.want {
			t.Errorf("bar(%v, %v) = %q, want %q", tt.v, tt.maxV, got, tt.want)
		}
	}
}

func TestFmtBytes(t *testing.T) {
	tests := map[int64]string{
		0:             "0 KB",
		12_345:        "12 KB",
		850_000_000:   "850 MB",
		2_340_000_000: "2.3 GB",
	}
	for n, want := range tests {
		if got := fmtBytes(n); got != want {
			t.Errorf("fmtBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPadding(t *testing.T) {
	if got := padLeft("São", 5); got != "  São" {
		t.Errorf("padLeft counts runes, not bytes: %q", got)
	}
	if got := padRight("toolong", 3); got != "toolong" {
		t.Errorf("padRight must not cut: %q", got)
	}
}

// The banner box must have straight edges: every line the same width.
func TestBannerIsABox(t *testing.T) {
	var b strings.Builder
	printBanner(&b, palette{})
	lines := strings.Split(strings.Trim(b.String(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("banner too short:\n%s", b.String())
	}
	w := width(lines[0])
	for _, l := range lines {
		if width(l) != w {
			t.Errorf("uneven banner line %q (%d, want %d)", l, width(l), w)
		}
	}
}

func TestReleaseVersion(t *testing.T) {
	tests := map[string]string{
		"":        "dev",
		"(devel)": "dev",
		"v1.2.0":  "v1.2.0",
		"v0.0.0-20261109153144-bb496671b87e+dirty": "dev",
		"v0.0.0-20261109153144-bb496671b87e":       "dev",
		"v1.2.1-0.20261109153144-bb496671b87e":     "dev",
	}
	for in, want := range tests {
		if got := releaseVersion(in); got != want {
			t.Errorf("releaseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanSummary(t *testing.T) {
	o := options{k: 1, parallel: 1, duration: 10e9, noUpload: true}
	if got := planSummary(3, o); got != "3 regions × 1 server, download only, about 45s" {
		t.Errorf("plan = %q", got)
	}
}

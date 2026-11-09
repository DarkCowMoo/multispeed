package ui

import (
	"strings"
	"testing"
	"time"
)

func TestPaletteOffIsPlain(t *testing.T) {
	var p Palette
	if got := p.Bad(p.Bold("x")); got != "x" {
		t.Errorf("colors off should leave text alone, got %q", got)
	}
	if got := (Palette{on: true}).Good("ok"); got != "\x1b[32mok\x1b[0m" {
		t.Errorf("Good = %q", got)
	}
}

func TestPaletteScales(t *testing.T) {
	on := Palette{on: true}
	if on.Ping(10, "x") != on.Good("x") || on.Ping(100, "x") != on.Fair("x") || on.Ping(300, "x") != on.Bad("x") {
		t.Error("Ping colors")
	}
	if on.Speed(90, 100, "x") != on.Good("x") || on.Speed(40, 100, "x") != on.Fair("x") || on.Speed(10, 100, "x") != on.Bad("x") {
		t.Error("Speed colors")
	}
	if on.Grade("A+", "x") != on.Good("x") || on.Grade("C", "x") != on.Fair("x") || on.Grade("F", "x") != on.Bad("x") {
		t.Error("Grade colors")
	}
	if on.Level("good", "x") != on.Good("x") || on.Level("ok", "x") != on.Fair("x") || on.Level("poor", "x") != on.Bad("x") {
		t.Error("Level colors")
	}
}

func TestBar(t *testing.T) {
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
		if got := barText(tt.v, tt.maxV, 4); got != tt.want {
			t.Errorf("barText(%v, %v) = %q, want %q", tt.v, tt.maxV, got, tt.want)
		}
	}
	on := Palette{on: true}
	if got := colorBar(on, "██░░"); got != on.Accent("██")+on.Dim("░░") {
		t.Errorf("colorBar = %q", got)
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
		if got := FmtBytes(n); got != want {
			t.Errorf("FmtBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFmtDur(t *testing.T) {
	tests := map[int64]string{ // milliseconds
		0:       "0s",
		45_000:  "45s",
		65_000:  "1m05s",
		902_000: "15m02s",
		1_499:   "1s",
	}
	for ms, want := range tests {
		if got := FmtDur(time.Duration(ms) * time.Millisecond); got != want {
			t.Errorf("FmtDur(%dms) = %q, want %q", ms, got, want)
		}
	}
}

func TestPaddingAndTrunc(t *testing.T) {
	if got := PadLeft("São", 5); got != "  São" {
		t.Errorf("PadLeft counts runes, not bytes: %q", got)
	}
	if got := PadRight("toolong", 3); got != "toolong" {
		t.Errorf("PadRight must not cut: %q", got)
	}
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly", 7, "exactly"},
		{"Frankfurt am Main", 6, "Frank…"},
	}
	for _, tt := range tests {
		if got := Trunc(tt.in, tt.n); got != tt.want {
			t.Errorf("Trunc(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

// The banner box must have straight edges: every line the same width.
func TestBannerIsABox(t *testing.T) {
	var b strings.Builder
	Banner(&b, Palette{}, "v1.2.3")
	lines := strings.Split(strings.Trim(b.String(), "\n"), "\n")
	if len(lines) < 3 || !strings.Contains(b.String(), "multispeed v1.2.3") {
		t.Fatalf("banner:\n%s", b.String())
	}
	w := Width(lines[0])
	for _, l := range lines {
		if Width(l) != w {
			t.Errorf("uneven banner line %q (%d, want %d)", l, Width(l), w)
		}
	}
}

func TestSectionAndInfo(t *testing.T) {
	var b strings.Builder
	Section(&b, Palette{}, "Results")
	Info(&b, Palette{}, "Run ID", "abcd-0123")
	out := b.String()
	if !strings.Contains(out, "━━ Results ━") || !strings.Contains(out, "  Run ID           abcd-0123") {
		t.Errorf("output:\n%s", out)
	}
}

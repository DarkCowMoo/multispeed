package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/results"
)

func TestPrintTableNoUpload(t *testing.T) {
	sums := []results.RegionSummary{{Region: "R", OK: 1, Tested: 1, PingMs: 5, BestDownMbps: 100, BestServer: "S (C)"}}
	var b strings.Builder
	if err := PrintTable(&b, sums, true, Palette{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "Upload") || strings.Contains(b.String(), " 0.0 Mbps") {
		t.Errorf("no upload column expected with -no-upload:\n%s", b.String())
	}
}

func TestPrintTableLayout(t *testing.T) {
	sums := []results.RegionSummary{
		{Region: "Local (nearest)", OK: 3, Tested: 3, PingMs: 6, JitterMs: 0.5, BestDownMbps: 900,
			MedDownMbps: 880, BestUpMbps: 97.8, BestServer: "Example Net (Springfield)"},
		{Region: "Tokyo", OK: 0, Tested: 1, Note: "all servers failed"},
		{Region: "Frankfurt", OK: 2, Tested: 3, PingMs: 65, JitterMs: 0.8, BestDownMbps: 450,
			MedDownMbps: 400, BestUpMbps: 90, BestServer: "NTT DATA, Inc. (Frankfurt)"},
	}
	sums[0].Grade, sums[0].BloatMs = "A", 12
	pct := -12.0
	sums[2].ChangePct, sums[2].PrevDownMbps = &pct, 511
	var b strings.Builder
	if err := PrintTable(&b, sums, false, Palette{}); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"  Region            Ping  Jitter    Download              Median     Upload  Bloat  vs last   OK  Best server",
		"  Local (nearest)   6 ms  0.5 ms  900.0 Mbps  ██████████     880  97.8 Mbps  A          new  3/3  Example Net (Springfield)",
		"  Tokyo  no result (all servers failed)",
		"  Frankfurt        65 ms  0.8 ms  450.0 Mbps  █████░░░░░     400  90.0 Mbps  -         ▼12%  2/3  NTT DATA, Inc. (Frankfurt)",
		"",
	}, "\n")
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

// With one server per region and no history or grades, the table leaves out
// the columns that would carry no information.
func TestPrintTableHidesEmptyColumns(t *testing.T) {
	sums := []results.RegionSummary{{Region: "R", OK: 1, Tested: 1, PingMs: 5, BestDownMbps: 100, BestServer: "S (C)"}}
	var b strings.Builder
	if err := PrintTable(&b, sums, false, Palette{}); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"Median", "OK", "Bloat", "vs last"} {
		if strings.Contains(b.String(), col) {
			t.Errorf("column %q should be hidden:\n%s", col, b.String())
		}
	}
}

func TestPrintTableUnmeasuredBloat(t *testing.T) {
	note := "not measured: the server ignored pings while busy"
	sums := []results.RegionSummary{
		{Region: "Tokyo", OK: 1, Tested: 1, PingMs: 250, BestDownMbps: 150, BloatNote: note},
		{Region: "Frankfurt", OK: 1, Tested: 1, PingMs: 60, BestDownMbps: 500, Grade: "B", BloatMs: 48},
	}
	var b strings.Builder
	if err := PrintTable(&b, sums, true, Palette{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(b.String(), "\n")
	if len(lines) < 3 || !slices.Contains(strings.Fields(lines[1]), "?") || strings.Contains(lines[1], " F") {
		t.Errorf("an unmeasured region should show ? in the Bloat column:\n%s", b.String())
	}
	b.Reset()
	PrintBloatNotes(&b, sums, Palette{})
	if b.String() != "  Bloat Tokyo: "+note+".\n" {
		t.Errorf("notes = %q", b.String())
	}
}

func TestPrintGoodFor(t *testing.T) {
	sums := []results.RegionSummary{
		{Region: "Fast", OK: 1, GoodFor: results.Verdicts(500, 100, 10, 1, 5)},
		{Region: "Slow", OK: 1, GoodFor: results.Verdicts(5, 1, 280, 40, 300)},
	}
	var b strings.Builder
	if err := PrintGoodFor(&b, sums, Palette{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"4K video", "Video calls", "Gaming", "✓", "✗", "1 GB download"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestPrintRoutes(t *testing.T) {
	hops := measure.ParseTrace(tracert)
	sums := []results.RegionSummary{
		{Region: "Frankfurt", Route: &measure.Route{Reached: true, Hops: hops, Jump: &measure.Jump{Hop: 5, AddedMs: 48}}},
		{Region: "Tokyo", Route: &measure.Route{Error: "tracert is not installed"}},
	}
	sums[0].Route.HopNumber(5).Name = "ae-3.r21.frnkge13.de.bb.example.net"
	var b strings.Builder
	PrintRoutes(&b, sums, Palette{})
	for _, want := range []string{"+48 ms at hop 5 (ae-3.r21.frnkge13", "route unknown: tracert is not installed"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in:\n%s", want, b.String())
		}
	}
}

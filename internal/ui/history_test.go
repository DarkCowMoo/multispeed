package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/results"
)

func TestSparkline(t *testing.T) {
	if got := sparkline([]float64{1, 2, 3, 4, 5, 6, 7, 8}); got != "▁▂▃▄▅▆▇█" {
		t.Errorf("sparkline = %q", got)
	}
	if got := sparkline([]float64{5, 5, 5}); got != "▅▅▅" {
		t.Errorf("flat sparkline = %q", got)
	}
	if got := lastN([]float64{1, 2, 3}, 2); len(got) != 2 || got[0] != 2 {
		t.Errorf("lastN = %v", got)
	}
}

func TestPrintHistory(t *testing.T) {
	var b strings.Builder
	if err := PrintHistory(&b, Palette{}, "h.jsonl", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No saved runs yet") {
		t.Errorf("empty history:\n%s", b.String())
	}

	hist := []results.HistoryEntry{
		{Time: time.Now(), Regions: []results.HistoryRegion{{Region: "Local (nearest)", PingMs: 5, DownMbps: 800}}},
		{Time: time.Now(), Regions: []results.HistoryRegion{{Region: "Local (nearest)", PingMs: 6, DownMbps: 600}}},
	}
	b.Reset()
	if err := PrintHistory(&b, Palette{}, "h.jsonl", hist); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2 runs", "600 Mbps", "700", "600–800", "Local 600"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in:\n%s", want, b.String())
		}
	}
}

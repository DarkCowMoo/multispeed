package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "history.jsonl")
	hist, err := loadHistory(path)
	if err != nil || hist != nil {
		t.Fatalf("missing file should be empty history, got %v, %v", hist, err)
	}
	day := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	for i, down := range []float64{400, 500} {
		e := HistoryEntry{Time: day.Add(time.Duration(i) * time.Hour), ISP: "X",
			Regions: []HistoryRegion{{Region: "Frankfurt", PingMs: 60, DownMbps: down}}}
		if err = appendHistory(path, e); err != nil {
			t.Fatal(err)
		}
	}
	// A corrupt line is skipped, not fatal.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString("{not json\n")
	_ = f.Close()

	hist, err = loadHistory(path)
	if err != nil || len(hist) != 2 || hist[1].Regions[0].DownMbps != 500 {
		t.Fatalf("history = %+v, %v", hist, err)
	}

	sums := []RegionSummary{
		{Region: "Frankfurt", OK: 1, BestDownMbps: 450},
		{Region: "Tokyo", OK: 1, BestDownMbps: 100},
	}
	compareWithHistory(sums, hist)
	if sums[0].PrevDownMbps != 500 || sums[0].ChangePct == nil || *sums[0].ChangePct != -10 {
		t.Errorf("Frankfurt compare = %+v", sums[0])
	}
	if sums[1].ChangePct != nil {
		t.Errorf("Tokyo has no history, got %+v", sums[1])
	}
}

func TestHistoryEntrySkipsFailedRegions(t *testing.T) {
	r := Report{Regions: []RegionSummary{{Region: "A", OK: 1, BestDownMbps: 9}, {Region: "B"}}}
	e := historyEntry(&r)
	if len(e.Regions) != 1 || e.Regions[0].Region != "A" {
		t.Errorf("entry = %+v", e)
	}
}

func TestSparkline(t *testing.T) {
	if got := sparkline([]float64{1, 2, 3, 4, 5, 6, 7, 8}); got != "▁▂▃▄▅▆▇█" {
		t.Errorf("sparkline = %q", got)
	}
	if got := sparkline([]float64{5, 5, 5}); got != "▅▅▅" {
		t.Errorf("flat sparkline = %q", got)
	}
}

func TestPrintHistory(t *testing.T) {
	var b strings.Builder
	if err := printHistory(&b, palette{}, "h.jsonl", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No saved runs yet") {
		t.Errorf("empty history:\n%s", b.String())
	}

	hist := []HistoryEntry{
		{Time: time.Now(), Regions: []HistoryRegion{{Region: "Local (nearest)", PingMs: 5, DownMbps: 800}}},
		{Time: time.Now(), Regions: []HistoryRegion{{Region: "Local (nearest)", PingMs: 6, DownMbps: 600}}},
	}
	b.Reset()
	if err := printHistory(&b, palette{}, "h.jsonl", hist); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"2 runs", "600 Mbps", "700", "600–800", "Local 600"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in:\n%s", want, b.String())
		}
	}
}

func TestHistoryPathEnv(t *testing.T) {
	t.Setenv(historyEnv, "custom.jsonl")
	if p, err := historyPath(); err != nil || p != "custom.jsonl" {
		t.Errorf("historyPath = %q, %v", p, err)
	}
}

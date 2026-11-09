package results

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "history.jsonl")
	hist, err := LoadHistory(path)
	if err != nil || hist != nil {
		t.Fatalf("missing file should be empty history, got %v, %v", hist, err)
	}
	day := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	for i, down := range []float64{400, 500} {
		e := HistoryEntry{Time: day.Add(time.Duration(i) * time.Hour), ISP: "X",
			Regions: []HistoryRegion{{Region: "Frankfurt", PingMs: 60, DownMbps: down}}}
		if err = AppendHistory(path, e); err != nil {
			t.Fatal(err)
		}
	}
	// A corrupt line is skipped, not fatal.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString("{not json\n")
	_ = f.Close()

	hist, err = LoadHistory(path)
	if err != nil || len(hist) != 2 || hist[1].Regions[0].DownMbps != 500 {
		t.Fatalf("history = %+v, %v", hist, err)
	}

	sums := []RegionSummary{
		{Region: "Frankfurt", OK: 1, BestDownMbps: 450},
		{Region: "Tokyo", OK: 1, BestDownMbps: 100},
	}
	CompareWithHistory(sums, hist)
	if sums[0].PrevDownMbps != 500 || sums[0].ChangePct == nil || *sums[0].ChangePct != -10 {
		t.Errorf("Frankfurt compare = %+v", sums[0])
	}
	if sums[1].ChangePct != nil {
		t.Errorf("Tokyo has no history, got %+v", sums[1])
	}
}

func TestEntryFromSkipsFailedRegions(t *testing.T) {
	r := Report{RunID: "x", Regions: []RegionSummary{{Region: "A", OK: 1, BestDownMbps: 9}, {Region: "B"}}}
	e := EntryFrom(&r)
	if len(e.Regions) != 1 || e.Regions[0].Region != "A" || e.RunID != "x" {
		t.Errorf("entry = %+v", e)
	}
	if err := AppendHistory(filepath.Join(t.TempDir(), "h"), HistoryEntry{}); err != nil {
		t.Errorf("an empty entry is not saved, and not an error: %v", err)
	}
}

func TestSeriesByRegion(t *testing.T) {
	hist := []HistoryEntry{
		{Regions: []HistoryRegion{{Region: "A", DownMbps: 1, PingMs: 9}, {Region: "B", DownMbps: 5}}},
		{Regions: []HistoryRegion{{Region: "A", DownMbps: 3, PingMs: 8}}},
	}
	s := SeriesByRegion(hist)
	if len(s) != 2 || s[0].Region != "A" || len(s[0].Downs) != 2 || s[0].Pings[1] != 8 || s[1].Region != "B" {
		t.Errorf("series = %+v", s)
	}
	if Mean(s[0].Downs) != 2 || Mean(nil) != 0 {
		t.Error("Mean")
	}
	if lo, hi := MinMax([]float64{3, 1, 2}); lo != 1 || hi != 3 {
		t.Error("MinMax")
	}
}

func TestHistoryPathEnv(t *testing.T) {
	t.Setenv(HistoryEnv, "custom.jsonl")
	if p, err := HistoryPath(); err != nil || p != "custom.jsonl" {
		t.Errorf("HistoryPath = %q, %v", p, err)
	}
}

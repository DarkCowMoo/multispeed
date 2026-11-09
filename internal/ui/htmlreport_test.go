package ui

import (
	stdhtml "html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/results"
)

// tracert is a short Windows trace whose biggest jump (11 -> 60 ms, +49 ms)
// is at hop 5.
const tracert = `
  1    <1 ms    <1 ms    <1 ms  192.168.1.1
  4    12 ms    11 ms    13 ms  198.51.100.4
  5    61 ms    60 ms    62 ms  198.51.100.77
  6    63 ms    62 ms    64 ms  203.0.113.9
`

func TestWriteHTML(t *testing.T) {
	pct := 8.0
	hops := measure.ParseTrace(tracert)
	r := results.Report{
		Tool: "multispeed", Version: "dev", RunID: "abcd-0123", Time: time.Now(), ISP: "Example ISP",
		Settings: results.Settings{Upload: true},
		Regions: []results.RegionSummary{
			{Region: "Local (nearest)", Lat: 32, Lon: 34.8, OK: 1, Tested: 1, PingMs: 6, BestDownMbps: 800,
				BestUpMbps: 90, Grade: "A", BloatMs: 12, ChangePct: &pct, BestServer: "S (C)",
				GoodFor: results.Verdicts(800, 90, 6, 1, 12)},
			{Region: "Tokyo", Lat: 35.7, Lon: 139.7, OK: 1, Tested: 1, PingMs: 250, BestDownMbps: 150,
				BestUpMbps: 50, Route: &measure.Route{Reached: true, Hops: hops, Jump: measure.BiggestJump(hops)}},
			{Region: "Atlantis", Tested: 1, Note: "all servers failed"},
		},
		DataBytes: 2_000_000_000,
	}
	hist := []results.HistoryEntry{
		{Regions: []results.HistoryRegion{{Region: "Tokyo", DownMbps: 120}}},
		{Regions: []results.HistoryRegion{{Region: "Tokyo", DownMbps: 150}}},
	}
	path := filepath.Join(t.TempDir(), "report.html")
	if err := WriteHTML(path, &r, hist); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := stdhtml.UnescapeString(string(b)) // the raw JSON is escaped inside <pre>
	for _, want := range []string{
		"<!doctype html>", "Example ISP", "Local (nearest)", "800.0", "▲8%", "Tokyo",
		"No result (all servers failed)", "4K video", "<polyline", "+49 ms at hop 5", "198.51.100.77",
		"2.0 GB used", "run abcd-0123", `"tool": "multispeed"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report is missing %q", want)
		}
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Error("report must be self-contained (no external links or resources)")
	}
}

package main

import (
	stdhtml "html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteHTML(t *testing.T) {
	pct := 8.0
	r := Report{
		Tool: "multispeed", Version: "dev", Time: time.Now(), ISP: "Example ISP",
		Settings: ReportSettings{Upload: true},
		Regions: []RegionSummary{
			{Region: "Local (nearest)", Lat: 32, Lon: 34.8, OK: 1, Tested: 1, PingMs: 6, BestDownMbps: 800,
				BestUpMbps: 90, Grade: "A", BloatMs: 12, ChangePct: &pct, BestServer: "S (C)",
				GoodFor: verdicts(800, 90, 6, 1, 12)},
			{Region: "Tokyo", Lat: 35.7, Lon: 139.7, OK: 1, Tested: 1, PingMs: 250, BestDownMbps: 150,
				BestUpMbps: 50, Route: &Route{Reached: true, Hops: parseTrace(tracertWindows), Jump: &Jump{Hop: 5, AddedMs: 48}}},
			{Region: "Atlantis", Tested: 1, Note: "all servers failed"},
		},
		DataBytes: 2_000_000_000,
	}
	hist := []HistoryEntry{
		{Regions: []HistoryRegion{{Region: "Tokyo", DownMbps: 120}}},
		{Regions: []HistoryRegion{{Region: "Tokyo", DownMbps: 150}}},
	}
	path := filepath.Join(t.TempDir(), "report.html")
	if err := writeHTML(path, &r, hist); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := stdhtml.UnescapeString(string(b)) // the raw JSON is escaped inside <pre>
	for _, want := range []string{
		"<!doctype html>", "Example ISP", "Local (nearest)", "800.0", "▲8%", "Tokyo",
		"No result (all servers failed)", "4K video", "<polyline", "+48 ms at hop 5",
		"2.0 GB used", `"tool": "multispeed"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report is missing %q", want)
		}
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		t.Error("report must be self-contained (no external links or resources)")
	}
}

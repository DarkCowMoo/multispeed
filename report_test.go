package main

import (
	"strings"
	"testing"
)

func TestAggregate(t *testing.T) {
	s := RegionSummary{Servers: []ServerResult{
		{Sponsor: "A", City: "X", PingMs: 50, JitterMs: 2, DownMbps: 100, UpMbps: 20},
		{Sponsor: "B", City: "Y", PingMs: 40, JitterMs: 1, DownMbps: 300, UpMbps: 10},
		{Sponsor: "C", City: "Z", Error: "timeout"},
		{Sponsor: "D", City: "W", PingMs: 45, DownMbps: 200, UpMbps: 30},
	}}
	aggregate(&s)
	if s.OK != 3 || s.Tested != 4 {
		t.Errorf("ok/tested = %d/%d", s.OK, s.Tested)
	}
	if s.BestDownMbps != 300 || s.BestServer != "B (Y)" || s.MedDownMbps != 200 || s.BestUpMbps != 30 || s.PingMs != 40 {
		t.Errorf("bad summary: %+v", s)
	}
}

func TestAggregateAllFailed(t *testing.T) {
	s := RegionSummary{Servers: []ServerResult{{Error: "x"}, {Error: "y"}}}
	aggregate(&s)
	if s.OK != 0 || s.Note != "all servers failed" {
		t.Errorf("bad summary: %+v", s)
	}
}

func TestPrintTableNoUpload(t *testing.T) {
	sums := []RegionSummary{{Region: "R", OK: 1, Tested: 1, PingMs: 5, BestDownMbps: 100, BestServer: "S (C)"}}
	var b strings.Builder
	if err := printTable(&b, sums, true, palette{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "Upload") || strings.Contains(b.String(), " 0.0 Mbps") {
		t.Errorf("no upload column expected with -no-upload:\n%s", b.String())
	}
}

func TestPrintTableLayout(t *testing.T) {
	sums := []RegionSummary{
		{Region: "Local (nearest)", OK: 3, Tested: 3, PingMs: 6, JitterMs: 0.5, BestDownMbps: 900,
			MedDownMbps: 880, BestUpMbps: 97.8, BestServer: "Cellcom Ltd. (Rosh HaAyin)"},
		{Region: "Tokyo", OK: 0, Tested: 1, Note: "all servers failed"},
		{Region: "Frankfurt", OK: 2, Tested: 3, PingMs: 65, JitterMs: 0.8, BestDownMbps: 450,
			MedDownMbps: 400, BestUpMbps: 90, BestServer: "NTT DATA, Inc. (Frankfurt)"},
	}
	var b strings.Builder
	if err := printTable(&b, sums, false, palette{}); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"  Region              Ping  Jitter     Download                Median       Upload   OK  Best server",
		"  Local (nearest)     6 ms  0.5 ms   900.0 Mbps  ████████████     880    97.8 Mbps  3/3  Cellcom Ltd. (Rosh HaAyin)",
		"  Tokyo             no result (all servers failed)",
		"  Frankfurt          65 ms  0.8 ms   450.0 Mbps  ██████░░░░░░     400    90.0 Mbps  2/3  NTT DATA, Inc. (Frankfurt)",
		"",
	}, "\n")
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestTrunc(t *testing.T) {
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
		if got := trunc(tt.in, tt.n); got != tt.want {
			t.Errorf("trunc(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{3}, 3},
		{[]float64{3, 1, 2}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
	}
	for _, tt := range tests {
		if got := median(tt.in); got != tt.want {
			t.Errorf("median(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

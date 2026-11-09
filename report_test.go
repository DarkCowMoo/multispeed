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
	if err := printTable(&b, sums, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "-  1/1") {
		t.Errorf("upload should be shown as '-' with -no-upload:\n%s", b.String())
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

package main

import (
	"math"
	"testing"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

func TestParseRegions(t *testing.T) {
	rs, err := ParseRegions("local, Frankfurt, New York, Home=31.77,35.21,tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 5 {
		t.Fatalf("want 5 regions, got %d: %+v", len(rs), rs)
	}
	if !rs[0].Local || rs[1].Name != "Frankfurt" || rs[2].Name != "New York" {
		t.Errorf("unexpected: %+v", rs[:3])
	}
	if rs[3].Name != "Home" || rs[3].Lat != 31.77 || rs[3].Lon != 35.21 {
		t.Errorf("custom region parsed wrong: %+v", rs[3])
	}
	if rs[4].Name != "Tokyo" {
		t.Errorf("want Tokyo, got %+v", rs[4])
	}
	if _, err := ParseRegions("atlantis"); err == nil {
		t.Error("expected error for unknown region")
	}
	if _, err := ParseRegions("X=100,0"); err == nil {
		t.Error("expected error for bad latitude")
	}
	if _, err := ParseRegions(DefaultRegionsSpec()); err != nil {
		t.Errorf("default regions must parse: %v", err)
	}
}

func DefaultRegionsSpec() string {
	s := ""
	for i, r := range DefaultRegions {
		if i > 0 {
			s += ","
		}
		s += r
	}
	return s
}

func cand(id string, latMs int, dist float64) candidate {
	l := time.Duration(latMs) * time.Millisecond
	if latMs < 0 {
		l = speedtest.PingTimeout
	}
	return candidate{s: &speedtest.Server{ID: id}, latency: l, dist: dist}
}

func ids(cs []candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.s.ID)
	}
	return out
}

func TestSelectCandidates(t *testing.T) {
	cs := []candidate{
		cand("far", 5, 5000),     // too far from region
		cand("dead", -1, 10),     // ping timeout
		cand("a", 80, 10),
		cand("b", 60, 20),
		cand("c", 70, 30),
		cand("d", 50, 40),
		cand("e", 40, 900),
	}
	got := ids(selectCandidates(cs, 3, 1000))
	want := []string{"e", "d", "b"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("got %v, want %v", got, want)
	}
	// k=1: only the 4 nearest are considered (a,b,c,d) -> lowest latency d.
	got = ids(selectCandidates(cs, 1, 1000))
	if len(got) != 1 || got[0] != "d" {
		t.Errorf("k=1: got %v, want [d]", got)
	}
	if n := len(selectCandidates(cs, 3, 1)); n != 0 {
		t.Errorf("expected none within 1 km, got %d", n)
	}
}

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

func TestHaversine(t *testing.T) {
	// Tel Aviv -> Frankfurt is roughly 2,950 km.
	d := haversineKm(32.0853, 34.7818, 50.1109, 8.6821)
	if math.Abs(d-2950) > 60 {
		t.Errorf("distance = %.0f km", d)
	}
}

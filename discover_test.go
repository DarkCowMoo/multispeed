package main

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

func cand(id string, latMs int, dist float64) candidate {
	l := time.Duration(latMs) * time.Millisecond
	if latMs < 0 {
		l = speedtest.PingTimeout
	}
	return candidate{s: &speedtest.Server{ID: id}, latency: l, dist: dist}
}

func ids(cs []candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.s.ID)
	}
	return out
}

func TestSelectCandidates(t *testing.T) {
	cs := []candidate{
		cand("far", 5, 5000), // too far from region
		cand("dead", -1, 10), // ping timeout
		cand("a", 80, 10),
		cand("b", 60, 20),
		cand("c", 70, 30),
		cand("d", 50, 40),
		cand("e", 40, 900),
	}
	tests := []struct {
		name    string
		k       int
		maxDist float64
		want    []string
	}{
		{"k=3 picks lowest latency", 3, 1000, []string{"e", "d", "b"}},
		// k=1: only the 4 nearest are considered (a,b,c,d) -> lowest latency d.
		{"k=1 stays among nearest", 1, 1000, []string{"d"}},
		{"nothing in range", 3, 1, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ids(selectCandidates(cs, tt.k, tt.maxDist))
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHaversine(t *testing.T) {
	// London -> Frankfurt is roughly 640 km.
	d := haversineKm(51.5072, -0.1276, 50.1109, 8.6821)
	if math.Abs(d-640) > 20 {
		t.Errorf("distance = %.0f km", d)
	}
}

func TestServerDistanceBadCoords(t *testing.T) {
	r := Region{Lat: 50, Lon: 8}
	if d := serverDistanceKm(r, &speedtest.Server{Lat: "x", Lon: "8"}); !math.IsInf(d, 1) {
		t.Errorf("unparsable coordinates should be infinitely far, got %v", d)
	}
}

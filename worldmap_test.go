package main

import (
	"strings"
	"testing"
)

func TestIsLand(t *testing.T) {
	land := map[string][2]float64{
		"Frankfurt": {8.7, 50.1}, "New York": {-74, 40.7}, "São Paulo": {-46.6, -23.5},
		"Johannesburg": {28, -26.2}, "Mumbai": {72.9, 19.1}, "Sydney": {151.2, -33.9},
		"Tokyo": {139.7, 35.7}, "Moscow": {37.6, 55.8}, "Denver": {-105, 39.7},
	}
	for name, ll := range land {
		if !isLand(ll[0], ll[1]) {
			t.Errorf("%s should be land", name)
		}
	}
	sea := map[string][2]float64{
		"mid-Atlantic": {-40, 30}, "mid-Pacific": {-150, 0}, "Indian Ocean": {80, -20},
	}
	for name, ll := range sea {
		if isLand(ll[0], ll[1]) {
			t.Errorf("%s should be sea", name)
		}
	}
}

func TestPlaceMarkersAvoidsOverlap(t *testing.T) {
	sums := []RegionSummary{
		{Region: "London", Lat: 51.5, Lon: -0.1},
		{Region: "Amsterdam", Lat: 52.4, Lon: 4.9},
		{Region: "Frankfurt", Lat: 50.1, Lon: 8.7},
		{Region: "Nowhere"}, // no location: not drawn
	}
	ms := placeMarkers(sums)
	if len(ms) != 3 {
		t.Fatalf("markers = %d, want 3", len(ms))
	}
	seen := map[[2]int]bool{}
	for _, m := range ms {
		cell := [2]int{m.col, m.row}
		if seen[cell] {
			t.Errorf("two markers share cell %v", cell)
		}
		seen[cell] = true
	}
	if ms[0].label != 'A' || ms[2].label != 'C' {
		t.Errorf("labels = %c..%c", ms[0].label, ms[2].label)
	}
}

func TestPrintMap(t *testing.T) {
	sums := []RegionSummary{
		{Region: "Local (nearest)", Lat: 32.1, Lon: 34.8, OK: 1, BestDownMbps: 800},
		{Region: "Tokyo", Lat: 35.7, Lon: 139.7, OK: 1, BestDownMbps: 150},
	}
	var b strings.Builder
	if err := printMap(&b, sums, palette{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "A Local 800 Mbps") || !strings.Contains(out, "B Tokyo 150 Mbps") {
		t.Errorf("legend missing:\n%s", out)
	}
	if lines := strings.Count(out, "\n"); lines < mapHeight {
		t.Errorf("map has %d lines", lines)
	}

	b.Reset()
	if err := printMap(&b, sums[:1], palette{}); err != nil || b.Len() != 0 {
		t.Error("a single region is not worth a map")
	}
}

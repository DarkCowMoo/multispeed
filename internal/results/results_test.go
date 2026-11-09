package results

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/regions"
)

func TestAggregate(t *testing.T) {
	s := RegionSummary{Servers: []measure.ServerResult{
		{Sponsor: "A", City: "X", Host: "a:8080", PingMs: 50, JitterMs: 2, DownMbps: 100, UpMbps: 20},
		{Sponsor: "B", City: "Y", Host: "b:8080", PingMs: 40, JitterMs: 1, DownMbps: 300, UpMbps: 10, Grade: "B", BloatMs: 40},
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
	if s.Grade != "B" || s.BloatMs != 40 || s.BestHost != "b:8080" {
		t.Errorf("grade and host should come from the best server: %+v", s)
	}
}

func TestAggregateAllFailed(t *testing.T) {
	s := RegionSummary{Servers: []measure.ServerResult{{Error: "x"}, {Error: "y"}}}
	aggregate(&s)
	if s.OK != 0 || s.Note != "all servers failed" {
		t.Errorf("bad summary: %+v", s)
	}
}

func TestSummarize(t *testing.T) {
	rs := []regions.Region{{Name: "A", Lat: 1, Lon: 2}, {Name: "B"}}
	res := []*measure.ServerResult{
		{Region: "A", Sponsor: "S", PingMs: 20, DownMbps: 300, UpMbps: 50, Grade: "C", BloatMs: 75, RegionIdx: 0},
		{Region: "B", Sponsor: "T", Error: "no data transferred", RegionIdx: 1},
		{Region: "?", RegionIdx: 7}, // out of range: ignored, not a panic
	}
	sums := Summarize(rs, res, []string{"", "only 1 responsive server(s) found"}, false)
	if len(sums) != 2 || sums[0].Lat != 1 || sums[0].Grade != "C" || len(sums[0].GoodFor) != 5 {
		t.Errorf("A = %+v", sums[0])
	}
	if sums[1].OK != 0 || sums[1].Note != "only 1 responsive server(s) found" || sums[1].GoodFor != nil {
		t.Errorf("B = %+v", sums[1])
	}
	// Without upload, there is no upload verdict.
	sums = Summarize(rs[:1], res[:1], nil, true)
	if len(sums[0].GoodFor) != 4 {
		t.Errorf("no-upload verdicts = %+v", sums[0].GoodFor)
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

func TestReportJSON(t *testing.T) {
	r := New(RunInfo{ID: "abcd-0123", Round: 2, ISP: "X", Version: "v1.0.0"},
		Settings{Regions: "local", ServersPerRegion: 1, Upload: true},
		[]RegionSummary{{Region: "Local (nearest)", OK: 1, BestDownMbps: 500, BestHost: "secret:8080"}},
		measure.Stats{DataBytes: 1e9, Elapsed: 61500 * time.Millisecond})
	var b bytes.Buffer
	if err := WriteJSON(&b, &r, false); err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tool", "version", "run_id", "round", "time", "isp", "settings", "regions", "data_used_bytes", "elapsed_seconds"} {
		if _, ok := back[key]; !ok {
			t.Errorf("JSON is missing %q", key)
		}
	}
	if back["elapsed_seconds"] != 61.5 || strings.Contains(b.String(), "secret") {
		t.Errorf("JSON = %s", b.String())
	}

	b.Reset()
	if err := WriteJSON(&b, &r, true); err != nil || strings.Count(b.String(), "\n") != 1 {
		t.Errorf("compact JSON should be one line: %q", b.String())
	}
}

func TestWriteCSVAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.csv")
	res := []*measure.ServerResult{{Region: "A", Sponsor: "S, Inc.", DownMbps: 10}}
	for i := 0; i < 2; i++ {
		if err := WriteCSV(path, res); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "time,region") || !strings.Contains(lines[1], `"S, Inc."`) {
		t.Errorf("CSV = %q", string(b))
	}
}

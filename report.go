package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// ServerResult is one test against one server.
type ServerResult struct {
	Region    string  `json:"region"`
	ID        string  `json:"server_id"`
	Sponsor   string  `json:"sponsor"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Host      string  `json:"host"`
	DistKm    float64 `json:"distance_from_region_km"`
	PingMs    float64 `json:"ping_ms"`
	JitterMs  float64 `json:"jitter_ms"`
	DownMbps  float64 `json:"download_mbps"`
	UpMbps    float64 `json:"upload_mbps"`
	Error     string  `json:"error,omitempty"`
	server    *speedtest.Server
	regionIdx int
}

// RegionSummary aggregates the k servers tested in one region.
type RegionSummary struct {
	Region       string         `json:"region"`
	BestServer   string         `json:"best_server"`
	PingMs       float64        `json:"ping_ms"`
	JitterMs     float64        `json:"jitter_ms"`
	BestDownMbps float64        `json:"best_download_mbps"`
	MedDownMbps  float64        `json:"median_download_mbps"`
	BestUpMbps   float64        `json:"best_upload_mbps"`
	Tested       int            `json:"servers_tested"`
	OK           int            `json:"servers_ok"`
	Note         string         `json:"note,omitempty"`
	Servers      []ServerResult `json:"servers"`
}

// summarize groups results by region and aggregates each region.
func summarize(regions []Region, results []*ServerResult, notes []string) []RegionSummary {
	sums := make([]RegionSummary, len(regions))
	for i, r := range regions {
		sums[i] = RegionSummary{Region: r.Name, Note: notes[i], Servers: []ServerResult{}}
	}
	for _, res := range results {
		s := &sums[res.regionIdx]
		s.Servers = append(s.Servers, *res)
	}
	for i := range sums {
		aggregate(&sums[i])
	}
	return sums
}

// aggregate picks the best server by download speed; ping is the lowest seen.
func aggregate(s *RegionSummary) {
	var downs []float64
	s.Tested = len(s.Servers)
	for i := range s.Servers {
		r := &s.Servers[i]
		if r.Error != "" {
			continue
		}
		s.OK++
		downs = append(downs, r.DownMbps)
		if r.DownMbps > s.BestDownMbps || s.BestServer == "" {
			s.BestDownMbps = r.DownMbps
			s.BestServer = fmt.Sprintf("%s (%s)", r.Sponsor, r.City)
		}
		if r.UpMbps > s.BestUpMbps {
			s.BestUpMbps = r.UpMbps
		}
		if s.PingMs == 0 || r.PingMs < s.PingMs {
			s.PingMs = r.PingMs
			s.JitterMs = r.JitterMs
		}
	}
	s.MedDownMbps = median(downs)
	if s.Tested > 0 && s.OK == 0 && s.Note == "" {
		s.Note = "all servers failed"
	}
}

// writeJSON prints the summaries as indented JSON.
func writeJSON(w io.Writer, sums []RegionSummary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sums)
}

// Column widths of the results table.
const (
	regionCol = 16
	pingCol   = 6  // "126 ms"
	jitterCol = 6  // "0.5 ms"
	speedCol  = 11 // "1000.0 Mbps"
	barCol    = 12
	medianCol = 6
	okCol     = 3
	bestCol   = 30
)

// printTable prints the summaries as an aligned, colored table. Speeds are
// colored against the best region, and each download gets a bar.
func printTable(w io.Writer, sums []RegionSummary, noUpload bool, p palette) error {
	best := 0.0
	regionW := regionCol
	for i := range sums {
		best = max(best, sums[i].BestDownMbps)
		regionW = max(regionW, width(sums[i].Region))
	}

	header := []string{
		padRight("Region", regionW), padLeft("Ping", pingCol), padLeft("Jitter", jitterCol),
		padLeft("Download", speedCol), padRight("", barCol), padLeft("Median", medianCol),
		padLeft("Upload", speedCol), padLeft("OK", okCol), "Best server",
	}
	if noUpload {
		header[6] = padLeft("", speedCol)
	}
	var b strings.Builder
	b.WriteString("  " + p.dim(strings.TrimRight(strings.Join(header, "  "), " ")) + "\n")

	for i := range sums {
		s := &sums[i]
		region := p.bold(padRight(s.Region, regionW))
		ok := padLeft(fmt.Sprintf("%d/%d", s.OK, s.Tested), okCol)
		if s.OK == 0 {
			b.WriteString("  " + region + "  " + p.bad(fmt.Sprintf("no result (%s)", s.Note)) + "\n")
			continue
		}
		if s.OK < s.Tested {
			ok = p.fair(ok)
		}
		up := padLeft(fmt.Sprintf("%.1f Mbps", s.BestUpMbps), speedCol)
		if noUpload {
			up = padLeft("", speedCol)
		}
		cols := []string{
			region,
			p.ping(s.PingMs, padLeft(fmt.Sprintf("%.0f ms", s.PingMs), pingCol)),
			padLeft(fmt.Sprintf("%.1f ms", s.JitterMs), jitterCol),
			p.speed(s.BestDownMbps, best, padLeft(fmt.Sprintf("%.1f Mbps", s.BestDownMbps), speedCol)),
			bar(p, s.BestDownMbps, best, barCol),
			p.dim(padLeft(fmt.Sprintf("%.0f", s.MedDownMbps), medianCol)),
			up,
			ok,
			p.dim(trunc(s.BestServer, bestCol)),
		}
		b.WriteString("  " + strings.TrimRight(strings.Join(cols, "  "), " ") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeCSV saves per-server results to path.
func writeCSV(path string, results []*ServerResult) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	w := csv.NewWriter(f)
	rows := [][]string{{
		"time", "region", "server_id", "sponsor", "city", "country", "distance_km",
		"ping_ms", "jitter_ms", "download_mbps", "upload_mbps", "error",
	}}
	now := time.Now().Format(time.RFC3339)
	for _, r := range results {
		rows = append(rows, []string{
			now, r.Region, r.ID, r.Sponsor, r.City, r.Country,
			f0(r.DistKm), f1(r.PingMs), f1(r.JitterMs), f1(r.DownMbps), f1(r.UpMbps), r.Error,
		})
	}
	return w.WriteAll(rows)
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func f0(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

// serverNames lists "sponsor/city" for each server, shortened for the log.
func serverNames(js []*ServerResult) string {
	n := make([]string, 0, len(js))
	for _, j := range js {
		n = append(n, j.Sponsor+"/"+j.City)
	}
	return trunc(strings.Join(n, ", "), 90)
}

// trunc shortens s to at most n runes, ending with an ellipsis if cut.
func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

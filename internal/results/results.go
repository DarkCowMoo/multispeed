// Package results turns measurements into per-region summaries, judges what
// each region is good for, keeps the history of past runs, and writes the
// JSON and CSV output.
package results

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/regions"
)

// RegionSummary aggregates the servers tested in one region.
type RegionSummary struct {
	Region       string                 `json:"region"`
	Lat          float64                `json:"lat"`
	Lon          float64                `json:"lon"`
	BestServer   string                 `json:"best_server"`
	PingMs       float64                `json:"ping_ms"`
	JitterMs     float64                `json:"jitter_ms"`
	BestDownMbps float64                `json:"best_download_mbps"`
	MedDownMbps  float64                `json:"median_download_mbps"`
	BestUpMbps   float64                `json:"best_upload_mbps"`
	BloatMs      float64                `json:"bufferbloat_ms,omitempty"`
	Grade        string                 `json:"bufferbloat_grade,omitempty"`
	GoodFor      []Verdict              `json:"good_for,omitempty"`
	PrevDownMbps float64                `json:"previous_download_mbps,omitempty"`
	ChangePct    *float64               `json:"download_change_pct,omitempty"`
	Route        *measure.Route         `json:"route,omitempty"`
	Tested       int                    `json:"servers_tested"`
	OK           int                    `json:"servers_ok"`
	Note         string                 `json:"note,omitempty"`
	Servers      []measure.ServerResult `json:"servers"`
	// BestHost is the host of the best server, for tracing its route.
	BestHost string `json:"-"`
}

// Summarize groups results by region, aggregates each region, and judges
// what each region's connection is good for. notes holds one note per region.
func Summarize(rs []regions.Region, res []*measure.ServerResult, notes []string, noUpload bool) []RegionSummary {
	sums := make([]RegionSummary, len(rs))
	for i, r := range rs {
		sums[i] = RegionSummary{Region: r.Name, Lat: r.Lat, Lon: r.Lon, Servers: []measure.ServerResult{}}
		if i < len(notes) {
			sums[i].Note = notes[i]
		}
	}
	for _, r := range res {
		if r.RegionIdx < 0 || r.RegionIdx >= len(sums) {
			continue
		}
		s := &sums[r.RegionIdx]
		s.Servers = append(s.Servers, *r)
	}
	for i := range sums {
		s := &sums[i]
		aggregate(s)
		if s.OK > 0 {
			up := s.BestUpMbps
			if noUpload {
				up = -1
			}
			bloat := -1.0
			if s.Grade != "" {
				bloat = s.BloatMs
			}
			s.GoodFor = Verdicts(s.BestDownMbps, up, s.PingMs, s.JitterMs, bloat)
		}
	}
	return sums
}

// aggregate picks the best server by download speed; ping is the lowest seen.
// The bufferbloat grade comes from the best server.
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
			s.BloatMs, s.Grade = r.BloatMs, r.Grade
			s.BestHost = r.Host
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

// Report is everything one run produced. It is what -json prints; its field
// names are a public interface (scripts, AI agents), so add, don't rename.
type Report struct {
	Tool      string          `json:"tool"`
	Version   string          `json:"version"`
	RunID     string          `json:"run_id"`
	Round     int             `json:"round,omitempty"` // with -every: 1, 2, 3...
	Time      time.Time       `json:"time"`
	ISP       string          `json:"isp,omitempty"`
	Settings  Settings        `json:"settings"`
	Regions   []RegionSummary `json:"regions"`
	DataBytes int64           `json:"data_used_bytes"`
	Seconds   float64         `json:"elapsed_seconds"`
}

// Settings records how the run was configured.
type Settings struct {
	Regions          string  `json:"regions"`
	ServersPerRegion int     `json:"servers_per_region"`
	Parallel         int     `json:"parallel"`
	DurationSeconds  float64 `json:"duration_seconds"`
	Upload           bool    `json:"upload"`
}

// RunInfo identifies a run.
type RunInfo struct {
	ID      string // the run ID shown at start, see NewRunID
	Round   int    // with -every: 1, 2, 3...; 0 otherwise
	ISP     string // the user's internet provider, if known
	Version string // multispeed's version
}

// NewRunID makes a short random ID, like "3f9a-07c2", to tell runs apart.
func NewRunID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("0102-1504")
	}
	h := hex.EncodeToString(b)
	return h[:4] + "-" + h[4:]
}

// New builds the report of a run.
func New(run RunInfo, settings Settings, sums []RegionSummary, stats measure.Stats) Report {
	return Report{
		Tool:      "multispeed",
		Version:   run.Version,
		RunID:     run.ID,
		Round:     run.Round,
		Time:      time.Now().UTC().Truncate(time.Second),
		ISP:       run.ISP,
		Settings:  settings,
		Regions:   sums,
		DataBytes: stats.DataBytes,
		Seconds:   round1(stats.Elapsed.Seconds()),
	}
}

// WriteJSON prints the report as indented JSON, or as one compact line
// (for JSON Lines output, one report per line).
func WriteJSON(w io.Writer, r *Report, compact bool) error {
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(r)
}

// WriteCSV appends per-server results to path, writing a header if the file
// is new, so repeated runs (or -every) build one file over time.
func WriteCSV(path string, res []*measure.ServerResult) (err error) {
	_, statErr := os.Stat(path)
	isNew := errors.Is(statErr, fs.ErrNotExist)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	w := csv.NewWriter(f)
	var rows [][]string
	if isNew {
		rows = append(rows, []string{
			"time", "region", "server_id", "sponsor", "city", "country", "distance_km",
			"ping_ms", "jitter_ms", "download_mbps", "upload_mbps",
			"idle_latency_ms", "loaded_latency_ms", "bufferbloat_grade", "error",
		})
	}
	now := time.Now().Format(time.RFC3339)
	for _, r := range res {
		rows = append(rows, []string{
			now, r.Region, r.ID, r.Sponsor, r.City, r.Country,
			f0(r.DistKm), f1(r.PingMs), f1(r.JitterMs), f1(r.DownMbps), f1(r.UpMbps),
			f1(r.IdleMs), f1(max(r.LoadedDownMs, r.LoadedUpMs)), r.Grade, r.Error,
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

func f0(v float64) string      { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string      { return strconv.FormatFloat(v, 'f', 1, 64) }
func round1(v float64) float64 { return math.Round(v*10) / 10 }

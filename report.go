package main

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
	"strings"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// ServerResult is one test against one server.
type ServerResult struct {
	Region       string  `json:"region"`
	ID           string  `json:"server_id"`
	Sponsor      string  `json:"sponsor"`
	City         string  `json:"city"`
	Country      string  `json:"country"`
	Host         string  `json:"host"`
	DistKm       float64 `json:"distance_from_region_km"`
	PingMs       float64 `json:"ping_ms"`
	JitterMs     float64 `json:"jitter_ms"`
	DownMbps     float64 `json:"download_mbps"`
	UpMbps       float64 `json:"upload_mbps"`
	IdleMs       float64 `json:"idle_latency_ms,omitempty"`
	LoadedDownMs float64 `json:"loaded_latency_download_ms,omitempty"`
	LoadedUpMs   float64 `json:"loaded_latency_upload_ms,omitempty"`
	BloatMs      float64 `json:"bufferbloat_ms,omitempty"`
	Grade        string  `json:"bufferbloat_grade,omitempty"`
	Error        string  `json:"error,omitempty"`
	server       *speedtest.Server
	regionIdx    int
	idleOK       bool // idle latency was measured (it can be 0 ms on a LAN)
	loadedOK     bool // latency under load was measured
}

// gradeBloat sets BloatMs and Grade from the idle and loaded latencies.
func (r *ServerResult) gradeBloat() {
	if !r.idleOK || !r.loadedOK {
		return
	}
	loaded := max(r.LoadedDownMs, r.LoadedUpMs)
	r.BloatMs = round1(max(0, loaded-r.IdleMs))
	r.Grade = bloatGrade(r.BloatMs)
}

// RegionSummary aggregates the servers tested in one region.
type RegionSummary struct {
	Region       string         `json:"region"`
	Lat          float64        `json:"lat"`
	Lon          float64        `json:"lon"`
	BestServer   string         `json:"best_server"`
	PingMs       float64        `json:"ping_ms"`
	JitterMs     float64        `json:"jitter_ms"`
	BestDownMbps float64        `json:"best_download_mbps"`
	MedDownMbps  float64        `json:"median_download_mbps"`
	BestUpMbps   float64        `json:"best_upload_mbps"`
	BloatMs      float64        `json:"bufferbloat_ms,omitempty"`
	Grade        string         `json:"bufferbloat_grade,omitempty"`
	GoodFor      []Verdict      `json:"good_for,omitempty"`
	PrevDownMbps float64        `json:"previous_download_mbps,omitempty"`
	ChangePct    *float64       `json:"download_change_pct,omitempty"`
	Route        *Route         `json:"route,omitempty"`
	Tested       int            `json:"servers_tested"`
	OK           int            `json:"servers_ok"`
	Note         string         `json:"note,omitempty"`
	Servers      []ServerResult `json:"servers"`
	bestHost     string
}

// summarize groups results by region, aggregates each region, and judges
// what each region's connection is good for.
func summarize(regions []Region, results []*ServerResult, notes []string, noUpload bool) []RegionSummary {
	sums := make([]RegionSummary, len(regions))
	for i, r := range regions {
		sums[i] = RegionSummary{Region: r.Name, Lat: r.Lat, Lon: r.Lon, Note: notes[i], Servers: []ServerResult{}}
	}
	for _, res := range results {
		s := &sums[res.regionIdx]
		s.Servers = append(s.Servers, *res)
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
			s.GoodFor = verdicts(s.BestDownMbps, up, s.PingMs, s.JitterMs, bloat)
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
			s.bestHost = r.Host
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

// Report is everything one run produced. It is what -json prints.
type Report struct {
	Tool      string          `json:"tool"`
	Version   string          `json:"version"`
	RunID     string          `json:"run_id"`
	Round     int             `json:"round,omitempty"` // with -every: 1, 2, 3...
	Time      time.Time       `json:"time"`
	ISP       string          `json:"isp,omitempty"`
	Settings  ReportSettings  `json:"settings"`
	Regions   []RegionSummary `json:"regions"`
	DataBytes int64           `json:"data_used_bytes"`
	Seconds   float64         `json:"elapsed_seconds"`
}

// ReportSettings records how the run was configured.
type ReportSettings struct {
	Regions          string  `json:"regions"`
	ServersPerRegion int     `json:"servers_per_region"`
	Parallel         int     `json:"parallel"`
	DurationSeconds  float64 `json:"duration_seconds"`
	Upload           bool    `json:"upload"`
}

// runInfo identifies a run: the run ID shown at start, and the round number
// with -every (0 otherwise).
type runInfo struct {
	id    string
	round int
	isp   string
}

// newRunID makes a short random ID, like "3f9a-07c2", to tell runs apart.
func newRunID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("0102-1504")
	}
	h := hex.EncodeToString(b)
	return h[:4] + "-" + h[4:]
}

func newReport(o options, run runInfo, sums []RegionSummary, stats runStats) Report {
	return Report{
		Tool:    "multispeed",
		Version: version(),
		RunID:   run.id,
		Round:   run.round,
		Time:    time.Now().UTC().Truncate(time.Second),
		ISP:     run.isp,
		Settings: ReportSettings{
			Regions: o.regions, ServersPerRegion: o.k, Parallel: o.parallel,
			DurationSeconds: o.duration.Seconds(), Upload: !o.noUpload,
		},
		Regions:   sums,
		DataBytes: stats.dataBytes,
		Seconds:   round1(stats.elapsed.Seconds()),
	}
}

// writeJSON prints the report as indented JSON.
func writeJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// column is one column of a table: a header and a cell per row. A cell is
// plain text (used to measure widths) plus an optional style applied after
// padding, so color codes never break the alignment.
type column[T any] struct {
	head  string
	right bool
	cell  func(T) (text string, style func(string) string)
}

// renderTable lays out rows under the columns, sized to fit, indented by two
// spaces. full(row) may return a line that replaces the row's cells.
func renderTable[T any](p palette, cols []column[T], rows []T, full func(T) string) string {
	widths := make([]int, len(cols))
	texts := make([][]string, len(rows))
	styles := make([][]func(string) string, len(rows))
	for c, col := range cols {
		widths[c] = width(col.head)
	}
	for r, row := range rows {
		texts[r] = make([]string, len(cols))
		styles[r] = make([]func(string) string, len(cols))
		if full != nil && full(row) != "" {
			continue
		}
		for c, col := range cols {
			texts[r][c], styles[r][c] = col.cell(row)
			widths[c] = max(widths[c], width(texts[r][c]))
		}
	}
	last := len(cols) - 1
	pad := func(c int, s string) string {
		switch {
		case cols[c].right:
			return padLeft(s, widths[c])
		case c == last:
			return s // no trailing spaces, even inside color codes
		default:
			return padRight(s, widths[c])
		}
	}

	var b strings.Builder
	head := make([]string, len(cols))
	for c, col := range cols {
		head[c] = pad(c, col.head)
	}
	b.WriteString("  " + p.dim(strings.TrimRight(strings.Join(head, "  "), " ")) + "\n")
	for r, row := range rows {
		if full != nil {
			if line := full(row); line != "" {
				b.WriteString("  " + line + "\n")
				continue
			}
		}
		cells := make([]string, len(cols))
		for c := range cols {
			cells[c] = pad(c, texts[r][c])
			if st := styles[r][c]; st != nil {
				cells[c] = st(cells[c])
			}
		}
		b.WriteString("  " + strings.TrimRight(strings.Join(cells, "  "), " ") + "\n")
	}
	return b.String()
}

// bestServerWidth caps the "Best server" column.
const bestServerWidth = 30

// barWidth is the width of the download bars.
const barWidth = 10

// printTable prints the results as an aligned, colored table. Speeds are
// colored against the best region, each download gets a bar, and columns
// that carry no information in this run (median and OK with one server per
// region, the change since last run without history) are left out.
func printTable(w io.Writer, sums []RegionSummary, noUpload bool, p palette) error {
	best := 0.0
	multi, history, graded := false, false, false
	for i := range sums {
		s := &sums[i]
		best = max(best, s.BestDownMbps)
		multi = multi || s.Tested > 1
		history = history || s.PrevDownMbps > 0
		graded = graded || s.Grade != ""
	}
	type row = *RegionSummary
	plain := func(s string) (string, func(string) string) { return s, nil }

	cols := []column[row]{
		{head: "Region", cell: func(s row) (string, func(string) string) { return s.Region, p.bold }},
		{head: "Ping", right: true, cell: func(s row) (string, func(string) string) {
			ms := s.PingMs
			return fmt.Sprintf("%.0f ms", ms), func(t string) string { return p.ping(ms, t) }
		}},
		{head: "Jitter", right: true, cell: func(s row) (string, func(string) string) {
			return plain(fmt.Sprintf("%.1f ms", s.JitterMs))
		}},
		{head: "Download", right: true, cell: func(s row) (string, func(string) string) {
			v := s.BestDownMbps
			return fmt.Sprintf("%.1f Mbps", v), func(t string) string { return p.speed(v, best, t) }
		}},
		{head: "", cell: func(s row) (string, func(string) string) {
			return barText(s.BestDownMbps, best, barWidth), func(t string) string { return colorBar(p, t) }
		}},
	}
	if multi {
		cols = append(cols, column[row]{head: "Median", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprintf("%.0f", s.MedDownMbps), p.dim
		}})
	}
	if !noUpload {
		cols = append(cols, column[row]{head: "Upload", right: true, cell: func(s row) (string, func(string) string) {
			return plain(fmt.Sprintf("%.1f Mbps", s.BestUpMbps))
		}})
	}
	if graded {
		cols = append(cols, column[row]{head: "Bloat", cell: func(s row) (string, func(string) string) {
			g := s.Grade
			if g == "" {
				return "-", p.dim
			}
			return g, func(t string) string { return p.grade(g, t) }
		}})
	}
	if history {
		cols = append(cols, column[row]{head: "vs last", right: true, cell: func(s row) (string, func(string) string) {
			return changeText(s.ChangePct, p)
		}})
	}
	if multi {
		cols = append(cols, column[row]{head: "OK", right: true, cell: func(s row) (string, func(string) string) {
			t := fmt.Sprintf("%d/%d", s.OK, s.Tested)
			if s.OK < s.Tested {
				return t, p.fair
			}
			return t, nil
		}})
	}
	cols = append(cols, column[row]{head: "Best server", cell: func(s row) (string, func(string) string) {
		return trunc(s.BestServer, bestServerWidth), p.dim
	}})

	rows := make([]row, len(sums))
	for i := range sums {
		rows[i] = &sums[i]
	}
	failed := func(s row) string {
		if s.OK > 0 {
			return ""
		}
		return p.bold(s.Region) + "  " + p.bad(fmt.Sprintf("no result (%s)", s.Note))
	}
	_, err := io.WriteString(w, renderTable(p, cols, rows, failed))
	return err
}

// changeText describes the change in download since the last run.
func changeText(pct *float64, p palette) (string, func(string) string) {
	switch {
	case pct == nil:
		return "new", p.dim
	case *pct >= 5:
		return fmt.Sprintf("▲%.0f%%", *pct), p.good
	case *pct <= -5:
		return fmt.Sprintf("▼%.0f%%", -*pct), p.bad
	default:
		return "≈", p.dim
	}
}

// printGoodFor prints, for each region, what its connection is good for.
func printGoodFor(w io.Writer, sums []RegionSummary, p palette) error {
	type row = *RegionSummary
	var rows []row
	uses := 0
	for i := range sums {
		if len(sums[i].GoodFor) > 0 {
			rows = append(rows, &sums[i])
			uses = max(uses, len(sums[i].GoodFor))
		}
	}
	if len(rows) == 0 {
		return nil
	}
	cols := []column[row]{{head: "Region", cell: func(s row) (string, func(string) string) { return s.Region, p.bold }}}
	for u := 0; u < uses; u++ {
		head := ""
		for _, r := range rows {
			if u < len(r.GoodFor) {
				head = r.GoodFor[u].Use
				break
			}
		}
		cols = append(cols, column[row]{head: head, cell: func(s row) (string, func(string) string) {
			if u >= len(s.GoodFor) {
				return "", nil
			}
			v := s.GoodFor[u]
			text := verdictMark(v.Level)
			if v.Note != "" {
				text += " " + v.Note
			}
			return text, func(t string) string { return p.level(v.Level, t) }
		}})
	}
	_, err := io.WriteString(w, renderTable(p, cols, rows, nil))
	return err
}

func verdictMark(level string) string {
	switch level {
	case levelGood:
		return "✓"
	case levelOK:
		return "~"
	default:
		return "✗"
	}
}

// writeCSV appends per-server results to path, writing a header if the file
// is new, so repeated runs (or -every) build one file over time.
func writeCSV(path string, results []*ServerResult) (err error) {
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
	for _, r := range results {
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

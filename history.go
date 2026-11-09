package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// historyEnv overrides where history is kept (used by tests, and handy for
// keeping separate histories).
const historyEnv = "MULTISPEED_HISTORY"

// sparkWidth is how many recent runs a history trend line shows.
const sparkWidth = 20

// HistoryEntry is one saved run: one JSON line in the history file.
type HistoryEntry struct {
	Time    time.Time       `json:"time"`
	RunID   string          `json:"run_id,omitempty"`
	ISP     string          `json:"isp,omitempty"`
	Regions []HistoryRegion `json:"regions"`
}

// HistoryRegion is one region's result in a saved run.
type HistoryRegion struct {
	Region   string  `json:"region"`
	PingMs   float64 `json:"ping_ms"`
	DownMbps float64 `json:"download_mbps"`
	UpMbps   float64 `json:"upload_mbps,omitempty"`
	Grade    string  `json:"bufferbloat_grade,omitempty"`
}

// historyPath is the history file: $MULTISPEED_HISTORY, or history.jsonl in
// the user's config folder (e.g. %AppData%\multispeed on Windows).
func historyPath() (string, error) {
	if p := os.Getenv(historyEnv); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "multispeed", "history.jsonl"), nil
}

// loadHistory reads all saved runs, oldest first. A missing file is no history;
// unreadable lines are skipped.
func loadHistory(path string) ([]HistoryEntry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []HistoryEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e HistoryEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && len(e.Regions) > 0 {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// appendHistory adds one run to the history file, creating it if needed.
func appendHistory(path string, e HistoryEntry) (err error) {
	if len(e.Regions) == 0 {
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = f.Write(append(line, '\n'))
	return err
}

// historyEntry turns a report into a history line, keeping regions with results.
func historyEntry(r *Report) HistoryEntry {
	e := HistoryEntry{Time: r.Time, RunID: r.RunID, ISP: r.ISP}
	for i := range r.Regions {
		s := &r.Regions[i]
		if s.OK == 0 {
			continue
		}
		e.Regions = append(e.Regions, HistoryRegion{
			Region: s.Region, PingMs: s.PingMs, DownMbps: s.BestDownMbps,
			UpMbps: s.BestUpMbps, Grade: s.Grade,
		})
	}
	return e
}

// compareWithHistory sets each region's previous download and the change
// since then, from the most recent saved run that tested the region.
func compareWithHistory(sums []RegionSummary, hist []HistoryEntry) {
	for i := range sums {
		s := &sums[i]
		if s.OK == 0 {
			continue
		}
		prev, ok := lastResult(hist, s.Region)
		if !ok || prev.DownMbps <= 0 {
			continue
		}
		s.PrevDownMbps = prev.DownMbps
		pct := round1((s.BestDownMbps - prev.DownMbps) / prev.DownMbps * 100)
		s.ChangePct = &pct
	}
}

func lastResult(hist []HistoryEntry, region string) (HistoryRegion, bool) {
	for i := len(hist) - 1; i >= 0; i-- {
		for _, r := range hist[i].Regions {
			if r.Region == region {
				return r, true
			}
		}
	}
	return HistoryRegion{}, false
}

// regionSeries is one region's download speeds over time, oldest first.
type regionSeries struct {
	region string
	downs  []float64
	pings  []float64
}

// seriesByRegion collects each region's results in order of first appearance.
func seriesByRegion(hist []HistoryEntry) []regionSeries {
	idx := map[string]int{}
	var out []regionSeries
	for _, e := range hist {
		for _, r := range e.Regions {
			i, ok := idx[r.Region]
			if !ok {
				i = len(out)
				idx[r.Region] = i
				out = append(out, regionSeries{region: r.Region})
			}
			out[i].downs = append(out[i].downs, r.DownMbps)
			out[i].pings = append(out[i].pings, r.PingMs)
		}
	}
	return out
}

// sparkline draws values as a small bar chart, e.g. "▃▅▇█▆".
func sparkline(v []float64) string {
	const ticks = "▁▂▃▄▅▆▇█"
	levels := []rune(ticks)
	if len(v) == 0 {
		return ""
	}
	lo, hi := v[0], v[0]
	for _, x := range v {
		lo, hi = min(lo, x), max(hi, x)
	}
	var b strings.Builder
	for _, x := range v {
		i := len(levels) / 2
		if hi > lo {
			i = int((x - lo) / (hi - lo) * float64(len(levels)-1))
		}
		b.WriteRune(levels[i])
	}
	return b.String()
}

// printHistory shows the saved runs: a trend per region and the latest runs.
func printHistory(w io.Writer, p palette, path string, hist []HistoryEntry) error {
	printSection(w, p, "History")
	if len(hist) == 0 {
		fmt.Fprintf(w, "\n  No saved runs yet. Each test is saved to:\n  %s\n\n", path)
		return nil
	}
	fmt.Fprintf(w, "\n  %s\n\n", p.dim(fmt.Sprintf("%d runs since %s, saved in %s",
		len(hist), hist[0].Time.Local().Format("2 Jan 2006"), path)))

	type row = regionSeries
	cols := []column[row]{
		{head: "Region", cell: func(s row) (string, func(string) string) { return s.region, p.bold }},
		{head: "Runs", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprint(len(s.downs)), nil
		}},
		{head: "Latest", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprintf("%.0f Mbps", s.downs[len(s.downs)-1]), p.bold
		}},
		{head: "Average", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprintf("%.0f", mean(s.downs)), nil
		}},
		{head: "Range", right: true, cell: func(s row) (string, func(string) string) {
			lo, hi := minMax(s.downs)
			return fmt.Sprintf("%.0f–%.0f", lo, hi), p.dim
		}},
		{head: "Ping", right: true, cell: func(s row) (string, func(string) string) {
			ms := s.pings[len(s.pings)-1]
			return fmt.Sprintf("%.0f ms", ms), func(t string) string { return p.ping(ms, t) }
		}},
		{head: fmt.Sprintf("Download, last %d runs", sparkWidth), cell: func(s row) (string, func(string) string) {
			d := s.downs
			if len(d) > sparkWidth {
				d = d[len(d)-sparkWidth:]
			}
			return sparkline(d), p.accent
		}},
	}
	if _, err := io.WriteString(w, renderTable(p, cols, seriesByRegion(hist), nil)); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n  %s\n", p.dim("Latest runs"))
	start := max(0, len(hist)-5)
	for _, e := range hist[start:] {
		parts := make([]string, 0, len(e.Regions))
		for _, r := range e.Regions {
			parts = append(parts, fmt.Sprintf("%s %.0f", shortRegion(r.Region), r.DownMbps))
		}
		fmt.Fprintf(w, "  %s  %s\n", p.dim(e.Time.Local().Format("2006-01-02 15:04")),
			trunc(strings.Join(parts, " · "), 100))
	}
	fmt.Fprintln(w)
	return nil
}

// shortRegion drops the "(nearest)" suffix for compact lists.
func shortRegion(name string) string {
	return strings.TrimSuffix(name, " (nearest)")
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func minMax(v []float64) (lo, hi float64) {
	if len(v) == 0 {
		return 0, 0
	}
	lo, hi = v[0], v[0]
	for _, x := range v {
		lo, hi = min(lo, x), max(hi, x)
	}
	return lo, hi
}

package results

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// HistoryEnv names the environment variable that overrides where history is
// kept (used by tests, and handy for keeping separate histories).
const HistoryEnv = "MULTISPEED_HISTORY"

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

// HistoryPath is the history file: $MULTISPEED_HISTORY, or history.jsonl in
// the user's config folder (e.g. %AppData%\multispeed on Windows).
func HistoryPath() (string, error) {
	if p := os.Getenv(HistoryEnv); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "multispeed", "history.jsonl"), nil
}

// LoadHistory reads all saved runs, oldest first. A missing file is no
// history; unreadable lines are skipped.
func LoadHistory(path string) ([]HistoryEntry, error) {
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

// AppendHistory adds one run to the history file, creating it if needed.
func AppendHistory(path string, e HistoryEntry) (err error) {
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

// EntryFrom turns a report into a history line, keeping regions with results.
func EntryFrom(r *Report) HistoryEntry {
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

// CompareWithHistory sets each region's previous download and the change
// since then, from the most recent saved run that tested the region.
func CompareWithHistory(sums []RegionSummary, hist []HistoryEntry) {
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

// Series is one region's results over time, oldest first.
type Series struct {
	Region string
	Downs  []float64 // download speeds, Mbps
	Pings  []float64 // pings, ms
}

// SeriesByRegion collects each region's results in order of first appearance.
func SeriesByRegion(hist []HistoryEntry) []Series {
	idx := map[string]int{}
	var out []Series
	for _, e := range hist {
		for _, r := range e.Regions {
			i, ok := idx[r.Region]
			if !ok {
				i = len(out)
				idx[r.Region] = i
				out = append(out, Series{Region: r.Region})
			}
			out[i].Downs = append(out[i].Downs, r.DownMbps)
			out[i].Pings = append(out[i].Pings, r.PingMs)
		}
	}
	return out
}

// Mean is the average of v, or 0 for none.
func Mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

// MinMax returns the smallest and largest of v, or 0, 0 for none.
func MinMax(v []float64) (lo, hi float64) {
	if len(v) == 0 {
		return 0, 0
	}
	lo, hi = v[0], v[0]
	for _, x := range v {
		lo, hi = min(lo, x), max(hi, x)
	}
	return lo, hi
}

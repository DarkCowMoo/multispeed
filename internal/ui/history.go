package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/DarkCowMoo/multispeed/internal/results"
)

// sparkWidth is how many recent runs a history trend line shows.
const sparkWidth = 20

// sparkline draws values as a small bar chart, e.g. "▃▅▇█▆".
func sparkline(v []float64) string {
	levels := []rune("▁▂▃▄▅▆▇█")
	if len(v) == 0 {
		return ""
	}
	lo, hi := results.MinMax(v)
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

// lastN returns the last n values of v.
func lastN(v []float64, n int) []float64 {
	if len(v) > n {
		return v[len(v)-n:]
	}
	return v
}

// PrintHistory shows the saved runs: a trend per region and the latest runs.
func PrintHistory(w io.Writer, p Palette, path string, hist []results.HistoryEntry) error {
	Section(w, p, "History")
	if len(hist) == 0 {
		fmt.Fprintf(w, "\n  No saved runs yet. Each test is saved to:\n  %s\n\n", path)
		return nil
	}
	fmt.Fprintf(w, "\n  %s\n\n", p.Dim(fmt.Sprintf("%d runs since %s, saved in %s",
		len(hist), hist[0].Time.Local().Format("2 Jan 2006"), path)))

	type row = results.Series
	cols := []column[row]{
		{head: "Region", cell: func(s row) (string, func(string) string) { return s.Region, p.Bold }},
		{head: "Runs", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprint(len(s.Downs)), nil
		}},
		{head: "Latest", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprintf("%.0f Mbps", s.Downs[len(s.Downs)-1]), p.Bold
		}},
		{head: "Average", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprintf("%.0f", results.Mean(s.Downs)), nil
		}},
		{head: "Range", right: true, cell: func(s row) (string, func(string) string) {
			lo, hi := results.MinMax(s.Downs)
			return fmt.Sprintf("%.0f–%.0f", lo, hi), p.Dim
		}},
		{head: "Ping", right: true, cell: func(s row) (string, func(string) string) {
			ms := s.Pings[len(s.Pings)-1]
			return fmt.Sprintf("%.0f ms", ms), func(t string) string { return p.Ping(ms, t) }
		}},
		{head: fmt.Sprintf("Download, last %d runs", sparkWidth), cell: func(s row) (string, func(string) string) {
			return sparkline(lastN(s.Downs, sparkWidth)), p.Accent
		}},
	}
	if _, err := io.WriteString(w, renderTable(p, cols, results.SeriesByRegion(hist), nil)); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n  %s\n", p.Dim("Latest runs"))
	start := max(0, len(hist)-5)
	for _, e := range hist[start:] {
		parts := make([]string, 0, len(e.Regions))
		for _, r := range e.Regions {
			parts = append(parts, fmt.Sprintf("%s %.0f", ShortRegion(r.Region), r.DownMbps))
		}
		fmt.Fprintf(w, "  %s  %s\n", p.Dim(e.Time.Local().Format("2006-01-02 15:04")),
			Trunc(strings.Join(parts, " · "), 100))
	}
	fmt.Fprintln(w)
	return nil
}

// ShortRegion drops the "(nearest)" suffix for compact lists.
func ShortRegion(name string) string {
	return strings.TrimSuffix(name, " (nearest)")
}

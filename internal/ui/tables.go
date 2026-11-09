package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/results"
)

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
func renderTable[T any](p Palette, cols []column[T], rows []T, full func(T) string) string {
	widths := make([]int, len(cols))
	texts := make([][]string, len(rows))
	styles := make([][]func(string) string, len(rows))
	for c, col := range cols {
		widths[c] = Width(col.head)
	}
	for r, row := range rows {
		texts[r] = make([]string, len(cols))
		styles[r] = make([]func(string) string, len(cols))
		if full != nil && full(row) != "" {
			continue
		}
		for c, col := range cols {
			texts[r][c], styles[r][c] = col.cell(row)
			widths[c] = max(widths[c], Width(texts[r][c]))
		}
	}
	last := len(cols) - 1
	pad := func(c int, s string) string {
		switch {
		case cols[c].right:
			return PadLeft(s, widths[c])
		case c == last:
			return s // no trailing spaces, even inside color codes
		default:
			return PadRight(s, widths[c])
		}
	}

	var b strings.Builder
	head := make([]string, len(cols))
	for c, col := range cols {
		head[c] = pad(c, col.head)
	}
	b.WriteString("  " + p.Dim(strings.TrimRight(strings.Join(head, "  "), " ")) + "\n")
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

const (
	// bestServerWidth caps the "Best server" column.
	bestServerWidth = 30
	// barWidth is the width of the download bars.
	barWidth = 10
)

// PrintTable prints the results as an aligned, colored table. Speeds are
// colored against the best region, each download gets a bar, and columns
// that carry no information in this run (median and OK with one server per
// region, the change since last run without history) are left out.
func PrintTable(w io.Writer, sums []results.RegionSummary, noUpload bool, p Palette) error {
	best := 0.0
	multi, history, graded := false, false, false
	for i := range sums {
		s := &sums[i]
		best = max(best, s.BestDownMbps)
		multi = multi || s.Tested > 1
		history = history || s.PrevDownMbps > 0
		graded = graded || s.Grade != ""
	}
	type row = *results.RegionSummary
	plain := func(s string) (string, func(string) string) { return s, nil }

	cols := []column[row]{
		{head: "Region", cell: func(s row) (string, func(string) string) { return s.Region, p.Bold }},
		{head: "Ping", right: true, cell: func(s row) (string, func(string) string) {
			ms := s.PingMs
			return fmt.Sprintf("%.0f ms", ms), func(t string) string { return p.Ping(ms, t) }
		}},
		{head: "Jitter", right: true, cell: func(s row) (string, func(string) string) {
			return plain(fmt.Sprintf("%.1f ms", s.JitterMs))
		}},
		{head: "Download", right: true, cell: func(s row) (string, func(string) string) {
			v := s.BestDownMbps
			return fmt.Sprintf("%.1f Mbps", v), func(t string) string { return p.Speed(v, best, t) }
		}},
		{head: "", cell: func(s row) (string, func(string) string) {
			return barText(s.BestDownMbps, best, barWidth), func(t string) string { return colorBar(p, t) }
		}},
	}
	if multi {
		cols = append(cols, column[row]{head: "Median", right: true, cell: func(s row) (string, func(string) string) {
			return fmt.Sprintf("%.0f", s.MedDownMbps), p.Dim
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
				return "-", p.Dim
			}
			return g, func(t string) string { return p.Grade(g, t) }
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
				return t, p.Fair
			}
			return t, nil
		}})
	}
	cols = append(cols, column[row]{head: "Best server", cell: func(s row) (string, func(string) string) {
		return Trunc(s.BestServer, bestServerWidth), p.Dim
	}})

	rows := make([]row, len(sums))
	for i := range sums {
		rows[i] = &sums[i]
	}
	failed := func(s row) string {
		if s.OK > 0 {
			return ""
		}
		return p.Bold(s.Region) + "  " + p.Bad(fmt.Sprintf("no result (%s)", s.Note))
	}
	_, err := io.WriteString(w, renderTable(p, cols, rows, failed))
	return err
}

// changeText describes the change in download since the last run.
func changeText(pct *float64, p Palette) (string, func(string) string) {
	switch {
	case pct == nil:
		return "new", p.Dim
	case *pct >= 5:
		return fmt.Sprintf("▲%.0f%%", *pct), p.Good
	case *pct <= -5:
		return fmt.Sprintf("▼%.0f%%", -*pct), p.Bad
	default:
		return "≈", p.Dim
	}
}

// PrintGoodFor prints, for each region, what its connection is good for.
func PrintGoodFor(w io.Writer, sums []results.RegionSummary, p Palette) error {
	type row = *results.RegionSummary
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
	cols := []column[row]{{head: "Region", cell: func(s row) (string, func(string) string) { return s.Region, p.Bold }}}
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
			return text, func(t string) string { return p.Level(v.Level, t) }
		}})
	}
	_, err := io.WriteString(w, renderTable(p, cols, rows, nil))
	return err
}

func verdictMark(level string) string {
	switch level {
	case results.LevelGood:
		return "✓"
	case results.LevelOK:
		return "~"
	default:
		return "✗"
	}
}

// PrintRoutes summarizes each traced route on one line.
func PrintRoutes(w io.Writer, sums []results.RegionSummary, p Palette) {
	type row = *results.RegionSummary
	var rows []row
	for i := range sums {
		if sums[i].Route != nil {
			rows = append(rows, &sums[i])
		}
	}
	if len(rows) == 0 {
		return
	}
	cols := []column[row]{
		{head: "Region", cell: func(s row) (string, func(string) string) { return s.Region, p.Bold }},
		{head: "Hops", right: true, cell: func(s row) (string, func(string) string) {
			if !s.Route.Reached {
				return fmt.Sprintf("%d+", len(s.Route.Hops)), p.Fair
			}
			return fmt.Sprint(len(s.Route.Hops)), nil
		}},
		{head: "Biggest delay step", cell: func(s row) (string, func(string) string) {
			j := s.Route.Jump
			if j == nil {
				return "-", p.Dim
			}
			text := fmt.Sprintf("+%.0f ms at hop %d", j.AddedMs, j.Hop)
			if where := s.Route.JumpWhere(); where != "" {
				text += " (" + Trunc(where, 48) + ")"
			}
			return text, nil
		}},
	}
	failed := func(s row) string {
		if s.Route.Error == "" {
			return ""
		}
		return p.Bold(s.Region) + "  " + p.Dim("route unknown: "+s.Route.Error)
	}
	_, _ = io.WriteString(w, renderTable(p, cols, rows, failed))
	fmt.Fprintf(w, "\n  %s\n", p.Dim(fmt.Sprintf(
		"Hops shown as \"N+\": the server itself did not answer the trace (many block it) within %d hops.",
		measure.MaxHops)))
}

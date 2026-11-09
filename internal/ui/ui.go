// Package ui draws multispeed's output: colors, the opening screen, the live
// status line, result tables, the world map and the HTML report.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DarkCowMoo/multispeed/internal/results"
)

// Palette adds ANSI colors to text, or leaves it plain when off.
type Palette struct{ on bool }

// ErrColors styles what goes to stderr. It stays off (plain text) unless
// SetupColors turns it on for a terminal.
var ErrColors Palette

// OutColors styles what goes to stdout, like ErrColors does for stderr.
var OutColors Palette

// SetupColors turns colors on for each stream that is a terminal, unless
// disabled by -no-color, NO_COLOR (https://no-color.org) or TERM=dumb.
func SetupColors(disabled bool) {
	if disabled || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return
	}
	ErrColors.on = IsTerminal(os.Stderr) && enableVT(os.Stderr)
	OutColors.on = IsTerminal(os.Stdout) && enableVT(os.Stdout)
	if ErrColors.on {
		con.style = ErrColors.Dim
	}
}

func (p Palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold styles emphasized text.
func (p Palette) Bold(s string) string { return p.wrap("1", s) }

// Dim styles secondary text.
func (p Palette) Dim(s string) string { return p.wrap("2", s) }

// Title styles section titles.
func (p Palette) Title(s string) string { return p.wrap("1;36", s) }

// Accent styles highlights such as commands and bars.
func (p Palette) Accent(s string) string { return p.wrap("36", s) }

// Good styles good news (green).
func (p Palette) Good(s string) string { return p.wrap("32", s) }

// Fair styles so-so news (yellow).
func (p Palette) Fair(s string) string { return p.wrap("33", s) }

// Bad styles bad news and errors (red).
func (p Palette) Bad(s string) string { return p.wrap("31", s) }

// Ping colors a latency: green when snappy, yellow when noticeable, red when slow.
func (p Palette) Ping(ms float64, s string) string {
	switch {
	case ms < 50:
		return p.Good(s)
	case ms < 150:
		return p.Fair(s)
	default:
		return p.Bad(s)
	}
}

// Speed colors a speed by how it compares with the best speed of this run.
func (p Palette) Speed(mbps, best float64, s string) string {
	if best <= 0 {
		return s
	}
	switch r := mbps / best; {
	case r >= 0.6:
		return p.Good(s)
	case r >= 0.3:
		return p.Fair(s)
	default:
		return p.Bad(s)
	}
}

// Grade colors a bufferbloat grade: A+ and A green, B and C yellow, D and F red.
func (p Palette) Grade(g, s string) string {
	switch g {
	case "A+", "A":
		return p.Good(s)
	case "B", "C":
		return p.Fair(s)
	default:
		return p.Bad(s)
	}
}

// Level colors text by a verdict level (good, ok, poor).
func (p Palette) Level(l, s string) string {
	switch l {
	case results.LevelGood:
		return p.Good(s)
	case results.LevelOK:
		return p.Fair(s)
	default:
		return p.Bad(s)
	}
}

// Width is the number of runes in s. Call it on plain text, not colored text.
func Width(s string) int { return utf8.RuneCountInString(s) }

// PadRight pads plain text to n runes, so it can be colored afterwards.
func PadRight(s string, n int) string { return s + strings.Repeat(" ", max(0, n-Width(s))) }

// PadLeft pads plain text to n runes on the left.
func PadLeft(s string, n int) string { return strings.Repeat(" ", max(0, n-Width(s))) + s }

// Trunc shortens s to at most n runes, ending with an ellipsis if cut.
func Trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// FmtDur formats a duration as "45s" or "3m05s".
func FmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// FmtBytes formats a byte count as "850 MB" or "2.3 GB".
func FmtBytes(n int64) string {
	const mb, gb = 1e6, 1e9
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%d KB", n/1000)
	}
}

// Banner draws the opening screen box with the version.
func Banner(w io.Writer, p Palette, version string) {
	tagline := "Your internet speed to the whole world"
	lines := []struct{ plain, styled string }{
		{"multispeed " + version, p.Title("multispeed") + " " + p.Dim(version)},
		{tagline, tagline},
	}
	inner := 0
	for _, l := range lines {
		inner = max(inner, Width(l.plain))
	}
	inner += 4 // two spaces of margin on each side
	edge := strings.Repeat("─", inner)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+p.Accent("╭"+edge+"╮"))
	for _, l := range lines {
		pad := strings.Repeat(" ", inner-2-Width(l.plain))
		fmt.Fprintln(w, "  "+p.Accent("│")+"  "+l.styled+pad+p.Accent("│"))
	}
	fmt.Fprintln(w, "  "+p.Accent("╰"+edge+"╯"))
}

// Info prints an aligned "label  value" line under the banner.
func Info(w io.Writer, p Palette, label, value string) {
	fmt.Fprintf(w, "  %s %s\n", p.Dim(PadRight(label, 16)), value)
}

// sectionWidth is the width of section titles and their rule.
const sectionWidth = 72

// Section prints a section title such as "━━ Testing ━━━━━━".
func Section(w io.Writer, p Palette, name string) {
	rule := strings.Repeat("━", max(2, sectionWidth-Width(name)-4))
	fmt.Fprintf(w, "\n%s %s %s\n", p.Accent("━━"), p.Title(name), p.Accent(rule))
}

// barText draws value/maxValue as a plain bar of n cells, e.g. "██████░░░░".
func barText(value, maxValue float64, n int) string {
	filled := 0
	if maxValue > 0 {
		filled = int(value/maxValue*float64(n) + 0.5)
	}
	filled = min(max(filled, 0), n)
	return strings.Repeat("█", filled) + strings.Repeat("░", n-filled)
}

// colorBar colors the filled part of a bar and dims the rest.
func colorBar(p Palette, bar string) string {
	i := strings.Index(bar, "░")
	if i < 0 {
		return p.Accent(bar)
	}
	return p.Accent(bar[:i]) + p.Dim(bar[i:])
}

package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"unicode/utf8"
)

// palette adds ANSI colors to text, or leaves it plain when off.
type palette struct{ on bool }

// errColors and outColors style what goes to stderr and stdout. They stay
// off (plain text) unless setupColors turns them on for a terminal.
var errColors, outColors palette

// setupColors turns colors on for each stream that is a terminal, unless
// disabled by -no-color, NO_COLOR (https://no-color.org) or TERM=dumb.
func setupColors(disabled bool) {
	if disabled || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return
	}
	errColors.on = isTerminal(os.Stderr) && enableVT(os.Stderr)
	outColors.on = isTerminal(os.Stdout) && enableVT(os.Stdout)
	if errColors.on {
		con.style = errColors.dim
	}
}

func (p palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) bold(s string) string   { return p.wrap("1", s) }
func (p palette) dim(s string) string    { return p.wrap("2", s) }
func (p palette) title(s string) string  { return p.wrap("1;36", s) }
func (p palette) accent(s string) string { return p.wrap("36", s) }
func (p palette) good(s string) string   { return p.wrap("32", s) }
func (p palette) fair(s string) string   { return p.wrap("33", s) }
func (p palette) bad(s string) string    { return p.wrap("31", s) }

// ping colors a latency: green when snappy, yellow when noticeable, red when slow.
func (p palette) ping(ms float64, s string) string {
	switch {
	case ms < 50:
		return p.good(s)
	case ms < 150:
		return p.fair(s)
	default:
		return p.bad(s)
	}
}

// speed colors a speed by how it compares with the best speed of this run.
func (p palette) speed(mbps, best float64, s string) string {
	if best <= 0 {
		return s
	}
	switch r := mbps / best; {
	case r >= 0.6:
		return p.good(s)
	case r >= 0.3:
		return p.fair(s)
	default:
		return p.bad(s)
	}
}

// width is the number of runes in s, ignoring nothing: call it on plain text.
func width(s string) int { return utf8.RuneCountInString(s) }

// padRight and padLeft pad plain text to n runes, so it can be colored after.
func padRight(s string, n int) string { return s + strings.Repeat(" ", max(0, n-width(s))) }
func padLeft(s string, n int) string  { return strings.Repeat(" ", max(0, n-width(s))) + s }

// version is the release tag for "go install ...@v1.2.3" builds, or "dev".
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		return releaseVersion(bi.Main.Version)
	}
	return "dev"
}

// releaseVersion keeps real release tags and turns Go's automatic build stamps
// (pseudo-versions such as v0.0.0-20261109153144-bb496671b87e+dirty) into "dev".
func releaseVersion(v string) string {
	if v == "" || v == "(devel)" || strings.Contains(v, "+dirty") ||
		strings.HasPrefix(v, "v0.0.0-") || strings.Contains(v, "-0.") {
		return "dev"
	}
	return v
}

// printBanner draws the opening screen.
func printBanner(w io.Writer, p palette) {
	v := version()
	tagline := "Your internet speed to the whole world"
	lines := []struct{ plain, styled string }{
		{"multispeed " + v, p.title("multispeed") + " " + p.dim(v)},
		{tagline, tagline},
	}
	inner := 0
	for _, l := range lines {
		inner = max(inner, width(l.plain))
	}
	inner += 4 // two spaces of margin on each side
	edge := strings.Repeat("─", inner)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+p.accent("╭"+edge+"╮"))
	for _, l := range lines {
		pad := strings.Repeat(" ", inner-2-width(l.plain))
		fmt.Fprintln(w, "  "+p.accent("│")+"  "+l.styled+pad+p.accent("│"))
	}
	fmt.Fprintln(w, "  "+p.accent("╰"+edge+"╯"))
}

// printInfo prints an aligned "label  value" line under the banner.
func printInfo(w io.Writer, p palette, label, value string) {
	fmt.Fprintf(w, "  %s %s\n", p.dim(padRight(label, 16)), value)
}

// sectionWidth is the width of section titles and their rule.
const sectionWidth = 72

// printSection prints a section title such as "━━ Testing ━━━━━━".
func printSection(w io.Writer, p palette, name string) {
	rule := strings.Repeat("━", max(2, sectionWidth-width(name)-4))
	fmt.Fprintf(w, "\n%s %s %s\n", p.accent("━━"), p.title(name), p.accent(rule))
}

// bar draws value/maxValue as a bar of n cells.
func bar(p palette, value, maxValue float64, n int) string {
	filled := 0
	if maxValue > 0 {
		filled = int(value/maxValue*float64(n) + 0.5)
	}
	filled = min(max(filled, 0), n)
	return p.accent(strings.Repeat("█", filled)) + p.dim(strings.Repeat("░", n-filled))
}

// fmtBytes formats a byte count as "850 MB" or "2.3 GB".
func fmtBytes(n int64) string {
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

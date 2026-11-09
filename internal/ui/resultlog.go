package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DarkCowMoo/multispeed/internal/measure"
)

// Column widths of the per-server lines printed while testing.
const (
	countWidth  = 7  // "  12/30"
	serverWidth = 28 // server (sponsor) name
	pingWidth   = 7  // "126 ms"
	speedWidth  = 12 // "916.2 Mbps"
	gradeWidth  = 2  // "A+"
)

// resultLog prints one aligned line per tested server, under a column header
// and a heading for each region.
type resultLog struct {
	mu         sync.Mutex
	printf     func(format string, a ...any)
	p          Palette
	total      int
	noUpload   bool
	headerDone bool
	lastRegion string
	bestDown   float64 // best download so far, to color the others against
}

func newResultLog(total int, noUpload bool, p Palette, printf func(string, ...any)) *resultLog {
	return &resultLog{total: total, noUpload: noUpload, p: p, printf: printf}
}

// row joins pre-padded columns and drops trailing spaces.
func row(cols ...string) string {
	return strings.TrimRight(strings.Join(cols, "  "), " ") + "\n"
}

// add prints the result of j, which was test number done. replaced lists
// servers that failed before j's server was used as their backup.
func (l *resultLog) add(done int, j *measure.ServerResult, replaced []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.p
	if !l.headerDone {
		cols := []string{
			PadLeft("", countWidth), PadRight("Server", serverWidth),
			PadLeft("Ping", pingWidth), PadLeft("Download", speedWidth),
		}
		if !l.noUpload {
			cols = append(cols, PadLeft("Upload", speedWidth))
		}
		cols = append(cols, PadRight("Bloat", gradeWidth))
		l.printf("%s", p.Dim(row(cols...)))
		l.headerDone = true
	}
	if j.Region != l.lastRegion {
		l.printf("%s\n", p.Bold(j.Region))
		l.lastRegion = j.Region
	}
	for _, f := range replaced {
		l.printf("%s  %s\n", PadLeft("", countWidth), p.Dim("skipped "+Trunc(f, 60)+", used a backup"))
	}

	count := p.Dim(PadLeft(fmt.Sprintf("%d/%d", done, l.total), countWidth))
	server := PadRight(Trunc(j.Sponsor, serverWidth), serverWidth)
	if j.Error != "" {
		l.printf("%s", row(count, server, p.Bad("failed: "+j.Error)))
		return
	}
	l.bestDown = max(l.bestDown, j.DownMbps)
	cols := []string{
		count, server,
		p.Ping(j.PingMs, PadLeft(fmt.Sprintf("%.0f ms", j.PingMs), pingWidth)),
		p.Speed(j.DownMbps, l.bestDown, PadLeft(fmt.Sprintf("%.1f Mbps", j.DownMbps), speedWidth)),
	}
	if !l.noUpload {
		cols = append(cols, PadLeft(fmt.Sprintf("%.1f Mbps", j.UpMbps), speedWidth))
	}
	if j.Grade != "" {
		cols = append(cols, p.Grade(j.Grade, PadRight(j.Grade, 2))+
			p.Dim(fmt.Sprintf(" +%.0f ms under load", j.BloatMs)))
	}
	l.printf("%s", row(cols...))
}

package main

import (
	"fmt"
	"strings"
	"sync"
)

// Column widths of the per-server lines printed while testing.
const (
	countWidth  = 7  // "  12/30"
	serverWidth = 28 // server (sponsor) name
	pingWidth   = 7  // "126 ms"
	speedWidth  = 12 // "916.2 Mbps"
)

// resultLog prints one aligned line per tested server, under a column header
// and a heading for each region.
type resultLog struct {
	mu         sync.Mutex
	printf     func(format string, a ...any)
	p          palette
	total      int
	noUpload   bool
	headerDone bool
	lastRegion string
	bestDown   float64 // best download so far, to color the others against
}

func newResultLog(total int, noUpload bool, p palette, printf func(string, ...any)) *resultLog {
	return &resultLog{total: total, noUpload: noUpload, p: p, printf: printf}
}

// row joins pre-padded columns and drops trailing spaces.
func row(cols ...string) string {
	return strings.TrimRight(strings.Join(cols, "  "), " ") + "\n"
}

// add prints the result of j, which was test number done. failed lists
// servers that failed before j's server was used as their backup.
func (l *resultLog) add(done int, j *ServerResult, failed []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.p
	if !l.headerDone {
		cols := []string{
			padLeft("", countWidth), padRight("Server", serverWidth),
			padLeft("Ping", pingWidth), padLeft("Download", speedWidth),
		}
		if !l.noUpload {
			cols = append(cols, padLeft("Upload", speedWidth))
		}
		l.printf("%s", p.dim(row(cols...)))
		l.headerDone = true
	}
	if j.Region != l.lastRegion {
		l.printf("%s\n", p.bold(j.Region))
		l.lastRegion = j.Region
	}
	for _, f := range failed {
		l.printf("%s  %s\n", padLeft("", countWidth), p.dim("skipped "+f+", used a backup"))
	}

	count := p.dim(padLeft(fmt.Sprintf("%d/%d", done, l.total), countWidth))
	server := padRight(trunc(j.Sponsor, serverWidth), serverWidth)
	if j.Error != "" {
		l.printf("%s", row(count, server, p.bad("failed: "+j.Error)))
		return
	}
	l.bestDown = max(l.bestDown, j.DownMbps)
	cols := []string{
		count, server,
		p.ping(j.PingMs, padLeft(fmt.Sprintf("%.0f ms", j.PingMs), pingWidth)),
		p.speed(j.DownMbps, l.bestDown, padLeft(fmt.Sprintf("%.1f Mbps", j.DownMbps), speedWidth)),
	}
	if !l.noUpload {
		cols = append(cols, padLeft(fmt.Sprintf("%.1f Mbps", j.UpMbps), speedWidth))
	}
	l.printf("%s", row(cols...))
}

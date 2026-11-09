package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/measure"
)

const (
	// statusWidth keeps the live status line on one row of a standard terminal,
	// so a carriage return can redraw it in place.
	statusWidth = 79
	// redrawEvery is how often the status line refreshes (elapsed time, live speed).
	redrawEvery = 250 * time.Millisecond
)

// con is where all progress output goes. Results go to stdout instead.
var con = newConsole(os.Stderr)

// Logf writes a progress message to stderr, keeping stdout for results and
// keeping the live status line (if any) below the message.
func Logf(format string, a ...any) { con.logf(format, a...) }

// ShowStatus displays a live status line built by status on stderr,
// refreshed until the returned stop function is called. It does nothing
// when stderr is not a terminal.
func ShowStatus(status func() string) (stop func()) { return con.show(status) }

// IsTerminal reports whether f is a terminal (not a file or a pipe).
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// console writes log lines to w. When w is a terminal it also keeps one live
// status line below them, which log lines scroll above.
type console struct {
	mu     sync.Mutex
	w      io.Writer
	live   bool                // w is a terminal, so the status line is drawn
	status func() string       // builds the status line; nil means none
	style  func(string) string // optional color for the status line
	shown  int                 // width of the status line currently on screen
}

func newConsole(f *os.File) *console {
	return &console{w: f, live: IsTerminal(f)}
}

// logf prints a log line, keeping the status line below it.
func (c *console) logf(format string, a ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearLocked()
	fmt.Fprintf(c.w, format, a...)
	c.drawLocked()
}

// show displays a live status line built by status, refreshed until the
// returned stop function is called. On a non-terminal it does nothing.
func (c *console) show(status func() string) (stop func()) {
	if !c.live {
		return func() {}
	}
	c.mu.Lock()
	c.status = status
	c.drawLocked()
	c.mu.Unlock()

	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(redrawEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c.mu.Lock()
				c.drawLocked()
				c.mu.Unlock()
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		<-finished
		c.mu.Lock()
		c.clearLocked()
		c.status = nil
		c.mu.Unlock()
	}
}

func (c *console) clearLocked() {
	if c.shown == 0 {
		return
	}
	fmt.Fprintf(c.w, "\r%s\r", strings.Repeat(" ", c.shown))
	c.shown = 0
}

func (c *console) drawLocked() {
	if !c.live || c.status == nil {
		return
	}
	line := Trunc(c.status(), statusWidth)
	n := Width(line)
	pad := ""
	if n < c.shown {
		pad = strings.Repeat(" ", c.shown-n) // erase the rest of a longer old line
	}
	if c.style != nil {
		line = c.style(line) // after measuring: color codes take no space
	}
	fmt.Fprintf(c.w, "\r%s%s", line, pad)
	c.shown = max(n, c.shown)
}

// TestProgress shows tests as they run: a live status line (see Status) and
// one result line per finished server. It implements measure.Observer.
type TestProgress struct {
	mu       sync.Mutex
	total    int
	done     int
	started  time.Time
	estimate time.Duration // expected total time, before any test has finished
	active   []*activeTest
	now      func() time.Time // replaceable in tests
	log      *resultLog
}

// activeTest is one server test in progress. It implements measure.Progress.
type activeTest struct {
	t      *TestProgress
	region string
	server string
	phase  string
}

// NewTestProgress follows total tests that are expected to take about
// estimate in all, printing result lines with Logf.
func NewTestProgress(total int, estimate time.Duration, noUpload bool) *TestProgress {
	return &TestProgress{
		total: total, estimate: estimate, started: time.Now(), now: time.Now,
		log: newResultLog(total, noUpload, ErrColors, Logf),
	}
}

// Begin records that a test started.
func (t *TestProgress) Begin(region, server string) measure.Progress {
	a := &activeTest{t: t, region: region, server: server, phase: "starting"}
	t.mu.Lock()
	t.active = append(t.active, a)
	t.mu.Unlock()
	return a
}

// Phase sets what a running test is doing, for example "download 512 Mbps".
func (a *activeTest) Phase(server, phase string) {
	a.t.mu.Lock()
	a.server, a.phase = server, phase
	a.t.mu.Unlock()
}

// End records that a test finished and prints its result line.
func (a *activeTest) End(r *measure.ServerResult, replaced []string) {
	a.t.log.add(a.t.end(a), r, replaced)
}

// end removes a from the running tests and returns how many have finished.
func (t *TestProgress) end(a *activeTest) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, x := range t.active {
		if x == a {
			t.active = append(t.active[:i], t.active[i+1:]...)
			break
		}
	}
	t.done++
	return t.done
}

// Status is the live line, e.g.
// "[2/9] Frankfurt NTT DATA: download 512 Mbps | 1m10s elapsed, ~4m left",
// where 2 is the test in progress.
func (t *TestProgress) Status() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	parts := make([]string, 0, len(t.active))
	for _, a := range t.active {
		if len(t.active) == 1 {
			parts = append(parts, fmt.Sprintf("%s %s: %s", a.region, Trunc(a.server, 20), a.phase))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %s", a.region, a.phase))
		}
	}
	elapsed := t.now().Sub(t.started)
	timing := FmtDur(elapsed) + " elapsed"
	if left, ok := t.timeLeft(elapsed); ok {
		timing += ", ~" + FmtDur(left) + " left"
	}
	parts = append(parts, timing)
	// Number the test(s) in progress, like the result lines do: while test 2
	// runs, show 2/10, not the 1 that has finished.
	current := min(t.done+len(t.active), t.total)
	return fmt.Sprintf("[%d/%d] %s", current, t.total, strings.Join(parts, " | "))
}

// timeLeft estimates the remaining time from the pace so far, or from the
// estimate before any test has finished.
func (t *TestProgress) timeLeft(elapsed time.Duration) (time.Duration, bool) {
	remaining := t.total - t.done
	if remaining <= 0 {
		return 0, false
	}
	var left time.Duration
	if t.done > 0 {
		left = elapsed * time.Duration(remaining) / time.Duration(t.done)
	} else {
		left = t.estimate - elapsed
	}
	if left < time.Second {
		left = time.Second
	}
	return left, true
}

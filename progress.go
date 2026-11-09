package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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
	return &console{w: f, live: isTerminal(f)}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
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
	line := trunc(c.status(), statusWidth)
	n := utf8.RuneCountInString(line)
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

// tracker follows the running tests and builds the status line for them.
type tracker struct {
	mu      sync.Mutex
	total   int
	done    int
	started time.Time
	o       options
	active  []*activeTest
	now     func() time.Time // replaceable in tests
}

// activeTest is one server test in progress.
type activeTest struct {
	region string
	server string
	phase  string
}

func newTracker(total int, o options) *tracker {
	return &tracker{total: total, o: o, started: time.Now(), now: time.Now}
}

// begin records that a test started.
func (t *tracker) begin(region, server string) *activeTest {
	a := &activeTest{region: region, server: server, phase: "starting"}
	t.mu.Lock()
	t.active = append(t.active, a)
	t.mu.Unlock()
	return a
}

// update sets what a running test is doing, for example "download 512 Mbps".
func (t *tracker) update(a *activeTest, server, phase string) {
	t.mu.Lock()
	a.server, a.phase = server, phase
	t.mu.Unlock()
}

// end records that a test finished and returns how many have finished.
func (t *tracker) end(a *activeTest) int {
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

// status is the live line, e.g.
// "[2/9] Frankfurt NTT DATA: download 512 Mbps | 1m10s elapsed, ~4m left".
func (t *tracker) status() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	parts := make([]string, 0, len(t.active))
	for _, a := range t.active {
		if len(t.active) == 1 {
			parts = append(parts, fmt.Sprintf("%s %s: %s", a.region, trunc(a.server, 20), a.phase))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %s", a.region, a.phase))
		}
	}
	elapsed := t.now().Sub(t.started)
	timing := fmtDur(elapsed) + " elapsed"
	if left, ok := t.timeLeft(elapsed); ok {
		timing += ", ~" + fmtDur(left) + " left"
	}
	parts = append(parts, timing)
	return fmt.Sprintf("[%d/%d] %s", t.done, t.total, strings.Join(parts, " | "))
}

// timeLeft estimates the remaining time from the pace so far, or from the
// configured test length before any test has finished.
func (t *tracker) timeLeft(elapsed time.Duration) (time.Duration, bool) {
	remaining := t.total - t.done
	if remaining <= 0 {
		return 0, false
	}
	var left time.Duration
	if t.done > 0 {
		left = elapsed * time.Duration(remaining) / time.Duration(t.done)
	} else {
		left = estimate(t.total, t.o) - elapsed
	}
	if left < time.Second {
		left = time.Second
	}
	return left, true
}

// fmtDur formats a duration as "45s" or "3m05s".
func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

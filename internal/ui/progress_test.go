package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/measure"
)

// The progress view must satisfy the measure package's observer interfaces.
var (
	_ measure.Observer = (*TestProgress)(nil)
	_ measure.Progress = (*activeTest)(nil)
)

func TestStatusLine(t *testing.T) {
	tp := NewTestProgress(3, 48*time.Second, true)
	now := tp.started
	tp.now = func() time.Time { return now }

	a := tp.Begin("Frankfurt", "NTT DATA, Inc.")
	a.Phase("NTT DATA, Inc.", "download 512 Mbps")
	now = now.Add(5 * time.Second)
	// Before any test finishes, time left comes from the estimate: 48s - 5s.
	want := "[1/3] Frankfurt NTT DATA, Inc.: download 512 Mbps | 5s elapsed, ~43s left"
	if got := tp.Status(); got != want {
		t.Errorf("status =\n  %q\nwant\n  %q", got, want)
	}

	if done := tp.end(a.(*activeTest)); done != 1 {
		t.Errorf("done = %d", done)
	}
	now = now.Add(15 * time.Second) // 20s for 1 test -> 2 left take ~40s
	if got := tp.Status(); !strings.HasSuffix(got, "20s elapsed, ~40s left") {
		t.Errorf("status after one test = %q", got)
	}
	tp.Begin("Tokyo", "Contabo") // the second test is now running
	if got := tp.Status(); !strings.HasPrefix(got, "[2/3] Tokyo Contabo") {
		t.Errorf("second test should be numbered 2/3, got %q", got)
	}
}

func TestStatusLineParallelShowsEachRegion(t *testing.T) {
	tp := NewTestProgress(4, time.Minute, true)
	tp.Begin("Frankfurt", "A")
	tp.Begin("Tokyo", "B")
	if got := tp.Status(); !strings.HasPrefix(got, "[2/4] Frankfurt: starting | Tokyo: starting") {
		t.Errorf("status = %q", got)
	}
}

func TestEndPrintsResultLine(t *testing.T) {
	var b strings.Builder
	tp := NewTestProgress(1, time.Minute, true)
	tp.log = newResultLog(1, true, Palette{}, logInto(&b))
	p := tp.Begin("Frankfurt", "dead")
	p.End(&measure.ServerResult{Region: "Frankfurt", Sponsor: "good", PingMs: 60, DownMbps: 500},
		[]string{"dead (no data transferred)"})
	out := b.String()
	for _, want := range []string{"Frankfurt", "skipped dead (no data transferred), used a backup", "1/1  good"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.HasPrefix(tp.Status(), "[1/1]") {
		t.Errorf("status after End = %q", tp.Status())
	}
}

func TestConsoleKeepsStatusBelowLogLines(t *testing.T) {
	var b strings.Builder
	c := &console{w: &b, live: true, status: func() string { return "status" }}
	c.logf("line one\n")
	c.logf("line two\n")
	// Each log line clears the old status line, prints, then redraws status.
	want := "line one\n\rstatus" + "\r      \r" + "line two\n\rstatus"
	if b.String() != want {
		t.Errorf("output =\n  %q\nwant\n  %q", b.String(), want)
	}
}

func TestConsoleNotLiveWritesOnlyLogLines(t *testing.T) {
	var b strings.Builder
	c := &console{w: &b}
	stop := c.show(func() string { return "status" })
	c.logf("hello\n")
	stop()
	if b.String() != "hello\n" {
		t.Errorf("output = %q", b.String())
	}
}

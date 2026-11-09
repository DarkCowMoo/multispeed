package main

import (
	"strings"
	"testing"
	"time"
)

func TestTrackerStatus(t *testing.T) {
	o := options{k: 1, parallel: 1, duration: 10 * time.Second, noUpload: true}
	tr := newTracker(3, o)
	now := tr.started
	tr.now = func() time.Time { return now }

	a := tr.begin("Frankfurt", "NTT DATA, Inc.")
	tr.update(a, "NTT DATA, Inc.", "download 512 Mbps")
	now = now.Add(5 * time.Second)
	got := tr.status()
	// Before any test finishes, time left comes from the estimate: 3*(10s+6s) - 5s.
	want := "[1/3] Frankfurt NTT DATA, Inc.: download 512 Mbps | 5s elapsed, ~43s left"
	if got != want {
		t.Errorf("status =\n  %q\nwant\n  %q", got, want)
	}

	if done := tr.end(a); done != 1 {
		t.Errorf("done = %d", done)
	}
	now = now.Add(15 * time.Second) // 20s for 1 test -> 2 left take ~40s
	if got := tr.status(); !strings.HasSuffix(got, "20s elapsed, ~40s left") {
		t.Errorf("status after one test = %q", got)
	}
	tr.begin("Tokyo", "Contabo") // the second test is now running
	if got := tr.status(); !strings.HasPrefix(got, "[2/3] Tokyo Contabo") {
		t.Errorf("second test should be numbered 2/3, got %q", got)
	}
}

func TestTrackerParallelShowsEachRegion(t *testing.T) {
	tr := newTracker(4, options{k: 1, parallel: 2, duration: time.Second})
	tr.begin("Frankfurt", "A")
	tr.begin("Tokyo", "B")
	got := tr.status()
	if !strings.HasPrefix(got, "[2/4] Frankfurt: starting | Tokyo: starting") {
		t.Errorf("status = %q", got)
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

func TestFmtDur(t *testing.T) {
	tests := map[time.Duration]string{
		0:                       "0s",
		45 * time.Second:        "45s",
		65 * time.Second:        "1m05s",
		15*time.Minute + 2e9:    "15m02s",
		1499 * time.Millisecond: "1s",
	}
	for d, want := range tests {
		if got := fmtDur(d); got != want {
			t.Errorf("fmtDur(%v) = %q, want %q", d, got, want)
		}
	}
}

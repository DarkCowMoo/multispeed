package main

import (
	"fmt"
	"strings"
	"testing"
)

func logInto(b *strings.Builder) func(string, ...any) {
	return func(f string, a ...any) { fmt.Fprintf(b, f, a...) }
}

func TestResultLogLayout(t *testing.T) {
	var b strings.Builder
	l := newResultLog(3, false, palette{}, logInto(&b))
	l.add(1, &ServerResult{Region: "Local (nearest)", Sponsor: "NetFiber", PingMs: 6, DownMbps: 916.2, UpMbps: 97.8}, nil)
	l.add(2, &ServerResult{Region: "Frankfurt", Sponsor: "NTT DATA, Inc.", PingMs: 65, DownMbps: 650.5, UpMbps: 94.5},
		[]string{"Pingless.com (no data transferred)"})
	l.add(3, &ServerResult{Region: "Frankfurt", Sponsor: "Dead", Error: "no data transferred"}, nil)

	want := strings.Join([]string{
		"         Server                           Ping      Download        Upload",
		"Local (nearest)",
		"    1/3  NetFiber                         6 ms    916.2 Mbps     97.8 Mbps",
		"Frankfurt",
		"         skipped Pingless.com (no data transferred), used a backup",
		"    2/3  NTT DATA, Inc.                  65 ms    650.5 Mbps     94.5 Mbps",
		"    3/3  Dead                          failed: no data transferred",
		"",
	}, "\n")
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestResultLogNoUpload(t *testing.T) {
	var b strings.Builder
	l := newResultLog(1, true, palette{}, logInto(&b))
	l.add(1, &ServerResult{Region: "R", Sponsor: "S", PingMs: 5, DownMbps: 100}, nil)
	if strings.Contains(b.String(), "Upload") || strings.Contains(b.String(), " 0.0 Mbps") {
		t.Errorf("no upload column expected:\n%s", b.String())
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing spaces in %q", line)
		}
	}
}

func TestResultLogColors(t *testing.T) {
	var b strings.Builder
	l := newResultLog(2, true, palette{on: true}, logInto(&b))
	l.add(1, &ServerResult{Region: "R", Sponsor: "Fast", PingMs: 5, DownMbps: 900}, nil)
	l.add(2, &ServerResult{Region: "R", Sponsor: "Slow", PingMs: 300, DownMbps: 90}, nil)
	out := b.String()
	if !strings.Contains(out, "\x1b[32m") || !strings.Contains(out, "\x1b[31m") {
		t.Errorf("expected green and red in:\n%q", out)
	}
}

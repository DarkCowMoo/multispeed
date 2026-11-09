package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// fakeSpeedtestServer mimics the endpoints a Speedtest.net server exposes.
func fakeSpeedtestServer(t *testing.T) *httptest.Server {
	t.Helper()
	chunk := make([]byte, 64*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "latency.txt"):
			_, _ = io.WriteString(w, "test=test\n")
		case strings.HasSuffix(r.URL.Path, ".jpg"):
			for i := 0; i < 400; i++ { // ~25 MB per request
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		case r.Method == http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = io.WriteString(w, "size=0")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// deadSpeedtestServer answers the ping but transfers no data.
func deadSpeedtestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "latency.txt") {
			_, _ = io.WriteString(w, "test=test\n")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func job(region, sponsor string, idx int, srv *httptest.Server) *ServerResult {
	return &ServerResult{
		Region:    region,
		Sponsor:   sponsor,
		server:    &speedtest.Server{URL: srv.URL + "/speedtest/upload.php"},
		regionIdx: idx,
	}
}

func TestEndToEndAgainstFakeServers(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	srvA, srvB := fakeSpeedtestServer(t), fakeSpeedtestServer(t)
	jobs := []*ServerResult{
		job("A", "fakeA", 0, srvA),
		job("A", "fakeA2", 0, srvA),
		job("B", "fakeB", 1, srvB),
	}
	o := options{k: 2, parallel: 3, duration: 2 * time.Second}
	res, stats := runTests(context.Background(), jobs, nil, o)
	if stats.dataBytes <= 0 {
		t.Errorf("data used = %d, want > 0", stats.dataBytes)
	}
	for _, r := range res {
		if r.Error != "" {
			t.Fatalf("%s: %s", r.Sponsor, r.Error)
		}
		// Loopback ping can round to 0.0 ms (it does on Windows), so only
		// reject negative values.
		if r.DownMbps <= 0 || r.UpMbps <= 0 || r.PingMs < 0 {
			t.Errorf("%s: expected positive results, got %+v", r.Sponsor, *r)
		}
	}
	sums := summarize([]Region{{Name: "A"}, {Name: "B"}}, res, []string{"", ""}, false)
	if sums[0].OK != 2 || sums[1].OK != 1 || sums[0].BestDownMbps <= 0 {
		t.Errorf("bad summaries: %+v", sums)
	}
}

// A server that answers the ping but sends no data should be replaced by a backup.
func TestFailedServerFallsBackToBackup(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	jobs := []*ServerResult{job("A", "dead", 0, deadSpeedtestServer(t))}
	backups := map[int][]*ServerResult{0: {job("A", "good", 0, fakeSpeedtestServer(t))}}
	o := options{k: 1, parallel: 1, duration: time.Second, noUpload: true}

	res, _ := runTests(context.Background(), jobs, backups, o)
	if res[0].Error != "" || res[0].Sponsor != "good" || res[0].DownMbps <= 0 {
		t.Fatalf("expected fallback to the good server, got %+v", *res[0])
	}
}

func TestFailedServerWithoutBackupKeepsError(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	jobs := []*ServerResult{job("A", "dead", 0, deadSpeedtestServer(t))}
	o := options{k: 1, parallel: 1, duration: time.Second, noUpload: true}

	res, _ := runTests(context.Background(), jobs, nil, o)
	if res[0].Error == "" {
		t.Fatalf("expected an error, got %+v", *res[0])
	}
}

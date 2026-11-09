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
	chunk := make([]byte, 64*1024)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}

func TestEndToEndAgainstFakeServers(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	srvA, srvB := fakeSpeedtestServer(t), fakeSpeedtestServer(t)
	defer srvA.Close()
	defer srvB.Close()

	jobs := []*ServerResult{
		{Region: "A", Sponsor: "fakeA", server: &speedtest.Server{URL: srvA.URL + "/speedtest/upload.php"}, regionIdx: 0},
		{Region: "A", Sponsor: "fakeA2", server: &speedtest.Server{URL: srvA.URL + "/speedtest/upload.php"}, regionIdx: 0},
		{Region: "B", Sponsor: "fakeB", server: &speedtest.Server{URL: srvB.URL + "/speedtest/upload.php"}, regionIdx: 1},
	}
	o := options{k: 2, parallel: 3, duration: 2 * time.Second}
	res := runTests(context.Background(), jobs, o)
	for _, r := range res {
		if r.Error != "" {
			t.Fatalf("%s: %s", r.Sponsor, r.Error)
		}
		if r.DownMbps <= 0 || r.UpMbps <= 0 || r.PingMs <= 0 {
			t.Errorf("%s: expected positive results, got %+v", r.Sponsor, *r)
		}
	}
	sums := summarize([]Region{{Name: "A"}, {Name: "B"}}, res, []string{"", ""})
	if sums[0].OK != 2 || sums[1].OK != 1 || sums[0].BestDownMbps <= 0 {
		t.Errorf("bad summaries: %+v", sums)
	}
	t.Logf("A: %.0f down / %.0f up Mbps, B: %.0f / %.0f", sums[0].BestDownMbps, sums[0].BestUpMbps, sums[1].BestDownMbps, sums[1].BestUpMbps)
}

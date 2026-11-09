package measure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		RegionIdx: idx,
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
	s := Settings{ServersPerRegion: 2, Parallel: 3, Duration: 2 * time.Second}
	res, stats := RunTests(context.Background(), jobs, nil, s, nil)
	if stats.DataBytes <= 0 {
		t.Errorf("data used = %d, want > 0", stats.DataBytes)
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
	for _, r := range res {
		if r.Grade == "" {
			t.Errorf("%s: latency under load should be graded: %+v", r.Sponsor, *r)
		}
	}
}

// recorder is an Observer that remembers what it was told.
type recorder struct {
	mu     sync.Mutex
	begun  []string
	ended  []string
	phases int
}

type recordedTest struct{ r *recorder }

func (r *recorder) Begin(region, server string) Progress {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.begun = append(r.begun, region+"/"+server)
	return recordedTest{r}
}

func (t recordedTest) Phase(string, string) {
	t.r.mu.Lock()
	t.r.phases++
	t.r.mu.Unlock()
}

func (t recordedTest) End(res *ServerResult, replaced []string) {
	t.r.mu.Lock()
	defer t.r.mu.Unlock()
	t.r.ended = append(t.r.ended, fmt.Sprintf("%s replaced=%v err=%q", res.Sponsor, replaced, res.Error))
}

func TestRunTestsReportsToObserver(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	jobs := []*ServerResult{job("A", "dead", 0, deadSpeedtestServer(t))}
	backups := map[int][]*ServerResult{0: {job("A", "good", 0, fakeSpeedtestServer(t))}}
	s := Settings{ServersPerRegion: 1, Parallel: 1, Duration: time.Second, NoUpload: true}

	rec := &recorder{}
	RunTests(context.Background(), jobs, backups, s, rec)
	if len(rec.begun) != 1 || rec.begun[0] != "A/dead" {
		t.Errorf("begun = %v", rec.begun)
	}
	want := `good replaced=[dead (no data transferred)] err=""`
	if len(rec.ended) != 1 || rec.ended[0] != want {
		t.Errorf("ended = %v, want [%s]", rec.ended, want)
	}
	if rec.phases == 0 {
		t.Error("no phases reported")
	}
}

func TestEstimate(t *testing.T) {
	s := Settings{Parallel: 1, Duration: 10 * time.Second, NoUpload: true}
	if got := Estimate(3, s); got != 48*time.Second {
		t.Errorf("Estimate = %v, want 48s (3 × (10s + 6s))", got)
	}
	s.Parallel, s.NoUpload = 2, false
	if got := Estimate(3, s); got != 52*time.Second {
		t.Errorf("parallel Estimate = %v, want 52s (2 rounds × (2×10s + 6s))", got)
	}
}

func TestGradeBloat(t *testing.T) {
	both := ServerResult{IdleMs: 20, LoadedDownMs: 95, LoadedUpMs: 60, idleOK: true, loadedDownOK: true, loadedUpOK: true}
	both.gradeBloat()
	if both.BloatMs != 75 || both.Grade != "C" || both.BloatNote != "" {
		t.Errorf("both phases: %v %q %q, want 75 C and no note", both.BloatMs, both.Grade, both.BloatNote)
	}

	noIdle := ServerResult{IdleMs: 20, LoadedDownMs: 95, loadedDownOK: true}
	noIdle.gradeBloat()
	if noIdle.Grade != "" {
		t.Error("no grade without an idle measurement")
	}

	// The upload pings went unanswered: grade from the download alone, and say so.
	upLost := ServerResult{IdleMs: 20, LoadedDownMs: 45, idleOK: true, loadedDownOK: true}
	upLost.record("upload", loadedLatency{unanswered: true})
	upLost.gradeBloat()
	if upLost.Grade != "A" || upLost.BloatMs != 25 ||
		upLost.BloatNote != "upload not measured: the server ignored pings while busy" {
		t.Errorf("upload lost: %v %q %q", upLost.BloatMs, upLost.Grade, upLost.BloatNote)
	}

	// Nothing answered under load: no grade (not an F), and a note.
	allLost := ServerResult{IdleMs: 20, idleOK: true}
	allLost.record("download", loadedLatency{unanswered: true})
	allLost.record("upload", loadedLatency{unanswered: true})
	allLost.gradeBloat()
	if allLost.Grade != "" || allLost.BloatNote != "not measured: the server ignored pings while busy" {
		t.Errorf("all lost: %q %q", allLost.Grade, allLost.BloatNote)
	}
}

func TestRecord(t *testing.T) {
	var r ServerResult
	if ms, ok := r.record("download", loadedLatency{median: 12345 * time.Microsecond, measured: true}); !ok || ms != 12.3 {
		t.Errorf("record = %v, %v", ms, ok)
	}
	if _, ok := r.record("upload", loadedLatency{unanswered: true}); ok || len(r.unanswered) != 1 {
		t.Errorf("unanswered phase should be noted, got %v", r.unanswered)
	}
}

func TestBloatGrade(t *testing.T) {
	tests := map[float64]string{0: "A+", 4.9: "A+", 5: "A", 29: "A", 59: "B", 199: "C", 399: "D", 400: "F"}
	for ms, want := range tests {
		if got := BloatGrade(ms); got != want {
			t.Errorf("BloatGrade(%v) = %s, want %s", ms, got, want)
		}
	}
}

// A server that answers the ping but sends no data should be replaced by a backup.
func TestFailedServerFallsBackToBackup(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	jobs := []*ServerResult{job("A", "dead", 0, deadSpeedtestServer(t))}
	backups := map[int][]*ServerResult{0: {job("A", "good", 0, fakeSpeedtestServer(t))}}
	s := Settings{ServersPerRegion: 1, Parallel: 1, Duration: time.Second, NoUpload: true}

	res, _ := RunTests(context.Background(), jobs, backups, s, nil)
	if res[0].Error != "" || res[0].Sponsor != "good" || res[0].DownMbps <= 0 {
		t.Fatalf("expected fallback to the good server, got %+v", *res[0])
	}
}

func TestFailedServerWithoutBackupKeepsError(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	jobs := []*ServerResult{job("A", "dead", 0, deadSpeedtestServer(t))}
	s := Settings{ServersPerRegion: 1, Parallel: 1, Duration: time.Second, NoUpload: true}

	res, _ := RunTests(context.Background(), jobs, nil, s, nil)
	if res[0].Error == "" {
		t.Fatalf("expected an error, got %+v", *res[0])
	}
}

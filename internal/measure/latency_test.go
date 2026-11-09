package measure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeHost(t *testing.T) {
	tests := []struct{ host, url, want string }{
		{"speed.example.net:8080", "", "speed.example.net:8080"},
		{"speed.example.net", "", "speed.example.net:8080"},
		{"", "http://127.0.0.1:5000/speedtest/upload.php", "127.0.0.1:5000"},
		{"", "https://s.example.org/upload.php", "s.example.org:443"},
		{"", "http://s.example.org/upload.php", "s.example.org:80"},
	}
	for _, tt := range tests {
		if got := probeHost(tt.host, tt.url); got != tt.want {
			t.Errorf("probeHost(%q, %q) = %q, want %q", tt.host, tt.url, got, tt.want)
		}
	}
}

func TestMedianDuration(t *testing.T) {
	if _, ok := medianDuration(nil); ok {
		t.Error("no samples should give no median")
	}
	got, _ := medianDuration([]time.Duration{30, 10, 20, 40})
	if got != 25 {
		t.Errorf("median = %v, want 25", got)
	}
}

func TestLatencyProbeAgainstLocalServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ctx := context.Background()

	if _, ok := idleLatency(ctx, host); !ok {
		t.Fatal("idle latency to a local server should work")
	}
	p := startProbe(ctx, host)
	time.Sleep(3 * probeEvery)
	l := p.finish()
	// Loopback can measure as 0; what matters is that samples were taken
	// and none timed out.
	if !l.measured || l.unanswered || l.median >= probeTimeout {
		t.Errorf("loaded latency = %+v", l)
	}
}

// A server that stops answering gives "unanswered", not a 2-second ping.
func TestLatencyProbeUnreachable(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	host := strings.TrimPrefix(srv.URL, "http://")
	srv.Close() // nothing listens there now
	if _, ok := tcpPing(context.Background(), host); ok {
		t.Fatal("ping to a closed port should fail")
	}
	p := startProbe(context.Background(), host)
	time.Sleep(3 * probeEvery)
	if l := p.finish(); l.measured || !l.unanswered {
		t.Errorf("probe of a dead server = %+v, want unanswered", l)
	}
}

func TestSummarizeProbes(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	tests := []struct {
		name       string
		answered   []time.Duration
		lost       int
		want       time.Duration
		measured   bool
		unanswered bool
	}{
		{"nothing", nil, 0, 0, false, false},
		{"all answered", []time.Duration{ms(10), ms(30), ms(20)}, 0, ms(20), true, false},
		// A few lost pings are real congestion: they count as the worst case.
		{"some lost", []time.Duration{ms(10), ms(20), ms(30)}, 2, ms(30), true, false},
		{"half lost", []time.Duration{ms(10), ms(20)}, 2, (ms(20) + probeTimeout) / 2, true, false},
		// Mostly lost: the server ignores us; nothing is measured.
		{"mostly lost", []time.Duration{ms(10)}, 3, 0, false, true},
		{"all lost", nil, 5, 0, false, true},
	}
	for _, tt := range tests {
		l := summarizeProbes(tt.answered, tt.lost)
		if l.median != tt.want || l.measured != tt.measured || l.unanswered != tt.unanswered {
			t.Errorf("%s: got %+v, want median=%v measured=%v unanswered=%v",
				tt.name, l, tt.want, tt.measured, tt.unanswered)
		}
	}
}

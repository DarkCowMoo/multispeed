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
	loaded, ok := p.finish()
	// Loopback can measure as 0; what matters is that samples were taken
	// and none timed out.
	if !ok || loaded >= probeTimeout {
		t.Errorf("loaded latency = %v, %v", loaded, ok)
	}
}

// A host that does not answer counts as the worst case, not as "no data".
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
}

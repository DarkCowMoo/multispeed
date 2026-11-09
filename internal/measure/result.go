// Package measure finds Speedtest.net servers for each region and measures
// them: ping, jitter, download, upload, latency under load (bufferbloat),
// and optionally the network route.
package measure

import (
	"math"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// perTestOverhead is a rough allowance for ping, latency probes and
// connection setup per server.
const perTestOverhead = 6 * time.Second

// Settings controls how servers are chosen and tested.
type Settings struct {
	ServersPerRegion int           // servers tested per region (-k)
	Parallel         int           // servers tested at the same time
	Duration         time.Duration // length of each download / upload measurement
	MaxDistKm        float64       // max distance from a region's center
	NoUpload         bool          // skip the upload test
}

// Estimate is the rough total run time for testing n servers.
func Estimate(n int, s Settings) time.Duration {
	perTest := s.Duration*2 + perTestOverhead
	if s.NoUpload {
		perTest = s.Duration + perTestOverhead
	}
	rounds := math.Ceil(float64(n) / float64(max(1, s.Parallel)))
	return time.Duration(rounds) * perTest
}

// ServerResult is one test against one server.
type ServerResult struct {
	Region       string  `json:"region"`
	ID           string  `json:"server_id"`
	Sponsor      string  `json:"sponsor"`
	City         string  `json:"city"`
	Country      string  `json:"country"`
	Host         string  `json:"host"`
	DistKm       float64 `json:"distance_from_region_km"`
	PingMs       float64 `json:"ping_ms"`
	JitterMs     float64 `json:"jitter_ms"`
	DownMbps     float64 `json:"download_mbps"`
	UpMbps       float64 `json:"upload_mbps"`
	IdleMs       float64 `json:"idle_latency_ms,omitempty"`
	LoadedDownMs float64 `json:"loaded_latency_download_ms,omitempty"`
	LoadedUpMs   float64 `json:"loaded_latency_upload_ms,omitempty"`
	BloatMs      float64 `json:"bufferbloat_ms,omitempty"`
	Grade        string  `json:"bufferbloat_grade,omitempty"`
	// BloatNote explains a missing or partial bufferbloat measurement,
	// e.g. "upload not measured: the server ignored pings while busy".
	BloatNote string `json:"bufferbloat_note,omitempty"`
	Error     string `json:"error,omitempty"`
	// RegionIdx is the index of the result's region in the list that was
	// passed to Discover.
	RegionIdx int `json:"-"`

	server       *speedtest.Server
	idleOK       bool     // idle latency was measured (it can be 0 ms on a LAN)
	loadedDownOK bool     // latency during the download was measured
	loadedUpOK   bool     // latency during the upload was measured
	unanswered   []string // phases ("download", "upload") whose pings went unanswered
}

// unansweredNote is shown when the server stopped answering pings under load.
const unansweredNote = "the server ignored pings while busy"

// gradeBloat sets BloatMs and Grade from the idle latency and whichever
// loaded latencies were measured, and BloatNote when one or both phases
// could not be measured.
func (r *ServerResult) gradeBloat() {
	loaded, measured := 0.0, false
	if r.loadedDownOK {
		loaded, measured = r.LoadedDownMs, true
	}
	if r.loadedUpOK {
		loaded, measured = max(loaded, r.LoadedUpMs), true
	}
	if r.idleOK && measured {
		r.BloatMs = round1(max(0, loaded-r.IdleMs))
		r.Grade = BloatGrade(r.BloatMs)
	}
	switch {
	case len(r.unanswered) == 0:
	case r.Grade == "":
		r.BloatNote = "not measured: " + unansweredNote
	default:
		r.BloatNote = r.unanswered[0] + " not measured: " + unansweredNote
	}
}

// BloatGrade grades how much latency rises when the line is busy
// ("bufferbloat"), using the common A+ to F scale.
func BloatGrade(increaseMs float64) string {
	switch {
	case increaseMs < 5:
		return "A+"
	case increaseMs < 30:
		return "A"
	case increaseMs < 60:
		return "B"
	case increaseMs < 200:
		return "C"
	case increaseMs < 400:
		return "D"
	default:
		return "F"
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// toMbps converts a byte rate to Mbps, rounded to 0.1.
func toMbps(r speedtest.ByteRate) float64 {
	if r < 0 {
		return 0
	}
	return round1(r.Mbps())
}

// toMs converts a duration to milliseconds, rounded to 0.1.
func toMs(d time.Duration) float64 {
	return round1(float64(d) / float64(time.Millisecond))
}

package measure

import (
	"context"
	"strings"
	"testing"
)

const tracertWindows = `
Tracing route to speedtest.example.net [203.0.113.9]
over a maximum of 20 hops:

  1    <1 ms    <1 ms    <1 ms  192.168.1.1
  2     6 ms     5 ms     7 ms  10.10.0.1
  3     *        *        *     Request timed out.
  4    12 ms    11 ms    13 ms  198.51.100.4
  5    61 ms    60 ms    62 ms  198.51.100.77
  6    63 ms    62 ms    64 ms  203.0.113.9

Trace complete.
`

const tracerouteLinux = `traceroute to speedtest.example.net (203.0.113.9), 20 hops max, 60 byte packets
 1  192.168.1.1  0.512 ms
 2  10.10.0.1  5.871 ms
 3  *
 4  198.51.100.4  12.004 ms
 5  198.51.100.77  60.3 ms
 6  203.0.113.9  62.1 ms
`

func TestParseTrace(t *testing.T) {
	for name, out := range map[string]string{"windows": tracertWindows, "linux": tracerouteLinux} {
		t.Run(name, func(t *testing.T) {
			hops := ParseTrace(out)
			if len(hops) != 6 {
				t.Fatalf("hops = %d: %+v", len(hops), hops)
			}
			if hops[0].IP != "192.168.1.1" || hops[0].RTTMs <= 0 || hops[0].RTTMs > 1 {
				t.Errorf("hop 1 = %+v", hops[0])
			}
			if hops[2].IP != "" || hops[2].RTTMs != 0 {
				t.Errorf("hop 3 should be silent: %+v", hops[2])
			}
			if hops[5].IP != "203.0.113.9" {
				t.Errorf("last hop = %+v", hops[5])
			}
			j := BiggestJump(hops)
			if j == nil || j.Hop != 5 || j.AddedMs < 47 || j.AddedMs > 49 {
				t.Errorf("biggest jump = %+v, want about +48 ms at hop 5", j)
			}
		})
	}
}

func TestTraceRouteRejectsOptionLikeHosts(t *testing.T) {
	for _, host := range []string{"-w 1 evil", "--help", "", "a b:8080"} {
		if r := TraceRoute(context.Background(), host); r.Error != "unusable server address" {
			t.Errorf("TraceRoute(%q) = %+v, want it rejected", host, r)
		}
	}
}

func TestParseTraceIgnoresHeaders(t *testing.T) {
	if hops := ParseTrace("Tracing route to x [1.2.3.4]\nTrace complete.\n"); len(hops) != 0 {
		t.Errorf("hops = %+v", hops)
	}
}

func TestJumpWhere(t *testing.T) {
	r := &Route{Hops: ParseTrace(tracertWindows)}
	r.Jump = BiggestJump(r.Hops)
	if got := r.JumpWhere(); got != "198.51.100.77" {
		t.Errorf("JumpWhere = %q, want the hop's IP", got)
	}
	r.HopNumber(5).Name = "ae-3.r21.frnkge13.de.bb.example.net"
	if got := r.JumpWhere(); !strings.HasPrefix(got, "ae-3.") {
		t.Errorf("JumpWhere = %q, want the hop's name", got)
	}
	if (&Route{}).JumpWhere() != "" || (*Route)(nil).JumpWhere() != "" {
		t.Error("no jump should give an empty answer")
	}
}

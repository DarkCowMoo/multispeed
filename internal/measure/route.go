package measure

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// MaxHops limits how far a route is traced.
	MaxHops = 20
	// traceConcurrency is how many routes are traced at the same time.
	traceConcurrency = 4
	// traceTimeout caps one whole trace.
	traceTimeout = 90 * time.Second
)

// Route is the network path to a region's best server.
type Route struct {
	Hops    []Hop  `json:"hops"`
	Reached bool   `json:"reached"`
	Jump    *Jump  `json:"biggest_jump,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Hop is one router on the way. RTTMs is 0 when the hop did not answer.
type Hop struct {
	N     int     `json:"hop"`
	IP    string  `json:"ip,omitempty"`
	Name  string  `json:"name,omitempty"`
	RTTMs float64 `json:"rtt_ms,omitempty"`
}

// Jump is the largest increase in delay between two answering hops: often
// where traffic crosses an ocean or a congested link.
type Jump struct {
	Hop     int     `json:"hop"`
	AddedMs float64 `json:"added_ms"`
}

// HopNumber returns hop n of the route, or nil.
func (r *Route) HopNumber(n int) *Hop {
	for i := range r.Hops {
		if r.Hops[i].N == n {
			return &r.Hops[i]
		}
	}
	return nil
}

// JumpWhere names the router of the biggest jump: its DNS name, or its IP.
func (r *Route) JumpWhere() string {
	if r == nil || r.Jump == nil {
		return ""
	}
	h := r.HopNumber(r.Jump.Hop)
	switch {
	case h == nil:
		return ""
	case h.Name != "":
		return h.Name
	default:
		return h.IP
	}
}

// traceCommand is the system traceroute for this OS. Both work without admin rights.
func traceCommand(host string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "tracert", []string{"-d", "-h", fmt.Sprint(MaxHops), "-w", "800", host}
	}
	return "traceroute", []string{"-n", "-q", "1", "-w", "1", "-m", fmt.Sprint(MaxHops), host}
}

// TraceRoute runs the system traceroute to host ("host" or "host:port").
func TraceRoute(ctx context.Context, host string) *Route {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// The host comes from the public server list: never let it be read as
	// a command-line option of tracert/traceroute.
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t") {
		return &Route{Error: "unusable server address"}
	}
	ctx, cancel := context.WithTimeout(ctx, traceTimeout)
	defer cancel()

	name, args := traceCommand(host)
	out, err := exec.CommandContext(ctx, name, args...).Output()
	hops := ParseTrace(string(out))
	if len(hops) == 0 {
		msg := "no route information"
		if err != nil {
			msg = err.Error()
			if errors.Is(err, exec.ErrNotFound) {
				msg = name + " is not installed"
			}
		}
		return &Route{Error: msg}
	}

	r := &Route{Hops: hops}
	if ips, err := net.DefaultResolver.LookupIPAddr(ctx, host); err == nil {
		for _, hop := range hops {
			for _, ip := range ips {
				if hop.IP == ip.IP.String() {
					r.Reached = true
				}
			}
		}
	}
	nameHops(ctx, r.Hops)
	r.Jump = BiggestJump(r.Hops)
	return r
}

// TraceRoutes traces the route to each host, a few at a time. done (which may
// be nil) is called after each trace finishes, from any goroutine.
func TraceRoutes(ctx context.Context, hosts []string, done func()) []*Route {
	routes := make([]*Route, len(hosts))
	sem := make(chan struct{}, traceConcurrency)
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			routes[i] = TraceRoute(ctx, h)
			if done != nil {
				done()
			}
		}(i, h)
	}
	wg.Wait()
	return routes
}

// ParseTrace reads tracert (Windows) or traceroute (macOS, Linux) output.
// Each hop line starts with the hop number, holds round-trip times written
// as "12 ms", "0.512 ms" or "<1 ms", and the router's IP address.
func ParseTrace(out string) []Hop {
	var hops []Hop
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n < 1 || n > 255 {
			continue
		}
		hop := Hop{N: n}
		for i, f := range fields[1:] {
			f = strings.Trim(f, "[]()")
			if ip := net.ParseIP(f); ip != nil && hop.IP == "" {
				hop.IP = ip.String()
				continue
			}
			// A time is a number followed by "ms", or glued to it ("12ms").
			num := strings.TrimSuffix(f, "ms")
			isTime := num != f || (i+2 < len(fields) && fields[i+2] == "ms")
			if !isTime {
				continue
			}
			var ms float64
			if num == "<1" {
				ms = 0.5
			} else if ms, err = strconv.ParseFloat(num, 64); err != nil {
				continue
			}
			if hop.RTTMs == 0 || ms < hop.RTTMs {
				hop.RTTMs = ms
			}
		}
		if hop.IP == "" {
			hop.RTTMs = 0 // no answer from this hop
		}
		hops = append(hops, hop)
	}
	return hops
}

// nameHops looks up DNS names of the hops at the same time. Router names
// often tell the network and city, e.g. "ae-3.r21.frnkge13.de.bb.gin.ntt.net".
func nameHops(ctx context.Context, hops []Hop) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range hops {
		if hops[i].IP == "" {
			continue
		}
		wg.Add(1)
		go func(h *Hop) {
			defer wg.Done()
			if names, err := net.DefaultResolver.LookupAddr(ctx, h.IP); err == nil && len(names) > 0 {
				h.Name = strings.TrimSuffix(names[0], ".")
			}
		}(&hops[i])
	}
	wg.Wait()
}

// BiggestJump finds the largest rise in delay between answering hops.
func BiggestJump(hops []Hop) *Jump {
	var best *Jump
	prev := -1.0
	for _, h := range hops {
		if h.RTTMs <= 0 {
			continue
		}
		if prev >= 0 {
			if added := h.RTTMs - prev; added > 0 && (best == nil || added > best.AddedMs) {
				best = &Jump{Hop: h.N, AddedMs: round1(added)}
			}
		}
		prev = h.RTTMs
	}
	return best
}

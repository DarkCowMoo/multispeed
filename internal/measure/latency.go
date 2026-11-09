package measure

import (
	"context"
	"net"
	"net/url"
	"sort"
	"sync"
	"time"
)

const (
	// probeEvery is how often latency is sampled while the line is busy.
	probeEvery = 200 * time.Millisecond
	// probeTimeout caps one latency sample; a slower answer counts as this long.
	probeTimeout = 2 * time.Second
	// idleSamples is how many idle latency samples are taken before loading the line.
	idleSamples = 5
)

// tcpPing measures how long a TCP connection to host takes to open. Opening
// a connection needs one round trip, so this is a ping that works without
// admin rights and passes through the same queues as the speed test.
func tcpPing(ctx context.Context, host string) (time.Duration, bool) {
	d := net.Dialer{Timeout: probeTimeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return 0, false
	}
	rtt := time.Since(start)
	_ = c.Close()
	return rtt, true
}

// idleLatency is the median of a few pings while the line is quiet.
func idleLatency(ctx context.Context, host string) (time.Duration, bool) {
	var samples []time.Duration
	for i := 0; i < idleSamples && ctx.Err() == nil; i++ {
		if rtt, ok := tcpPing(ctx, host); ok {
			samples = append(samples, rtt)
		}
	}
	return medianDuration(samples)
}

// maxUnansweredShare is the share of pings under load that may go unanswered
// before the measurement is discarded. A few lost pings mean real congestion
// and count as the worst case; mostly lost pings more likely mean the server
// refuses new connections while it is busy, which says nothing about your line.
const maxUnansweredShare = 0.5

// loadedLatency is what a probe measured while the line was busy.
type loadedLatency struct {
	median     time.Duration
	measured   bool // enough pings were answered to trust the median
	unanswered bool // most pings went unanswered, so nothing was measured
}

// latencyProbe pings a host in the background while a download or upload runs.
type latencyProbe struct {
	mu       sync.Mutex
	answered []time.Duration
	lost     int
	cancel   context.CancelFunc
	done     chan struct{}
}

func startProbe(ctx context.Context, host string) *latencyProbe {
	ctx, cancel := context.WithCancel(ctx)
	p := &latencyProbe{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		t := time.NewTicker(probeEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rtt, ok := tcpPing(ctx, host)
				if ctx.Err() != nil {
					return // canceled mid-sample: not a real measurement
				}
				p.mu.Lock()
				if ok {
					p.answered = append(p.answered, rtt)
				} else {
					p.lost++
				}
				p.mu.Unlock()
			}
		}
	}()
	return p
}

// finish stops the probe and returns the latency under load.
func (p *latencyProbe) finish() loadedLatency {
	p.cancel()
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return summarizeProbes(p.answered, p.lost)
}

// summarizeProbes turns answered pings and the number of lost ones into a
// measurement. Lost pings count as the worst case (probeTimeout), unless so
// many were lost that the server, not the line, is the likely cause.
func summarizeProbes(answered []time.Duration, lost int) loadedLatency {
	total := len(answered) + lost
	if total == 0 {
		return loadedLatency{}
	}
	if float64(lost)/float64(total) > maxUnansweredShare {
		return loadedLatency{unanswered: true}
	}
	all := append([]time.Duration(nil), answered...)
	for i := 0; i < lost; i++ {
		all = append(all, probeTimeout)
	}
	m, ok := medianDuration(all)
	return loadedLatency{median: m, measured: ok}
}

func medianDuration(v []time.Duration) (time.Duration, bool) {
	if len(v) == 0 {
		return 0, false
	}
	c := append([]time.Duration(nil), v...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	n := len(c)
	if n%2 == 1 {
		return c[n/2], true
	}
	return (c[n/2-1] + c[n/2]) / 2, true
}

// probeHost is the "host:port" to ping for a server: its Host field, or the
// host of its URL.
func probeHost(host, rawURL string) string {
	if host == "" {
		u, err := url.Parse(rawURL)
		if err != nil {
			return ""
		}
		host = u.Host
		if u.Port() == "" {
			if u.Scheme == "https" {
				host += ":443"
			} else {
				host += ":80"
			}
		}
		return host
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host += ":8080" // Speedtest.net servers listen on 8080 by default
	}
	return host
}

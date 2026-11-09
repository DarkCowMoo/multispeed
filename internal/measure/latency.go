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

// latencyProbe pings a host in the background while a download or upload runs.
type latencyProbe struct {
	mu      sync.Mutex
	samples []time.Duration
	cancel  context.CancelFunc
	done    chan struct{}
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
				if !ok {
					rtt = probeTimeout // no answer under load is the worst case
				}
				p.mu.Lock()
				p.samples = append(p.samples, rtt)
				p.mu.Unlock()
			}
		}
	}()
	return p
}

// finish stops the probe and returns the median latency under load.
func (p *latencyProbe) finish() (time.Duration, bool) {
	p.cancel()
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return medianDuration(p.samples)
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

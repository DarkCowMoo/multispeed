package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// runStats describes a whole test run.
type runStats struct {
	dataBytes int64         // bytes downloaded and uploaded
	elapsed   time.Duration // wall time of the tests
}

// runTests runs every job, o.parallel at a time. When a server fails, the job
// retries with the next unused backup server of the same region (backups may be nil).
func runTests(ctx context.Context, jobs []*ServerResult, backups map[int][]*ServerResult, o options) ([]*ServerResult, runStats) {
	queue := make(chan *ServerResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var data atomic.Int64
	started := time.Now()
	prog := newTracker(len(jobs), o)
	log := newResultLog(len(jobs), o.noUpload, errColors, logf)
	stopStatus := con.show(prog.status)
	defer stopStatus()

	nextBackup := func(region int) *ServerResult {
		mu.Lock()
		defer mu.Unlock()
		b := backups[region]
		if len(b) == 0 {
			return nil
		}
		backups[region] = b[1:]
		return b[0]
	}

	for w := 0; w < o.parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				a := prog.begin(j.Region, j.Sponsor)
				failed, used := testWithFallback(ctx, j, o, nextBackup, prog, a)
				data.Add(used)
				log.add(prog.end(a), j, failed)
			}
		}()
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		queue <- j
	}
	close(queue)
	wg.Wait()
	return jobs, runStats{dataBytes: data.Load(), elapsed: time.Since(started)}
}

// testWithFallback tests j and, while it fails, replaces it with backups.
// Progress is reported to prog under the entry a. It returns a description
// of each server that failed and was replaced, and the bytes moved.
func testWithFallback(ctx context.Context, j *ServerResult, o options,
	nextBackup func(int) *ServerResult, prog *tracker, a *activeTest,
) (failed []string, used int64) {
	// Each attempt gets its own callback with the server name fixed, so a late
	// callback from a previous attempt never reads data being overwritten.
	phaseFor := func(server string) func(string) {
		return func(phase string) { prog.update(a, server, phase) }
	}
	used = testOne(ctx, j, o, phaseFor(j.Sponsor))
	for j.Error != "" && ctx.Err() == nil {
		b := nextBackup(j.regionIdx)
		if b == nil {
			break
		}
		failed = append(failed, fmt.Sprintf("%s (%s)", trunc(j.Sponsor, serverWidth), j.Error))
		*j = *b
		used += testOne(ctx, j, o, phaseFor(j.Sponsor))
	}
	return failed, used
}

// testOne measures ping, download and (optionally) upload on one server,
// reporting each phase and the live speed to setPhase. It returns the number
// of bytes moved.
func testOne(ctx context.Context, j *ServerResult, o options, setPhase func(string)) int64 {
	if ctx.Err() != nil {
		j.Error = "canceled"
		return 0
	}
	// Each test gets its own client so parallel tests don't share counters.
	client := newClient(nil)
	used := func() int64 { return client.GetTotalDownload() + client.GetTotalUpload() }
	client.SetCaptureTime(o.duration)
	client.SetCallbackDownload(func(r speedtest.ByteRate) {
		setPhase(fmt.Sprintf("download %.0f Mbps", r.Mbps()))
	})
	client.SetCallbackUpload(func(r speedtest.ByteRate) {
		setPhase(fmt.Sprintf("upload %.0f Mbps", r.Mbps()))
	})
	s := j.server
	s.Context = client

	setPhase("ping")
	if err := s.PingTestContext(ctx, nil); err != nil {
		j.Error = "ping: " + err.Error()
		return used()
	}
	j.PingMs = toMs(s.Latency)
	j.JitterMs = toMs(s.Jitter)

	// Latency under load: ping the same server while the line is busy and
	// compare with the idle latency measured the same way.
	host := probeHost(s.Host, s.URL)
	if idle, ok := idleLatency(ctx, host); ok {
		j.IdleMs, j.idleOK = toMs(idle), true
	}

	setPhase("download")
	probe := startProbe(ctx, host)
	err := s.DownloadTestContext(ctx)
	if loaded, ok := probe.finish(); ok {
		j.LoadedDownMs, j.loadedOK = toMs(loaded), true
	}
	if err != nil {
		j.Error = "download: " + err.Error()
		return used()
	}
	j.DownMbps = toMbps(s.DLSpeed)
	if !o.noUpload {
		setPhase("upload")
		probe = startProbe(ctx, host)
		err = s.UploadTestContext(ctx)
		if loaded, ok := probe.finish(); ok {
			j.LoadedUpMs, j.loadedOK = toMs(loaded), true
		}
		if err != nil {
			j.Error = "upload: " + err.Error()
			return used()
		}
		j.UpMbps = toMbps(s.ULSpeed)
	}
	if j.DownMbps <= 0 && (o.noUpload || j.UpMbps <= 0) {
		j.Error = "no data transferred"
		return used()
	}
	j.gradeBloat()
	return used()
}

// toMbps converts a byte rate to Mbps, rounded to 0.1.
func toMbps(r speedtest.ByteRate) float64 {
	if r < 0 {
		return 0
	}
	return math.Round(r.Mbps()*10) / 10
}

// toMs converts a duration to milliseconds, rounded to 0.1.
func toMs(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*10) / 10
}

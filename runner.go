package main

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// runTests runs every job, o.parallel at a time. When a server fails, the job
// retries with the next unused backup server of the same region (backups may be nil).
func runTests(ctx context.Context, jobs []*ServerResult, backups map[int][]*ServerResult, o options) []*ServerResult {
	queue := make(chan *ServerResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

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
				testWithFallback(ctx, j, o, nextBackup)
				mu.Lock()
				done++
				logProgress(done, len(jobs), j, o.noUpload)
				mu.Unlock()
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
	return jobs
}

// testWithFallback tests j and, while it fails, replaces it with backups.
func testWithFallback(ctx context.Context, j *ServerResult, o options, nextBackup func(int) *ServerResult) {
	testOne(ctx, j, o)
	for j.Error != "" && ctx.Err() == nil {
		b := nextBackup(j.regionIdx)
		if b == nil {
			return
		}
		logf("  %-16s %s failed (%s), trying %s\n", j.Region, trunc(j.Sponsor, 28), j.Error, trunc(b.Sponsor, 28))
		*j = *b
		testOne(ctx, j, o)
	}
}

func logProgress(done, total int, j *ServerResult, noUpload bool) {
	switch {
	case j.Error != "":
		logf("  [%d/%d] %-16s %-28s error: %s\n", done, total, j.Region, trunc(j.Sponsor, 28), j.Error)
	case noUpload:
		logf("  [%d/%d] %-16s %-28s ping %4.0f ms  down %7.1f Mbps\n",
			done, total, j.Region, trunc(j.Sponsor, 28), j.PingMs, j.DownMbps)
	default:
		logf("  [%d/%d] %-16s %-28s ping %4.0f ms  down %7.1f  up %7.1f Mbps\n",
			done, total, j.Region, trunc(j.Sponsor, 28), j.PingMs, j.DownMbps, j.UpMbps)
	}
}

// testOne measures ping, download and (optionally) upload on one server.
func testOne(ctx context.Context, j *ServerResult, o options) {
	if ctx.Err() != nil {
		j.Error = "canceled"
		return
	}
	// Each test gets its own client so parallel tests don't share counters.
	client := speedtest.New()
	client.SetCaptureTime(o.duration)
	s := j.server
	s.Context = client

	if err := s.PingTestContext(ctx, nil); err != nil {
		j.Error = "ping: " + err.Error()
		return
	}
	j.PingMs = toMs(s.Latency)
	j.JitterMs = toMs(s.Jitter)
	if err := s.DownloadTestContext(ctx); err != nil {
		j.Error = "download: " + err.Error()
		return
	}
	j.DownMbps = toMbps(s.DLSpeed)
	if !o.noUpload {
		if err := s.UploadTestContext(ctx); err != nil {
			j.Error = "upload: " + err.Error()
			return
		}
		j.UpMbps = toMbps(s.ULSpeed)
	}
	if j.DownMbps <= 0 && (o.noUpload || j.UpMbps <= 0) {
		j.Error = "no data transferred"
	}
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

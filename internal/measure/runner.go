package measure

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// Stats describes a whole test run.
type Stats struct {
	DataBytes int64         // bytes downloaded and uploaded
	Elapsed   time.Duration // wall time of the tests
}

// Observer follows the tests as they run. Begin may be called from several
// goroutines at once.
type Observer interface {
	// Begin is called when a server test starts.
	Begin(region, server string) Progress
}

// Progress follows one running test.
type Progress interface {
	// Phase reports what the test is doing, e.g. "download 512 Mbps", and on
	// which server (it changes when a backup takes over). The speed test
	// library may call it late, even after End; that must be harmless.
	Phase(server, phase string)
	// End reports the finished result. replaced lists the servers that failed
	// before this one was used as their backup, as "sponsor (error)".
	End(r *ServerResult, replaced []string)
}

// noObserver is used when RunTests is given a nil Observer.
type noObserver struct{}

type noProgress struct{}

func (noObserver) Begin(string, string) Progress { return noProgress{} }
func (noProgress) Phase(string, string)          {}
func (noProgress) End(*ServerResult, []string)   {}

// RunTests runs every job, s.Parallel at a time, and reports progress to obs
// (which may be nil). When a server fails, the job retries with the next
// unused backup server of the same region (backups may be nil). The jobs are
// updated in place and returned.
func RunTests(ctx context.Context, jobs []*ServerResult, backups map[int][]*ServerResult,
	s Settings, obs Observer,
) ([]*ServerResult, Stats) {
	if obs == nil {
		obs = noObserver{}
	}
	queue := make(chan *ServerResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var data atomic.Int64
	started := time.Now()

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

	for w := 0; w < max(1, s.Parallel); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				p := obs.Begin(j.Region, j.Sponsor)
				replaced, used := testWithFallback(ctx, j, s, nextBackup, p)
				data.Add(used)
				p.End(j, replaced)
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
	return jobs, Stats{DataBytes: data.Load(), Elapsed: time.Since(started)}
}

// testWithFallback tests j and, while it fails, replaces it with backups.
// It returns a description of each server that failed and was replaced, and
// the bytes moved.
func testWithFallback(ctx context.Context, j *ServerResult, s Settings,
	nextBackup func(int) *ServerResult, p Progress,
) (replaced []string, used int64) {
	// Each attempt gets its own callback with the server name fixed, so a late
	// callback from a previous attempt never reads data being overwritten.
	phaseFor := func(server string) func(string) {
		return func(phase string) { p.Phase(server, phase) }
	}
	used = testOne(ctx, j, s, phaseFor(j.Sponsor))
	for j.Error != "" && ctx.Err() == nil {
		b := nextBackup(j.RegionIdx)
		if b == nil {
			break
		}
		replaced = append(replaced, fmt.Sprintf("%s (%s)", j.Sponsor, j.Error))
		*j = *b
		used += testOne(ctx, j, s, phaseFor(j.Sponsor))
	}
	return replaced, used
}

// testOne measures ping, download and (optionally) upload on one server,
// reporting each phase and the live speed to setPhase. It returns the number
// of bytes moved.
func testOne(ctx context.Context, j *ServerResult, s Settings, setPhase func(string)) int64 {
	if ctx.Err() != nil {
		j.Error = "canceled"
		return 0
	}
	// Each test gets its own client so parallel tests don't share counters.
	client := newClient(nil)
	used := func() int64 { return client.GetTotalDownload() + client.GetTotalUpload() }
	client.SetCaptureTime(s.Duration)
	client.SetCallbackDownload(func(r speedtest.ByteRate) {
		setPhase(fmt.Sprintf("download %.0f Mbps", r.Mbps()))
	})
	client.SetCallbackUpload(func(r speedtest.ByteRate) {
		setPhase(fmt.Sprintf("upload %.0f Mbps", r.Mbps()))
	})
	srv := j.server
	srv.Context = client

	setPhase("ping")
	if err := srv.PingTestContext(ctx, nil); err != nil {
		j.Error = "ping: " + err.Error()
		return used()
	}
	j.PingMs = toMs(srv.Latency)
	j.JitterMs = toMs(srv.Jitter)

	// Latency under load: ping the same server while the line is busy and
	// compare with the idle latency measured the same way.
	host := probeHost(srv.Host, srv.URL)
	if idle, ok := idleLatency(ctx, host); ok {
		j.IdleMs, j.idleOK = toMs(idle), true
	}

	setPhase("download")
	probe := startProbe(ctx, host)
	err := srv.DownloadTestContext(ctx)
	if loaded, ok := probe.finish(); ok {
		j.LoadedDownMs, j.loadedOK = toMs(loaded), true
	}
	if err != nil {
		j.Error = "download: " + err.Error()
		return used()
	}
	j.DownMbps = toMbps(srv.DLSpeed)
	if !s.NoUpload {
		setPhase("upload")
		probe = startProbe(ctx, host)
		err = srv.UploadTestContext(ctx)
		if loaded, ok := probe.finish(); ok {
			j.LoadedUpMs, j.loadedOK = toMs(loaded), true
		}
		if err != nil {
			j.Error = "upload: " + err.Error()
			return used()
		}
		j.UpMbps = toMbps(srv.ULSpeed)
	}
	if j.DownMbps <= 0 && (s.NoUpload || j.UpMbps <= 0) {
		j.Error = "no data transferred"
		return used()
	}
	j.gradeBloat()
	return used()
}

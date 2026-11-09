package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

const (
	// backupsPerRegion is how many spare servers each region keeps for
	// retries: some listed servers answer the ping but then transfer nothing.
	backupsPerRegion = 2
	// nearestFactor limits selection to the nearestFactor*k nearest servers,
	// so a region such as "Frankfurt" does not drift to a faster city.
	nearestFactor = 4
	// lookupConcurrency is how many regions are looked up at the same time.
	lookupConcurrency = 4
	// regionLabelWidth aligns region names in the discovery log.
	regionLabelWidth = 16
)

// candidate is a server considered for a region.
type candidate struct {
	s       *speedtest.Server
	latency time.Duration
	dist    float64
}

// discovery holds the servers chosen for every region.
type discovery struct {
	jobs    []*ServerResult         // servers to test
	backups map[int][]*ServerResult // spare servers, keyed by region index
	notes   []string                // per-region notes, indexed like the regions
}

// regionPick is the outcome of looking up one region.
type regionPick struct {
	jobs    []*ServerResult
	backups []*ServerResult
	note    string
}

// discover finds candidate servers for each region, looking regions up concurrently.
func discover(ctx context.Context, regions []Region, o options) discovery {
	picks := make([]regionPick, len(regions))
	var wg sync.WaitGroup
	var looked atomic.Int32
	started := time.Now()
	stopStatus := con.show(func() string {
		return fmt.Sprintf("Finding servers... %d/%d regions | %s elapsed",
			looked.Load(), len(regions), fmtDur(time.Since(started)))
	})
	sem := make(chan struct{}, lookupConcurrency)
	for i, r := range regions {
		wg.Add(1)
		go func(i int, r Region) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			picks[i] = discoverRegion(ctx, i, r, o)
			looked.Add(1)
		}(i, r)
	}
	wg.Wait()
	stopStatus()

	d := discovery{
		backups: make(map[int][]*ServerResult, len(regions)),
		notes:   make([]string, len(regions)),
	}
	for i, p := range picks {
		d.jobs = append(d.jobs, p.jobs...)
		d.backups[i] = p.backups
		d.notes[i] = p.note
		if p.note != "" && len(p.jobs) == 0 {
			logf("  %s %s\n", errColors.bold(padRight(regions[i].Name, regionLabelWidth)),
				errColors.bad("skipped: "+p.note))
		}
	}
	return d
}

// discoverRegion fetches the server list near one region and picks servers from it.
func discoverRegion(ctx context.Context, idx int, r Region, o options) regionPick {
	cfg := &speedtest.UserConfig{UserAgent: speedtest.DefaultUserAgent}
	if !r.Local {
		cfg.Location = &speedtest.Location{Name: r.Name, Lat: r.Lat, Lon: r.Lon}
	}
	client := newClient(cfg)
	servers, err := client.FetchServerListContext(ctx)
	if err != nil {
		return regionPick{note: "server lookup failed: " + err.Error()}
	}

	cands := make([]candidate, 0, len(servers))
	for _, s := range servers {
		c := candidate{s: s, latency: s.Latency, dist: s.Distance}
		if !r.Local {
			c.dist = serverDistanceKm(r, s)
		}
		cands = append(cands, c)
	}
	maxDist := o.maxDistKm
	if r.Local {
		maxDist = math.Inf(1)
	}

	var p regionPick
	picked := selectCandidates(cands, o.k+backupsPerRegion, maxDist)
	if len(picked) == 0 {
		p.note = fmt.Sprintf("no responsive servers within %.0f km", o.maxDistKm)
		return p
	}
	if len(picked) < o.k {
		p.note = fmt.Sprintf("only %d responsive server(s) found", len(picked))
	}
	for n, c := range picked {
		res := newResult(r.Name, idx, c)
		if n < o.k {
			p.jobs = append(p.jobs, res)
		} else {
			p.backups = append(p.backups, res)
		}
	}
	ec := errColors
	logf("  %s %s %s\n", ec.bold(padRight(r.Name, regionLabelWidth)), serverNames(p.jobs),
		ec.dim(fmt.Sprintf("(+%d backup)", len(p.backups))))
	return p
}

// serverDistanceKm is the distance from a region's center to a server.
// Servers with unparsable coordinates are treated as infinitely far away.
func serverDistanceKm(r Region, s *speedtest.Server) float64 {
	lat, errLat := strconv.ParseFloat(s.Lat, 64)
	lon, errLon := strconv.ParseFloat(s.Lon, 64)
	if errLat != nil || errLon != nil {
		return math.Inf(1)
	}
	return haversineKm(r.Lat, r.Lon, lat, lon)
}

// selectCandidates keeps servers that answered the quick ping and are close to
// the region, then picks the k with the lowest latency from you. Among the
// nearest servers, low latency usually means a better-connected host.
func selectCandidates(cands []candidate, k int, maxDistKm float64) []candidate {
	var ok []candidate
	for _, c := range cands {
		if c.latency <= 0 { // PingTimeout (-1) or never answered
			continue
		}
		if c.dist > maxDistKm {
			continue
		}
		ok = append(ok, c)
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].dist < ok[j].dist })
	if len(ok) > nearestFactor*k {
		ok = ok[:nearestFactor*k]
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].latency < ok[j].latency })
	if len(ok) > k {
		ok = ok[:k]
	}
	return ok
}

func newResult(region string, idx int, c candidate) *ServerResult {
	return &ServerResult{
		Region:    region,
		ID:        c.s.ID,
		Sponsor:   c.s.Sponsor,
		City:      c.s.Name,
		Country:   c.s.Country,
		Host:      c.s.Host,
		DistKm:    math.Round(c.dist),
		server:    c.s,
		regionIdx: idx,
	}
}

// haversineKm is the great-circle distance between two points, in km.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
}

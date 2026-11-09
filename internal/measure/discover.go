package measure

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/showwin/speedtest-go/speedtest"

	"github.com/DarkCowMoo/multispeed/internal/regions"
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
)

// candidate is a server considered for a region.
type candidate struct {
	s       *speedtest.Server
	latency time.Duration
	dist    float64
}

// Discovery holds the servers chosen for every region.
type Discovery struct {
	Jobs    []*ServerResult         // servers to test
	Backups map[int][]*ServerResult // spare servers, keyed by region index
	Notes   []string                // per-region notes, indexed like the regions
}

// RegionFound is told about each region as soon as its servers are chosen:
// the servers to test, how many backups were kept, and a note when there is
// a problem. It may be called from several goroutines at once.
type RegionFound func(r regions.Region, jobs []*ServerResult, backups int, note string)

// regionPick is the outcome of looking up one region.
type regionPick struct {
	jobs    []*ServerResult
	backups []*ServerResult
	note    string
}

// Discover finds servers for each region, looking regions up concurrently.
// found may be nil.
func Discover(ctx context.Context, rs []regions.Region, s Settings, found RegionFound) Discovery {
	picks := make([]regionPick, len(rs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, lookupConcurrency)
	for i, r := range rs {
		wg.Add(1)
		go func(i int, r regions.Region) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			picks[i] = discoverRegion(ctx, i, r, s)
			if found != nil {
				found(r, picks[i].jobs, len(picks[i].backups), picks[i].note)
			}
		}(i, r)
	}
	wg.Wait()

	d := Discovery{
		Backups: make(map[int][]*ServerResult, len(rs)),
		Notes:   make([]string, len(rs)),
	}
	for i, p := range picks {
		d.Jobs = append(d.Jobs, p.jobs...)
		d.Backups[i] = p.backups
		d.Notes[i] = p.note
	}
	return d
}

// discoverRegion fetches the server list near one region and picks servers from it.
func discoverRegion(ctx context.Context, idx int, r regions.Region, s Settings) regionPick {
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
	for _, srv := range servers {
		c := candidate{s: srv, latency: srv.Latency, dist: srv.Distance}
		if !r.Local {
			c.dist = serverDistanceKm(r, srv)
		}
		cands = append(cands, c)
	}
	maxDist := s.MaxDistKm
	if r.Local {
		maxDist = math.Inf(1)
	}

	var p regionPick
	k := max(1, s.ServersPerRegion)
	picked := selectCandidates(cands, k+backupsPerRegion, maxDist)
	if len(picked) == 0 {
		p.note = fmt.Sprintf("no responsive servers within %.0f km", s.MaxDistKm)
		return p
	}
	if len(picked) < k {
		p.note = fmt.Sprintf("only %d responsive server(s) found", len(picked))
	}
	for n, c := range picked {
		res := newResult(r.Name, idx, c)
		if n < k {
			p.jobs = append(p.jobs, res)
		} else {
			p.backups = append(p.backups, res)
		}
	}
	return p
}

// serverDistanceKm is the distance from a region's center to a server.
// Servers with unparsable coordinates are treated as infinitely far away.
func serverDistanceKm(r regions.Region, s *speedtest.Server) float64 {
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
		RegionIdx: idx,
		server:    c.s,
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

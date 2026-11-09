// multispeed: test your internet speed to many regions at once, using public
// Speedtest.net servers that are picked automatically for each region.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

type options struct {
	regions     string
	k           int
	parallel    int
	duration    time.Duration
	maxDistKm   float64
	noUpload    bool
	jsonOut     bool
	csvPath     string
	listRegions bool
}

// ServerResult is one test against one server.
type ServerResult struct {
	Region    string  `json:"region"`
	ID        string  `json:"server_id"`
	Sponsor   string  `json:"sponsor"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Host      string  `json:"host"`
	DistKm    float64 `json:"distance_from_region_km"`
	PingMs    float64 `json:"ping_ms"`
	JitterMs  float64 `json:"jitter_ms"`
	DownMbps  float64 `json:"download_mbps"`
	UpMbps    float64 `json:"upload_mbps"`
	Error     string  `json:"error,omitempty"`
	server    *speedtest.Server
	regionIdx int
}

// RegionSummary aggregates the k servers tested in one region.
type RegionSummary struct {
	Region       string         `json:"region"`
	BestServer   string         `json:"best_server"`
	PingMs       float64        `json:"ping_ms"`
	JitterMs     float64        `json:"jitter_ms"`
	BestDownMbps float64        `json:"best_download_mbps"`
	MedDownMbps  float64        `json:"median_download_mbps"`
	BestUpMbps   float64        `json:"best_upload_mbps"`
	Tested       int            `json:"servers_tested"`
	OK           int            `json:"servers_ok"`
	Note         string         `json:"note,omitempty"`
	Servers      []ServerResult `json:"servers"`
}

func main() {
	var o options
	flag.StringVar(&o.regions, "regions", strings.Join(DefaultRegions, ","),
		`comma-separated regions: "local", preset names, or Name=lat,lon`)
	flag.IntVar(&o.k, "k", 3, "servers to test per region (best result is kept)")
	flag.IntVar(&o.parallel, "parallel", 1, "how many servers to test at the same time (1 = one by one)")
	flag.DurationVar(&o.duration, "duration", 10*time.Second, "length of each download/upload measurement")
	flag.Float64Var(&o.maxDistKm, "max-distance", 1000, "only use servers within this many km of a region's center")
	flag.BoolVar(&o.noUpload, "no-upload", false, "skip the upload test (faster)")
	flag.BoolVar(&o.jsonOut, "json", false, "print full results as JSON instead of a table")
	flag.StringVar(&o.csvPath, "csv", "", "also write per-server results to this CSV file")
	flag.BoolVar(&o.listRegions, "list-regions", false, "show preset region names and exit")
	flag.Parse()

	if o.listRegions {
		fmt.Println("local  (your nearest servers)")
		for _, n := range PresetNames() {
			fmt.Printf("%-13s %s\n", n, Presets[n].Name)
		}
		return
	}
	if o.k < 1 || o.parallel < 1 {
		fatalf("-k and -parallel must be at least 1")
	}
	regions, err := ParseRegions(o.regions)
	if err != nil {
		fatalf("%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logf("Finding servers for %d regions...\n", len(regions))
	jobs, notes := discover(ctx, regions, o)
	if len(jobs) == 0 {
		fatalf("no reachable servers found")
	}
	if o.parallel > 1 {
		logf("Note: parallel tests share your bandwidth, so each one will show lower speed.\n")
	}
	est := time.Duration(math.Ceil(float64(len(jobs))/float64(o.parallel))) * (o.duration*2 + 5*time.Second)
	if o.noUpload {
		est = time.Duration(math.Ceil(float64(len(jobs))/float64(o.parallel))) * (o.duration + 5*time.Second)
	}
	logf("Testing %d servers, %d at a time (about %s)...\n", len(jobs), o.parallel, est.Round(time.Minute))

	results := runTests(ctx, jobs, o)
	summaries := summarize(regions, results, notes)

	if o.csvPath != "" {
		if err := writeCSV(o.csvPath, results); err != nil {
			logf("could not write CSV: %v\n", err)
		}
	}
	if o.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summaries)
		return
	}
	printTable(summaries)
}

// discover finds candidate servers for each region (regions are looked up concurrently).
func discover(ctx context.Context, regions []Region, o options) ([]*ServerResult, []string) {
	type found struct {
		jobs []*ServerResult
		note string
	}
	out := make([]found, len(regions))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, r := range regions {
		wg.Add(1)
		go func(i int, r Region) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cfg := &speedtest.UserConfig{UserAgent: speedtest.DefaultUserAgent}
			if !r.Local {
				cfg.Location = &speedtest.Location{Name: r.Name, Lat: r.Lat, Lon: r.Lon}
			}
			client := speedtest.New(speedtest.WithUserConfig(cfg))
			servers, err := client.FetchServerListContext(ctx)
			if err != nil {
				out[i].note = "server lookup failed: " + err.Error()
				return
			}
			cands := make([]candidate, 0, len(servers))
			for _, s := range servers {
				c := candidate{s: s, latency: s.Latency, dist: s.Distance}
				if !r.Local {
					lat, _ := strconv.ParseFloat(s.Lat, 64)
					lon, _ := strconv.ParseFloat(s.Lon, 64)
					c.dist = haversineKm(r.Lat, r.Lon, lat, lon)
				}
				cands = append(cands, c)
			}
			maxDist := o.maxDistKm
			if r.Local {
				maxDist = math.Inf(1)
			}
			picked := selectCandidates(cands, o.k, maxDist)
			if len(picked) == 0 {
				out[i].note = fmt.Sprintf("no responsive servers within %.0f km", o.maxDistKm)
				return
			}
			if len(picked) < o.k {
				out[i].note = fmt.Sprintf("only %d responsive server(s) found", len(picked))
			}
			for _, c := range picked {
				out[i].jobs = append(out[i].jobs, &ServerResult{
					Region: r.Name, ID: c.s.ID, Sponsor: c.s.Sponsor, City: c.s.Name,
					Country: c.s.Country, Host: c.s.Host, DistKm: math.Round(c.dist),
					server: c.s, regionIdx: i,
				})
			}
			logf("  %-16s %d server(s): %s\n", r.Name, len(picked), names(out[i].jobs))
		}(i, r)
	}
	wg.Wait()

	var jobs []*ServerResult
	notes := make([]string, len(regions))
	for i, f := range out {
		jobs = append(jobs, f.jobs...)
		notes[i] = f.note
		if f.note != "" && len(f.jobs) == 0 {
			logf("  %-16s skipped: %s\n", regions[i].Name, f.note)
		}
	}
	return jobs, notes
}

type candidate struct {
	s       *speedtest.Server
	latency time.Duration
	dist    float64
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
	// Look only at the 4k nearest so "Frankfurt" doesn't drift to a faster city.
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].dist < ok[j].dist })
	if len(ok) > 4*k {
		ok = ok[:4*k]
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].latency < ok[j].latency })
	if len(ok) > k {
		ok = ok[:k]
	}
	return ok
}

func runTests(ctx context.Context, jobs []*ServerResult, o options) []*ServerResult {
	queue := make(chan *ServerResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for w := 0; w < o.parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				testOne(ctx, j, o)
				mu.Lock()
				done++
				if j.Error != "" {
					logf("  [%d/%d] %-16s %-28s error: %s\n", done, len(jobs), j.Region, trunc(j.Sponsor, 28), j.Error)
				} else {
					logf("  [%d/%d] %-16s %-28s ping %4.0f ms  down %7.1f  up %7.1f Mbps\n",
						done, len(jobs), j.Region, trunc(j.Sponsor, 28), j.PingMs, j.DownMbps, j.UpMbps)
				}
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

func testOne(ctx context.Context, j *ServerResult, o options) {
	if ctx.Err() != nil {
		j.Error = "cancelled"
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
	j.PingMs = ms(s.Latency)
	j.JitterMs = ms(s.Jitter)
	if err := s.DownloadTestContext(ctx); err != nil {
		j.Error = "download: " + err.Error()
		return
	}
	j.DownMbps = rate(s.DLSpeed)
	if !o.noUpload {
		if err := s.UploadTestContext(ctx); err != nil {
			j.Error = "upload: " + err.Error()
			return
		}
		j.UpMbps = rate(s.ULSpeed)
	}
	if j.DownMbps <= 0 && (o.noUpload || j.UpMbps <= 0) {
		j.Error = "no data transferred"
	}
}

func summarize(regions []Region, results []*ServerResult, notes []string) []RegionSummary {
	sums := make([]RegionSummary, len(regions))
	for i, r := range regions {
		sums[i] = RegionSummary{Region: r.Name, Note: notes[i], Servers: []ServerResult{}}
	}
	for _, res := range results {
		s := &sums[res.regionIdx]
		s.Servers = append(s.Servers, *res)
	}
	for i := range sums {
		aggregate(&sums[i])
	}
	return sums
}

// aggregate picks the best server by download speed; ping is the lowest seen.
func aggregate(s *RegionSummary) {
	var downs []float64
	s.Tested = len(s.Servers)
	for _, r := range s.Servers {
		if r.Error != "" {
			continue
		}
		s.OK++
		downs = append(downs, r.DownMbps)
		if r.DownMbps > s.BestDownMbps || s.BestServer == "" {
			s.BestDownMbps = r.DownMbps
			s.BestServer = fmt.Sprintf("%s (%s)", r.Sponsor, r.City)
		}
		if r.UpMbps > s.BestUpMbps {
			s.BestUpMbps = r.UpMbps
		}
		if s.PingMs == 0 || r.PingMs < s.PingMs {
			s.PingMs = r.PingMs
			s.JitterMs = r.JitterMs
		}
	}
	s.MedDownMbps = median(downs)
	if s.Tested > 0 && s.OK == 0 && s.Note == "" {
		s.Note = "all servers failed"
	}
}

func printTable(sums []RegionSummary) {
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "Region\tPing ms\tJitter\tDown Mbps\t(median)\tUp Mbps\tOK\tBest server\t")
	for _, s := range sums {
		if s.OK == 0 {
			fmt.Fprintf(tw, "%s\t-\t-\t-\t-\t-\t0/%d\t%s\t\n", s.Region, s.Tested, s.Note)
			continue
		}
		fmt.Fprintf(tw, "%s\t%.0f\t%.1f\t%.1f\t%.1f\t%.1f\t%d/%d\t%s\t\n",
			s.Region, s.PingMs, s.JitterMs, s.BestDownMbps, s.MedDownMbps, s.BestUpMbps,
			s.OK, s.Tested, trunc(s.BestServer, 40))
	}
	tw.Flush()
}

func writeCSV(path string, results []*ServerResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"time", "region", "server_id", "sponsor", "city", "country", "distance_km",
		"ping_ms", "jitter_ms", "download_mbps", "upload_mbps", "error"})
	now := time.Now().Format(time.RFC3339)
	for _, r := range results {
		_ = w.Write([]string{now, r.Region, r.ID, r.Sponsor, r.City, r.Country,
			f0(r.DistKm), f1(r.PingMs), f1(r.JitterMs), f1(r.DownMbps), f1(r.UpMbps), r.Error})
	}
	w.Flush()
	return w.Error()
}

// ---- helpers ----

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * R * math.Asin(math.Sqrt(a))
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func rate(r speedtest.ByteRate) float64 {
	if r < 0 {
		return 0
	}
	return math.Round(r.Mbps()*10) / 10
}

func ms(d time.Duration) float64 { return math.Round(float64(d)/float64(time.Millisecond)*10) / 10 }
func f0(v float64) string        { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string        { return strconv.FormatFloat(v, 'f', 1, 64) }

func names(js []*ServerResult) string {
	var n []string
	for _, j := range js {
		n = append(n, fmt.Sprintf("%s/%s", j.Sponsor, j.City))
	}
	return trunc(strings.Join(n, ", "), 90)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "multispeed: "+format+"\n", a...)
	os.Exit(1)
}

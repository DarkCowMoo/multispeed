// Command multispeed tests your internet speed to many regions at once, using
// public Speedtest.net servers that are picked automatically for each region.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"time"
)

// perTestOverhead is a rough allowance for ping and connection setup per server.
const perTestOverhead = 5 * time.Second

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

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "multispeed: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() options {
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
	return o
}

func run() error {
	o := parseFlags()
	if o.listRegions {
		printRegions()
		return nil
	}
	if o.k < 1 || o.parallel < 1 {
		return errors.New("-k and -parallel must be at least 1")
	}
	regions, err := ParseRegions(o.regions)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logf("Finding servers for %d regions...\n", len(regions))
	found := discover(ctx, regions, o)
	if len(found.jobs) == 0 {
		return errors.New("no reachable servers found")
	}
	if o.parallel > 1 {
		logf("Note: parallel tests share your bandwidth, so each one will show lower speed.\n")
	}
	logf("Testing %d servers, %d at a time (about %s)...\n",
		len(found.jobs), o.parallel, estimate(len(found.jobs), o).Round(time.Minute))

	results := runTests(ctx, found.jobs, found.backups, o)
	summaries := summarize(regions, results, found.notes)

	if o.csvPath != "" {
		if err := writeCSV(o.csvPath, results); err != nil {
			logf("could not write CSV: %v\n", err)
		}
	}
	if o.jsonOut {
		return writeJSON(os.Stdout, summaries)
	}
	return printTable(os.Stdout, summaries, o.noUpload)
}

// estimate is the rough total run time for n servers.
func estimate(n int, o options) time.Duration {
	perTest := o.duration*2 + perTestOverhead
	if o.noUpload {
		perTest = o.duration + perTestOverhead
	}
	rounds := math.Ceil(float64(n) / float64(o.parallel))
	return time.Duration(rounds) * perTest
}

func printRegions() {
	fmt.Println("local  (your nearest servers)")
	for _, n := range PresetNames() {
		fmt.Printf("%-13s %s\n", n, Presets[n].Name)
	}
}

// logf writes progress messages to stderr, keeping stdout for results.
func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }

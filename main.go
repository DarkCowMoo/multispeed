// Command multispeed tests your internet speed to many regions at once, using
// public Speedtest.net servers that are picked automatically for each region.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

// perTestOverhead is a rough allowance for ping, latency probes and
// connection setup per server.
const perTestOverhead = 6 * time.Second

// minEvery is the shortest interval allowed for -every.
const minEvery = 10 * time.Minute

func main() {
	// net/http reports harmless protocol oddities from servers (such as
	// "Unsolicited response received on idle HTTP channel") through the
	// standard logger, which would print raw bytes over our output. All of
	// our own messages go through logf, so the standard logger is silenced.
	log.SetOutput(io.Discard)

	// A double-clicked .exe gets its own console window, which Windows closes
	// as soon as the program exits; keep it open so the results can be read.
	keepOpen := ownsConsoleWindow()
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "multispeed: %v\n", err)
	}
	if keepOpen {
		fmt.Fprint(os.Stderr, "\nPress Enter to close this window...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if err != nil {
		os.Exit(1)
	}
}

func run() error {
	o, set, err := parseFlags(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if o.listRegions {
		printRegions()
		return nil
	}
	setupColors(o.noColor)
	ec := errColors

	histPath, histErr := historyPath()
	if o.showHistory {
		if histErr != nil {
			return histErr
		}
		hist, loadErr := loadHistory(histPath)
		if loadErr != nil {
			return loadErr
		}
		return printHistory(os.Stdout, outColors, histPath, hist)
	}
	if o.k < 1 || o.parallel < 1 {
		return errors.New("-k and -parallel must be at least 1")
	}
	// The servers are mostly run by volunteers: keep repeated tests polite.
	if o.every != 0 && o.every < minEvery {
		return fmt.Errorf("-every must be at least %.0f minutes (test servers are run by volunteers)",
			minEvery.Minutes())
	}
	if histErr != nil {
		histPath = "" // no history, but the test still runs
	}

	printBanner(os.Stderr, ec)
	me := fetchUser()
	runID := newRunID()
	if me.isp != "" {
		printInfo(os.Stderr, ec, "Your connection", me.isp)
	}
	if me.ip != "" {
		// Shown for visibility only; the IP is not saved to history or reports.
		printInfo(os.Stderr, ec, "Your IP", me.ip)
	}
	printInfo(os.Stderr, ec, "Run ID", runID)
	// The picker runs before Ctrl+C is captured, so Ctrl+C still quits it.
	if o.pick {
		if !isTerminal(os.Stdin) {
			return errors.New("-pick needs a terminal to ask in; use -regions instead")
		}
		printSection(os.Stderr, ec, "Choose regions")
		o.regions, err = pickRegions(os.Stdin, os.Stderr, o)
		if err != nil {
			return err
		}
		set["regions"] = true
		delete(set, "pick") // the re-run command lists the picked regions instead
	}
	regions, err := ParseRegions(o.regions)
	if err != nil {
		return err
	}
	for i := range regions {
		if regions[i].Local && me.located {
			regions[i].Lat, regions[i].Lon = me.lat, me.lon // for the map
		}
	}
	printInfo(os.Stderr, ec, "Plan", planSummary(len(regions), o))
	if o.every > 0 {
		printInfo(os.Stderr, ec, "Repeat", "every "+fmtDur(o.every)+", until you press Ctrl+C")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for round := 1; ; round++ {
		info := runInfo{id: runID, isp: me.isp}
		if o.every > 0 {
			info.round = round
			printSection(os.Stderr, ec, fmt.Sprintf("Round %d · %s", round, time.Now().Format("15:04")))
		}
		err = testRound(ctx, regions, o, info, histPath)
		if o.every == 0 || ctx.Err() != nil {
			break
		}
		if err != nil {
			logf("  %s\n", ec.bad("This round failed: "+err.Error()))
		}
		next := time.Now().Add(o.every)
		logf("\n  %s\n", ec.dim(fmt.Sprintf("Next round at %s. Press Ctrl+C to stop.", next.Format("15:04"))))
		select {
		case <-ctx.Done():
		case <-time.After(time.Until(next)):
		}
		if ctx.Err() != nil {
			break
		}
	}
	logf("\n  %s\n  %s\n\n", ec.dim("Run exactly this test again:"),
		ec.accent(rerunCommand(programName(), o, set)))
	return err
}

// testRound finds servers, tests them, compares with history and shows,
// saves and exports the results.
func testRound(ctx context.Context, regions []Region, o options, info runInfo, histPath string) error {
	ec := errColors
	printSection(os.Stderr, ec, "Finding servers")
	found := discover(ctx, regions, o)
	if len(found.jobs) == 0 {
		return errors.New("no reachable servers found")
	}

	printSection(os.Stderr, ec, "Testing")
	pace := "one at a time"
	if o.parallel > 1 {
		pace = fmt.Sprintf("%d at a time; they share your bandwidth, so each shows a lower speed", o.parallel)
	}
	logf("%s\n\n", ec.dim(fmt.Sprintf("%d servers, %s, about %s. Press Ctrl+C to stop.",
		len(found.jobs), pace, fmtDur(estimate(len(found.jobs), o)))))

	results, stats := runTests(ctx, found.jobs, found.backups, o)
	sums := summarize(regions, results, found.notes, o.noUpload)

	var hist []HistoryEntry
	if histPath != "" {
		var err error
		if hist, err = loadHistory(histPath); err != nil {
			logf("  could not read history: %v\n", err)
		}
	}
	compareWithHistory(sums, hist)

	if o.route && ctx.Err() == nil {
		printSection(os.Stderr, ec, "Tracing routes")
		traceRoutes(ctx, sums)
	}
	report := newReport(o, info, sums, stats)

	saved := false
	if !o.noSave && histPath != "" {
		e := historyEntry(&report)
		if err := appendHistory(histPath, e); err != nil {
			logf("  could not save history: %v\n", err)
		} else if len(e.Regions) > 0 {
			hist = append(hist, e)
			saved = true
		}
	}
	if o.csvPath != "" {
		if err := writeCSV(o.csvPath, results); err != nil {
			logf("  could not write CSV: %v\n", err)
		}
	}
	if o.htmlPath != "" {
		if err := writeHTML(o.htmlPath, &report, hist); err != nil {
			logf("  could not write HTML report: %v\n", err)
		}
	}

	if o.jsonOut {
		if o.every > 0 { // one compact line per round: JSON Lines
			return json.NewEncoder(os.Stdout).Encode(report)
		}
		return writeJSON(os.Stdout, report)
	}
	return printResults(os.Stdout, &report, o, saved)
}

// printResults shows the results, what each region is good for, the map and
// the routes.
func printResults(w io.Writer, r *Report, o options, saved bool) error {
	oc := outColors
	printSection(w, oc, "Results")
	fmt.Fprintln(w)
	if err := printTable(w, r.Regions, o.noUpload, oc); err != nil {
		return err
	}
	footer := fmt.Sprintf("Run %s · data used %s · took %s", r.RunID, fmtBytes(r.DataBytes),
		fmtDur(time.Duration(r.Seconds*float64(time.Second))))
	if saved {
		footer += " · saved to history (multispeed -history)"
	}
	fmt.Fprintf(w, "\n  %s\n", oc.dim(footer))

	printSection(w, oc, "What it's good for")
	fmt.Fprintln(w)
	if err := printGoodFor(w, r.Regions, oc); err != nil {
		return err
	}
	if anyGraded(r.Regions) {
		fmt.Fprintf(w, "\n  %s\n", oc.dim("Bloat = how much ping rises while your line is busy. "+
			"A+/A: smooth calls and games even during downloads; D/F: lag spikes."))
	}

	if !o.noMap {
		var m strings.Builder
		if err := printMap(&m, r.Regions, oc); err == nil && m.Len() > 0 {
			printSection(w, oc, "Map")
			fmt.Fprintln(w)
			_, _ = io.WriteString(w, m.String())
		}
	}
	if o.route {
		printSection(w, oc, "Routes")
		fmt.Fprintln(w)
		printRoutes(w, r.Regions, oc)
	}
	if o.htmlPath != "" {
		fmt.Fprintf(w, "\n  %s %s\n", oc.dim("Report saved to"), oc.accent(o.htmlPath))
	}
	return nil
}

func anyGraded(sums []RegionSummary) bool {
	for i := range sums {
		if sums[i].Grade != "" {
			return true
		}
	}
	return false
}

// planSummary describes the run, e.g. "3 regions × 1 server, download only, about 1m".
func planSummary(regions int, o options) string {
	servers := "servers"
	if o.k == 1 {
		servers = "server"
	}
	what := "download and upload"
	if o.noUpload {
		what = "download only"
	}
	return fmt.Sprintf("%d regions × %d %s, %s, about %s",
		regions, o.k, servers, what, fmtDur(estimate(regions*o.k, o)))
}

// user is what Speedtest.net knows about this connection.
type user struct {
	isp      string
	ip       string
	lat, lon float64
	located  bool
}

// fetchUser looks up your internet provider and approximate location.
func fetchUser() user {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u, err := newClient(nil).FetchUserInfoContext(ctx)
	if err != nil || u == nil {
		return user{}
	}
	me := user{isp: u.Isp, ip: u.IP}
	lat, errLat := strconv.ParseFloat(u.Lat, 64)
	lon, errLon := strconv.ParseFloat(u.Lon, 64)
	if errLat == nil && errLon == nil && (lat != 0 || lon != 0) {
		me.lat, me.lon, me.located = lat, lon, true
	}
	return me
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
	fmt.Println("local         Your nearest servers")
	for _, n := range PresetNames() {
		fmt.Printf("%-13s %s\n", n, Presets[n].Name)
	}
}

// logf writes progress messages to stderr, keeping stdout for results.
func logf(format string, a ...any) { con.logf(format, a...) }

// Package cli is multispeed's command line: flags, the region picker, and the
// flow of a run, from the opening screen to the results.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/regions"
	"github.com/DarkCowMoo/multispeed/internal/results"
	"github.com/DarkCowMoo/multispeed/internal/ui"
)

// minEvery is the shortest interval allowed for -every.
const minEvery = 10 * time.Minute

// regionLabelWidth aligns region names in the discovery log.
const regionLabelWidth = 16

// Main runs multispeed with the command-line arguments and returns the exit
// code. buildVersion is the version set by release builds (may be empty).
func Main(buildVersion string) int {
	// net/http reports harmless protocol oddities from servers (such as
	// "Unsolicited response received on idle HTTP channel") through the
	// standard logger, which would print raw bytes over our output. All of
	// our own messages go through ui.Logf, so the standard logger is silenced.
	log.SetOutput(io.Discard)

	// A double-clicked .exe gets its own console window, which Windows closes
	// as soon as the program exits; keep it open so the results can be read.
	keepOpen := ui.OwnsConsoleWindow()
	err := run(os.Args[1:], version(buildVersion))
	if err != nil {
		fmt.Fprintf(os.Stderr, "multispeed: %v\n", err)
	}
	if keepOpen {
		fmt.Fprint(os.Stderr, "\nPress Enter to close this window...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if err != nil {
		return 1
	}
	return 0
}

func run(args []string, ver string) error {
	o, set, err := parseFlags(args)
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
	ui.SetupColors(o.noColor)
	ec := ui.ErrColors

	histPath, histErr := results.HistoryPath()
	if o.showHistory {
		if histErr != nil {
			return histErr
		}
		hist, loadErr := results.LoadHistory(histPath)
		if loadErr != nil {
			return loadErr
		}
		return ui.PrintHistory(os.Stdout, ui.OutColors, histPath, hist)
	}
	if err = o.validate(); err != nil {
		return err
	}
	if histErr != nil {
		histPath = "" // no history, but the test still runs
	}

	ui.Banner(os.Stderr, ec, ver)
	me := measure.FetchUser(context.Background())
	runID := results.NewRunID()
	if me.ISP != "" {
		ui.Info(os.Stderr, ec, "Your connection", me.ISP)
	}
	if me.IP != "" {
		// Shown for visibility only; the IP is not saved to history or reports.
		ui.Info(os.Stderr, ec, "Your IP", me.IP)
	}
	ui.Info(os.Stderr, ec, "Run ID", runID)
	// The picker runs before Ctrl+C is captured, so Ctrl+C still quits it.
	if o.pick {
		if !ui.IsTerminal(os.Stdin) {
			return errors.New("-pick needs a terminal to ask in; use -regions instead")
		}
		ui.Section(os.Stderr, ec, "Choose regions")
		o.regions, err = pickRegions(os.Stdin, os.Stderr, o)
		if err != nil {
			return err
		}
		set["regions"] = true
		delete(set, "pick") // the re-run command lists the picked regions instead
	}
	rs, err := regions.ParseRegions(o.regions)
	if err != nil {
		return err
	}
	for i := range rs {
		if rs[i].Local && me.Located {
			rs[i].Lat, rs[i].Lon = me.Lat, me.Lon // for the map
		}
	}
	ui.Info(os.Stderr, ec, "Plan", planSummary(len(rs), o))
	if o.every > 0 {
		ui.Info(os.Stderr, ec, "Repeat", "every "+ui.FmtDur(o.every)+", until you press Ctrl+C")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for round := 1; ; round++ {
		info := results.RunInfo{ID: runID, ISP: me.ISP, Version: ver}
		if o.every > 0 {
			info.Round = round
			ui.Section(os.Stderr, ec, fmt.Sprintf("Round %d · %s", round, time.Now().Format("15:04")))
		}
		err = testRound(ctx, rs, o, info, histPath)
		if o.every == 0 || ctx.Err() != nil {
			break
		}
		if err != nil {
			ui.Logf("  %s\n", ec.Bad("This round failed: "+err.Error()))
		}
		next := time.Now().Add(o.every)
		ui.Logf("\n  %s\n", ec.Dim(fmt.Sprintf("Next round at %s. Press Ctrl+C to stop.", next.Format("15:04"))))
		select {
		case <-ctx.Done():
		case <-time.After(time.Until(next)):
		}
		if ctx.Err() != nil {
			break
		}
	}
	ui.Logf("\n  %s\n  %s\n\n", ec.Dim("Run exactly this test again:"),
		ec.Accent(rerunCommand(programName(), o, set)))
	return err
}

// testRound finds servers, tests them, compares with history and shows,
// saves and exports the results.
func testRound(ctx context.Context, rs []regions.Region, o options, info results.RunInfo, histPath string) error {
	ec := ui.ErrColors
	s := o.settings()

	ui.Section(os.Stderr, ec, "Finding servers")
	found := discover(ctx, rs, s)
	if len(found.Jobs) == 0 {
		return errors.New("no reachable servers found")
	}

	ui.Section(os.Stderr, ec, "Testing")
	pace := "one at a time"
	if o.parallel > 1 {
		pace = fmt.Sprintf("%d at a time; they share your bandwidth, so each shows a lower speed", o.parallel)
	}
	estimate := measure.Estimate(len(found.Jobs), s)
	ui.Logf("%s\n\n", ec.Dim(fmt.Sprintf("%d servers, %s, about %s. Press Ctrl+C to stop.",
		len(found.Jobs), pace, ui.FmtDur(estimate))))

	progress := ui.NewTestProgress(len(found.Jobs), estimate, o.noUpload)
	stopStatus := ui.ShowStatus(progress.Status)
	tested, stats := measure.RunTests(ctx, found.Jobs, found.Backups, s, progress)
	stopStatus()
	sums := results.Summarize(rs, tested, found.Notes, o.noUpload)

	var hist []results.HistoryEntry
	if histPath != "" {
		var err error
		if hist, err = results.LoadHistory(histPath); err != nil {
			ui.Logf("  could not read history: %v\n", err)
		}
	}
	results.CompareWithHistory(sums, hist)

	if o.route && ctx.Err() == nil {
		ui.Section(os.Stderr, ec, "Tracing routes")
		traceRoutes(ctx, sums)
	}
	report := results.New(info, o.reportSettings(), sums, stats)

	saved := false
	if !o.noSave && histPath != "" {
		e := results.EntryFrom(&report)
		if err := results.AppendHistory(histPath, e); err != nil {
			ui.Logf("  could not save history: %v\n", err)
		} else if len(e.Regions) > 0 {
			hist = append(hist, e)
			saved = true
		}
	}
	if o.csvPath != "" {
		if err := results.WriteCSV(o.csvPath, tested); err != nil {
			ui.Logf("  could not write CSV: %v\n", err)
		}
	}
	if o.htmlPath != "" {
		if err := ui.WriteHTML(o.htmlPath, &report, hist); err != nil {
			ui.Logf("  could not write HTML report: %v\n", err)
		}
	}

	if o.jsonOut {
		// With -every, one compact line per round: JSON Lines.
		return results.WriteJSON(os.Stdout, &report, o.every > 0)
	}
	return printResults(os.Stdout, &report, o, saved)
}

// discover finds servers for every region, showing a status line and one log
// line per region.
func discover(ctx context.Context, rs []regions.Region, s measure.Settings) measure.Discovery {
	ec := ui.ErrColors
	var looked atomic.Int32
	started := time.Now()
	stopStatus := ui.ShowStatus(func() string {
		return fmt.Sprintf("Finding servers... %d/%d regions | %s elapsed",
			looked.Load(), len(rs), ui.FmtDur(time.Since(started)))
	})
	defer stopStatus()

	return measure.Discover(ctx, rs, s, func(r regions.Region, jobs []*measure.ServerResult, backups int, note string) {
		looked.Add(1)
		name := ec.Bold(ui.PadRight(r.Name, regionLabelWidth))
		if len(jobs) == 0 {
			ui.Logf("  %s %s\n", name, ec.Bad("skipped: "+note))
			return
		}
		ui.Logf("  %s %s %s\n", name, serverNames(jobs), ec.Dim(fmt.Sprintf("(+%d backup)", backups)))
	})
}

// serverNames lists "sponsor/city" for each server, shortened for the log.
func serverNames(js []*measure.ServerResult) string {
	n := make([]string, 0, len(js))
	for _, j := range js {
		n = append(n, j.Sponsor+"/"+j.City)
	}
	return ui.Trunc(strings.Join(n, ", "), 90)
}

// traceRoutes traces the route to each region's best server, with a status line.
func traceRoutes(ctx context.Context, sums []results.RegionSummary) {
	var todo []*results.RegionSummary
	var hosts []string
	for i := range sums {
		if sums[i].OK > 0 && sums[i].BestHost != "" {
			todo = append(todo, &sums[i])
			hosts = append(hosts, sums[i].BestHost)
		}
	}
	if len(todo) == 0 {
		return
	}
	var finished atomic.Int32
	started := time.Now()
	stopStatus := ui.ShowStatus(func() string {
		return fmt.Sprintf("Tracing routes... %d/%d | %s elapsed",
			finished.Load(), len(todo), ui.FmtDur(time.Since(started)))
	})
	routes := measure.TraceRoutes(ctx, hosts, func() { finished.Add(1) })
	stopStatus()
	for i, s := range todo {
		s.Route = routes[i]
	}
}

// printResults shows the results, what each region is good for, the map and
// the routes.
func printResults(w io.Writer, r *results.Report, o options, saved bool) error {
	oc := ui.OutColors
	ui.Section(w, oc, "Results")
	fmt.Fprintln(w)
	if err := ui.PrintTable(w, r.Regions, o.noUpload, oc); err != nil {
		return err
	}
	footer := fmt.Sprintf("Run %s · data used %s · took %s", r.RunID, ui.FmtBytes(r.DataBytes),
		ui.FmtDur(time.Duration(r.Seconds*float64(time.Second))))
	if saved {
		footer += " · saved to history (multispeed -history)"
	}
	fmt.Fprintf(w, "\n  %s\n", oc.Dim(footer))

	ui.Section(w, oc, "What it's good for")
	fmt.Fprintln(w)
	if err := ui.PrintGoodFor(w, r.Regions, oc); err != nil {
		return err
	}
	if anyGraded(r.Regions) {
		fmt.Fprintf(w, "\n  %s\n", oc.Dim("Bloat = how much ping rises while your line is busy. "+
			"A+/A: smooth calls and games even during downloads; D/F: lag spikes."))
		ui.PrintBloatNotes(w, r.Regions, oc)
	}

	if !o.noMap {
		var m strings.Builder
		if err := ui.PrintMap(&m, r.Regions, oc); err == nil && m.Len() > 0 {
			ui.Section(w, oc, "Map")
			fmt.Fprintln(w)
			_, _ = io.WriteString(w, m.String())
		}
	}
	if o.route {
		ui.Section(w, oc, "Routes")
		fmt.Fprintln(w)
		ui.PrintRoutes(w, r.Regions, oc)
	}
	if o.htmlPath != "" {
		fmt.Fprintf(w, "\n  %s %s\n", oc.Dim("Report saved to"), oc.Accent(o.htmlPath))
	}
	return nil
}

func anyGraded(sums []results.RegionSummary) bool {
	for i := range sums {
		if sums[i].Grade != "" || sums[i].BloatNote != "" {
			return true
		}
	}
	return false
}

// planSummary describes the run, e.g. "3 regions × 1 server, download only, about 1m".
func planSummary(n int, o options) string {
	servers := "servers"
	if o.k == 1 {
		servers = "server"
	}
	what := "download and upload"
	if o.noUpload {
		what = "download only"
	}
	return fmt.Sprintf("%d regions × %d %s, %s, about %s",
		n, o.k, servers, what, ui.FmtDur(measure.Estimate(n*o.k, o.settings())))
}

func printRegions() {
	fmt.Println("local         Your nearest servers")
	for _, n := range regions.PresetNames() {
		fmt.Printf("%-13s %s\n", n, regions.Presets[n].Name)
	}
}

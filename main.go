// Command multispeed tests your internet speed to many regions at once, using
// public Speedtest.net servers that are picked automatically for each region.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
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
	pick        bool
	noColor     bool
}

func main() {
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

// defineFlags registers every flag on fs. The help text is shown by -h,
// grouped by flagGroups.
func defineFlags(fs *flag.FlagSet, o *options) {
	fs.StringVar(&o.regions, "regions", strings.Join(DefaultRegions, ","),
		"Regions to test, comma separated. Each is \"local\" (the servers nearest to you),\n"+
			"a preset such as frankfurt (see -list-regions), or Name=lat,lon for any place.")
	fs.BoolVar(&o.pick, "pick", false,
		"Choose the regions from a numbered list of all of them before testing.")
	fs.BoolVar(&o.listRegions, "list-regions", false,
		"Show the preset region names and exit.")
	fs.IntVar(&o.k, "k", 3,
		"Servers to test in each region. The table shows the best and the median, so one\n"+
			"overloaded server can't make a whole region look slow. Use 1 for a quick check.")
	fs.IntVar(&o.parallel, "parallel", 1,
		"How many servers to test at the same time. 1 (one by one) gives honest per-region\n"+
			"speeds. Higher is faster, but parallel tests share your bandwidth.")
	fs.DurationVar(&o.duration, "duration", 10*time.Second,
		"How long each download and each upload measurement runs, e.g. 5s or 15s.\n"+
			"Shorter is faster but less accurate.")
	fs.Float64Var(&o.maxDistKm, "max-distance", 1000,
		"Only use servers within this many km of a region's center. Raise it for regions\n"+
			"with few servers; lower it to stay close to the exact city.")
	fs.BoolVar(&o.noUpload, "no-upload", false,
		"Skip the upload test. Roughly halves the run time and data use.")
	fs.BoolVar(&o.jsonOut, "json", false,
		"Print full results, including every server, as JSON instead of a table.")
	fs.StringVar(&o.csvPath, "csv", "",
		"Also save one row per tested server to this CSV `file` (handy for tracking over time).")
	fs.BoolVar(&o.noColor, "no-color", false,
		"Plain text, no colors. Colors are also off when output is not a terminal,\n"+
			"or when the NO_COLOR environment variable is set.")
}

// flagGroups orders the flags in -h output. Every flag must appear here.
var flagGroups = []struct {
	title string
	flags []string
}{
	{"What to test", []string{"regions", "pick", "list-regions"}},
	{"How to test", []string{"k", "parallel", "duration", "max-distance", "no-upload"}},
	{"Output", []string{"json", "csv", "no-color"}},
}

// flagArgs names the value each non-boolean flag takes, for -h output.
var flagArgs = map[string]string{
	"regions": "list", "k": "n", "parallel": "n", "duration": "time",
	"max-distance": "km", "csv": "file",
}

func printUsage(fs *flag.FlagSet) {
	w := fs.Output()
	fmt.Fprint(w, `multispeed tests your internet speed to several regions of the world,
using public Speedtest.net servers picked automatically in each region.
Progress is shown while it runs; results are printed as a table at the end.

Usage:
  multispeed [flags]

Examples:
  multispeed                                    your connection + 9 world hubs
  multispeed -pick                              choose regions from a list
  multispeed -regions local,frankfurt,newyork   test only these regions
  multispeed -k 1 -no-upload                    quick check of the default regions
  multispeed -regions "Home=31.77,35.21"        any place, by latitude,longitude
  multispeed -parallel 3 -csv results.csv       faster, and save every result
`)
	for _, g := range flagGroups {
		fmt.Fprintf(w, "\n%s:\n", g.title)
		for _, name := range g.flags {
			f := fs.Lookup(name)
			if f == nil {
				continue
			}
			head := "  -" + f.Name
			if arg := flagArgs[f.Name]; arg != "" {
				head += " " + arg
			}
			fmt.Fprintln(w, head)
			for _, line := range strings.Split(strings.ReplaceAll(f.Usage, "`", ""), "\n") {
				fmt.Fprintf(w, "        %s\n", line)
			}
			if f.DefValue != "" && f.DefValue != "false" {
				fmt.Fprintf(w, "        (default %s)\n", f.DefValue)
			}
		}
	}
}

func parseFlags(args []string) (options, map[string]bool, error) {
	var o options
	fs := flag.NewFlagSet("multispeed", flag.ContinueOnError)
	defineFlags(fs, &o)
	fs.Usage = func() { printUsage(fs) }
	if err := fs.Parse(args); err != nil {
		return o, nil, err
	}
	if fs.NArg() > 0 {
		return o, nil, fmt.Errorf("unexpected argument %q (run with -h for help)", fs.Arg(0))
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return o, set, nil
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
	if o.k < 1 || o.parallel < 1 {
		return errors.New("-k and -parallel must be at least 1")
	}
	setupColors(o.noColor)
	ec := errColors

	printBanner(os.Stderr, ec)
	if isp := fetchISP(); isp != "" {
		printInfo(os.Stderr, ec, "Your connection", isp)
	}
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
	printInfo(os.Stderr, ec, "Plan", planSummary(len(regions), o))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

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
	summaries := summarize(regions, results, found.notes)

	if o.csvPath != "" {
		if csvErr := writeCSV(o.csvPath, results); csvErr != nil {
			logf("could not write CSV: %v\n", csvErr)
		}
	}
	if o.jsonOut {
		err = writeJSON(os.Stdout, summaries)
	} else {
		oc := outColors
		printSection(os.Stdout, oc, "Results")
		fmt.Println()
		err = printTable(os.Stdout, summaries, o.noUpload, oc)
		fmt.Printf("\n  %s\n", oc.dim(fmt.Sprintf("Data used %s · took %s",
			fmtBytes(stats.dataBytes), fmtDur(stats.elapsed))))
	}
	logf("\n  %s\n  %s\n\n", ec.dim("Run exactly this test again:"),
		ec.accent(rerunCommand(programName(), o, set)))
	return err
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

// fetchISP returns the name of your internet provider, or "" if unknown.
func fetchISP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u, err := speedtest.New().FetchUserInfoContext(ctx)
	if err != nil || u == nil {
		return ""
	}
	return u.Isp
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

// rerunCommand rebuilds the command line for the flags that were set
// (including regions picked interactively), in a stable order.
func rerunCommand(prog string, o options, set map[string]bool) string {
	values := map[string]string{
		"regions":      o.regions,
		"k":            fmt.Sprint(o.k),
		"parallel":     fmt.Sprint(o.parallel),
		"duration":     o.duration.String(),
		"max-distance": fmt.Sprint(o.maxDistKm),
		"csv":          o.csvPath,
	}
	bools := map[string]bool{"no-upload": o.noUpload, "json": o.jsonOut}

	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	// regions first: it is the main choice.
	sort.SliceStable(names, func(i, j int) bool { return names[i] == "regions" && names[j] != "regions" })

	parts := []string{prog}
	for _, name := range names {
		if v, ok := bools[name]; ok {
			if v {
				parts = append(parts, "-"+name)
			}
			continue
		}
		if v, ok := values[name]; ok {
			parts = append(parts, "-"+name, shellQuote(v))
		}
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes s if it contains characters a shell would split or expand.
func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'$&|;<>()*?!`") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// programName is how to type this program again: "go run ." for a go run
// build, ".\multispeed.exe" (or "./multispeed") when it is in the current
// folder, its bare name when it is on PATH, or else its full path.
func programName() string {
	self := os.Args[0]
	if abs, err := filepath.Abs(self); err == nil {
		self = abs
	}
	if strings.Contains(filepath.ToSlash(self), "/go-build") {
		return "go run ."
	}
	base := filepath.Base(self)
	if wd, err := os.Getwd(); err == nil && sameDir(filepath.Dir(self), wd) {
		return "." + string(filepath.Separator) + base
	}
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil && strings.EqualFold(abs, self) {
			return name
		}
	}
	return shellQuote(self)
}

func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func printRegions() {
	fmt.Println("local         Your nearest servers")
	for _, n := range PresetNames() {
		fmt.Printf("%-13s %s\n", n, Presets[n].Name)
	}
}

// logf writes progress messages to stderr, keeping stdout for results.
func logf(format string, a ...any) { con.logf(format, a...) }

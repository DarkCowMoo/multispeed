package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type options struct {
	regions     string
	k           int
	parallel    int
	duration    time.Duration
	maxDistKm   float64
	noUpload    bool
	route       bool
	jsonOut     bool
	csvPath     string
	htmlPath    string
	noMap       bool
	noColor     bool
	listRegions bool
	pick        bool
	showHistory bool
	noSave      bool
	every       time.Duration
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

	fs.IntVar(&o.k, "k", 1,
		"Servers to test in each region. With more than one, the table shows the best and\n"+
			"the median, so one overloaded server can't make a whole region look slow.\n"+
			"Try 3 if a region looks oddly slow (it takes 3 times as long).")
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
	fs.BoolVar(&o.route, "route", false,
		"Also trace the network route to each region and show where the delay comes from.\n"+
			"Uses the system's tracert/traceroute; adds up to a minute.")

	fs.BoolVar(&o.jsonOut, "json", false,
		"Print the full results as JSON on stdout instead of tables (for scripts and AI\n"+
			"agents). Progress still goes to stderr. With -every, one JSON line per round.")
	fs.StringVar(&o.csvPath, "csv", "",
		"Also append one row per tested server to this CSV `file`.")
	fs.StringVar(&o.htmlPath, "html", "",
		"Also save a self-contained HTML report to this `file`, easy to share.")
	fs.BoolVar(&o.noMap, "no-map", false,
		"Don't draw the world map under the results.")
	fs.BoolVar(&o.noColor, "no-color", false,
		"Plain text, no colors. Colors are also off when output is not a terminal,\n"+
			"or when the NO_COLOR environment variable is set.")

	fs.BoolVar(&o.showHistory, "history", false,
		"Show the saved results of past runs, with a trend per region, and exit.")
	fs.BoolVar(&o.noSave, "no-save", false,
		"Don't save this run to the history. (Runs are saved by default, so each run can\n"+
			"show the change since the last one.)")
	fs.DurationVar(&o.every, "every", 0,
		"Keep testing on a schedule, e.g. 30m or 2h, until Ctrl+C. Each round is saved to\n"+
			"the history (and the CSV file, if given). Good for catching evening slowdowns.")
}

// flagGroups orders the flags in -h output. Every flag must appear here.
var flagGroups = []struct {
	title string
	flags []string
}{
	{"What to test", []string{"regions", "pick", "list-regions"}},
	{"How to test", []string{"k", "parallel", "duration", "max-distance", "no-upload", "route"}},
	{"Output", []string{"json", "csv", "html", "no-map", "no-color"}},
	{"History and monitoring", []string{"history", "no-save", "every"}},
}

// flagArgs names the value each non-boolean flag takes, for -h output.
var flagArgs = map[string]string{
	"regions": "list", "k": "n", "parallel": "n", "duration": "time",
	"max-distance": "km", "csv": "file", "html": "file", "every": "time",
}

func printUsage(fs *flag.FlagSet) {
	w := fs.Output()
	fmt.Fprint(w, `multispeed tests your internet speed to several regions of the world,
using public Speedtest.net servers picked automatically in each region.
It shows speed, ping, latency under load (bufferbloat), what each region is
good for, a world map, and the change since your last run.

Usage:
  multispeed [flags]

Examples:
  multispeed                                    your connection + 9 world hubs
  multispeed -pick                              choose regions from a list
  multispeed -regions local,frankfurt,newyork   test only these regions
  multispeed -no-upload                         quicker: download only
  multispeed -k 3                               3 servers per region, more reliable
  multispeed -regions "Home=31.77,35.21"        any place, by latitude,longitude
  multispeed -html report.html -route           shareable report, with routes
  multispeed -every 1h -csv log.csv             test every hour until Ctrl+C
  multispeed -history                           trends from past runs
  multispeed -json -no-upload -regions local    machine-readable, for scripts
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
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0s" {
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
		"html":         o.htmlPath,
		"every":        o.every.String(),
	}
	bools := map[string]bool{
		"no-upload": o.noUpload, "json": o.jsonOut, "route": o.route, "no-map": o.noMap,
		"no-color": o.noColor, "no-save": o.noSave,
	}

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

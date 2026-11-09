package main

import (
	"flag"
	"strings"
	"testing"
	"time"
)

// Every flag must be listed in flagGroups, or it would be missing from -h.
func TestEveryFlagHasHelp(t *testing.T) {
	var o options
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	defineFlags(fs, &o)
	grouped := map[string]bool{}
	for _, g := range flagGroups {
		for _, name := range g.flags {
			if fs.Lookup(name) == nil {
				t.Errorf("flagGroups lists unknown flag -%s", name)
			}
			grouped[name] = true
		}
	}
	fs.VisitAll(func(f *flag.Flag) {
		if !grouped[f.Name] {
			t.Errorf("flag -%s is not in flagGroups", f.Name)
		}
	})

	var b strings.Builder
	fs.SetOutput(&b)
	printUsage(fs)
	for _, want := range []string{"-regions list", "-k n", "-csv file", "(default 1)", "Examples:"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help is missing %q", want)
		}
	}
}

func TestParseFlags(t *testing.T) {
	o, set, err := parseFlags([]string{"-k", "1", "-no-upload", "-regions", "local,tokyo"})
	if err != nil {
		t.Fatal(err)
	}
	if o.k != 1 || !o.noUpload || o.regions != "local,tokyo" || o.duration != 10*time.Second {
		t.Errorf("options = %+v", o)
	}
	if !set["k"] || !set["regions"] || set["duration"] {
		t.Errorf("set = %v", set)
	}
	if _, _, err := parseFlags([]string{"extra"}); err == nil {
		t.Error("expected an error for a stray argument")
	}
}

func TestRerunCommand(t *testing.T) {
	o, set, err := parseFlags([]string{"-no-upload", "-k", "1", "-csv", "my results.csv"})
	if err != nil {
		t.Fatal(err)
	}
	o.regions, set["regions"] = "local,Lisbon=38.72,-9.14", true // as if picked
	got := rerunCommand("multispeed", o, set)
	want := `multispeed -regions local,Lisbon=38.72,-9.14 -csv "my results.csv" -k 1 -no-upload`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	// The command must parse back to the same options.
	args := []string{"-regions", o.regions, "-csv", o.csvPath, "-k", "1", "-no-upload"}
	o2, _, err := parseFlags(args)
	if err != nil || o2 != o {
		t.Errorf("round trip: %+v, %v", o2, err)
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"plain":       "plain",
		"a b":         `"a b"`,
		"":            `""`,
		`say "hi"`:    `"say \"hi\""`,
		"local,tokyo": "local,tokyo",
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

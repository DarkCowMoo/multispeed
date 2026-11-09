// Command multispeed tests your internet speed to many regions at once, using
// public Speedtest.net servers that are picked automatically for each region.
//
// The program lives in internal/: cli (flags and the flow of a run), measure
// (finding and testing servers), results (summaries, history, JSON/CSV),
// ui (terminal and HTML output) and regions (the places to test).
package main

import (
	"os"

	"github.com/DarkCowMoo/multispeed/internal/cli"
)

// buildVersion is set by release builds:
// go build -ldflags "-X main.buildVersion=v1.2.3".
var buildVersion string

func main() {
	os.Exit(cli.Main(buildVersion))
}

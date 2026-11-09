# multispeed

**Test your internet speed to many places at once, not just the nearest server.**

Normal speed tests measure the connection to the closest server, so they say little about how fast
you can reach Europe, the US or Asia. multispeed picks good public Speedtest.net servers in each
region automatically, tests them one by one or in parallel, and gives you one comparison table.

```
Region           Ping ms  Jitter  Down Mbps  (median)  Up Mbps   OK  Best server
Local (nearest)        4     0.6      912.3     905.1    94.2   3/3  Bezeq (Tel Aviv)
Frankfurt             58     1.2      431.0     402.7    90.8   3/3  Hetzner (Frankfurt)
New York             132     2.4      188.5     160.2    71.0   3/3  Ookla (New York, NY)
Tokyo                208     3.1       96.4      90.0    40.3   2/3  IPA (Tokyo)
...
```
*(example output)*

## Quick start

Download the file for your system from the releases, then run it in a terminal:

On Windows you can simply **double-click `multispeed.exe`**: it tests your connection plus 9 world
hubs, and the window stays open at the end until you press Enter.

```sh
./multispeed                     # your connection + 9 world hubs, 3 servers each
./multispeed -pick               # choose regions from a numbered list of all 29
./multispeed -h                  # every option, explained, with examples
./multispeed -parallel 3         # test 3 servers at once (faster)
./multispeed -regions local,frankfurt,london,newyork -k 2
./multispeed -regions "local,Home=31.77,35.21,singapore"   # any place by coordinates
./multispeed -list-regions       # show the built-in region names
```

With `-pick`, it lists every region (your local servers plus 28 world hubs) and asks which to
test: type numbers or names, `all`, or press Enter for the default set (local + 9 hubs).

It opens with your internet provider and the plan (regions, servers, estimated time). While it
runs, a live line shows the current server, what it is doing (ping, download or upload, with the
speed so far), and the time elapsed and left. Results are colored: ping and speed are green, yellow
or red, and each region gets a bar comparing its download with your best region. At the end it
shows the data used and prints the exact command to run the same test again, for example
`.\multispeed.exe -regions local,frankfurt,tokyo -k 1`. Use `-no-color` (or set `NO_COLOR`) for
plain text.

On Windows, use `multispeed.exe`. On macOS, if the file is blocked the first time, run
`xattr -d com.apple.quarantine multispeed`.

## How it works

1. For each region, it asks Speedtest.net for servers near that region's coordinates.
2. It drops servers that don't answer, or that are more than `-max-distance` km (default 1000)
   from the region's center.
3. From the nearest candidates, it keeps the `-k` servers (default 3) with the lowest ping from you.
4. It measures ping, jitter, download and upload on each one, then reports the **best** and
   **median** download for the region. Testing several servers protects against one overloaded
   volunteer server making a whole region look slow.

## Options

`multispeed -h` shows the same explanations.

| Flag | Default | Meaning |
|---|---|---|
| `-regions` | local + 9 hubs | `local`, preset names, or `Name=lat,lon`, comma separated |
| `-pick` | off | choose the regions from a numbered list |
| `-list-regions` | | show the preset region names and exit |
| `-k` | 3 | servers tested per region; the table shows the best and the median |
| `-parallel` | 1 | servers tested at the same time (faster, but they share your bandwidth) |
| `-duration` | 10s | length of each download / upload measurement |
| `-max-distance` | 1000 | max km between a server and its region's center |
| `-no-upload` | off | skip upload (about twice as fast) |
| `-json` | off | full results as JSON on stdout |
| `-csv file` | | also save per-server results (handy for tracking over time) |
| `-no-color` | off | plain text without colors |

## Reading the results

- **One by one vs parallel:** parallel tests share your bandwidth, so each one shows a lower speed.
  Use one by one (the default) for honest per-region numbers, and parallel to compare ping quickly
  or to see your total international capacity.
- **Far = slower is partly normal:** with high ping, a single connection can't fill a fast line.
  multispeed uses several connections at once, but expect some drop with distance even on a perfect
  network. A big drop for one region compared with others at similar distance is the interesting signal.
- **Each full run uses data:** on a fast line, 30 tests can use several GB. Use `-k 1`,
  `-no-upload` or fewer regions on metered connections.

## Building from source

Requires Go 1.21 or newer.

```sh
go install github.com/DarkCowMoo/multispeedtest@latest   # installs as "multispeedtest"
```

Or from a clone:

```sh
go build -o multispeed .
go test ./...
```

Cross-compile, for example for Windows: `GOOS=windows GOARCH=amd64 go build -o multispeed.exe .`

## Development

| File | Contents |
|---|---|
| `main.go` | flags, `-h` help, the top-level `run` flow, the re-run command |
| `regions.go` | preset regions and `-regions` parsing |
| `picker.go` | the interactive region list shown with `-pick` |
| `progress.go` | log lines and the live status line on stderr |
| `discover.go` | finding and choosing servers for each region (with backups) |
| `runner.go` | running the tests, retrying failed servers with backups, data used |
| `resultlog.go` | the per-server lines printed while testing |
| `report.go` | result types, per-region summary, results table / JSON / CSV |
| `ui.go` | colors, opening screen, section titles, bars |
| `vt_windows.go`, `vt_other.go` | ANSI colors in Windows consoles; keeping a double-clicked window open |

Each file has a matching `_test.go`. Before sending changes, run:

```sh
gofmt -l .                 # must print nothing
golangci-lint run ./...    # config in .golangci.yml (golangci-lint v2)
go test ./...              # add -short to skip the slower fake-server tests
```

CI (`.github/workflows/ci.yml`) runs the linter, plus tests with `-race` on Linux, Windows and macOS.
Line endings are LF everywhere (`.gitattributes`), so gofmt gives the same result on Windows.

## Credits

Speed measurement uses [showwin/speedtest-go](https://github.com/showwin/speedtest-go) (MIT).
Test servers are provided by Speedtest.net (Ookla) and its volunteer hosts. This project is not
affiliated with Ookla; please use it at a reasonable rate.

## License

MIT

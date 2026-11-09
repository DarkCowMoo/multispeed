# multispeed

Hi there,
I was always frustrated with SpeedTest's inability to multi test different regions so I can actually see how my internet provider is holding against multiple regions.
This simple go based application should be a fairly quick glimpse of how you are doing.
Goodluck!


[![CI](https://github.com/DarkCowMoo/multispeed/actions/workflows/ci.yml/badge.svg)](https://github.com/DarkCowMoo/multispeed/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/DarkCowMoo/multispeed)](https://github.com/DarkCowMoo/multispeed/releases/latest)

**Test your internet speed to many places at once, not just the nearest server.**

Normal speed tests measure the connection to the closest server, so they say little about how fast
you can reach Europe, the US or Asia. multispeed picks good public Speedtest.net servers in each
region automatically, tests them, and tells you in one screen:

- **speed and ping** to every region, compared with your best one
- **bufferbloat**: how much your ping rises while the line is busy, graded A+ to F
- **what each region is good for**: 4K video, video calls, gaming, big downloads
- **the change since your last run**, with trends over time
- a **world map** of the results, and optionally the **network route** to each region

```
━━ Results ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Region             Ping    Jitter    Download                 Upload  Bloat  vs last  Best server
  Local (nearest)    6 ms    1.4 ms  750.8 Mbps  ██████████  89.9 Mbps  B         ▲17%  Local ISP (Your City)
  Frankfurt         56 ms    0.6 ms  619.8 Mbps  ████████░░  86.6 Mbps  B            ≈  Melbicom (Frankfurt)
  New York         130 ms    0.9 ms   97.4 Mbps  █░░░░░░░░░  90.3 Mbps  B         ▼56%  Surfshark Ltd (New York, NY)
  Tokyo            349 ms  241.6 ms  205.0 Mbps  ███░░░░░░░  71.3 Mbps  A         ▲46%  Contabo (Tokyo)
  Sydney           299 ms    0.7 ms  159.3 Mbps  ██░░░░░░░░  59.7 Mbps  A+        ▲48%  GSL Networks (Sydney)

━━ What it's good for ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Region           4K video  Video calls         Gaming        1 GB download  1 GB upload
  Local (nearest)  ✓         ✓                   ~ 62 ms lag   ✓ 11s          ~ 89s
  Frankfurt        ✓         ✓                   ~ 114 ms lag  ✓ 13s          ~ 2m
  New York         ✓         ✓                   ✗ 187 ms lag  ~ 82s          ~ 89s
  Tokyo            ✓         ✗ noticeable delay  ✗ 354 ms lag  ✓ 39s          ~ 2m
```
*(real output, shortened, with the local server renamed; in a terminal everything is colored
green, yellow or red)*

## Quick start

**Windows:** download `multispeed_windows_amd64.exe` from the
[latest release](https://github.com/DarkCowMoo/multispeed/releases/latest) and double-click it.
It tests your connection plus 9 world hubs (about 5 minutes), and the window stays open at the end
until you press Enter.

**macOS / Linux:** download the archive for your system from the same page, unpack it and run
`./multispeed` in a terminal. On macOS, if the file is blocked the first time, run
`xattr -d com.apple.quarantine multispeed`.

**With Go 1.26+:** `go install github.com/DarkCowMoo/multispeed@latest` (installs as `multispeed`).

```sh
multispeed                       # your connection + 9 world hubs, 1 server each
multispeed -pick                 # choose regions from a numbered list of all 29
multispeed -h                    # every option, explained, with examples
multispeed -regions local,frankfurt,london,newyork
multispeed -regions "local,Lisbon=38.72,-9.14,singapore"   # any place by coordinates
multispeed -k 3                  # 3 servers per region: more reliable, 3x longer
multispeed -route                # also trace the network route to each region
multispeed -html report.html     # save a report you can share
multispeed -history              # trends from your past runs
multispeed -every 1h -csv log.csv   # test every hour until Ctrl+C
```

## What you see

- **Opening screen:** your provider, your public IP, a run ID, and the plan with an estimated time.
  (The IP is shown on screen only; it is not saved in history or reports.)
- **While testing:** a live line with the current server, what it is doing (ping, download or
  upload, with the speed so far), and the time left. Each finished server prints a line, with its
  bufferbloat grade.
- **Results:** the table above. Download bars compare each region with your best one. "vs last"
  compares with the last saved run that tested the region.
- **What it's good for:** plain-language verdicts per region. Gaming counts ping *plus* bufferbloat,
  because that is the lag you feel when someone else is downloading.
- **Map:** a world map with a colored letter per region.
- **Routes** (with `-route`): hops to each region, and the hop with the biggest jump in delay,
  with its router name. That is often an ocean crossing, or a detour through another country.
- **At the end:** data used, and the exact command to run the same test again.

Every run is saved to your history (`%AppData%\multispeed\history.jsonl` on Windows,
`~/.config/multispeed/` on Linux, `~/Library/Application Support/multispeed/` on macOS).
Use `-no-save` to skip that, or set `MULTISPEED_HISTORY` to keep the history elsewhere.

## How it works

1. For each region, it asks Speedtest.net for servers near that region's coordinates.
2. It drops servers that don't answer, or that are more than `-max-distance` km (default 1000)
   from the region's center.
3. From the nearest candidates, it keeps the `-k` servers (default 1) with the lowest ping from
   you, plus 2 backups in case a server answers but then sends no data.
4. On each server it measures ping and jitter, then download and upload. While the download and
   upload run, it keeps pinging the same server (a TCP connect every 200 ms) and compares that
   with the idle ping: the rise is the **bufferbloat**. Grades: A+ under 5 ms, A under 30, B under
   60, C under 200, D under 400, F above.
5. It reports the best server per region (and the median, with `-k` above 1), compares with your
   history, and saves the run.

## Options

`multispeed -h` shows the same explanations.

| Flag | Default | Meaning |
|---|---|---|
| **What to test** | | |
| `-regions` | local + 9 hubs | `local`, preset names, or `Name=lat,lon`, comma separated |
| `-pick` | off | choose the regions from a numbered list |
| `-list-regions` | | show the preset region names and exit |
| **How to test** | | |
| `-k` | 1 | servers tested per region; with more, the table shows the best and the median |
| `-parallel` | 1 | servers tested at the same time (faster, but they share your bandwidth) |
| `-duration` | 10s | length of each download / upload measurement |
| `-max-distance` | 1000 | max km between a server and its region's center |
| `-no-upload` | off | skip upload (about twice as fast) |
| `-route` | off | also trace the network route to each region (adds up to a minute) |
| **Output** | | |
| `-json` | off | full results as JSON on stdout (see below) |
| `-csv file` | | also append one row per tested server to a CSV file |
| `-html file` | | also save a self-contained HTML report |
| `-no-map` | off | don't draw the world map |
| `-no-color` | off | plain text without colors (also when `NO_COLOR` is set) |
| **History and monitoring** | | |
| `-history` | | show past runs with a trend per region, and exit |
| `-no-save` | off | don't save this run to the history |
| `-every` | | repeat on a schedule, e.g. `30m` or `2h` (at least `10m`), until Ctrl+C |

## Use it from Claude (or another AI agent)

multispeed is easy for an AI assistant to run for you: with `-json`, results go to stdout as one
JSON object and all progress goes to stderr, so the agent gets clean data to explain.

**Claude Code:** this repo includes a ready-made skill. Install it once:

```sh
# macOS / Linux
git clone https://github.com/DarkCowMoo/multispeed
mkdir -p ~/.claude/skills && cp -r multispeed/skills/multispeed ~/.claude/skills/
```
```powershell
# Windows (PowerShell)
git clone https://github.com/DarkCowMoo/multispeed
New-Item -ItemType Directory -Force "$HOME\.claude\skills" | Out-Null
Copy-Item -Recurse multispeed\skills\multispeed "$HOME\.claude\skills\"
```

Then just ask Claude things like *"How fast is my internet to Europe and the US?"*,
*"Why is my game laggy on Tokyo servers?"* or *"Check my connection every hour tonight"*.
The skill tells Claude how to install multispeed if needed, which flags to use, how to read the
JSON, and how to explain the results (it also warns you about time and data use before testing).

**Any other agent or script:** run `multispeed -json` and parse stdout. The main fields:

```jsonc
{
  "tool": "multispeed", "version": "v0.1.0", "run_id": "c93f-9c9d",
  "time": "2026-11-09T16:30:00Z", "isp": "...",
  "settings": { "regions": "local,frankfurt", "servers_per_region": 1, "upload": true, ... },
  "data_used_bytes": 1200000000, "elapsed_seconds": 67,
  "regions": [{
    "region": "Frankfurt", "lat": 50.11, "lon": 8.68,
    "ping_ms": 56, "jitter_ms": 0.6,
    "best_download_mbps": 619.8, "best_upload_mbps": 86.6,
    "bufferbloat_ms": 58, "bufferbloat_grade": "B",
    "good_for": [{ "use": "Gaming", "level": "ok", "note": "114 ms lag" }, ...],
    "download_change_pct": 1.2,          // vs last saved run, when there is one
    "route": { "hops": [...], "reached": true, "biggest_jump": { "hop": 6, "added_ms": 49 } },
    "servers_ok": 1, "servers_tested": 1, "servers": [ ...per-server details... ]
  }]
}
```

Field names are stable: new versions add fields but don't rename or remove them. With `-every`,
each round prints one JSON object per line (JSON Lines).

## Reading the results

- **Far = slower is partly normal:** with high ping, a single connection can't fill a fast line.
  multispeed uses several connections at once, but expect some drop with distance even on a
  perfect network. A big drop for one region, compared with others at a similar ping, is the
  interesting signal. `-route` often shows why.
- **Bufferbloat C or worse** means calls and games lag whenever someone downloads. Many routers
  can fix this with "SQM" or "smart queue" settings.
- **One by one vs parallel:** parallel tests share your bandwidth, so each one shows a lower speed.
  Use one by one (the default) for honest per-region numbers.
- **Each run uses data:** on a fast line, the default run uses a few GB. Use `-no-upload`, a
  shorter `-duration` or fewer regions on metered connections.
- **One server can be unlucky:** if a region looks oddly slow, run it again with `-k 3`.

## Development

```
main.go                 entry point: just calls cli.Main (keeps `go install …/multispeed@latest`)
internal/
  cli/       flags and -h help, the -pick region list, the flow of a run, the re-run command
  measure/   speedtest clients, finding servers per region (with backups), running the tests,
             latency under load (bufferbloat), traceroute
  results/   per-region summaries, "good for" verdicts, history, the -json report, CSV
  ui/        colors, opening screen, live status line, tables, world map, HTML report,
             Windows console handling
  regions/   preset regions and -regions parsing
skills/multispeed/   the Claude Code skill
```

Dependencies only point one way: `cli → ui → results → measure → regions`. `measure` does no
output of its own: it reports progress through the `measure.Observer` interface, which `ui`
implements. Each file has a matching `_test.go`. Before sending changes, run:

```sh
gofmt -l .                 # must print nothing
golangci-lint run ./...    # config in .golangci.yml (golangci-lint v2)
go test ./...              # add -short to skip the slower fake-server tests
```

CI runs the linter, plus tests with `-race` on Linux, Windows and macOS. Pushing a tag such as
`v0.2.0` builds and publishes release binaries with GoReleaser. `CLAUDE.md` has the conventions
for anyone (or any Claude) working on the code.

## Credits

Speed measurement uses [showwin/speedtest-go](https://github.com/showwin/speedtest-go) (MIT).
Test servers are provided by Speedtest.net (Ookla) and its volunteer hosts. This project is not
affiliated with Ookla; please use it at a reasonable rate.

## License

MIT

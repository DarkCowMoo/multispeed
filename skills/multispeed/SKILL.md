---
name: multispeed
description: Measure the user's internet speed, ping and bufferbloat to many world regions (Europe, US, Asia...) with the multispeed CLI, and explain the results in plain words. Use when the user asks how fast their internet is, why a game, call or download to some country is slow, how their ISP compares across regions, or wants their connection tracked over time.
---

# multispeed

`multispeed` tests the internet connection against public Speedtest.net servers in several
regions and reports download, upload, ping, jitter, latency under load (bufferbloat grade A+ to F)
and what each region is good for. Source and releases: https://github.com/DarkCowMoo/multispeed

## 1. Make sure it is installed

Check with `multispeed -h` (or `multispeed.exe -h` on Windows). If it is missing, either:

- download the binary for the OS from https://github.com/DarkCowMoo/multispeed/releases/latest
  (Windows: `multispeed_windows_amd64.exe`), or
- with Go 1.26+: `go install github.com/DarkCowMoo/multispeed@latest` (installs as `multispeed`).

## 2. Run it for machine-readable results

Always use `-json`: results go to stdout as one JSON object; progress goes to stderr (ignore it).

```sh
multispeed -json                                     # local + 9 world hubs, ~5 min, a few GB
multispeed -json -no-upload -regions local           # quick check of the local line, ~20 s
multispeed -json -regions local,frankfurt,newyork    # specific regions
multispeed -json -regions "local,Office=40.71,-74.0" # any place by latitude,longitude
multispeed -json -route -regions local,tokyo         # also trace the network route
multispeed -list-regions                             # preset region names
```

Before running, tell the user how long it takes and that it uses data (each server downloads for
10 s at full speed; on a fast line the default run uses a few GB). Prefer few regions and
`-no-upload` for a quick answer. Do not start `-every` (repeating) runs without asking.

Runs are saved to the user's history by default; add `-no-save` for throwaway checks.
`multispeed -history` prints past runs with trends (human-readable text, not JSON).

## 3. Read the JSON

Top level: `run_id`, `time`, `isp`, `settings`, `data_used_bytes`, `elapsed_seconds`, `regions[]`.
Each region:

| Field | Meaning |
|---|---|
| `region` | e.g. "Local (nearest)", "Frankfurt" |
| `ping_ms`, `jitter_ms` | idle latency and its variation |
| `best_download_mbps`, `best_upload_mbps` | speed of the best server in the region |
| `bufferbloat_ms`, `bufferbloat_grade` | how much latency rises while the line is busy; A+ < 5 ms, A < 30, B < 60, C < 200, D < 400, F above |
| `good_for[]` | `{use, level: good/ok/poor, note}` for 4K video, video calls, gaming, 1 GB download/upload |
| `download_change_pct` | change since the last saved run that tested this region (absent if none) |
| `route` | with `-route`: `hops[]`, `reached`, `biggest_jump {hop, added_ms}` |
| `servers_ok`, `servers_tested`, `note` | if `servers_ok` is 0 the region has no result; `note` says why |
| `servers[]` | per-server details, including sponsor, city, host and errors |

## 4. Explain the results

- Lead with the local result (the user's own line), then how far each region falls behind it.
- Some drop with distance is normal physics (high ping limits a single connection); a large drop
  compared with other regions at a similar ping points at the ISP's route to that region.
- A `biggest_jump` of +100 ms or more at one hop usually marks an ocean crossing; a route to Asia
  that jumps through an unexpected country (look at hop names) explains high ping.
- Bufferbloat C or worse means calls and games will lag while someone downloads; suggest enabling
  SQM / "smart queue" on the router, or a router that supports it.
- One slow server can be unlucky: suggest rerunning that region with `-k 3`.

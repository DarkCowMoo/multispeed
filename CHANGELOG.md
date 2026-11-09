# Changelog

All notable changes to multispeed are listed here, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/). Changes waiting for the next release are under
**Unreleased**.

## [Unreleased]

## [0.1.0] - 2026-11-09

The first release: test your internet speed to many regions of the world at once.

### Added

- **Many regions in one run:** your nearest servers plus 9 world hubs by default, 28 presets in
  all (`-list-regions`), or any place by coordinates (`-regions "Lisbon=38.72,-9.14"`). `-pick`
  lets you choose them from a numbered list.
- **Measurements per region:** download, upload, ping and jitter, with a spare server tried
  automatically when one fails. `-k 3` tests several servers per region and reports the best and
  the median.
- **Bufferbloat grade (A+ to F):** how much ping rises while your line is busy. When a server
  ignores pings under load, that part is reported as not measured instead of being graded.
- **"What it's good for":** 4K video, video calls, gaming, and how long 1 GB takes to download
  and upload, for each region.
- **History:** every run is saved; results show the change since the last run, and `-history`
  shows trends per region. Turn it off with `-no-save`.
- **World map** of the results in the terminal.
- **Routes** (`-route`): the network path to each region and the hop that adds the most delay.
- **Shareable HTML report** (`-html report.html`).
- **Monitoring** (`-every 1h`): repeat the test on a schedule, at least 10 minutes apart.
- **Output for scripts and AI agents:** `-json` (one JSON object, or JSON Lines with `-every`)
  and `-csv`. A Claude Code skill is included in `skills/multispeed`.
- **Easy to run:** on Windows, double-click the `.exe`; the window stays open at the end.
  Colors, a live progress line, and the exact command to repeat a run.

[Unreleased]: https://github.com/DarkCowMoo/multispeed/compare/v0.1.0...dev
[0.1.0]: https://github.com/DarkCowMoo/multispeed/releases/tag/v0.1.0

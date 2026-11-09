# multispeed: notes for working on the code

Go CLI that tests internet speed to several regions using Speedtest.net servers
(library: github.com/showwin/speedtest-go). See README.md for what it does.

## Layout

`main.go` only calls `cli.Main`. Everything else is in `internal/`, with dependencies pointing
one way: `cli → ui → results → measure → regions`.

- `regions`: preset regions, `-regions` parsing.
- `measure`: speedtest clients, server discovery, running tests, bufferbloat probes, traceroute.
  No output: progress goes through the `Observer`/`Progress` interfaces.
- `results`: summaries, verdicts, history, the JSON report, CSV.
- `ui`: all terminal and HTML output; `TestProgress` implements `measure.Observer`.
- `cli`: flags, picker, the flow of a run.

## Branches

- `dev` is the default branch: all work and all pull requests go there.
- `main` holds released code. It is updated only by a pull request from `dev`, and release
  tags (`vX.Y.Z`) are made on `main`; `.github/workflows/release.yml` runs GoReleaser.
- User-facing changes get a line under **Unreleased** in `CHANGELOG.md`.
- `VERSION` holds the current version. Releasing is `scripts/release.sh X.Y.Z` (maintainer
  only: it merges with admin rights). It prepares the release on `dev` (merges `main` in, so
  the previous tag stays in `dev`'s history; sets `VERSION`; turns **Unreleased** into
  `## [X.Y.Z] - date`), merges `dev` into `main`, and follows the Release workflow, which
  tags `vX.Y.Z` itself when `main` has a `VERSION` without a tag, then publishes with that
  version's changelog section as notes. `--prepare-only` stops after the local commit.
- Merge release PRs with a merge commit, never squash (squash would drop the merge of `main`).
- Both are protected: changes need a pull request, an approving review and passing CI.

## Before finishing a change

```sh
gofmt -l .                 # must print nothing
golangci-lint run ./...    # config in .golangci.yml (v2); must report 0 issues
go test ./...              # -short skips the slower fake-server tests
```

Also check `GOOS=linux` and `GOOS=darwin` builds when touching `internal/ui/term_*.go` or
`internal/measure/route.go`.

## Conventions

- One concern per file, each with a matching `_test.go`. Tests must not touch the network
  except loopback `httptest` servers; set `MULTISPEED_HISTORY` to a temp file in tests that save.
- Progress and messages go to stderr through `ui.Logf` (never `fmt.Print` or the `log` package);
  results go to stdout. `-json` output must stay valid JSON on stdout.
- Create speedtest clients only with `newClient` (`internal/measure/client.go`), never
  `speedtest.New` directly: the library rewires the global `http.DefaultClient` and races
  under `-race` otherwise.
- Colors only through `ui.Palette` (`ui.ErrColors` for stderr, `ui.OutColors` for stdout). Pad
  plain text first, then color it, so alignment never counts escape codes.
- The JSON field names in `internal/results` and `internal/measure` are a public interface (used
  by scripts and the Claude skill in `skills/multispeed/SKILL.md`): add fields, don't rename or
  remove them.

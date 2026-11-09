# multispeed: notes for working on the code

Go CLI (package `main`) that tests internet speed to several regions using Speedtest.net servers
(library: github.com/showwin/speedtest-go). See README.md for what it does and the file map.

## Before finishing a change

```sh
gofmt -l .                 # must print nothing
golangci-lint run ./...    # config in .golangci.yml (v2); must report 0 issues
go test ./...              # -short skips the slower fake-server tests
```

Also check `GOOS=linux` and `GOOS=darwin` builds when touching `vt_*.go` or `route.go`.

## Conventions

- One concern per file, each with a matching `_test.go`. Tests must not touch the network
  except loopback `httptest` servers; set `MULTISPEED_HISTORY` to a temp file in tests that save.
- Progress and messages go to stderr through `logf` (never `fmt.Print` or the `log` package);
  results go to stdout. `-json` output must stay valid JSON on stdout.
- Colors only through `palette` (`errColors` for stderr, `outColors` for stdout). Pad plain text
  first, then color it, so alignment never counts escape codes.
- The JSON field names in `report.go` are a public interface (used by scripts and the Claude
  skill in `skills/multispeed/SKILL.md`): add fields, don't rename or remove them.
- Releases: push a tag `vX.Y.Z`; `.github/workflows/release.yml` runs GoReleaser.

# Contributing

Thanks for helping! A few things to know first.

## Branches

- **`dev`** is where development happens. Open your pull request against `dev` (it is the
  default branch, so GitHub picks it for you).
- **`main`** holds released code. It only changes when `dev` is merged into it for a release.

Both branches are protected: changes go through a pull request, need an approving review from
the maintainer, and CI must pass (lint, and tests on Linux, Windows and macOS).

## Making a change

1. Fork the repository and create a branch from `dev`.
2. Make your change, with tests. Keep to the layout described in `CLAUDE.md` and the README's
   Development section.
3. Before pushing, run:

   ```sh
   gofmt -l .                 # must print nothing
   golangci-lint run ./...    # must report 0 issues
   go test ./...
   ```

4. Open a pull request against `dev` and describe what changes for people using multispeed.

## Please don't

- Rename or remove fields in the `-json` output: scripts and AI agents rely on them. Adding
  fields is fine.
- Make tests that reach the internet. Use loopback `httptest` servers, as the existing tests do.
- Make the tool hit Speedtest.net servers harder (shorter `-every` intervals, more parallel
  load by default): they are mostly run by volunteers.

Security problems: please report them privately, see [SECURITY.md](SECURITY.md).

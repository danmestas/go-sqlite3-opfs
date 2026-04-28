# Contributing to go-sqlite3-opfs

Thanks for your interest in `go-sqlite3-opfs`, an OPFS-backed VFS for
`ncruces/go-sqlite3` enabling SQLite persistence in browser WebAssembly
contexts. This document covers everything you need to get a working checkout,
run the tests, and submit changes.

## Development setup

Requires Go 1.26 or newer.

```
git clone https://github.com/danmestas/go-sqlite3-opfs
cd go-sqlite3-opfs
go test ./...
```

There is no Makefile — the standard Go toolchain is enough. The repo is a
single package at the root.

## Running tests

```
go test ./...                # full suite (unit + integration + fault + torture)
go test -race ./...          # race detector
go test -run Torture ./...   # torture tests only
go test -run Integration ./...   # integration tests only
```

The `testharness/` package provides shared fixtures used across the test
files. The same suite runs in CI on every push and PR.

## Code layout

- `opfs.go`, `api.go`, `errors.go`, `handle.go` — core VFS implementation.
- `handle_js.go` — browser/JS-specific handle code (built only under
  `GOOS=js`).
- `observer.go`, `stats.go` — instrumentation hooks and metrics.
- `testharness/` — shared test fixtures.
- `playground/` — interactive examples and exploration code.
- `*_test.go` at the root — unit, integration, fault-injection, and torture
  tests.

## Submitting changes

1. Open a feature branch off `main`. Direct commits to `main` are not accepted.
2. Run `go test ./...` and `go test -race ./...` locally before pushing.
3. Open a PR; CI will re-run the full test suite.

## Reporting issues

Open an issue at https://github.com/danmestas/go-sqlite3-opfs/issues with
reproduction steps, the browser and Go versions involved, expected vs.
actual behavior, and a minimal repro if possible.

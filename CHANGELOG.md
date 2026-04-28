# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Relicensed from MIT to Apache-2.0 to add an explicit patent grant.

## [0.2.0] - 2026-03-21

### Changed

- Bumped `ncruces/go-sqlite3` to v0.33.0.

## [0.1.0] - 2026-03-21

Initial release of `go-sqlite3-opfs`, an OPFS-backed VFS for `ncruces/go-sqlite3`
that enables SQLite persistence in browser WebAssembly contexts via the
Origin Private File System API.

### Added

- OPFS VFS implementation registered against `ncruces/go-sqlite3`.
- File handle and stats abstractions for browser persistence.
- Observer hooks for tracking VFS operations.
- Test harness covering unit, integration, fault-injection, and torture cases.

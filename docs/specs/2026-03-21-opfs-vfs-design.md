# OPFS VFS for ncruces/go-sqlite3

**Date:** 2026-03-21
**Status:** Approved
**Repo:** github.com/danmestas/go-sqlite3-opfs

## Problem

ncruces/go-sqlite3 has no browser persistence. The VFS uses OS file I/O which doesn't exist in `GOOS=js`. Browser WASM applications using ncruces have no way to persist SQLite databases across page reloads.

## Solution

A standalone VFS package that implements `vfs.VFS` and `vfs.File` using the Origin Private File System (OPFS) `FileSystemSyncAccessHandle` API. Registers with ncruces via `vfs.Register()`. Follows ncruces conventions idiomatically.

## Spike

Proven in EdgeSync `spike/opfs-vfs` branch. Full stack verified: Go WASM → ncruces → wazero → sqlite3.wasm → custom OPFS VFS → FileSystemSyncAccessHandle. Data persists across page reloads.

## Architecture

### JS/Go Boundary

OPFS handle creation is async (Promises). Go WASM cannot await Promises inside wazero's call stack. Solution: **JS does all async work before Go needs handles.**

```
Worker starts
  → JS: navigator.storage.getDirectory()
  → JS: for each pool slot: getFileHandle → createSyncAccessHandle
  → JS: pass all handles to Go via _opfs_pool_init()
  → Go: VFS ready, all subsequent I/O is synchronous
```

### Handle Pool (opfs-sahpool pattern)

Following sqlite.org's battle-tested opfs-sahpool design:

- Pre-allocate N sync access handles at Worker startup
- Map virtual filenames (e.g. "myrepo.fossil") to pool slots
- Slot 0 is the metadata slot — stores name→slot JSON mapping
- On page reload: JS reopens same OPFS files, Go reads metadata from slot 0 to rebuild name map

```go
type Pool struct {
    slots    []Handle    // pre-allocated sync access handles
    names    []string    // virtual filename per slot ("" = free)
    meta     int         // metadata slot index (0)
    stats    Stats       // atomic counters
    observer Observer    // nil = no-op
}
```

**Default pool size:** 6 (2 databases + journals, matching sqlite.org default).

**Capacity management:** `AddCapacity(n)` and `ReduceCapacity(n)` adjust the pool at runtime. Requires JS cooperation to open/close OPFS handles.

### Handle Interface

Abstracts `FileSystemSyncAccessHandle` for clean VFS code:

```go
type Handle interface {
    Read(buf []byte, offset int64) (int, error)
    Write(buf []byte, offset int64) (int, error)
    GetSize() (int64, error)
    Truncate(size int64) error
    Flush() error
    Close() error
}
```

`//go:build js` implementation wraps `syscall/js` — each method is a single synchronous `js.Value.Call`. No promises, no channels, no async bridging.

### VFS Implementation

```go
type opfsVFS struct {
    pool *Pool
}
```

**VFS.Open(name, flags):**
- Database files (MAIN_DB, TEMP_DB) → `pool.Acquire(name)`
- Journal files (MAIN_JOURNAL) → `pool.Acquire(name)` (separate slot)
- Temp journals (TEMP_JOURNAL, DELETEONCLOSE) → `vfsutil.SliceFile{}` with `OPEN_MEMORY`
- Everything else → `OPEN_MEMORY` flag

**VFS.Delete(name, syncDir):**
- `pool.Release(name)` — clears metadata, truncates OPFS file to zero. Handle stays in pool.

**VFS.Access(name, flag):**
- `pool.Has(name)` — checks if virtual name is mapped to a slot.

**VFS.FullPathname(name):**
- Returns name unchanged (virtual names).

### File Implementation

```go
type opfsFile struct {
    handle Handle
    name   string
    pool   *Pool
    lock   vfs.LockLevel
}
```

| vfs.File method | OPFS call | Notes |
|----------------|-----------|-------|
| `ReadAt(b, off)` | `handle.Read(b, off)` | `io.EOF` if n == 0 |
| `WriteAt(b, off)` | `handle.Write(b, off)` | `io.ErrShortWrite` if n < len(b) |
| `Size()` | `handle.GetSize()` | |
| `Truncate(size)` | `handle.Truncate(size)` | |
| `Sync(flags)` | `handle.Flush()` | |
| `Close()` | No-op — handle stays in pool | Decrements ref count |
| `Lock/Unlock` | No-op state machine | Single connection per Worker |
| `SectorSize` | 4096 | |
| `DeviceCharacteristics` | `IOCAP_ATOMIC \| IOCAP_SEQUENTIAL \| IOCAP_SAFE_APPEND` | |

### Metadata Persistence

Slot 0 stores a JSON blob mapping virtual names to slot indices:

```json
{"version":1,"slots":{"myrepo.fossil":1,"myrepo.fossil-journal":2}}
```

Written on every name assignment/release (truncate + write + flush). Small, infrequent, atomic. On corrupt read, all slots treated as empty (files still exist, names lost — recovery function can scan non-empty slots).

## Public API

```go
// Auto-register with defaults (blank import):
//   import _ "github.com/danmestas/go-sqlite3-opfs"
func init() {
    Register()
}

// Register with default options.
func Register(opts ...Options)

// New creates a VFS with custom configuration.
func New(opts Options) vfs.VFS

type Options struct {
    PoolSize int    // default 6
    Prefix   string // OPFS directory prefix, default "sqlite3-opfs"
    Name     string // VFS name, default "opfs"
    Observer Observer // nil = no-op
}

// Runtime pool management.
func AddCapacity(n int) error
func ReduceCapacity(n int) error
```

**Consumer usage:**

```go
import (
    "database/sql"
    _ "github.com/danmestas/go-sqlite3-opfs"
    _ "github.com/ncruces/go-sqlite3/driver"
    _ "github.com/ncruces/go-sqlite3/embed"
)

db, err := sql.Open("sqlite3", "file:myrepo.fossil?vfs=opfs&nolock=1")
```

**JS Worker setup:**

```js
importScripts("wasm_exec.js");

async function init(poolSize) {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle("sqlite3-opfs", { create: true });
    const handles = [];
    for (let i = 0; i < poolSize; i++) {
        const fh = await dir.getFileHandle(`slot-${i}.db`, { create: true });
        handles.push(await fh.createSyncAccessHandle());
    }
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject);
    go.run(result.instance);
    _opfs_pool_init(handles);
}
```

## Observability

### Structured Errors

Every Handle method wraps JS calls with context:

```go
type OpfsError struct {
    Op     string // read, write, truncate, flush, getSize, close
    File   string // slot filename
    Name   string // virtual name
    Offset int64
    Size   int
    Err    error  // underlying JS error
}
```

JS exceptions recovered and converted to Go errors (not panics). Errors also logged to `console.error` for browser DevTools visibility.

### Performance Counters

Always-on atomic counters, zero allocation:

```go
type Stats struct {
    Reads, Writes, Flushes       int64
    BytesRead, BytesWritten      int64
    ReadTimeNs, WriteTimeNs      int64
    FlushTimeNs                  int64
    SlotHits, SlotAllocs, SlotFull int64
}

func (p *Pool) Stats() Stats
func (p *Pool) ResetStats()
```

Exposed to JS via `_opfs_stats()` for console inspection.

### Observer Interface

Optional detailed tracing — no OTel dependency in the package:

```go
type Observer interface {
    OnRead(file string, offset int64, bytes int, duration time.Duration, err error)
    OnWrite(file string, offset int64, bytes int, duration time.Duration, err error)
    OnFlush(file string, duration time.Duration, err error)
    OnError(err *OpfsError)
    OnPoolEvent(event PoolEvent)
}
```

Consumers wire their own implementation (OTel, Honeycomb, etc.). EdgeSync will provide an OTel observer that emits spans as children of sync round spans.

## Testing

**All tests run in a real browser.** The VFS targets browser Workers — that's where it must be tested.

### Execution Architecture

```
go test ./...
  → e2e_test.go (native, orchestrator):
    1. GOOS=js GOARCH=wasm go test -c -tags ncruces -o testbin.wasm
    2. Start HTTP server with COOP/COEP headers
    3. Launch headless Chrome via chromedp
    4. Browser Worker: init OPFS pool → load testbin.wasm → run tests
    5. Results posted via postMessage → orchestrator collects → go test output
```

### Test Tiers

| Tier | File | Coverage |
|------|------|----------|
| VFS methods | `opfs_test.go` | Open, Delete, Access, FullPathname. Pool slot assignment/release. |
| File I/O | `file_test.go` | ReadAt/WriteAt at various offsets. Size. Truncate. Sync. EOF. Short writes. |
| Pool | `pool_test.go` | Capacity limits (CANTOPEN when full). AddCapacity/ReduceCapacity. Slot reuse. Metadata persistence across simulated reload. |
| Integration | `integration_test.go` | database/sql CRUD. Close → reopen → data persists. Multiple databases simultaneously. |
| Torture | `torture_test.go` | 10MB database. Rapid open/close. Large blobs. Journal rollback. |
| Fault injection | `fault_test.go` | FaultHandle wrapping real Handle. Short reads, write failures, flush errors. BuggifyChecker with deterministic seeds. |
| Observer | `observer_test.go` | RecordingObserver captures all events. Verify correct parameters on every operation. |

### CI

GitHub Actions with `chromedp` headless Chrome. Test binary is WASM, Chrome is the runner. No special infrastructure.

## Package Structure

```
github.com/danmestas/go-sqlite3-opfs/
├── api.go               # Public API: init(), Register(), New(), Options
├── opfs.go              # VFS + File implementation
├── pool.go              # Handle pool management, metadata persistence
├── handle.go            # Handle interface + jsHandle (//go:build js)
├── errors.go            # OpfsError, JS exception recovery
├── stats.go             # Stats counters, _opfs_stats() JS export
├── observer.go          # Observer interface, RecordingObserver
├── opfs_test.go         # VFS method tests
├── file_test.go         # File I/O tests
├── pool_test.go         # Pool tests
├── integration_test.go  # database/sql integration
├── torture_test.go      # Stress tests
├── fault_test.go        # Fault injection
├── observer_test.go     # Observer tests
├── testharness/         # Browser test infrastructure
│   ├── harness.go       # //go:build !js — orchestrator (build, serve, chromedp)
│   ├── runner.go        # //go:build js — test runner inside Worker
│   ├── serve.go         # //go:build !js — HTTP server with COOP/COEP
│   ├── worker.js        # Web Worker: OPFS init → WASM → tests
│   └── index.html       # Loads worker
├── e2e_test.go          # //go:build !js — top-level orchestrator test
├── _reference/          # gitignored — ncruces VFS source
├── go.mod
└── README.md
```

## Constraints

- `//go:build js` on all OPFS code — package does not compile on non-js targets
- Worker-only — main thread cannot use OPFS sync handles
- COOP/COEP headers required for `SharedArrayBuffer` (needed by wazero)
- One exclusive handle per OPFS file — no concurrent access to same DB across tabs
- Browser support: Chrome 102+, Firefox 111+, Safari 16.4+
- DELETE journal mode only (WAL requires shared memory, not available in OPFS)

## What We're NOT Doing

- No WAL mode support (OPFS has no shared memory)
- No multi-tab concurrency (OPFS exclusive locks)
- No OTel dependency in the package (Observer interface only)
- No non-js build target (this package is browser-only)
- No WASI support (WASI has real filesystem, doesn't need OPFS)
- No service worker integration
- No IndexedDB fallback

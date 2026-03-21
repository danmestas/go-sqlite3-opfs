# go-sqlite3-opfs

OPFS VFS for [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3) — SQLite persistence in browser WASM via the Origin Private File System.

## Installation

```bash
go get github.com/danmestas/go-sqlite3-opfs
```

## Usage

### Go

```go
import (
    "database/sql"

    _ "github.com/danmestas/go-sqlite3-opfs"
    _ "github.com/ncruces/go-sqlite3/driver"
    _ "github.com/ncruces/go-sqlite3/embed"
)

db, err := sql.Open("sqlite3", "file:mydb.db?vfs=opfs")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
db.SetMaxOpenConns(1) // Required: one connection per Worker
```

### JavaScript Worker

```javascript
const root = await navigator.storage.getDirectory();
const dir = await root.getDirectoryHandle("sqlite3-opfs", { create: true });

const handles = {};
for (const suffix of ["", "-journal", "-wal"]) {
    const name = "mydb.db" + suffix;
    const fh = await dir.getFileHandle(name, { create: true });
    handles[name] = await fh.createSyncAccessHandle();
}

importScripts("wasm_exec.js");
const go = new Go();
const result = await WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject);
go.run(result.instance);
_opfs_init(handles);
```

## WAL Mode

```go
db.SetMaxOpenConns(1)
db.Exec("PRAGMA locking_mode=EXCLUSIVE")
db.Exec("PRAGMA journal_mode=WAL")
```

WAL requires exclusive locking mode because OPFS does not support shared memory for multi-connection concurrency.

## Playground

Interactive SQL playground with FTS5, JSON, triggers, views, and more:

```bash
go run ./playground
```

## Options

```go
import opfsvfs "github.com/danmestas/go-sqlite3-opfs"

opfsvfs.New(opfsvfs.Options{
    Name:     "opfs",       // VFS name (default: "opfs")
    Observer: myObserver,   // Optional I/O observer
})
```

## Validated Features

All features tested in-browser and verified against native SQLite:

FTS5, foreign keys (CASCADE, SET NULL), triggers, JSON functions, generated columns, views, recursive CTEs, window functions, BLOB storage, indexes, transactions, WAL mode.

## Limitations

- **One connection per Worker** — `db.SetMaxOpenConns(1)`
- **One database per VFS** — each VFS instance manages a single database
- **Worker-only** — `FileSystemSyncAccessHandle` unavailable on main thread
- **COOP/COEP headers required** — `Cross-Origin-Opener-Policy: same-origin`, `Cross-Origin-Embedder-Policy: require-corp`
- **No shared memory** — WAL requires `PRAGMA locking_mode=EXCLUSIVE`

## Browser Compatibility

Chrome/Edge 102+, Firefox 111+, Safari 16.4+

## License

MIT

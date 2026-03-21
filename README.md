# go-sqlite3-opfs

OPFS VFS for [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3) — SQLite persistence in browser WASM via the Origin Private File System.

## Installation

```bash
go get github.com/danmestas/go-sqlite3-opfs
```

## Usage

### Go Application

```go
package main

import (
    "database/sql"
    "log"

    _ "github.com/danmestas/go-sqlite3-opfs"
    _ "github.com/ncruces/go-sqlite3/driver"
    _ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
    db, err := sql.Open("sqlite3", "file:mydb.db?vfs=opfs")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Required: one connection per Worker
    db.SetMaxOpenConns(1)

    // Create table and insert data
    _, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)`)
    if err != nil {
        log.Fatal(err)
    }
}
```

### JavaScript Worker Setup

Initialize OPFS file handles before starting the Go WASM application:

```javascript
// worker.js
const root = await navigator.storage.getDirectory();
const dir = await root.getDirectoryHandle("sqlite3-opfs", { create: true });

// Create sync access handles for main db, journal, and WAL files
const handles = {};
for (const suffix of ["", "-journal", "-wal"]) {
    const name = "mydb.db" + suffix;
    const fh = await dir.getFileHandle(name, { create: true });
    handles[name] = await fh.createSyncAccessHandle();
}

// Load and run Go WASM
importScripts("wasm_exec.js");
const go = new Go();
const result = await WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject);
go.run(result.instance);

// Pass handles to Go VFS
_opfs_init(handles);
```

## WAL Mode

The OPFS VFS supports WAL mode with exclusive locking:

```go
db.SetMaxOpenConns(1)
db.Exec("PRAGMA locking_mode=EXCLUSIVE")
db.Exec("PRAGMA journal_mode=WAL")
```

**Note:** WAL mode requires `locking_mode=EXCLUSIVE` and `SetMaxOpenConns(1)` because OPFS VFS does not support shared memory for multi-connection WAL concurrency.

## Options

Customize VFS behavior with `RegisterVFS`:

```go
import "github.com/danmestas/go-sqlite3-opfs"

opfsvfs.RegisterVFS("opfs", opfsvfs.Options{
    Name: "opfs",           // VFS name to register
    Observer: myObserver,   // Optional: file operation notifications
})
```

The `Observer` interface:

```go
type Observer interface {
    OnOpen(filename string)
    OnClose(filename string)
    OnSync(filename string)
}
```

## Limitations

- **One connection per Worker**: Use `db.SetMaxOpenConns(1)`. Multiple connections require separate Workers.
- **One database per VFS registration**: Each VFS instance manages a single database (main file + journal/WAL).
- **OPFS Worker-only**: `FileSystemSyncAccessHandle` is unavailable on the main thread.
- **COOP/COEP headers required**: Serve with `Cross-Origin-Opener-Policy: same-origin` and `Cross-Origin-Embedder-Policy: require-corp`.
- **No shared memory**: WAL mode requires `PRAGMA locking_mode=EXCLUSIVE` for single-connection access.

## Browser Compatibility

Requires support for OPFS `FileSystemSyncAccessHandle`:

- Chrome/Edge 102+
- Firefox 111+
- Safari 16.4+

Check [caniuse.com](https://caniuse.com/native-filesystem-api) for current compatibility.

## License

MIT

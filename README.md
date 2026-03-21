# go-sqlite3-opfs

OPFS VFS for [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3) enabling SQLite persistence in browser WASM via the Origin Private File System API.

## What it does

This package provides a custom VFS (Virtual File System) implementation that allows SQLite databases to persist data in the browser using OPFS (Origin Private File System). When compiled to WebAssembly, Go applications can now store SQLite databases with true file system semantics in the browser, with synchronous access via `FileSystemSyncAccessHandle`.

## Installation

```bash
go get github.com/danmestas/go-sqlite3-opfs
```

## Usage

### JavaScript Worker Setup

OPFS requires running in a Web Worker context. Set up your worker with OPFS pool initialization:

```javascript
// worker.js
importScripts('wasm_exec.js');

const go = new Go();

WebAssembly.instantiateStreaming(fetch('app.wasm'), go.importObject)
  .then(result => {
    // Initialize OPFS pool before starting Go
    return initOPFSPool(8).then(() => result);
  })
  .then(result => {
    go.run(result.instance);
  });

// Initialize OPFS pool with specified size
async function initOPFSPool(poolSize) {
  const root = await navigator.storage.getDirectory();
  const metaHandle = await root.getFileHandle('opfs-meta.db', { create: true });
  const metaSync = await metaHandle.createSyncAccessHandle();

  // Write pool size to metadata file (slot 0)
  const metaBuf = new Uint8Array(8);
  new DataView(metaBuf.buffer).setUint32(0, poolSize, true);
  metaSync.write(metaBuf, { at: 0 });
  metaSync.close();

  console.log(`OPFS pool initialized with ${poolSize} slots`);
}
```

### Go Application

Import the required packages and open a database with `vfs=opfs`:

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
    // Open database with OPFS VFS
    db, err := sql.Open("sqlite3", "file:mydb.db?vfs=opfs&nolock=1")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Use database as normal
    _, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)`)
    if err != nil {
        log.Fatal(err)
    }

    _, err = db.Exec(`INSERT INTO users (name) VALUES (?)`, "Alice")
    if err != nil {
        log.Fatal(err)
    }
}
```

## Options

The OPFS VFS supports the following options via `RegisterVFS`:

```go
import "github.com/danmestas/go-sqlite3-opfs"

opfsvfs.RegisterVFS("opfs", opfsvfs.Options{
    Name: "opfs",  // VFS name to register
    Observer: myObserver,  // Optional: receive file operation notifications
})
```

### Observer Interface

Implement the `Observer` interface to receive notifications about file operations:

```go
type Observer interface {
    OnOpen(filename string, slot int)
    OnClose(filename string, slot int)
    OnSync(filename string, slot int)
}
```

## Limitations

- **Single connection per Worker**: Each OPFS VFS instance supports one database connection. Multiple connections require separate Workers.
- **Pool size fixed at initialization**: The OPFS pool size must be set during JavaScript initialization and cannot be changed at runtime.
- **Slot 0 reserved for metadata**: File slots 1 through N-1 are available for database files (where N is the pool size).
- **No locking support**: Use `nolock=1` in the DSN as OPFS VFS does not implement SQLite's locking protocol.
- **Worker context required**: OPFS SyncAccessHandle API is only available in Web Workers, not the main thread.

## Browser Compatibility

Requires browsers with support for:
- Origin Private File System (OPFS)
- FileSystemSyncAccessHandle API (available in Web Workers)

Supported browsers:
- Chrome/Edge 102+
- Firefox 111+
- Safari 15.2+

Check [caniuse.com](https://caniuse.com/native-filesystem-api) for current compatibility.

## Architecture

The OPFS VFS uses a slot-based architecture:
- Slot 0: Metadata file (`opfs-meta.db`) storing pool configuration
- Slots 1-N: Database file slots, mapped by hash of filename
- Each slot corresponds to an OPFS file handle with synchronous access

## License

MIT

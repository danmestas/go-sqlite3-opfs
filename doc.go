// Package opfsvfs implements an OPFS VFS for [github.com/ncruces/go-sqlite3],
// enabling SQLite persistence in browser WebAssembly applications via the
// Origin Private File System.
//
// # Usage
//
// Blank-import to register the "opfs" VFS, then open databases with the vfs=opfs
// query parameter:
//
//	import (
//	    "database/sql"
//	    _ "github.com/danmestas/go-sqlite3-opfs"
//	    _ "github.com/ncruces/go-sqlite3/driver"
//	    _ "github.com/ncruces/go-sqlite3/embed"
//	)
//
//	db, err := sql.Open("sqlite3", "file:mydb.db?vfs=opfs")
//	db.SetMaxOpenConns(1)
//
// # JavaScript Worker Setup
//
// OPFS sync access handles must be pre-opened in a dedicated Worker before
// the Go WASM binary starts. The Worker creates named file handles and passes
// them to Go via the _opfs_init callback:
//
//	const handles = {};
//	for (const suffix of ["", "-journal", "-wal"]) {
//	    const fh = await dir.getFileHandle("mydb.db" + suffix, { create: true });
//	    handles["mydb.db" + suffix] = await fh.createSyncAccessHandle();
//	}
//	_opfs_init(handles);
//
// # WAL Mode
//
// WAL mode is supported with exclusive locking (required because OPFS does not
// provide shared memory):
//
//	db.SetMaxOpenConns(1)
//	db.Exec("PRAGMA locking_mode=EXCLUSIVE")
//	db.Exec("PRAGMA journal_mode=WAL")
//
// # Requirements
//
//   - GOOS=js GOARCH=wasm build target
//   - Dedicated Worker (FileSystemSyncAccessHandle is Worker-only)
//   - COOP/COEP headers (required by wazero's SharedArrayBuffer usage)
//   - Chrome 102+, Firefox 111+, Safari 16.4+
package opfsvfs

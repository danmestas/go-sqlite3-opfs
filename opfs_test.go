//go:build js

package opfsvfs

import (
	"testing"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
)

// truncateAll resets all OPFS files to zero for a clean test state.
func truncateAll(t *testing.T) {
	t.Helper()
	for name, h := range globalVFS.handles {
		if err := h.Truncate(0); err != nil {
			t.Fatalf("Truncate %s: %v", name, err)
		}
	}
}

func TestVFSOpenMainDB(t *testing.T) {
	truncateAll(t)
	f, flags, err := globalVFS.Open("test.db", vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open main DB: %v", err)
	}
	if f == nil {
		t.Fatal("Open main DB: returned nil file")
	}
	// Should NOT have OPEN_MEMORY (files are in OPFS)
	if flags&vfs.OPEN_MEMORY != 0 {
		t.Error("Open main DB: unexpected OPEN_MEMORY flag")
	}
	f.Close()
}

func TestVFSOpenJournal(t *testing.T) {
	truncateAll(t)
	f, _, err := globalVFS.Open("test.db-journal", vfs.OPEN_MAIN_JOURNAL|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open journal: %v", err)
	}
	if f == nil {
		t.Fatal("Open journal: returned nil file")
	}
	f.Close()
}

func TestVFSOpenWAL(t *testing.T) {
	truncateAll(t)
	f, _, err := globalVFS.Open("test.db-wal", vfs.OPEN_WAL|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open WAL: %v", err)
	}
	if f == nil {
		t.Fatal("Open WAL: returned nil file")
	}
	f.Close()
}

func TestVFSOpenTempJournal(t *testing.T) {
	f, flags, err := globalVFS.Open("", vfs.OPEN_TEMP_JOURNAL|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open temp journal: %v", err)
	}
	if f == nil {
		t.Fatal("Open temp journal: returned nil file")
	}
	if flags&vfs.OPEN_MEMORY == 0 {
		t.Error("Open temp journal: expected OPEN_MEMORY flag")
	}
	f.Close()
}

func TestVFSOpenUnknown(t *testing.T) {
	_, _, err := globalVFS.Open("nonexistent.db", vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != sqlite3.CANTOPEN {
		t.Fatalf("Open unknown: got %v, want CANTOPEN", err)
	}
}

func TestVFSDelete(t *testing.T) {
	truncateAll(t)
	h := globalVFS.handles["test.db"]
	// Write data so file "exists"
	if _, err := h.Write([]byte("data"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Delete truncates to zero
	if err := globalVFS.Delete("test.db", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	size, _ := h.GetSize()
	if size != 0 {
		t.Fatalf("After Delete: size=%d, want 0", size)
	}
}

func TestVFSAccess(t *testing.T) {
	truncateAll(t)
	// Empty file: ACCESS_EXISTS should return false
	exists, err := globalVFS.Access("test.db", vfs.ACCESS_EXISTS)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if exists {
		t.Error("Access empty file: got true, want false")
	}
	// Write data
	h := globalVFS.handles["test.db"]
	if _, err := h.Write([]byte("data"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Now should exist
	exists, err = globalVFS.Access("test.db", vfs.ACCESS_EXISTS)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if !exists {
		t.Error("Access with data: got false, want true")
	}
	// Unregistered name: always false
	exists, _ = globalVFS.Access("nope.db", vfs.ACCESS_EXISTS)
	if exists {
		t.Error("Access unregistered: got true, want false")
	}
}

func TestVFSFullPathname(t *testing.T) {
	name, err := globalVFS.FullPathname("test.db")
	if err != nil {
		t.Fatalf("FullPathname: %v", err)
	}
	if name != "test.db" {
		t.Fatalf("FullPathname: got %q, want %q", name, "test.db")
	}
}

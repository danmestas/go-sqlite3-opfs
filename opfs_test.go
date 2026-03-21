//go:build js

package opfsvfs

import (
	"testing"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
)

func TestVFSOpenDatabase(t *testing.T) {
	// Test opening a main database file
	name := "test-vfs-open-db"

	f, flags, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q, OPEN_MAIN_DB|OPEN_CREATE|OPEN_READWRITE): %v", name, err)
	}
	if f == nil {
		t.Fatalf("Open(%q): got nil file", name)
	}

	// Verify OPEN_MEMORY flag is set (indicates we handle journals in-memory)
	if flags&vfs.OPEN_MEMORY == 0 {
		t.Fatalf("Open(%q): flags=%d, missing OPEN_MEMORY flag", name, flags)
	}

	// Cleanup
	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})
}

func TestVFSOpenTempJournal(t *testing.T) {
	// Test opening a temp journal (should return SliceFile with OPEN_MEMORY)
	name := ""

	f, flags, err := globalVFS.Open(name, vfs.OPEN_TEMP_JOURNAL)
	if err != nil {
		t.Fatalf("Open(%q, OPEN_TEMP_JOURNAL): %v", name, err)
	}
	if f == nil {
		t.Fatalf("Open(%q, OPEN_TEMP_JOURNAL): got nil file", name)
	}

	// Verify OPEN_MEMORY flag is set
	if flags&vfs.OPEN_MEMORY == 0 {
		t.Fatalf("Open(%q, OPEN_TEMP_JOURNAL): flags=%d, missing OPEN_MEMORY flag", name, flags)
	}

	// Cleanup
	t.Cleanup(func() {
		f.Close()
	})
}

func TestVFSOpenNonDatabase(t *testing.T) {
	// Test opening a non-database file (should return CANTOPEN)
	name := "test-vfs-open-non-db"

	// Try to open with flags that aren't a database
	f, _, err := globalVFS.Open(name, vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != sqlite3.CANTOPEN {
		t.Fatalf("Open(%q) with non-database flags: got error %v, want CANTOPEN", name, err)
	}
	if f != nil {
		t.Fatalf("Open(%q) with non-database flags: got file %v, want nil", name, f)
	}
}

func TestVFSDelete(t *testing.T) {
	// Test deleting a file
	name := "test-vfs-delete"

	// Open to acquire a slot
	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}
	if f == nil {
		t.Fatalf("Open(%q): got nil file", name)
	}
	f.Close()

	// Verify pool has it
	if !globalPool.Has(name) {
		t.Fatalf("Has(%q): returned false after Open", name)
	}

	// Delete it
	err = globalVFS.Delete(name, false)
	if err != nil {
		t.Fatalf("Delete(%q): %v", name, err)
	}

	// Verify pool no longer has it
	if globalPool.Has(name) {
		t.Fatalf("Has(%q): returned true after Delete", name)
	}
}

func TestVFSAccess(t *testing.T) {
	// Test Access method
	name := "test-vfs-access"

	// Before acquiring, Access should return false
	exists, err := globalVFS.Access(name, vfs.ACCESS_EXISTS)
	if err != nil {
		t.Fatalf("Access(%q) before acquire: %v", name, err)
	}
	if exists {
		t.Fatalf("Access(%q) before acquire: got true, want false", name)
	}

	// Acquire a name
	_, _, err = globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name, err)
	}

	// After acquiring, Access should return true
	exists, err = globalVFS.Access(name, vfs.ACCESS_EXISTS)
	if err != nil {
		t.Fatalf("Access(%q) after acquire: %v", name, err)
	}
	if !exists {
		t.Fatalf("Access(%q) after acquire: got false, want true", name)
	}

	// Release it
	err = globalPool.Release(name)
	if err != nil {
		t.Fatalf("Release(%q): %v", name, err)
	}

	// After releasing, Access should return false
	exists, err = globalVFS.Access(name, vfs.ACCESS_EXISTS)
	if err != nil {
		t.Fatalf("Access(%q) after release: %v", name, err)
	}
	if exists {
		t.Fatalf("Access(%q) after release: got true, want false", name)
	}
}

func TestVFSFullPathname(t *testing.T) {
	// Test FullPathname returns input unchanged
	name := "test-vfs-fullpathname"

	fullPath, err := globalVFS.FullPathname(name)
	if err != nil {
		t.Fatalf("FullPathname(%q): %v", name, err)
	}
	if fullPath != name {
		t.Fatalf("FullPathname(%q): got %q, want %q", name, fullPath, name)
	}
}

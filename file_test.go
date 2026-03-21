//go:build js

package opfsvfs

import (
	"io"
	"testing"

	"github.com/ncruces/go-sqlite3/vfs"
)

func openTestFile(t *testing.T) vfs.File {
	t.Helper()
	truncateAll(t)
	f, _, err := globalVFS.Open("test.db", vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return f
}

func TestFileReadWriteRoundTrip(t *testing.T) {
	f := openTestFile(t)
	data := []byte("hello file")
	n, err := f.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if n != len(data) {
		t.Fatalf("WriteAt: wrote %d, want %d", n, len(data))
	}
	buf := make([]byte, len(data))
	n, err = f.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("ReadAt: got %q, want %q", buf[:n], data)
	}
}

func TestFileWriteAtOffset(t *testing.T) {
	f := openTestFile(t)
	data := []byte("offset write")
	if _, err := f.WriteAt(data, 100); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if size < 100+int64(len(data)) {
		t.Fatalf("Size after offset write: got %d, want >= %d", size, 100+len(data))
	}
}

func TestFileReadAtPastEOF(t *testing.T) {
	f := openTestFile(t)
	buf := make([]byte, 16)
	_, err := f.ReadAt(buf, 10000)
	if err != io.EOF {
		t.Fatalf("ReadAt past EOF: got %v, want io.EOF", err)
	}
}

func TestFileSizeGrowsWithWrites(t *testing.T) {
	f := openTestFile(t)
	data := []byte("grow")
	if _, err := f.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if size != int64(len(data)) {
		t.Fatalf("Size: got %d, want %d", size, len(data))
	}
}

func TestFileTruncate(t *testing.T) {
	f := openTestFile(t)
	if _, err := f.WriteAt([]byte("truncate me please"), 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if err := f.Truncate(5); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if size != 5 {
		t.Fatalf("Size after Truncate: got %d, want 5", size)
	}
}

func TestFileSync(t *testing.T) {
	f := openTestFile(t)
	if _, err := f.WriteAt([]byte("sync test"), 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if err := f.Sync(0); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func TestFileClose(t *testing.T) {
	f := openTestFile(t)
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestFileLockStateMachine(t *testing.T) {
	f := openTestFile(t)
	of := f.(*opfsFile)

	// Start at NONE
	if of.LockState() != vfs.LOCK_NONE {
		t.Fatalf("Initial lock: got %d, want LOCK_NONE", of.LockState())
	}

	// Lock up
	for _, level := range []vfs.LockLevel{vfs.LOCK_SHARED, vfs.LOCK_RESERVED, vfs.LOCK_EXCLUSIVE} {
		if err := f.Lock(level); err != nil {
			t.Fatalf("Lock(%d): %v", level, err)
		}
		if of.LockState() != level {
			t.Fatalf("After Lock(%d): got %d", level, of.LockState())
		}
	}

	// Unlock down
	for _, level := range []vfs.LockLevel{vfs.LOCK_SHARED, vfs.LOCK_NONE} {
		if err := f.Unlock(level); err != nil {
			t.Fatalf("Unlock(%d): %v", level, err)
		}
		if of.LockState() != level {
			t.Fatalf("After Unlock(%d): got %d", level, of.LockState())
		}
	}
}

func TestFileDeviceCharacteristics(t *testing.T) {
	f := openTestFile(t)
	chars := f.DeviceCharacteristics()
	expected := vfs.IOCAP_ATOMIC | vfs.IOCAP_SEQUENTIAL | vfs.IOCAP_SAFE_APPEND
	if chars != expected {
		t.Fatalf("DeviceCharacteristics: got 0x%x, want 0x%x", chars, expected)
	}
}

func TestFileSectorSize(t *testing.T) {
	f := openTestFile(t)
	if f.SectorSize() != 4096 {
		t.Fatalf("SectorSize: got %d, want 4096", f.SectorSize())
	}
}

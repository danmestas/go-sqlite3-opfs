//go:build js

package opfsvfs

import (
	"io"
	"testing"

	"github.com/ncruces/go-sqlite3/vfs"
)

func TestFileReadWriteRoundTrip(t *testing.T) {
	// Test writing data and reading it back
	name := "test-file-readwrite"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Write data at offset 0
	data := []byte("Hello, OPFS VFS!")
	n, err := f.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if n != len(data) {
		t.Fatalf("WriteAt: wrote %d bytes, want %d", n, len(data))
	}

	// Read it back
	buf := make([]byte, len(data))
	n, err = f.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != len(data) {
		t.Fatalf("ReadAt: read %d bytes, want %d", n, len(data))
	}
	if string(buf) != string(data) {
		t.Fatalf("ReadAt: got %q, want %q", buf, data)
	}
}

func TestFileWriteAtOffset(t *testing.T) {
	// Test writing at non-zero offset grows the file
	name := "test-file-write-offset"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Write at offset 100
	data := []byte("offset data")
	offset := int64(100)
	n, err := f.WriteAt(data, offset)
	if err != nil {
		t.Fatalf("WriteAt(offset=%d): %v", offset, err)
	}
	if n != len(data) {
		t.Fatalf("WriteAt: wrote %d bytes, want %d", n, len(data))
	}

	// Verify size grew
	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	expectedSize := offset + int64(len(data))
	if size != expectedSize {
		t.Fatalf("Size: got %d, want %d", size, expectedSize)
	}

	// Read back the data at the offset
	buf := make([]byte, len(data))
	n, err = f.ReadAt(buf, offset)
	if err != nil {
		t.Fatalf("ReadAt(offset=%d): %v", offset, err)
	}
	if n != len(data) {
		t.Fatalf("ReadAt: read %d bytes, want %d", n, len(data))
	}
	if string(buf) != string(data) {
		t.Fatalf("ReadAt: got %q, want %q", buf, data)
	}
}

func TestFileReadAtPastEOF(t *testing.T) {
	// Test reading past EOF returns io.EOF
	name := "test-file-read-eof"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Write some data
	data := []byte("short data")
	_, err = f.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	// Try to read more bytes than available (should get io.EOF)
	buf := make([]byte, len(data)*2)
	n, err := f.ReadAt(buf, 0)
	if err != io.EOF {
		t.Fatalf("ReadAt past EOF: got error %v, want io.EOF", err)
	}
	if n != len(data) {
		t.Fatalf("ReadAt past EOF: read %d bytes, want %d", n, len(data))
	}

	// Try to read at offset beyond file size (should get io.EOF with 0 bytes)
	buf = make([]byte, 10)
	n, err = f.ReadAt(buf, 1000)
	if err != io.EOF {
		t.Fatalf("ReadAt beyond size: got error %v, want io.EOF", err)
	}
	if n != 0 {
		t.Fatalf("ReadAt beyond size: read %d bytes, want 0", n)
	}
}

func TestFileSizeGrowsWithWrites(t *testing.T) {
	// Test that Size() reflects writes
	name := "test-file-size-grows"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Initial size should be 0
	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size (initial): %v", err)
	}
	if size != 0 {
		t.Fatalf("Size (initial): got %d, want 0", size)
	}

	// Write some data
	data1 := []byte("first write")
	_, err = f.WriteAt(data1, 0)
	if err != nil {
		t.Fatalf("WriteAt (first): %v", err)
	}

	size, err = f.Size()
	if err != nil {
		t.Fatalf("Size (after first write): %v", err)
	}
	if size != int64(len(data1)) {
		t.Fatalf("Size (after first write): got %d, want %d", size, len(data1))
	}

	// Write more data at the end
	data2 := []byte(" second write")
	_, err = f.WriteAt(data2, int64(len(data1)))
	if err != nil {
		t.Fatalf("WriteAt (second): %v", err)
	}

	size, err = f.Size()
	if err != nil {
		t.Fatalf("Size (after second write): %v", err)
	}
	expectedSize := int64(len(data1) + len(data2))
	if size != expectedSize {
		t.Fatalf("Size (after second write): got %d, want %d", size, expectedSize)
	}
}

func TestFileTruncate(t *testing.T) {
	// Test truncating a file
	name := "test-file-truncate"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Write some data
	data := []byte("data to be truncated")
	_, err = f.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	// Verify initial size
	size, err := f.Size()
	if err != nil {
		t.Fatalf("Size (before truncate): %v", err)
	}
	if size != int64(len(data)) {
		t.Fatalf("Size (before truncate): got %d, want %d", size, len(data))
	}

	// Truncate to smaller size
	newSize := int64(10)
	err = f.Truncate(newSize)
	if err != nil {
		t.Fatalf("Truncate(%d): %v", newSize, err)
	}

	// Verify new size
	size, err = f.Size()
	if err != nil {
		t.Fatalf("Size (after truncate): %v", err)
	}
	if size != newSize {
		t.Fatalf("Size (after truncate): got %d, want %d", size, newSize)
	}

	// Truncate to 0
	err = f.Truncate(0)
	if err != nil {
		t.Fatalf("Truncate(0): %v", err)
	}

	size, err = f.Size()
	if err != nil {
		t.Fatalf("Size (after truncate to 0): %v", err)
	}
	if size != 0 {
		t.Fatalf("Size (after truncate to 0): got %d, want 0", size)
	}
}

func TestFileSync(t *testing.T) {
	// Test Sync operation
	name := "test-file-sync"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Write data
	data := []byte("data to sync")
	_, err = f.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	// Sync should succeed
	err = f.Sync(vfs.SYNC_NORMAL)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func TestFileClose(t *testing.T) {
	// Test Close operation (should be a no-op)
	name := "test-file-close"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	// Close should return nil (no-op)
	err = f.Close()
	if err != nil {
		t.Fatalf("Close: got error %v, want nil", err)
	}

	// Cleanup via VFS
	globalVFS.Delete(name, false)
}

func TestFileLockStateMachine(t *testing.T) {
	// Test lock state machine: NONE → SHARED → RESERVED → EXCLUSIVE and back
	name := "test-file-lock-state"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	// Get opfsFile to access LockState
	opfsF, ok := f.(*opfsFile)
	if !ok {
		t.Fatalf("Open: got file type %T, want *opfsFile", f)
	}

	// Initial state should be LOCK_NONE
	if opfsF.LockState() != vfs.LOCK_NONE {
		t.Fatalf("Initial LockState: got %d, want LOCK_NONE (%d)", opfsF.LockState(), vfs.LOCK_NONE)
	}

	// Lock to SHARED
	err = f.Lock(vfs.LOCK_SHARED)
	if err != nil {
		t.Fatalf("Lock(LOCK_SHARED): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_SHARED {
		t.Fatalf("LockState after SHARED: got %d, want LOCK_SHARED (%d)", opfsF.LockState(), vfs.LOCK_SHARED)
	}

	// CheckReservedLock should return false (not at RESERVED yet)
	reserved, err := f.CheckReservedLock()
	if err != nil {
		t.Fatalf("CheckReservedLock at SHARED: %v", err)
	}
	if reserved {
		t.Fatalf("CheckReservedLock at SHARED: got true, want false")
	}

	// Lock to RESERVED
	err = f.Lock(vfs.LOCK_RESERVED)
	if err != nil {
		t.Fatalf("Lock(LOCK_RESERVED): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_RESERVED {
		t.Fatalf("LockState after RESERVED: got %d, want LOCK_RESERVED (%d)", opfsF.LockState(), vfs.LOCK_RESERVED)
	}

	// CheckReservedLock should now return true
	reserved, err = f.CheckReservedLock()
	if err != nil {
		t.Fatalf("CheckReservedLock at RESERVED: %v", err)
	}
	if !reserved {
		t.Fatalf("CheckReservedLock at RESERVED: got false, want true")
	}

	// Lock to PENDING
	err = f.Lock(vfs.LOCK_PENDING)
	if err != nil {
		t.Fatalf("Lock(LOCK_PENDING): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_PENDING {
		t.Fatalf("LockState after PENDING: got %d, want LOCK_PENDING (%d)", opfsF.LockState(), vfs.LOCK_PENDING)
	}

	// Lock to EXCLUSIVE
	err = f.Lock(vfs.LOCK_EXCLUSIVE)
	if err != nil {
		t.Fatalf("Lock(LOCK_EXCLUSIVE): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_EXCLUSIVE {
		t.Fatalf("LockState after EXCLUSIVE: got %d, want LOCK_EXCLUSIVE (%d)", opfsF.LockState(), vfs.LOCK_EXCLUSIVE)
	}

	// Unlock back to PENDING
	err = f.Unlock(vfs.LOCK_PENDING)
	if err != nil {
		t.Fatalf("Unlock(LOCK_PENDING): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_PENDING {
		t.Fatalf("LockState after unlock to PENDING: got %d, want LOCK_PENDING (%d)", opfsF.LockState(), vfs.LOCK_PENDING)
	}

	// Unlock back to RESERVED
	err = f.Unlock(vfs.LOCK_RESERVED)
	if err != nil {
		t.Fatalf("Unlock(LOCK_RESERVED): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_RESERVED {
		t.Fatalf("LockState after unlock to RESERVED: got %d, want LOCK_RESERVED (%d)", opfsF.LockState(), vfs.LOCK_RESERVED)
	}

	// Unlock back to SHARED
	err = f.Unlock(vfs.LOCK_SHARED)
	if err != nil {
		t.Fatalf("Unlock(LOCK_SHARED): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_SHARED {
		t.Fatalf("LockState after unlock to SHARED: got %d, want LOCK_SHARED (%d)", opfsF.LockState(), vfs.LOCK_SHARED)
	}

	// Unlock back to NONE
	err = f.Unlock(vfs.LOCK_NONE)
	if err != nil {
		t.Fatalf("Unlock(LOCK_NONE): %v", err)
	}
	if opfsF.LockState() != vfs.LOCK_NONE {
		t.Fatalf("LockState after unlock to NONE: got %d, want LOCK_NONE (%d)", opfsF.LockState(), vfs.LOCK_NONE)
	}
}

func TestFileDeviceCharacteristics(t *testing.T) {
	// Test DeviceCharacteristics returns expected flags
	name := "test-file-device-char"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	chars := f.DeviceCharacteristics()

	// Verify expected flags are set
	expectedFlags := vfs.IOCAP_ATOMIC | vfs.IOCAP_SEQUENTIAL | vfs.IOCAP_SAFE_APPEND | vfs.IOCAP_POWERSAFE_OVERWRITE

	if chars&expectedFlags != expectedFlags {
		t.Fatalf("DeviceCharacteristics: got 0x%x, want 0x%x (missing expected flags)", chars, expectedFlags)
	}

	// Log all flags for visibility
	t.Logf("DeviceCharacteristics: 0x%x", chars)
	if chars&vfs.IOCAP_ATOMIC != 0 {
		t.Logf("  - IOCAP_ATOMIC")
	}
	if chars&vfs.IOCAP_SEQUENTIAL != 0 {
		t.Logf("  - IOCAP_SEQUENTIAL")
	}
	if chars&vfs.IOCAP_SAFE_APPEND != 0 {
		t.Logf("  - IOCAP_SAFE_APPEND")
	}
	if chars&vfs.IOCAP_POWERSAFE_OVERWRITE != 0 {
		t.Logf("  - IOCAP_POWERSAFE_OVERWRITE")
	}
}

func TestFileSectorSize(t *testing.T) {
	// Test SectorSize returns 4096
	name := "test-file-sector-size"

	f, _, err := globalVFS.Open(name, vfs.OPEN_MAIN_DB|vfs.OPEN_CREATE|vfs.OPEN_READWRITE)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}

	t.Cleanup(func() {
		f.Close()
		globalVFS.Delete(name, false)
	})

	sectorSize := f.SectorSize()
	expectedSize := 4096

	if sectorSize != expectedSize {
		t.Fatalf("SectorSize: got %d, want %d", sectorSize, expectedSize)
	}
}

//go:build js

package opfsvfs

import (
	"bytes"
	"testing"
)

// acquireTestHandle obtains a handle from the global pool for the given test name.
// It registers a cleanup to release the handle when the test finishes.
func acquireTestHandle(t *testing.T, name string) Handle {
	t.Helper()
	if globalPool == nil {
		t.Fatal("globalPool is nil — _opfs_pool_init was not called")
	}
	h, _, err := globalPool.Acquire(name)
	if err != nil {
		t.Fatalf("Acquire(%q): %v", name, err)
	}
	t.Cleanup(func() {
		if err := globalPool.Release(name); err != nil {
			t.Errorf("Release(%q): %v", name, err)
		}
	})
	return h
}

func TestHandleReadWriteRoundTrip(t *testing.T) {
	h := acquireTestHandle(t, "test-handle-rw")

	data := []byte("hello opfs")
	n, err := h.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: wrote %d, want %d", n, len(data))
	}

	buf := make([]byte, len(data))
	n, err = h.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Read: read %d, want %d", n, len(data))
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("Read: got %q, want %q", buf, data)
	}
}

func TestHandleGetSize(t *testing.T) {
	h := acquireTestHandle(t, "test-handle-size")

	data := []byte("size check")
	if _, err := h.Write(data, 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	size, err := h.GetSize()
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != int64(len(data)) {
		t.Fatalf("GetSize: got %d, want %d", size, len(data))
	}
}

func TestHandleTruncate(t *testing.T) {
	h := acquireTestHandle(t, "test-handle-trunc")

	data := []byte("truncate me please")
	if _, err := h.Write(data, 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := h.Truncate(5); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	size, err := h.GetSize()
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size != 5 {
		t.Fatalf("GetSize after Truncate: got %d, want 5", size)
	}
}

func TestHandleFlush(t *testing.T) {
	h := acquireTestHandle(t, "test-handle-flush")

	data := []byte("flush test")
	if _, err := h.Write(data, 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := h.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestHandleReadPastEOF(t *testing.T) {
	h := acquireTestHandle(t, "test-handle-eof")

	// File is empty after acquire+release (truncated to 0). Read past EOF.
	buf := make([]byte, 16)
	n, err := h.Read(buf, 1000)
	if err != nil {
		t.Fatalf("Read past EOF: unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("Read past EOF: got n=%d, want 0", n)
	}
}

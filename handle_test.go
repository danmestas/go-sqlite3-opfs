//go:build js

package opfsvfs

import (
	"bytes"
	"testing"
)

func getTestHandle(t *testing.T) Handle {
	t.Helper()
	h, ok := globalVFS.handles["test.db"]
	if !ok {
		t.Fatal("test.db handle not registered")
	}
	if err := h.Truncate(0); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	return h
}

func TestHandleReadWriteRoundTrip(t *testing.T) {
	h := getTestHandle(t)
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
	if !bytes.Equal(buf[:n], data) {
		t.Fatalf("Read: got %q, want %q", buf[:n], data)
	}
}

func TestHandleGetSize(t *testing.T) {
	h := getTestHandle(t)
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
	h := getTestHandle(t)
	if _, err := h.Write([]byte("truncate me"), 0); err != nil {
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
	h := getTestHandle(t)
	if _, err := h.Write([]byte("flush test"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := h.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestHandleReadPastEOF(t *testing.T) {
	h := getTestHandle(t)
	buf := make([]byte, 16)
	n, err := h.Read(buf, 1000)
	if err != nil {
		t.Fatalf("Read past EOF: unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("Read past EOF: got n=%d, want 0", n)
	}
}

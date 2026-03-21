//go:build js

package opfsvfs

import (
	"errors"
	"testing"
)

// faultHandle wraps a real Handle and injects faults for testing.
type faultHandle struct {
	inner     Handle
	readErr   error // if set, Read returns this error
	writeErr  error // if set, Write returns this error
	flushErr  error // if set, Flush returns this error
	shortRead int   // if > 0, Read returns at most this many bytes
}

func (f *faultHandle) Read(buf []byte, offset int64) (int, error) {
	if f.inner == nil {
		panic("faultHandle: inner handle is nil")
	}
	if f.readErr != nil {
		return 0, f.readErr
	}
	if f.shortRead > 0 && len(buf) > f.shortRead {
		return f.inner.Read(buf[:f.shortRead], offset)
	}
	return f.inner.Read(buf, offset)
}

func (f *faultHandle) Write(buf []byte, offset int64) (int, error) {
	if f.inner == nil {
		panic("faultHandle: inner handle is nil")
	}
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.inner.Write(buf, offset)
}

func (f *faultHandle) GetSize() (int64, error) {
	if f.inner == nil {
		panic("faultHandle: inner handle is nil")
	}
	return f.inner.GetSize()
}

func (f *faultHandle) Truncate(size int64) error {
	if f.inner == nil {
		panic("faultHandle: inner handle is nil")
	}
	return f.inner.Truncate(size)
}

func (f *faultHandle) Flush() error {
	if f.inner == nil {
		panic("faultHandle: inner handle is nil")
	}
	if f.flushErr != nil {
		return f.flushErr
	}
	return f.inner.Flush()
}

func (f *faultHandle) Close() error {
	if f.inner == nil {
		panic("faultHandle: inner handle is nil")
	}
	return f.inner.Close()
}

func getFaultTestHandle(t *testing.T) Handle {
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

func TestFaultHandleShortRead(t *testing.T) {
	inner := getFaultTestHandle(t)

	data := []byte("0123456789")
	n, err := inner.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: wrote %d, want %d", n, len(data))
	}

	fh := &faultHandle{inner: inner, shortRead: 5}

	buf := make([]byte, 10)
	n, err = fh.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("Read: got n=%d, want 5", n)
	}
	// Verify correct bytes were read.
	if string(buf[:5]) != "01234" {
		t.Fatalf("Read: got %q, want %q", buf[:5], "01234")
	}
}

func TestFaultHandleWriteFailure(t *testing.T) {
	inner := getFaultTestHandle(t)
	expectedErr := errors.New("simulated write failure")
	fh := &faultHandle{inner: inner, writeErr: expectedErr}

	n, err := fh.Write([]byte("this should fail"), 0)
	if err == nil {
		t.Fatal("Write: expected error, got nil")
	}
	if err != expectedErr {
		t.Fatalf("Write: got error %v, want %v", err, expectedErr)
	}
	if n != 0 {
		t.Fatalf("Write: got n=%d, want 0", n)
	}
}

func TestFaultHandleFlushFailure(t *testing.T) {
	inner := getFaultTestHandle(t)
	if _, err := inner.Write([]byte("data to flush"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	expectedErr := errors.New("simulated flush failure")
	fh := &faultHandle{inner: inner, flushErr: expectedErr}

	err := fh.Flush()
	if err == nil {
		t.Fatal("Flush: expected error, got nil")
	}
	if err != expectedErr {
		t.Fatalf("Flush: got error %v, want %v", err, expectedErr)
	}
}

func TestFaultHandleReadError(t *testing.T) {
	inner := getFaultTestHandle(t)
	if _, err := inner.Write([]byte("data to read"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	expectedErr := errors.New("simulated read error")
	fh := &faultHandle{inner: inner, readErr: expectedErr}

	buf := make([]byte, 16)
	n, err := fh.Read(buf, 0)
	if err == nil {
		t.Fatal("Read: expected error, got nil")
	}
	if err != expectedErr {
		t.Fatalf("Read: got error %v, want %v", err, expectedErr)
	}
	if n != 0 {
		t.Fatalf("Read: got n=%d, want 0", n)
	}
}

func TestFaultHandleCombinedFaults(t *testing.T) {
	inner := getFaultTestHandle(t)
	fh := &faultHandle{inner: inner}

	// Write succeeds.
	data := []byte("combined test")
	n, err := fh.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: got n=%d, want %d", n, len(data))
	}

	// Read with short read.
	fh.shortRead = 3
	buf := make([]byte, 10)
	n, err = fh.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read (short): %v", err)
	}
	if n != 3 {
		t.Fatalf("Read (short): got n=%d, want 3", n)
	}

	// Flush fails.
	fh.shortRead = 0
	fh.flushErr = errors.New("flush error")
	if err := fh.Flush(); err == nil {
		t.Fatal("Flush: expected error, got nil")
	}

	// Read fails.
	fh.flushErr = nil
	fh.readErr = errors.New("read error")
	if _, err := fh.Read(buf, 0); err == nil {
		t.Fatal("Read (error): expected error, got nil")
	}
}

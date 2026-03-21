//go:build js

package opfsvfs

import (
	"errors"
	"testing"
)

// FaultHandle wraps a real Handle and injects faults for testing.
// Used to simulate read/write/flush errors and short reads.
type FaultHandle struct {
	inner     Handle
	readErr   error // if set, Read returns this error
	writeErr  error // if set, Write returns this error
	flushErr  error // if set, Flush returns this error
	shortRead int   // if > 0, Read returns at most this many bytes
}

// Read delegates to inner handle but injects faults.
func (f *FaultHandle) Read(buf []byte, offset int64) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	if f.shortRead > 0 && len(buf) > f.shortRead {
		// Limit the read to shortRead bytes
		shortBuf := buf[:f.shortRead]
		return f.inner.Read(shortBuf, offset)
	}
	return f.inner.Read(buf, offset)
}

// Write delegates to inner handle but injects faults.
func (f *FaultHandle) Write(buf []byte, offset int64) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.inner.Write(buf, offset)
}

// GetSize delegates to inner handle.
func (f *FaultHandle) GetSize() (int64, error) {
	return f.inner.GetSize()
}

// Truncate delegates to inner handle.
func (f *FaultHandle) Truncate(size int64) error {
	return f.inner.Truncate(size)
}

// Flush delegates to inner handle but injects faults.
func (f *FaultHandle) Flush() error {
	if f.flushErr != nil {
		return f.flushErr
	}
	return f.inner.Flush()
}

// Close delegates to inner handle.
func (f *FaultHandle) Close() error {
	return f.inner.Close()
}

// TestFaultHandleShortRead tests that FaultHandle with shortRead=5
// returns at most 5 bytes even when asked for more.
func TestFaultHandleShortRead(t *testing.T) {
	inner := acquireTestHandle(t, "test-fault-short-read")

	// Write 10 bytes
	data := []byte("0123456789")
	n, err := inner.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: wrote %d, want %d", n, len(data))
	}

	// Create FaultHandle with shortRead=5
	fh := &FaultHandle{
		inner:     inner,
		shortRead: 5,
	}

	// Read with 10-byte buffer, should only get 5 bytes
	buf := make([]byte, 10)
	n, err = fh.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("Read: got n=%d, want 5", n)
	}

	expected := []byte("01234")
	for i := 0; i < 5; i++ {
		if buf[i] != expected[i] {
			t.Errorf("Read: buf[%d]=%c, want %c", i, buf[i], expected[i])
		}
	}

	t.Logf("Short read successful: requested 10 bytes, got %d bytes", n)
}

// TestFaultHandleWriteFailure tests that FaultHandle with writeErr
// returns the error on Write.
func TestFaultHandleWriteFailure(t *testing.T) {
	inner := acquireTestHandle(t, "test-fault-write-fail")

	expectedErr := errors.New("simulated write failure")

	// Create FaultHandle with writeErr
	fh := &FaultHandle{
		inner:    inner,
		writeErr: expectedErr,
	}

	// Attempt to write, should fail
	data := []byte("this should fail")
	n, err := fh.Write(data, 0)
	if err == nil {
		t.Fatalf("Write: expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("Write: got error %v, want %v", err, expectedErr)
	}
	if n != 0 {
		t.Errorf("Write: got n=%d, want 0", n)
	}

	t.Logf("Write failure injected successfully: %v", err)
}

// TestFaultHandleFlushFailure tests that FaultHandle with flushErr
// returns the error on Flush.
func TestFaultHandleFlushFailure(t *testing.T) {
	inner := acquireTestHandle(t, "test-fault-flush-fail")

	// Write some data first
	data := []byte("data to flush")
	_, err := inner.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	expectedErr := errors.New("simulated flush failure")

	// Create FaultHandle with flushErr
	fh := &FaultHandle{
		inner:    inner,
		flushErr: expectedErr,
	}

	// Attempt to flush, should fail
	err = fh.Flush()
	if err == nil {
		t.Fatalf("Flush: expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("Flush: got error %v, want %v", err, expectedErr)
	}

	t.Logf("Flush failure injected successfully: %v", err)
}

// TestFaultHandleReadError tests that FaultHandle with readErr
// returns the error on Read.
func TestFaultHandleReadError(t *testing.T) {
	inner := acquireTestHandle(t, "test-fault-read-error")

	// Write some data first
	data := []byte("data to read")
	_, err := inner.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	expectedErr := errors.New("simulated read error")

	// Create FaultHandle with readErr
	fh := &FaultHandle{
		inner:   inner,
		readErr: expectedErr,
	}

	// Attempt to read, should fail
	buf := make([]byte, 16)
	n, err := fh.Read(buf, 0)
	if err == nil {
		t.Fatalf("Read: expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("Read: got error %v, want %v", err, expectedErr)
	}
	if n != 0 {
		t.Errorf("Read: got n=%d, want 0", n)
	}

	t.Logf("Read error injected successfully: %v", err)
}

// TestFaultHandleCombinedFaults tests multiple faults in sequence.
func TestFaultHandleCombinedFaults(t *testing.T) {
	inner := acquireTestHandle(t, "test-fault-combined")

	// Test 1: Write succeeds
	fh := &FaultHandle{inner: inner}
	data := []byte("combined test")
	_, err := fh.Write(data, 0)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Test 2: Read with short read
	fh.shortRead = 3
	buf := make([]byte, 10)
	n, err := fh.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read (short): %v", err)
	}
	if n != 3 {
		t.Errorf("Read (short): got n=%d, want 3", n)
	}

	// Test 3: Flush fails
	fh.shortRead = 0
	fh.flushErr = errors.New("flush error")
	err = fh.Flush()
	if err == nil {
		t.Fatalf("Flush: expected error, got nil")
	}

	// Test 4: Read fails
	fh.flushErr = nil
	fh.readErr = errors.New("read error")
	_, err = fh.Read(buf, 0)
	if err == nil {
		t.Fatalf("Read (error): expected error, got nil")
	}

	t.Logf("Combined faults test successful")
}

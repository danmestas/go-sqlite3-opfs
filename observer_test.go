//go:build js

package opfsvfs

import (
	"errors"
	"testing"
)

// TestObserverOnRead tests that RecordingObserver captures read events
// when ReadAt is called on an opfsFile.
func TestObserverOnRead(t *testing.T) {
	obs := &RecordingObserver{}
	h := acquireTestHandle(t, "test-obs-read")

	// Write some data first
	data := []byte("hello observer")
	if _, err := h.Write(data, 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Create an opfsFile with the recording observer
	f := &opfsFile{
		handle:   h,
		name:     "test-obs-read",
		slot:     1,
		stats:    &Stats{},
		observer: obs,
	}

	// Perform a ReadAt operation
	buf := make([]byte, len(data))
	n, err := f.ReadAt(buf, 5) // Read from offset 5
	if err == nil {
		t.Fatalf("ReadAt: expected io.EOF for short read, got nil")
	}

	// Verify observer captured the read event
	if len(obs.Reads) != 1 {
		t.Fatalf("RecordingObserver: got %d read events, want 1", len(obs.Reads))
	}

	event := obs.Reads[0]
	if event.File != "test-obs-read" {
		t.Errorf("ReadEvent.File: got %q, want %q", event.File, "test-obs-read")
	}
	if event.Offset != 5 {
		t.Errorf("ReadEvent.Offset: got %d, want 5", event.Offset)
	}
	if event.Bytes != n {
		t.Errorf("ReadEvent.Bytes: got %d, want %d", event.Bytes, n)
	}
	if event.Duration <= 0 {
		t.Errorf("ReadEvent.Duration: got %v, want > 0", event.Duration)
	}
	// Observer receives the raw handle error (nil), not the io.EOF added by ReadAt.
	// The handle read succeeded; io.EOF is a SQLite-level protocol detail.
	if event.Err != nil {
		t.Errorf("ReadEvent.Err: got %v, want nil (handle read succeeded)", event.Err)
	}

	t.Logf("Read event captured: file=%q, offset=%d, bytes=%d, duration=%v, err=%v",
		event.File, event.Offset, event.Bytes, event.Duration, event.Err)
}

// TestObserverOnWrite tests that RecordingObserver captures write events
// when WriteAt is called on an opfsFile.
func TestObserverOnWrite(t *testing.T) {
	obs := &RecordingObserver{}
	h := acquireTestHandle(t, "test-obs-write")

	// Create an opfsFile with the recording observer
	f := &opfsFile{
		handle:   h,
		name:     "test-obs-write",
		slot:     2,
		stats:    &Stats{},
		observer: obs,
	}

	// Perform a WriteAt operation
	data := []byte("write test data")
	n, err := f.WriteAt(data, 10) // Write at offset 10
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if n != len(data) {
		t.Fatalf("WriteAt: wrote %d, want %d", n, len(data))
	}

	// Verify observer captured the write event
	if len(obs.Writes) != 1 {
		t.Fatalf("RecordingObserver: got %d write events, want 1", len(obs.Writes))
	}

	event := obs.Writes[0]
	if event.File != "test-obs-write" {
		t.Errorf("WriteEvent.File: got %q, want %q", event.File, "test-obs-write")
	}
	if event.Offset != 10 {
		t.Errorf("WriteEvent.Offset: got %d, want 10", event.Offset)
	}
	if event.Bytes != len(data) {
		t.Errorf("WriteEvent.Bytes: got %d, want %d", event.Bytes, len(data))
	}
	if event.Duration <= 0 {
		t.Errorf("WriteEvent.Duration: got %v, want > 0", event.Duration)
	}
	if event.Err != nil {
		t.Errorf("WriteEvent.Err: got %v, want nil", event.Err)
	}

	t.Logf("Write event captured: file=%q, offset=%d, bytes=%d, duration=%v",
		event.File, event.Offset, event.Bytes, event.Duration)
}

// TestObserverOnFlush tests that RecordingObserver captures flush events
// when Sync is called on an opfsFile.
func TestObserverOnFlush(t *testing.T) {
	obs := &RecordingObserver{}
	h := acquireTestHandle(t, "test-obs-flush")

	// Write some data first
	data := []byte("flush test")
	if _, err := h.Write(data, 0); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Create an opfsFile with the recording observer
	f := &opfsFile{
		handle:   h,
		name:     "test-obs-flush",
		slot:     3,
		stats:    &Stats{},
		observer: obs,
	}

	// Perform a Sync operation
	err := f.Sync(0)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Verify observer captured the flush event
	if len(obs.Flushes) != 1 {
		t.Fatalf("RecordingObserver: got %d flush events, want 1", len(obs.Flushes))
	}

	event := obs.Flushes[0]
	if event.File != "test-obs-flush" {
		t.Errorf("FlushEvent.File: got %q, want %q", event.File, "test-obs-flush")
	}
	if event.Duration <= 0 {
		t.Errorf("FlushEvent.Duration: got %v, want > 0", event.Duration)
	}
	if event.Err != nil {
		t.Errorf("FlushEvent.Err: got %v, want nil", event.Err)
	}

	t.Logf("Flush event captured: file=%q, duration=%v", event.File, event.Duration)
}

// TestObserverOnError tests that RecordingObserver captures errors
// when operations fail. We use FaultHandle to inject errors.
func TestObserverOnError(t *testing.T) {
	obs := &RecordingObserver{}
	inner := acquireTestHandle(t, "test-obs-error")

	// Create a FaultHandle that injects read errors
	readErr := errors.New("simulated read error")
	fh := &FaultHandle{
		inner:   inner,
		readErr: readErr,
	}

	// Create an opfsFile with the FaultHandle and recording observer
	f := &opfsFile{
		handle:   fh,
		name:     "test-obs-error",
		slot:     4,
		stats:    &Stats{},
		observer: obs,
	}

	// Attempt to read - should fail with injected error
	buf := make([]byte, 16)
	_, err := f.ReadAt(buf, 0)
	if err == nil {
		t.Fatalf("ReadAt: expected error, got nil")
	}

	// Verify observer captured the read event with error
	if len(obs.Reads) != 1 {
		t.Fatalf("RecordingObserver: got %d read events, want 1", len(obs.Reads))
	}

	event := obs.Reads[0]
	if event.Err == nil {
		t.Errorf("ReadEvent.Err: got nil, want error")
	}
	if event.Err != readErr {
		t.Errorf("ReadEvent.Err: got %v, want %v", event.Err, readErr)
	}

	// Test OnError method directly
	opfsErr := &OpfsError{
		Op:   "TestOp",
		Name: "test-file",
		Err:  errors.New("test error"),
	}
	obs.OnError(opfsErr)

	if len(obs.Errors) != 1 {
		t.Fatalf("RecordingObserver.Errors: got %d errors, want 1", len(obs.Errors))
	}
	if obs.Errors[0] != opfsErr {
		t.Errorf("RecordingObserver.Errors[0]: got %v, want %v", obs.Errors[0], opfsErr)
	}

	t.Logf("Read error captured: %v", event.Err)
	t.Logf("OnError captured: %v", obs.Errors[0])
}

// TestObserverOnPoolEvent tests that RecordingObserver captures pool events
// by calling observer methods directly (avoids creating a conflicting pool).
func TestObserverOnPoolEvent(t *testing.T) {
	obs := &RecordingObserver{}

	// Test PoolEventAlloc
	obs.OnPoolEvent(PoolEvent{Type: PoolEventAlloc, Name: "test-obs-pool", Slot: 1})
	if len(obs.PoolEvents) != 1 {
		t.Fatalf("After Alloc: got %d events, want 1", len(obs.PoolEvents))
	}
	if obs.PoolEvents[0].Type != PoolEventAlloc {
		t.Errorf("Event type: got %d, want PoolEventAlloc", obs.PoolEvents[0].Type)
	}
	if obs.PoolEvents[0].Name != "test-obs-pool" {
		t.Errorf("Event name: got %q, want %q", obs.PoolEvents[0].Name, "test-obs-pool")
	}
	if obs.PoolEvents[0].Slot != 1 {
		t.Errorf("Event slot: got %d, want 1", obs.PoolEvents[0].Slot)
	}

	// Test PoolEventRelease
	obs.OnPoolEvent(PoolEvent{Type: PoolEventRelease, Name: "test-obs-pool", Slot: 1})
	if len(obs.PoolEvents) != 2 {
		t.Fatalf("After Release: got %d events, want 2", len(obs.PoolEvents))
	}
	if obs.PoolEvents[1].Type != PoolEventRelease {
		t.Errorf("Event type: got %d, want PoolEventRelease", obs.PoolEvents[1].Type)
	}

	// Test PoolEventFull
	obs.OnPoolEvent(PoolEvent{Type: PoolEventFull, Name: "overflow", Slot: -1})
	if len(obs.PoolEvents) != 3 {
		t.Fatalf("After Full: got %d events, want 3", len(obs.PoolEvents))
	}
	if obs.PoolEvents[2].Type != PoolEventFull {
		t.Errorf("Event type: got %d, want PoolEventFull", obs.PoolEvents[2].Type)
	}
	if obs.PoolEvents[2].Slot != -1 {
		t.Errorf("Full event slot: got %d, want -1", obs.PoolEvents[2].Slot)
	}

	// Test PoolEventMeta
	obs.OnPoolEvent(PoolEvent{Type: PoolEventMeta, Name: "", Slot: 0})
	if len(obs.PoolEvents) != 4 {
		t.Fatalf("After Meta: got %d events, want 4", len(obs.PoolEvents))
	}

	t.Logf("Pool events captured: %d events", len(obs.PoolEvents))
}

// TestNopObserverDoesNotPanic tests that nopObserver safely handles all calls.
func TestNopObserverDoesNotPanic(t *testing.T) {
	obs := nopObserver{}

	// Test all methods - none should panic
	obs.OnRead("test", 0, 100, 0, nil)
	obs.OnRead("test", 0, 100, 0, errors.New("test error"))

	obs.OnWrite("test", 0, 100, 0, nil)
	obs.OnWrite("test", 0, 100, 0, errors.New("test error"))

	obs.OnFlush("test", 0, nil)
	obs.OnFlush("test", 0, errors.New("test error"))

	obs.OnError(&OpfsError{Op: "test", Name: "test", Err: errors.New("test")})
	obs.OnError(nil)

	obs.OnPoolEvent(PoolEvent{Type: PoolEventAlloc, Name: "test", Slot: 1})
	obs.OnPoolEvent(PoolEvent{Type: PoolEventRelease, Name: "test", Slot: 1})
	obs.OnPoolEvent(PoolEvent{Type: PoolEventFull, Name: "test", Slot: -1})
	obs.OnPoolEvent(PoolEvent{Type: PoolEventMeta, Name: "", Slot: 0})

	t.Logf("nopObserver handled all method calls without panicking")
}

// TestObserverWithNilHandle tests that observer still captures events
// even when underlying operations might have issues.
func TestObserverMultipleEvents(t *testing.T) {
	obs := &RecordingObserver{}
	h := acquireTestHandle(t, "test-obs-multi")

	// Create an opfsFile with the recording observer
	f := &opfsFile{
		handle:   h,
		name:     "test-obs-multi",
		slot:     5,
		stats:    &Stats{},
		observer: obs,
	}

	// Perform multiple operations
	data := []byte("multi event test")

	// Write 1
	if _, err := f.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt 1: %v", err)
	}

	// Write 2
	if _, err := f.WriteAt(data, 100); err != nil {
		t.Fatalf("WriteAt 2: %v", err)
	}

	// Read 1
	buf := make([]byte, len(data))
	if _, err := f.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt 1: %v", err)
	}

	// Read 2
	if _, err := f.ReadAt(buf, 100); err != nil {
		t.Fatalf("ReadAt 2: %v", err)
	}

	// Flush 1
	if err := f.Sync(0); err != nil {
		t.Fatalf("Sync 1: %v", err)
	}

	// Flush 2
	if err := f.Sync(0); err != nil {
		t.Fatalf("Sync 2: %v", err)
	}

	// Verify all events were captured
	if len(obs.Writes) != 2 {
		t.Errorf("RecordingObserver.Writes: got %d, want 2", len(obs.Writes))
	}
	if len(obs.Reads) != 2 {
		t.Errorf("RecordingObserver.Reads: got %d, want 2", len(obs.Reads))
	}
	if len(obs.Flushes) != 2 {
		t.Errorf("RecordingObserver.Flushes: got %d, want 2", len(obs.Flushes))
	}

	// Verify offsets are correct
	if obs.Writes[0].Offset != 0 {
		t.Errorf("Writes[0].Offset: got %d, want 0", obs.Writes[0].Offset)
	}
	if obs.Writes[1].Offset != 100 {
		t.Errorf("Writes[1].Offset: got %d, want 100", obs.Writes[1].Offset)
	}
	if obs.Reads[0].Offset != 0 {
		t.Errorf("Reads[0].Offset: got %d, want 0", obs.Reads[0].Offset)
	}
	if obs.Reads[1].Offset != 100 {
		t.Errorf("Reads[1].Offset: got %d, want 100", obs.Reads[1].Offset)
	}

	t.Logf("Multiple events captured: %d writes, %d reads, %d flushes",
		len(obs.Writes), len(obs.Reads), len(obs.Flushes))
}

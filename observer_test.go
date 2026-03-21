//go:build js

package opfsvfs

import (
	"errors"
	"testing"
	"time"
)

func TestObserverOnRead(t *testing.T) {
	obs := &RecordingObserver{}
	h, ok := globalVFS.handles["test.db"]
	if !ok {
		t.Fatal("test.db handle not registered")
	}
	if err := h.Truncate(0); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if _, err := h.Write([]byte("hello observer"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	f := &opfsFile{handle: h, name: "test.db", stats: &Stats{}, observer: obs}
	buf := make([]byte, 14)
	n, err := f.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if n != 14 {
		t.Fatalf("ReadAt: got n=%d, want 14", n)
	}
	// Verify observer captured the read event.
	if len(obs.Reads) != 1 {
		t.Fatalf("got %d read events, want 1", len(obs.Reads))
	}
	if obs.Reads[0].File != "test.db" {
		t.Errorf("File: got %q, want %q", obs.Reads[0].File, "test.db")
	}
	if obs.Reads[0].Duration <= 0 {
		t.Errorf("Duration: got %v, want > 0", obs.Reads[0].Duration)
	}
	// Verify observer got nil error (read succeeded at handle level).
	if obs.Reads[0].Err != nil {
		t.Errorf("Err: got %v, want nil", obs.Reads[0].Err)
	}
}

func TestObserverOnWrite(t *testing.T) {
	obs := &RecordingObserver{}
	h, ok := globalVFS.handles["test.db"]
	if !ok {
		t.Fatal("test.db handle not registered")
	}
	if err := h.Truncate(0); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	f := &opfsFile{handle: h, name: "test.db", stats: &Stats{}, observer: obs}
	n, err := f.WriteAt([]byte("write test"), 0)
	if err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if n != 10 {
		t.Fatalf("WriteAt: got n=%d, want 10", n)
	}
	if len(obs.Writes) != 1 {
		t.Fatalf("got %d write events, want 1", len(obs.Writes))
	}
	if obs.Writes[0].Bytes != 10 {
		t.Errorf("Bytes: got %d, want 10", obs.Writes[0].Bytes)
	}
	if obs.Writes[0].Err != nil {
		t.Errorf("Err: got %v, want nil", obs.Writes[0].Err)
	}
}

func TestObserverOnFlush(t *testing.T) {
	obs := &RecordingObserver{}
	h, ok := globalVFS.handles["test.db"]
	if !ok {
		t.Fatal("test.db handle not registered")
	}
	f := &opfsFile{handle: h, name: "test.db", stats: &Stats{}, observer: obs}
	if err := f.Sync(0); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(obs.Flushes) != 1 {
		t.Fatalf("got %d flush events, want 1", len(obs.Flushes))
	}
	if obs.Flushes[0].Err != nil {
		t.Errorf("Err: got %v, want nil", obs.Flushes[0].Err)
	}
}

func TestObserverOnError(t *testing.T) {
	// Verify that when a FaultHandle injects a read error, the observer
	// captures it in the ReadEvent.Err field.
	obs := &RecordingObserver{}
	readErr := errors.New("simulated read error")
	fh := &faultHandle{inner: globalVFS.handles["test.db"], readErr: readErr}
	f := &opfsFile{handle: fh, name: "test.db", stats: &Stats{}, observer: obs}
	buf := make([]byte, 16)
	_, err := f.ReadAt(buf, 0)
	if err == nil {
		t.Fatal("ReadAt: expected error, got nil")
	}
	if len(obs.Reads) != 1 {
		t.Fatalf("got %d read events, want 1", len(obs.Reads))
	}
	if obs.Reads[0].Err != readErr {
		t.Errorf("Err: got %v, want %v", obs.Reads[0].Err, readErr)
	}
}

func TestNopObserverDoesNotPanic(t *testing.T) {
	obs := nopObserver{}
	obs.OnRead("test", 0, 100, time.Millisecond, nil)
	obs.OnRead("test", 0, 100, time.Millisecond, errors.New("test error"))
	obs.OnWrite("test", 0, 100, time.Millisecond, nil)
	obs.OnWrite("test", 0, 100, time.Millisecond, errors.New("test error"))
	obs.OnFlush("test", time.Millisecond, nil)
	obs.OnFlush("test", time.Millisecond, errors.New("test error"))
	t.Log("nopObserver handled all calls without panicking")
}

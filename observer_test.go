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
	f.ReadAt(buf, 0)
	if len(obs.Reads) != 1 {
		t.Fatalf("got %d read events, want 1", len(obs.Reads))
	}
	if obs.Reads[0].File != "test.db" {
		t.Errorf("File: got %q, want %q", obs.Reads[0].File, "test.db")
	}
	if obs.Reads[0].Duration <= 0 {
		t.Errorf("Duration: got %v, want > 0", obs.Reads[0].Duration)
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
	f.WriteAt([]byte("write test"), 0)
	if len(obs.Writes) != 1 {
		t.Fatalf("got %d write events, want 1", len(obs.Writes))
	}
	if obs.Writes[0].Bytes != 10 {
		t.Errorf("Bytes: got %d, want 10", obs.Writes[0].Bytes)
	}
}

func TestObserverOnFlush(t *testing.T) {
	obs := &RecordingObserver{}
	h, ok := globalVFS.handles["test.db"]
	if !ok {
		t.Fatal("test.db handle not registered")
	}
	f := &opfsFile{handle: h, name: "test.db", stats: &Stats{}, observer: obs}
	f.Sync(0)
	if len(obs.Flushes) != 1 {
		t.Fatalf("got %d flush events, want 1", len(obs.Flushes))
	}
}

func TestObserverOnError(t *testing.T) {
	obs := &RecordingObserver{}
	readErr := errors.New("simulated read error")
	fh := &FaultHandle{inner: globalVFS.handles["test.db"], readErr: readErr}
	f := &opfsFile{handle: fh, name: "test.db", stats: &Stats{}, observer: obs}
	buf := make([]byte, 16)
	f.ReadAt(buf, 0)
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
	obs.OnWrite("test", 0, 100, time.Millisecond, nil)
	obs.OnFlush("test", time.Millisecond, nil)
	obs.OnError(&OpfsError{Op: "test", Err: errors.New("test")})
	t.Log("nopObserver handled all calls without panicking")
}

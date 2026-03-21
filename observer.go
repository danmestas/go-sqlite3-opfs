package opfsvfs

import "time"

// maxRecordedEvents prevents unbounded growth in RecordingObserver.
const maxRecordedEvents = 10000

// Observer receives callbacks for I/O operations.
// Pass nil for no-op. Implementations must be safe for concurrent use.
type Observer interface {
	OnRead(file string, offset int64, bytes int, duration time.Duration, err error)
	OnWrite(file string, offset int64, bytes int, duration time.Duration, err error)
	OnFlush(file string, duration time.Duration, err error)
}

// nopObserver implements Observer as no-ops.
type nopObserver struct{}

func (nopObserver) OnRead(string, int64, int, time.Duration, error) {}
func (nopObserver) OnWrite(string, int64, int, time.Duration, error) {}
func (nopObserver) OnFlush(string, time.Duration, error) {}

func resolveObserver(obs Observer) Observer {
	if obs == nil {
		return nopObserver{}
	}
	return obs
}

// RecordingObserver captures events for test assertions.
// Stops recording after maxRecordedEvents to prevent unbounded growth.
type RecordingObserver struct {
	Reads   []ReadEvent
	Writes  []WriteEvent
	Flushes []FlushEvent
}

type ReadEvent struct {
	File     string
	Offset   int64
	Bytes    int
	Duration time.Duration
	Err      error
}

type WriteEvent struct {
	File     string
	Offset   int64
	Bytes    int
	Duration time.Duration
	Err      error
}

type FlushEvent struct {
	File     string
	Duration time.Duration
	Err      error
}

func (r *RecordingObserver) OnRead(file string, offset int64, bytes int, dur time.Duration, err error) {
	if len(r.Reads) < maxRecordedEvents {
		r.Reads = append(r.Reads, ReadEvent{file, offset, bytes, dur, err})
	}
}

func (r *RecordingObserver) OnWrite(file string, offset int64, bytes int, dur time.Duration, err error) {
	if len(r.Writes) < maxRecordedEvents {
		r.Writes = append(r.Writes, WriteEvent{file, offset, bytes, dur, err})
	}
}

func (r *RecordingObserver) OnFlush(file string, dur time.Duration, err error) {
	if len(r.Flushes) < maxRecordedEvents {
		r.Flushes = append(r.Flushes, FlushEvent{file, dur, err})
	}
}

// Verify interface compliance.
var _ Observer = nopObserver{}
var _ Observer = (*RecordingObserver)(nil)

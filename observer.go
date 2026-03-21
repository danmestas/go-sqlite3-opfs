package opfsvfs

import "time"

// PoolEventType classifies pool lifecycle events.
type PoolEventType int

const (
	PoolEventAlloc   PoolEventType = iota // new slot assigned
	PoolEventRelease                       // slot released
	PoolEventFull                          // pool exhausted
	PoolEventMeta                          // metadata written
)

// PoolEvent describes a pool lifecycle event.
type PoolEvent struct {
	Type PoolEventType
	Name string // virtual filename
	Slot int    // slot index, -1 if not applicable
}

// Observer receives callbacks for I/O operations and pool events.
// Pass nil for no-op. Implementations must be safe for concurrent use.
type Observer interface {
	OnRead(file string, offset int64, bytes int, duration time.Duration, err error)
	OnWrite(file string, offset int64, bytes int, duration time.Duration, err error)
	OnFlush(file string, duration time.Duration, err error)
	OnError(err *OpfsError)
	OnPoolEvent(event PoolEvent)
}

// nopObserver implements Observer as no-ops.
type nopObserver struct{}

func (nopObserver) OnRead(string, int64, int, time.Duration, error) {}
func (nopObserver) OnWrite(string, int64, int, time.Duration, error) {}
func (nopObserver) OnFlush(string, time.Duration, error) {}
func (nopObserver) OnError(*OpfsError) {}
func (nopObserver) OnPoolEvent(PoolEvent) {}

func resolveObserver(obs Observer) Observer {
	if obs == nil {
		return nopObserver{}
	}
	return obs
}

// RecordingObserver captures all events for test assertions.
type RecordingObserver struct {
	Reads      []ReadEvent
	Writes     []WriteEvent
	Flushes    []FlushEvent
	Errors     []*OpfsError
	PoolEvents []PoolEvent
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
	r.Reads = append(r.Reads, ReadEvent{file, offset, bytes, dur, err})
}

func (r *RecordingObserver) OnWrite(file string, offset int64, bytes int, dur time.Duration, err error) {
	r.Writes = append(r.Writes, WriteEvent{file, offset, bytes, dur, err})
}

func (r *RecordingObserver) OnFlush(file string, dur time.Duration, err error) {
	r.Flushes = append(r.Flushes, FlushEvent{file, dur, err})
}

func (r *RecordingObserver) OnError(err *OpfsError) {
	r.Errors = append(r.Errors, err)
}

func (r *RecordingObserver) OnPoolEvent(event PoolEvent) {
	r.PoolEvents = append(r.PoolEvents, event)
}

// Verify interface compliance.
var _ Observer = nopObserver{}
var _ Observer = (*RecordingObserver)(nil)

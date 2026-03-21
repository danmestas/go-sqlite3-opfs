package opfsvfs

import (
	"sync/atomic"
)

// Stats holds atomic performance counters. Zero allocation on read.
type Stats struct {
	Reads        atomic.Int64
	Writes       atomic.Int64
	Flushes      atomic.Int64
	BytesRead    atomic.Int64
	BytesWritten atomic.Int64
	ReadTimeNs   atomic.Int64
	WriteTimeNs  atomic.Int64
	FlushTimeNs  atomic.Int64
}

// Snapshot returns a non-atomic copy of all counters.
type StatsSnapshot struct {
	Reads, Writes, Flushes             int64
	BytesRead, BytesWritten            int64
	ReadTimeNs, WriteTimeNs, FlushTimeNs int64
}

// Snapshot returns a point-in-time copy of the counters.
func (s *Stats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		Reads:        s.Reads.Load(),
		Writes:       s.Writes.Load(),
		Flushes:      s.Flushes.Load(),
		BytesRead:    s.BytesRead.Load(),
		BytesWritten: s.BytesWritten.Load(),
		ReadTimeNs:   s.ReadTimeNs.Load(),
		WriteTimeNs:  s.WriteTimeNs.Load(),
		FlushTimeNs:  s.FlushTimeNs.Load(),
	}
}

// Reset zeroes all counters.
func (s *Stats) Reset() {
	s.Reads.Store(0)
	s.Writes.Store(0)
	s.Flushes.Store(0)
	s.BytesRead.Store(0)
	s.BytesWritten.Store(0)
	s.ReadTimeNs.Store(0)
	s.WriteTimeNs.Store(0)
	s.FlushTimeNs.Store(0)
}

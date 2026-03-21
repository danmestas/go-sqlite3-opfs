//go:build js

package opfsvfs

import (
	"fmt"
	"io"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/util/vfsutil"
	"github.com/ncruces/go-sqlite3/vfs"
)

// Compile-time interface assertions.
var (
	_ vfs.VFS           = (*opfsVFS)(nil)
	_ vfs.File          = (*opfsFile)(nil)
	_ vfs.FileLockState = (*opfsFile)(nil)
)

// opfsVFS implements vfs.VFS using a pre-allocated pool of OPFS handles.
type opfsVFS struct {
	pool     *Pool
	stats    *Stats
	observer Observer
}

// opfsFile implements vfs.File backed by an OPFS Handle.
type opfsFile struct {
	handle   Handle
	name     string // virtual name
	slot     int
	lock     vfs.LockLevel
	stats    *Stats
	observer Observer
}

// Open implements vfs.VFS.
func (v *opfsVFS) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	const databases = vfs.OPEN_MAIN_DB | vfs.OPEN_TEMP_DB | vfs.OPEN_TRANSIENT_DB

	// Temp journals (used by the sorter) use an in-memory SliceFile.
	if flags&vfs.OPEN_TEMP_JOURNAL != 0 {
		return &vfsutil.SliceFile{}, flags | vfs.OPEN_MEMORY, nil
	}

	// Refuse non-database files. Returning OPEN_MEMORY tells SQLite
	// not to ask us to open journals — they use in-memory journals automatically.
	if flags&databases == 0 {
		return nil, flags, sqlite3.CANTOPEN
	}

	// Precondition: name must not be empty for database files.
	if name == "" {
		return nil, flags, sqlite3.CANTOPEN
	}

	handle, slot, err := v.pool.Acquire(name)
	if err != nil {
		return nil, flags, sqlite3.CANTOPEN
	}

	return &opfsFile{
		handle:   handle,
		name:     name,
		slot:     slot,
		lock:     vfs.LOCK_NONE,
		stats:    v.stats,
		observer: v.observer,
	}, flags | vfs.OPEN_MEMORY, nil
}

// Delete implements vfs.VFS.
func (v *opfsVFS) Delete(name string, syncDir bool) error {
	if v.pool.Has(name) {
		return v.pool.Release(name)
	}
	return sqlite3.IOERR_DELETE_NOENT
}

// Access implements vfs.VFS.
func (v *opfsVFS) Access(name string, flags vfs.AccessFlag) (bool, error) {
	return v.pool.Has(name), nil
}

// FullPathname implements vfs.VFS.
func (v *opfsVFS) FullPathname(name string) (string, error) {
	return name, nil
}

// ReadAt implements io.ReaderAt.
func (f *opfsFile) ReadAt(b []byte, off int64) (int, error) {
	// Preconditions.
	if f.handle == nil {
		panic("opfsvfs: ReadAt called on file with nil handle")
	}
	if off < 0 {
		panic(fmt.Sprintf("opfsvfs: ReadAt negative offset %d", off))
	}
	if b == nil {
		panic("opfsvfs: ReadAt called with nil buffer")
	}

	start := time.Now()
	n, err := f.handle.Read(b, off)
	dur := time.Since(start)

	f.stats.Reads.Add(1)
	f.stats.BytesRead.Add(int64(n))
	f.stats.ReadTimeNs.Add(dur.Nanoseconds())
	f.observer.OnRead(f.name, off, n, dur, err)

	// Short read: SQLite expects io.EOF when fewer bytes than requested.
	if n < len(b) && err == nil {
		return n, io.EOF
	}
	return n, err
}

// WriteAt implements io.WriterAt.
func (f *opfsFile) WriteAt(b []byte, off int64) (int, error) {
	// Preconditions.
	if f.handle == nil {
		panic("opfsvfs: WriteAt called on file with nil handle")
	}
	if off < 0 {
		panic(fmt.Sprintf("opfsvfs: WriteAt negative offset %d", off))
	}
	if b == nil {
		panic("opfsvfs: WriteAt called with nil buffer")
	}

	start := time.Now()
	n, err := f.handle.Write(b, off)
	dur := time.Since(start)

	f.stats.Writes.Add(1)
	f.stats.BytesWritten.Add(int64(n))
	f.stats.WriteTimeNs.Add(dur.Nanoseconds())
	f.observer.OnWrite(f.name, off, n, dur, err)

	return n, err
}

// Size implements vfs.File.
func (f *opfsFile) Size() (int64, error) {
	if f.handle == nil {
		panic("opfsvfs: Size called on file with nil handle")
	}
	return f.handle.GetSize()
}

// Truncate implements vfs.File.
func (f *opfsFile) Truncate(size int64) error {
	if f.handle == nil {
		panic("opfsvfs: Truncate called on file with nil handle")
	}
	return f.handle.Truncate(size)
}

// Sync implements vfs.File.
func (f *opfsFile) Sync(flags vfs.SyncFlag) error {
	if f.handle == nil {
		panic("opfsvfs: Sync called on file with nil handle")
	}

	start := time.Now()
	err := f.handle.Flush()
	dur := time.Since(start)

	f.stats.Flushes.Add(1)
	f.stats.FlushTimeNs.Add(dur.Nanoseconds())
	f.observer.OnFlush(f.name, dur, err)

	return err
}

// Close implements vfs.File.
// The handle is NOT closed here — the pool owns its lifecycle.
func (f *opfsFile) Close() error {
	return nil
}

// Lock implements vfs.File.
// No-op state machine: single connection per Worker, no real locking needed.
func (f *opfsFile) Lock(lock vfs.LockLevel) error {
	if f.lock >= lock {
		return nil
	}
	f.lock = lock
	return nil
}

// Unlock implements vfs.File.
// No-op state machine: single connection per Worker, no real locking needed.
func (f *opfsFile) Unlock(lock vfs.LockLevel) error {
	if f.lock <= lock {
		return nil
	}
	f.lock = lock
	return nil
}

// CheckReservedLock implements vfs.File.
func (f *opfsFile) CheckReservedLock() (bool, error) {
	return f.lock >= vfs.LOCK_RESERVED, nil
}

// LockState implements vfs.FileLockState.
func (f *opfsFile) LockState() vfs.LockLevel {
	return f.lock
}

// SectorSize implements vfs.File.
func (f *opfsFile) SectorSize() int {
	return 4096
}

// DeviceCharacteristics implements vfs.File.
func (f *opfsFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	return vfs.IOCAP_ATOMIC |
		vfs.IOCAP_SEQUENTIAL |
		vfs.IOCAP_SAFE_APPEND |
		vfs.IOCAP_POWERSAFE_OVERWRITE
}

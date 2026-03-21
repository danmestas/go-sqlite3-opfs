//go:build js

package opfsvfs

import (
	"io"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/util/vfsutil"
	"github.com/ncruces/go-sqlite3/vfs"
)

var (
	_ vfs.VFS           = (*opfsVFS)(nil)
	_ vfs.File          = (*opfsFile)(nil)
	_ vfs.FileLockState = (*opfsFile)(nil)
)

// opfsVFS implements vfs.VFS using named OPFS file handles.
type opfsVFS struct {
	handles  map[string]Handle
	stats    *Stats
	observer Observer
}

// opfsFile implements vfs.File backed by a named OPFS Handle.
type opfsFile struct {
	handle   Handle
	name     string
	flags    vfs.OpenFlag
	lock     vfs.LockLevel
	stats    *Stats
	observer Observer
}

func (v *opfsVFS) Open(name string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	// Temp journals use in-memory SliceFile (sorter temp files).
	if flags&vfs.OPEN_TEMP_JOURNAL != 0 {
		return &vfsutil.SliceFile{}, flags | vfs.OPEN_MEMORY, nil
	}

	// Temp/transient DBs and delete-on-close files use memory.
	if name == "" || flags&vfs.OPEN_DELETEONCLOSE != 0 {
		return &vfsutil.SliceFile{}, flags | vfs.OPEN_MEMORY, nil
	}

	// Look up the pre-registered OPFS handle by name.
	h, ok := v.handles[name]
	if !ok {
		return nil, flags, sqlite3.CANTOPEN
	}

	return &opfsFile{
		handle:   h,
		name:     name,
		flags:    flags,
		lock:     vfs.LOCK_NONE,
		stats:    v.stats,
		observer: v.observer,
	}, flags, nil
}

func (v *opfsVFS) Delete(name string, syncDir bool) error {
	h, ok := v.handles[name]
	if !ok {
		return sqlite3.IOERR_DELETE_NOENT
	}
	// Truncate to zero — handle stays registered for reuse.
	return h.Truncate(0)
}

func (v *opfsVFS) Access(name string, flags vfs.AccessFlag) (bool, error) {
	h, ok := v.handles[name]
	if !ok {
		return false, nil
	}
	if flags == vfs.ACCESS_EXISTS {
		// File "exists" if handle is registered and has data.
		size, err := h.GetSize()
		if err != nil {
			return false, nil
		}
		return size > 0, nil
	}
	// Read/write access — always available for registered handles.
	return true, nil
}

func (v *opfsVFS) FullPathname(name string) (string, error) {
	return name, nil
}

// --- opfsFile methods ---

func (f *opfsFile) ReadAt(b []byte, off int64) (int, error) {
	if f.handle == nil {
		panic("opfsvfs: ReadAt on nil handle")
	}

	start := time.Now()
	n, err := f.handle.Read(b, off)
	dur := time.Since(start)

	f.stats.Reads.Add(1)
	f.stats.BytesRead.Add(int64(n))
	f.stats.ReadTimeNs.Add(dur.Nanoseconds())
	f.observer.OnRead(f.name, off, n, dur, err)

	if n < len(b) && err == nil {
		return n, io.EOF
	}
	return n, err
}

func (f *opfsFile) WriteAt(b []byte, off int64) (int, error) {
	if f.handle == nil {
		panic("opfsvfs: WriteAt on nil handle")
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

func (f *opfsFile) Size() (int64, error) {
	return f.handle.GetSize()
}

func (f *opfsFile) Truncate(size int64) error {
	return f.handle.Truncate(size)
}

func (f *opfsFile) Sync(flags vfs.SyncFlag) error {
	start := time.Now()
	err := f.handle.Flush()
	dur := time.Since(start)

	f.stats.Flushes.Add(1)
	f.stats.FlushTimeNs.Add(dur.Nanoseconds())
	f.observer.OnFlush(f.name, dur, err)

	return err
}

// Close is a no-op — OPFS handles are owned by the VFS, not individual files.
func (f *opfsFile) Close() error { return nil }

// Lock/Unlock: no-op state machine. Single connection per Worker.
func (f *opfsFile) Lock(lock vfs.LockLevel) error {
	if f.lock < lock {
		f.lock = lock
	}
	return nil
}

func (f *opfsFile) Unlock(lock vfs.LockLevel) error {
	if f.lock > lock {
		f.lock = lock
	}
	return nil
}

func (f *opfsFile) CheckReservedLock() (bool, error) {
	return f.lock >= vfs.LOCK_RESERVED, nil
}

func (f *opfsFile) LockState() vfs.LockLevel { return f.lock }

func (f *opfsFile) SectorSize() int { return 4096 }

func (f *opfsFile) DeviceCharacteristics() vfs.DeviceCharacteristic {
	return vfs.IOCAP_ATOMIC |
		vfs.IOCAP_SEQUENTIAL |
		vfs.IOCAP_SAFE_APPEND |
		vfs.IOCAP_POWERSAFE_OVERWRITE
}

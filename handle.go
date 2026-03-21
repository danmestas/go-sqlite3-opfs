package opfsvfs

// Handle abstracts a FileSystemSyncAccessHandle.
// On js builds, wraps syscall/js. All methods are synchronous.
type Handle interface {
	Read(buf []byte, offset int64) (int, error)
	Write(buf []byte, offset int64) (int, error)
	GetSize() (int64, error)
	Truncate(size int64) error
	Flush() error
	Close() error
}

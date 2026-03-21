package opfsvfs

import "fmt"

// OpfsError provides structured context for OPFS I/O failures.
type OpfsError struct {
	Op     string // read, write, truncate, flush, getSize, close
	File   string // OPFS filename (e.g. "test.db", "test.db-wal")
	Offset int64  // byte offset, -1 if not applicable
	Size   int    // requested bytes, -1 if not applicable
	Err    error  // underlying error
}

func (e *OpfsError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("opfs: %s %s at offset %d len %d: %v",
			e.Op, e.File, e.Offset, e.Size, e.Err)
	}
	return fmt.Sprintf("opfs: %s %s: %v", e.Op, e.File, e.Err)
}

func (e *OpfsError) Unwrap() error { return e.Err }

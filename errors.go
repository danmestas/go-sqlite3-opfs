package opfsvfs

import "fmt"

// OpfsError provides structured context for OPFS I/O failures.
type OpfsError struct {
	Op     string // read, write, truncate, flush, getSize, close
	File   string // slot filename (e.g. "slot-3.db")
	Name   string // virtual name (e.g. "myrepo.fossil")
	Offset int64  // -1 if not applicable
	Size   int    // requested bytes, -1 if not applicable
	Err    error  // underlying error
}

func (e *OpfsError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("opfs: %s %s (%s) at offset %d len %d: %v",
			e.Op, e.File, e.Name, e.Offset, e.Size, e.Err)
	}
	return fmt.Sprintf("opfs: %s %s (%s): %v", e.Op, e.File, e.Name, e.Err)
}

func (e *OpfsError) Unwrap() error { return e.Err }

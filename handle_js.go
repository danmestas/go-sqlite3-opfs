//go:build js

package opfsvfs

import (
	"fmt"
	"syscall/js"
)

// jsHandle wraps a FileSystemSyncAccessHandle via syscall/js.
// All methods are synchronous — OPFS sync access handles do not return promises.
type jsHandle struct {
	val  js.Value
	name string // OPFS filename for error messages
}

func newJSHandle(val js.Value, name string) *jsHandle {
	if val.IsUndefined() || val.IsNull() {
		panic("opfsvfs: jsHandle value must not be nil")
	}
	if name == "" {
		panic("opfsvfs: jsHandle name must not be empty")
	}
	return &jsHandle{val: val, name: name}
}

// callSafe recovers from JS panics and returns them as errors.
func callSafe(fn func()) error {
	var jsErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				jsErr = fmt.Errorf("%v", r)
			}
		}()
		fn()
	}()
	return jsErr
}

func (h *jsHandle) Read(buf []byte, offset int64) (n int, err error) {
	// Preconditions: offset must be non-negative, buf must be non-nil.
	if offset < 0 {
		panic(fmt.Sprintf("opfsvfs: Read negative offset %d", offset))
	}
	if buf == nil {
		panic("opfsvfs: Read called with nil buffer")
	}
	if len(buf) == 0 {
		return 0, nil
	}

	jsBuf := js.Global().Get("Uint8Array").New(len(buf))
	opts := js.Global().Get("Object").New()
	opts.Set("at", offset)

	if jsErr := callSafe(func() {
		n = h.val.Call("read", jsBuf, opts).Int()
	}); jsErr != nil {
		return 0, &OpfsError{Op: "read", File: h.name, Offset: offset, Size: len(buf), Err: jsErr}
	}

	// Postcondition: n must be within buffer bounds.
	if n < 0 || n > len(buf) {
		panic(fmt.Sprintf("opfsvfs: Read returned n=%d for buf len %d", n, len(buf)))
	}

	js.CopyBytesToGo(buf[:n], jsBuf)
	return n, nil
}

func (h *jsHandle) Write(buf []byte, offset int64) (n int, err error) {
	// Preconditions: offset must be non-negative, buf must be non-nil.
	if offset < 0 {
		panic(fmt.Sprintf("opfsvfs: Write negative offset %d", offset))
	}
	if buf == nil {
		panic("opfsvfs: Write called with nil buffer")
	}
	if len(buf) == 0 {
		return 0, nil
	}

	jsBuf := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(jsBuf, buf)
	opts := js.Global().Get("Object").New()
	opts.Set("at", offset)

	if jsErr := callSafe(func() {
		n = h.val.Call("write", jsBuf, opts).Int()
	}); jsErr != nil {
		return 0, &OpfsError{Op: "write", File: h.name, Offset: offset, Size: len(buf), Err: jsErr}
	}

	// Postcondition: n must be within buffer bounds.
	if n < 0 || n > len(buf) {
		panic(fmt.Sprintf("opfsvfs: Write returned n=%d for buf len %d", n, len(buf)))
	}

	return n, nil
}

func (h *jsHandle) GetSize() (size int64, err error) {
	if jsErr := callSafe(func() {
		size = int64(h.val.Call("getSize").Int())
	}); jsErr != nil {
		return 0, &OpfsError{Op: "getSize", File: h.name, Offset: -1, Size: -1, Err: jsErr}
	}
	// Postcondition: size must be non-negative.
	if size < 0 {
		panic(fmt.Sprintf("opfsvfs: GetSize returned negative size %d", size))
	}
	return size, nil
}

func (h *jsHandle) Truncate(size int64) error {
	// Precondition: size must be non-negative.
	if size < 0 {
		panic(fmt.Sprintf("opfsvfs: Truncate negative size %d", size))
	}
	if jsErr := callSafe(func() {
		h.val.Call("truncate", size)
	}); jsErr != nil {
		return &OpfsError{Op: "truncate", File: h.name, Offset: size, Size: -1, Err: jsErr}
	}
	return nil
}

func (h *jsHandle) Flush() error {
	if jsErr := callSafe(func() {
		h.val.Call("flush")
	}); jsErr != nil {
		return &OpfsError{Op: "flush", File: h.name, Offset: -1, Size: -1, Err: jsErr}
	}
	return nil
}

func (h *jsHandle) Close() error {
	if jsErr := callSafe(func() {
		h.val.Call("close")
	}); jsErr != nil {
		return &OpfsError{Op: "close", File: h.name, Offset: -1, Size: -1, Err: jsErr}
	}
	return nil
}

// Verify interface compliance.
var _ Handle = (*jsHandle)(nil)
var _ error = (*OpfsError)(nil)
var _ interface{ Unwrap() error } = (*OpfsError)(nil)

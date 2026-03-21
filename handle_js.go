//go:build js

package opfsvfs

import (
	"fmt"
	"syscall/js"
)

// jsHandle wraps a FileSystemSyncAccessHandle via syscall/js.
type jsHandle struct {
	val  js.Value
	slot string // slot filename for error messages
	name string // virtual name for error messages
}

func newJSHandle(val js.Value, slot, name string) *jsHandle {
	if val.IsUndefined() || val.IsNull() {
		panic("opfsvfs: jsHandle value must not be nil")
	}
	return &jsHandle{val: val, slot: slot, name: name}
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
	jsBuf := js.Global().Get("Uint8Array").New(len(buf))
	opts := js.Global().Get("Object").New()
	opts.Set("at", offset)

	if jsErr := callSafe(func() {
		n = h.val.Call("read", jsBuf, opts).Int()
	}); jsErr != nil {
		return 0, &OpfsError{Op: "read", File: h.slot, Name: h.name, Offset: offset, Size: len(buf), Err: jsErr}
	}

	js.CopyBytesToGo(buf[:n], jsBuf)
	return n, nil
}

func (h *jsHandle) Write(buf []byte, offset int64) (n int, err error) {
	jsBuf := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(jsBuf, buf)
	opts := js.Global().Get("Object").New()
	opts.Set("at", offset)

	if jsErr := callSafe(func() {
		n = h.val.Call("write", jsBuf, opts).Int()
	}); jsErr != nil {
		return 0, &OpfsError{Op: "write", File: h.slot, Name: h.name, Offset: offset, Size: len(buf), Err: jsErr}
	}

	return n, nil
}

func (h *jsHandle) GetSize() (size int64, err error) {
	if jsErr := callSafe(func() {
		size = int64(h.val.Call("getSize").Int())
	}); jsErr != nil {
		return 0, &OpfsError{Op: "getSize", File: h.slot, Name: h.name, Offset: -1, Size: -1, Err: jsErr}
	}
	return size, nil
}

func (h *jsHandle) Truncate(size int64) error {
	if jsErr := callSafe(func() {
		h.val.Call("truncate", size)
	}); jsErr != nil {
		return &OpfsError{Op: "truncate", File: h.slot, Name: h.name, Offset: size, Size: -1, Err: jsErr}
	}
	return nil
}

func (h *jsHandle) Flush() error {
	if jsErr := callSafe(func() {
		h.val.Call("flush")
	}); jsErr != nil {
		return &OpfsError{Op: "flush", File: h.slot, Name: h.name, Offset: -1, Size: -1, Err: jsErr}
	}
	return nil
}

func (h *jsHandle) Close() error {
	if jsErr := callSafe(func() {
		h.val.Call("close")
	}); jsErr != nil {
		return &OpfsError{Op: "close", File: h.slot, Name: h.name, Offset: -1, Size: -1, Err: jsErr}
	}
	return nil
}

// Verify interface compliance.
var _ Handle = (*jsHandle)(nil)
var _ error = (*OpfsError)(nil)
var _ interface{ Unwrap() error } = (*OpfsError)(nil)

//go:build js

package opfsvfs

import (
	"fmt"
	"syscall/js"

	"github.com/ncruces/go-sqlite3/vfs"
)

// Options configures the OPFS VFS.
type Options struct {
	Name     string   // VFS name (default: "opfs")
	Observer Observer // optional observer
}

// DefaultOptions returns options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		Name: "opfs",
	}
}

// global state set during init
var (
	globalPool *Pool
	globalVFS  *opfsVFS
	globalOpts Options
)

func init() {
	Register()
}

// Register registers the _opfs_pool_init and _opfs_stats JS callbacks.
// Called automatically from init().
func Register() {
	globalOpts = DefaultOptions()

	// Register _opfs_pool_init callback.
	js.Global().Set("_opfs_pool_init", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			panic("opfsvfs: _opfs_pool_init requires 1 argument (handles array)")
		}
		jsHandles := args[0]
		length := jsHandles.Length()

		handles := make([]Handle, length)
		for i := 0; i < length; i++ {
			slot := fmt.Sprintf("slot-%d.db", i)
			handles[i] = newJSHandle(jsHandles.Index(i), slot, "")
		}

		globalPool = NewPool(handles, globalOpts.Observer)
		globalVFS = &opfsVFS{
			pool:     globalPool,
			stats:    globalPool.Stats(),
			observer: resolveObserver(globalOpts.Observer),
		}

		vfs.Register(globalOpts.Name, globalVFS)
		return nil
	}))

	// Register _opfs_stats callback.
	js.Global().Set("_opfs_stats", js.FuncOf(func(this js.Value, args []js.Value) any {
		if globalPool == nil {
			return nil
		}
		snap := globalPool.Stats().Snapshot()
		obj := js.Global().Get("Object").New()
		obj.Set("reads", snap.Reads)
		obj.Set("writes", snap.Writes)
		obj.Set("flushes", snap.Flushes)
		obj.Set("bytesRead", snap.BytesRead)
		obj.Set("bytesWritten", snap.BytesWritten)
		obj.Set("slotHits", snap.SlotHits)
		obj.Set("slotAllocs", snap.SlotAllocs)
		obj.Set("slotFull", snap.SlotFull)
		return obj
	}))
}

// New creates an OPFS VFS with custom options. Must be called before _opfs_pool_init.
func New(opts Options) {
	if opts.Name == "" {
		opts.Name = "opfs"
	}
	globalOpts = opts
}

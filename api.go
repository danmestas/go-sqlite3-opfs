//go:build js

package opfsvfs

import (
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

var (
	globalVFS  *opfsVFS
	globalOpts Options
	initReady  = make(chan struct{}) // closed when _opfs_init completes
)

func init() {
	Register()
}

// Register registers the _opfs_init and _opfs_stats JS callbacks.
func Register() {
	globalOpts = DefaultOptions()

	js.Global().Set("_opfs_init", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			panic("opfsvfs: _opfs_init requires 1 argument (handles object)")
		}

		jsHandles := args[0]
		keys := js.Global().Get("Object").Call("keys", jsHandles)
		handles := make(map[string]Handle, keys.Length())
		for i := 0; i < keys.Length(); i++ {
			name := keys.Index(i).String()
			handles[name] = newJSHandle(jsHandles.Get(name), name, name)
		}

		globalVFS = &opfsVFS{
			handles:  handles,
			stats:    &Stats{},
			observer: resolveObserver(globalOpts.Observer),
		}

		vfs.Register(globalOpts.Name, globalVFS)
		close(initReady)
		return nil
	}))

	js.Global().Set("_opfs_stats", js.FuncOf(func(this js.Value, args []js.Value) any {
		if globalVFS == nil {
			return nil
		}
		snap := globalVFS.stats.Snapshot()
		obj := js.Global().Get("Object").New()
		obj.Set("reads", snap.Reads)
		obj.Set("writes", snap.Writes)
		obj.Set("flushes", snap.Flushes)
		obj.Set("bytesRead", snap.BytesRead)
		obj.Set("bytesWritten", snap.BytesWritten)
		return obj
	}))
}

// New sets custom options. Must be called before _opfs_init.
func New(opts Options) {
	if opts.Name == "" {
		opts.Name = "opfs"
	}
	globalOpts = opts
}

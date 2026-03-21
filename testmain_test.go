//go:build js

package opfsvfs

import (
	"os"
	"testing"

	"github.com/danmestas/go-sqlite3-opfs/testharness"
)

func TestMain(m *testing.M) {
	// Block until _opfs_pool_init is called from JS.
	// This yields to the JS event loop, allowing the Worker to call
	// _opfs_pool_init(handles) after go.run() starts the Go runtime.
	<-poolReady

	code := m.Run()
	testharness.PostDone(code == 0, "")
	os.Exit(code)
}

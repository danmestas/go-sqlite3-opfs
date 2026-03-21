//go:build !js

package opfsvfs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danmestas/go-sqlite3-opfs/testharness"
)

func TestHarnessSmoke(t *testing.T) {
	// For now, just test that we can build WASM and start the server.
	// Real test orchestration comes after we have Go code to test.

	buildDir, err := testharness.BuildWASM(".")
	if err != nil {
		t.Skipf("Cannot build WASM (expected until we have js-tagged code): %v", err)
	}
	defer os.RemoveAll(buildDir)

	// Copy assets.
	assetsDir := filepath.Join("testharness")
	if err := testharness.CopyTestAssets(buildDir, assetsDir); err != nil {
		t.Fatalf("CopyTestAssets: %v", err)
	}

	srv, err := testharness.NewServer(buildDir)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()
	srv.Start()

	t.Logf("Test server at %s", srv.URL())

	// Full browser test will be wired in Task 8.
	_ = time.Second
}

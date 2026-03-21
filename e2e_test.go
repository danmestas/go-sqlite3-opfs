//go:build !js

package opfsvfs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danmestas/go-sqlite3-opfs/testharness"
)

func TestBrowserE2E(t *testing.T) {
	if os.Getenv("OPFS_E2E") == "" {
		t.Skip("Set OPFS_E2E=1 to run browser end-to-end tests (requires Chrome)")
	}

	// Step 1: Build the WASM test binary.
	buildDir, err := testharness.BuildWASM(".")
	if err != nil {
		t.Fatalf("BuildWASM: %v", err)
	}
	defer os.RemoveAll(buildDir)
	t.Logf("WASM build dir: %s", buildDir)

	// Step 2: Copy test assets (worker.js, index.html).
	assetsDir := filepath.Join("testharness")
	if err := testharness.CopyTestAssets(buildDir, assetsDir); err != nil {
		t.Fatalf("CopyTestAssets: %v", err)
	}

	// Step 3: Start the test server with COOP/COEP headers.
	srv, err := testharness.NewServer(buildDir)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()
	srv.Start()
	t.Logf("Test server at %s", srv.URL())

	// Step 4: Launch headless Chrome and run tests.
	timeout := 60 * time.Second
	msgs, err := testharness.RunInBrowser(srv.URL(), timeout)
	if err != nil {
		t.Logf("Browser messages collected before error:")
		for _, m := range msgs {
			t.Logf("  [%s] %s", m.Type, m.Text)
		}
		t.Fatalf("RunInBrowser: %v", err)
	}

	// Step 5: Log all messages and check for pass/fail.
	t.Log("Browser test output:")
	passed := false
	for _, m := range msgs {
		t.Logf("  [%s] %s", m.Type, m.Text)
		if strings.Contains(m.Text, `"passed":true`) && strings.Contains(m.Text, `"type":"done"`) {
			passed = true
		}
	}

	if !passed {
		t.Fatal("Browser tests did not report passed:true in done message")
	}
}

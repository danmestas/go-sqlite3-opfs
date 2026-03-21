//go:build !js

package testharness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Harness manages the lifecycle of a browser test run.
type Harness struct {
	server *Server
	tmpDir string
	cancel context.CancelFunc
}

// BuildWASM compiles the test binary for js/wasm.
// Returns the output directory containing testbin.wasm + support files.
func BuildWASM(pkg string) (string, error) {
	dir, err := os.MkdirTemp("", "opfs-vfs-test-*")
	if err != nil {
		return "", err
	}

	// Build the WASM test binary.
	outPath := filepath.Join(dir, "testbin.wasm")
	cmd := exec.Command("go", "test", "-c",
		"-tags", "ncruces",
		"-o", outPath,
		pkg,
	)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build WASM: %s: %w", out, err)
	}

	// Copy wasm_exec.js from GOROOT.
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		out, _ := exec.Command("go", "env", "GOROOT").Output()
		goroot = strings.TrimSpace(string(out))
	}
	execJS := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js")
	data, err := os.ReadFile(execJS)
	if err != nil {
		// Try alternate path.
		execJS = filepath.Join(goroot, "misc", "wasm", "wasm_exec.js")
		data, err = os.ReadFile(execJS)
		if err != nil {
			return "", fmt.Errorf("wasm_exec.js not found: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), data, 0644); err != nil {
		return "", fmt.Errorf("write wasm_exec.js: %w", err)
	}

	return dir, nil
}

// CopyTestAssets copies worker.js and index.html to the build directory.
func CopyTestAssets(buildDir, assetsDir string) error {
	for _, name := range []string{"worker.js", "index.html"} {
		data, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, name), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// RunInBrowser launches headless Chrome, navigates to the test page,
// and collects console messages until tests complete or timeout.
// Completion is detected by polling the #log element for a "done" message
// posted by the Go test binary via testharness.PostDone.
func RunInBrowser(url string, timeout time.Duration) ([]ConsoleMsg, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	var msgs []ConsoleMsg

	// Capture main-thread console messages (errors, logs from index.html).
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if consoleEv, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			for _, arg := range consoleEv.Args {
				text := strings.Trim(string(arg.Value), `"`)
				msgs = append(msgs, ConsoleMsg{
					Type: string(consoleEv.Type),
					Text: text,
				})
			}
		}
	})

	// Navigate and enable runtime events so we get console messages.
	if err := chromedp.Run(ctx,
		runtime.Enable(),
		chromedp.Navigate(url),
	); err != nil {
		return msgs, fmt.Errorf("navigate: %w", err)
	}

	// Poll the #log div for the "done" message posted by the Worker.
	// The index.html page appends all Worker postMessage data as JSON lines.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return msgs, fmt.Errorf("timeout waiting for test completion: %w", ctx.Err())
		case <-ticker.C:
			var logText string
			err := chromedp.Run(ctx,
				chromedp.Text("#log", &logText, chromedp.ByID),
			)
			if err != nil {
				continue // element may not exist yet
			}
			if strings.Contains(logText, `"type":"done"`) {
				// Parse individual lines into ConsoleMsg entries.
				for _, line := range strings.Split(logText, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					msgs = append(msgs, ConsoleMsg{Type: "worker", Text: line})
				}
				return msgs, nil
			}
			// Also check for error messages from the Worker.
			if strings.Contains(logText, `"type":"error"`) && !strings.Contains(logText, `"type":"log"`) {
				// Only error, no log — likely a fatal error before tests ran.
				for _, line := range strings.Split(logText, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					msgs = append(msgs, ConsoleMsg{Type: "worker", Text: line})
				}
				return msgs, fmt.Errorf("worker error detected in log")
			}
		}
	}
}

// ConsoleMsg represents a message from the browser console.
type ConsoleMsg struct {
	Type string // "log", "error", "result", "done"
	Text string
}

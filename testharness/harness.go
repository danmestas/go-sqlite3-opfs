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
// Caller must call os.RemoveAll on the returned directory when done.
func BuildWASM(pkg string) (dir string, err error) {
	dir, err = os.MkdirTemp("", "opfs-vfs-test-*")
	if err != nil {
		return "", err
	}
	// Clean up on any error after this point.
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()

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

	if err := copyWasmExecJS(dir); err != nil {
		return "", err
	}

	return dir, nil
}

// copyWasmExecJS copies wasm_exec.js from GOROOT into dir.
func copyWasmExecJS(dir string) error {
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		out, _ := exec.Command("go", "env", "GOROOT").Output()
		goroot = strings.TrimSpace(string(out))
	}
	execJS := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js")
	data, err := os.ReadFile(execJS)
	if err != nil {
		execJS = filepath.Join(goroot, "misc", "wasm", "wasm_exec.js")
		data, err = os.ReadFile(execJS)
		if err != nil {
			return fmt.Errorf("wasm_exec.js not found: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), data, 0644); err != nil {
		return fmt.Errorf("write wasm_exec.js: %w", err)
	}
	return nil
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
	if url == "" {
		panic("testharness: RunInBrowser url must not be empty")
	}

	ctx, msgs, cleanup, err := launchBrowser(url, timeout)
	if err != nil {
		return *msgs, err
	}
	defer cleanup()

	return pollForCompletion(ctx, msgs)
}

// launchBrowser creates a Chrome context and navigates to the URL.
func launchBrowser(url string, timeout time.Duration) (
	context.Context, *[]ConsoleMsg, func(), error,
) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)

	cleanup := func() {
		timeoutCancel()
		ctxCancel()
		allocCancel()
	}

	msgs := &[]ConsoleMsg{}

	// Capture main-thread console messages.
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if consoleEv, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			for _, arg := range consoleEv.Args {
				text := strings.Trim(string(arg.Value), `"`)
				*msgs = append(*msgs, ConsoleMsg{
					Type: string(consoleEv.Type),
					Text: text,
				})
			}
		}
	})

	if err := chromedp.Run(ctx,
		runtime.Enable(),
		chromedp.Navigate(url),
	); err != nil {
		cleanup()
		return nil, msgs, nil, fmt.Errorf("navigate: %w", err)
	}

	return ctx, msgs, cleanup, nil
}

// pollForCompletion polls the #log div until "done" or error is detected.
func pollForCompletion(ctx context.Context, msgs *[]ConsoleMsg) ([]ConsoleMsg, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	const maxPolls = 600 // Safety bound: 5 minutes at 500ms intervals.
	for polls := 0; polls < maxPolls; polls++ {
		select {
		case <-ctx.Done():
			return *msgs, fmt.Errorf("timeout waiting for test completion: %w", ctx.Err())
		case <-ticker.C:
			var logText string
			if err := chromedp.Run(ctx,
				chromedp.Text("#log", &logText, chromedp.ByID),
			); err != nil {
				continue
			}
			if result, err := parseLogResult(logText, msgs); result {
				return *msgs, err
			}
		}
	}
	return *msgs, fmt.Errorf("exceeded maximum poll count (%d)", maxPolls)
}

// parseLogResult checks log text for done/error markers and appends lines to msgs.
func parseLogResult(logText string, msgs *[]ConsoleMsg) (done bool, err error) {
	if strings.Contains(logText, `"type":"done"`) {
		appendLogLines(logText, msgs)
		return true, nil
	}
	if strings.Contains(logText, `"type":"error"`) && !strings.Contains(logText, `"type":"log"`) {
		appendLogLines(logText, msgs)
		return true, fmt.Errorf("worker error detected in log")
	}
	return false, nil
}

func appendLogLines(logText string, msgs *[]ConsoleMsg) {
	for _, line := range strings.Split(logText, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			*msgs = append(*msgs, ConsoleMsg{Type: "worker", Text: line})
		}
	}
}

// ConsoleMsg represents a message from the browser console.
type ConsoleMsg struct {
	Type string // "log", "error", "result", "done"
	Text string
}

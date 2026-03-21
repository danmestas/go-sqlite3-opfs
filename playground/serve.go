//go:build !js

// Playground server: builds the playground WASM binary and serves it with
// COOP/COEP headers required for SharedArrayBuffer (needed by wazero).
//
// Usage: go run ./playground

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	dir, err := os.MkdirTemp("", "opfs-playground-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	log.Println("Building playground WASM...")
	if err := buildWASM(dir); err != nil {
		log.Fatalf("Build failed: %v", err)
	}

	if err := copyAssets(dir); err != nil {
		log.Fatalf("Copy assets failed: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	url := "http://" + ln.Addr().String()
	log.Printf("Playground ready at %s", url)
	log.Println("Press Ctrl+C to stop")

	// Try to open browser.
	openBrowser(url)

	fs := http.FileServer(http.Dir(dir))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		fs.ServeHTTP(w, r)
	})
	if err := http.Serve(ln, handler); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Fatal(err)
	}
}

func buildWASM(dir string) error {
	out := filepath.Join(dir, "playground.wasm")
	cmd := exec.Command("go", "build", "-tags", "ncruces", "-o", out, ".")
	cmd.Dir = "playground"
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	// Copy wasm_exec.js.
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		out, _ := exec.Command("go", "env", "GOROOT").Output()
		goroot = strings.TrimSpace(string(out))
	}
	for _, p := range []string{
		filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(goroot, "misc", "wasm", "wasm_exec.js"),
	} {
		data, err := os.ReadFile(p)
		if err == nil {
			return os.WriteFile(filepath.Join(dir, "wasm_exec.js"), data, 0644)
		}
	}
	return fmt.Errorf("wasm_exec.js not found in GOROOT")
}

func copyAssets(dir string) error {
	for _, name := range []string{"index.html", "worker.js"} {
		data, err := os.ReadFile(filepath.Join("playground", name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "start"
	default:
		return
	}
	exec.Command(cmd, url).Start()
}

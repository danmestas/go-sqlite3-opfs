// Web Worker: creates named OPFS files, loads Go WASM test binary,
// registers handles, and runs tests.
//
// Why a Worker? OPFS createSyncAccessHandle() only works in dedicated Workers.
// Why intercept console? Go WASM stdout goes to console.log in the Worker,
// which is invisible to chromedp. Forwarding via postMessage makes it visible.
// Why _opfs_init after go.run? Go's init() registers _opfs_init during go.run(),
// so it must be called after go.run() starts but before tests need the VFS.

const DB_NAME = "test.db";
const OPFS_DIR = "sqlite3-opfs";
const RUN_TIMEOUT_MS = 120000; // 2 minutes max for test execution.

// Intercept console so Go test output is forwarded via postMessage.
const _origLog = console.log;
const _origError = console.error;
console.log = function(...args) {
    postMessage({ type: "stdout", text: args.join(" ") });
    _origLog.apply(console, args);
};
console.error = function(...args) {
    postMessage({ type: "stderr", text: args.join(" ") });
    _origError.apply(console, args);
};

function log(msg) {
    postMessage({ type: "log", text: msg });
}

async function initHandles(dbName) {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle(OPFS_DIR, { create: true });
    const handles = {};
    const suffixes = ["", "-journal", "-wal"];
    for (const suffix of suffixes) {
        const name = dbName + suffix;
        const fh = await dir.getFileHandle(name, { create: true });
        handles[name] = await fh.createSyncAccessHandle();
    }
    return handles;
}

async function run() {
    try {
        log("Creating OPFS files for: " + DB_NAME);
        const handles = await initHandles(DB_NAME);
        log(`OPFS files ready: ${Object.keys(handles).join(", ")}`);

        log("Loading Go WASM test binary...");
        importScripts("wasm_exec.js");
        const go = new Go();

        go.argv = ["test.wasm", "-test.v"];
        if (self._testArgs) {
            go.argv = go.argv.concat(self._testArgs);
        }

        const result = await WebAssembly.instantiateStreaming(
            fetch("testbin.wasm"), go.importObject
        );

        log("Starting Go WASM...");
        const exitPromise = go.run(result.instance);

        // _opfs_init is available because Go's init() registered it during go.run().
        log("Registering OPFS handles...");
        _opfs_init(handles);

        log("Running tests...");
        // Race against timeout to prevent hanging Worker.
        const timeout = new Promise((_, reject) =>
            setTimeout(() => reject(new Error("test execution timeout")), RUN_TIMEOUT_MS)
        );
        await Promise.race([exitPromise, timeout]);
        log("Go program exited.");
    } catch (e) {
        postMessage({ type: "error", text: e.message + "\n" + (e.stack || "") });
    }
}

// Single entry point: auto-start on Worker creation.
// The onmessage handler is for future use (e.g., passing args before start).
run();

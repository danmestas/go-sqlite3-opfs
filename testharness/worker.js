// Web Worker: initializes OPFS handle pool, loads Go WASM test binary,
// runs tests, and posts results back to the orchestrator.
const POOL_SIZE = 6;
const PREFIX = "sqlite3-opfs-test";

function log(msg) {
    postMessage({ type: "log", text: msg });
}

async function initPool() {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle(PREFIX, { create: true });
    const handles = [];
    for (let i = 0; i < POOL_SIZE; i++) {
        const fh = await dir.getFileHandle(`slot-${i}.db`, { create: true });
        handles.push(await fh.createSyncAccessHandle());
    }
    return handles;
}

async function run() {
    try {
        log("Initializing OPFS pool...");
        const handles = await initPool();
        log(`Pool ready: ${handles.length} slots`);

        log("Loading Go WASM test binary...");
        importScripts("wasm_exec.js");
        const go = new Go();

        // Pass test flags via argv if present.
        if (self._testArgs) {
            go.argv = ["test.wasm"].concat(self._testArgs);
        }

        const result = await WebAssembly.instantiateStreaming(
            fetch("testbin.wasm"), go.importObject
        );

        log("Starting Go WASM...");
        const exitPromise = go.run(result.instance);

        // Register handles with Go pool.
        log("Registering OPFS handles...");
        _opfs_pool_init(handles);

        log("Running tests...");
        // Tests run via Go's testing framework inside the WASM.
        // Wait for the Go program to exit, then signal completion.
        await exitPromise;
        log("Go program exited.");
    } catch (e) {
        postMessage({ type: "error", text: e.message + "\n" + e.stack });
    }
}

self.onmessage = function(e) {
    if (e.data.type === "run") {
        if (e.data.args) {
            self._testArgs = e.data.args;
        }
        run();
    }
};

// Auto-start.
run();

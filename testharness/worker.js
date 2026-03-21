// Web Worker: creates named OPFS files, loads Go WASM test binary,
// registers handles, and runs tests.
const DB_NAME = "test.db";
const OPFS_DIR = "sqlite3-opfs";

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
    // Open all files SQLite may need for this database.
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

        log("Registering OPFS handles...");
        _opfs_init(handles);

        log("Running tests...");
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

run();

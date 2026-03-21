// Playground Worker: creates OPFS files, loads the playground WASM app,
// and bridges JS ↔ Go for the interactive SQL console.

const DB_NAME = "playground.db";
const OPFS_DIR = "sqlite3-opfs-playground";

const _origLog = console.log;
const _origError = console.error;
console.log = function(...args) {
    postMessage({ type: "log", text: args.join(" ") });
    _origLog.apply(console, args);
};
console.error = function(...args) {
    postMessage({ type: "error", text: args.join(" ") });
    _origError.apply(console, args);
};

function log(msg) {
    postMessage({ type: "log", text: msg });
}

// Bridge: app.go calls these globals to communicate with the UI.
self._app_error = function(msg) {
    postMessage({ type: "error", text: msg });
};
self._app_ready = function() {
    postMessage({ type: "ready" });
};

async function initHandles(dbName) {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle(OPFS_DIR, { create: true });
    const handles = {};
    for (const suffix of ["", "-journal", "-wal"]) {
        const name = dbName + suffix;
        const fh = await dir.getFileHandle(name, { create: true });
        handles[name] = await fh.createSyncAccessHandle();
    }
    return handles;
}

async function run() {
    try {
        log("Creating OPFS files...");
        const handles = await initHandles(DB_NAME);
        log("OPFS ready: " + Object.keys(handles).join(", "));

        log("Loading WASM...");
        importScripts("wasm_exec.js");
        const go = new Go();

        const result = await WebAssembly.instantiateStreaming(
            fetch("playground.wasm"), go.importObject
        );

        log("Starting Go...");
        go.run(result.instance);

        // Register OPFS handles with the VFS.
        _opfs_init(handles);

        // Signal app.go that OPFS is ready — it's waiting on _app_init.
        _app_init();

        log("App initialized.");
    } catch (e) {
        postMessage({ type: "error", text: e.message + "\n" + (e.stack || "") });
    }
}

// Handle queries from the UI.
self.onmessage = function(e) {
    if (e.data.type === "query") {
        try {
            const result = _query(e.data.sql);
            postMessage({ type: "result", data: result });
        } catch (err) {
            postMessage({ type: "result", data: JSON.stringify({ error: err.message }) });
        }
    } else if (e.data.type === "schema") {
        try {
            const result = _schema();
            postMessage({ type: "schema", data: result });
        } catch (err) {
            postMessage({ type: "schema", data: JSON.stringify({ error: err.message }) });
        }
    }
};

run();

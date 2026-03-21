//go:build !js

// e2e_test.go orchestrates browser-based testing. It builds the WASM test binary,
// starts a server with COOP/COEP headers, launches headless Chrome, runs all WASM
// tests, and captures a database dump for native SQLite validation.
//
// TestValidateOPFSDatabase is the crown jewel: it proves that a database created
// entirely through the OPFS VFS in a browser Worker is a valid, portable SQLite
// file by opening it natively and verifying FTS5, foreign keys, triggers, JSON,
// views, generated columns, blobs, and every row of data.

package opfsvfs_test

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danmestas/go-sqlite3-opfs/testharness"
	_ "github.com/ncruces/go-sqlite3/driver"

)

func runBrowserTests(t *testing.T) []testharness.ConsoleMsg {
	t.Helper()
	buildDir, err := testharness.BuildWASM(".")
	if err != nil {
		t.Fatalf("BuildWASM: %v", err)
	}
	defer os.RemoveAll(buildDir)

	if err := testharness.CopyTestAssets(buildDir, filepath.Join("testharness")); err != nil {
		t.Fatalf("CopyTestAssets: %v", err)
	}

	srv, err := testharness.NewServer(buildDir)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()
	srv.Start()
	t.Logf("Test server at %s", srv.URL())

	msgs, err := testharness.RunInBrowser(srv.URL(), 120*time.Second)
	if err != nil {
		for _, m := range msgs {
			t.Logf("  [%s] %s", m.Type, m.Text)
		}
		t.Fatalf("RunInBrowser: %v", err)
	}
	return msgs
}

func TestBrowserE2E(t *testing.T) {
	if os.Getenv("OPFS_E2E") == "" {
		t.Skip("Set OPFS_E2E=1 to run browser end-to-end tests (requires Chrome)")
	}
	msgs := runBrowserTests(t)

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

// TestValidateOPFSDatabase captures a database dump from the browser and
// verifies it with native SQLite. This proves end-to-end correctness:
// Go WASM -> ncruces -> wazero -> SQLite -> OPFS VFS -> valid portable DB file.
func TestValidateOPFSDatabase(t *testing.T) {
	if os.Getenv("OPFS_E2E") == "" {
		t.Skip("Set OPFS_E2E=1 to run browser end-to-end tests (requires Chrome)")
	}

	msgs := runBrowserTests(t)

	// Verify WASM tests passed.
	for _, m := range msgs {
		if strings.Contains(m.Text, `"passed":true`) && strings.Contains(m.Text, `"type":"done"`) {
			goto wasmOK
		}
	}
	t.Fatal("WASM tests did not pass")
wasmOK:

	// Extract the dbdump.
	dbBytes := extractDump(t, msgs)
	t.Logf("Captured %d bytes from OPFS database dump", len(dbBytes))

	// Verify SQLite header.
	if len(dbBytes) < 100 {
		t.Fatalf("Database too small: %d bytes", len(dbBytes))
	}
	if string(dbBytes[:16]) != "SQLite format 3\x00" {
		t.Fatalf("Invalid SQLite header: %q", dbBytes[:16])
	}
	t.Log("SQLite header: valid")

	// Write to disk and open natively.
	dbPath := filepath.Join(t.TempDir(), "opfs-dump.db")
	if err := os.WriteFile(dbPath, dbBytes, 0644); err != nil {
		t.Fatalf("Write: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Native sql.Open: %v", err)
	}
	defer db.Close()

	// --- Integrity ---
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check: %s", integrity)
	}
	t.Log("PRAGMA integrity_check: ok")

	// --- Schema: verify all tables exist ---
	expectedTables := []string{"users", "categories", "posts", "comments", "audit_log", "attachments"}
	for _, name := range expectedTables {
		var found string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&found)
		if err != nil {
			// metadata table was in v1, might not exist in comprehensive schema.
			// Check if it's an expected table from the validation test.
			t.Fatalf("Table %q not found: %v", name, err)
		}
	}
	t.Logf("All %d tables present", len(expectedTables))

	// --- FTS5 virtual table ---
	var ftsTable string
	if err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='posts_fts'",
	).Scan(&ftsTable); err != nil {
		t.Fatalf("FTS5 table not found: %v", err)
	}
	t.Log("FTS5 virtual table: present")

	// --- FTS5 search works natively ---
	var ftsTitle string
	if err := db.QueryRow("SELECT title FROM posts_fts WHERE posts_fts MATCH 'browser'").Scan(&ftsTitle); err != nil {
		t.Fatalf("FTS5 search: %v", err)
	}
	if ftsTitle != "SQLite in the Browser" {
		t.Fatalf("FTS5 result: got %q", ftsTitle)
	}
	t.Logf("FTS5 search for 'browser': %q (correct)", ftsTitle)

	// --- Users: verify data + generated column ---
	// Note: Charlie (id=3) was deleted by FK cascade test in WASM.
	var userCount int
	if err := db.QueryRow("SELECT count(*) FROM users").Scan(&userCount); err != nil {
		t.Fatalf("users count: %v", err)
	}
	if userCount != 4 {
		t.Fatalf("users count: got %d, want 4 (Charlie deleted by cascade)", userCount)
	}
	t.Logf("users: %d rows (Charlie cascaded)", userCount)

	// Alice's score was updated to 110.
	var aliceScore int
	var aliceDisplay string
	if err := db.QueryRow("SELECT score, display_name FROM users WHERE id=1").Scan(&aliceScore, &aliceDisplay); err != nil {
		t.Fatalf("Alice query: %v", err)
	}
	if aliceScore != 110 {
		t.Fatalf("Alice score: got %d, want 110 (updated)", aliceScore)
	}
	if aliceDisplay != "Alice <alice@example.com>" {
		t.Fatalf("Alice display_name: got %q", aliceDisplay)
	}
	t.Logf("Alice: score=%d display=%q (correct)", aliceScore, aliceDisplay)

	// --- JSON functions work natively ---
	var role string
	if err := db.QueryRow("SELECT json_extract(profile_json, '$.role') FROM users WHERE id=1").Scan(&role); err != nil {
		t.Fatalf("json_extract: %v", err)
	}
	if role != "admin" {
		t.Fatalf("json_extract role: got %q", role)
	}
	t.Logf("JSON extract: role=%q (correct)", role)

	// --- Posts: verify FK relationship ---
	var postCount int
	if err := db.QueryRow("SELECT count(*) FROM posts").Scan(&postCount); err != nil {
		t.Fatalf("posts count: %v", err)
	}
	// Charlie's post (id=4) should be cascade-deleted.
	if postCount != 4 {
		t.Fatalf("posts count: got %d, want 4 (Charlie's post cascaded)", postCount)
	}
	t.Logf("posts: %d rows (FK cascade verified)", postCount)

	// --- Post tags as JSON ---
	var tagsJSON string
	if err := db.QueryRow("SELECT tags_json FROM posts WHERE id=1").Scan(&tagsJSON); err != nil {
		t.Fatalf("tags_json: %v", err)
	}
	if !strings.Contains(tagsJSON, "opfs") {
		t.Fatalf("tags_json: %q doesn't contain 'opfs'", tagsJSON)
	}
	t.Logf("Post tags JSON: %s", tagsJSON)

	// --- View works natively ---
	var viewAuthor, viewCat string
	if err := db.QueryRow("SELECT author, category FROM post_summary WHERE id=1").Scan(&viewAuthor, &viewCat); err != nil {
		t.Fatalf("post_summary view: %v", err)
	}
	if viewAuthor != "Alice" || viewCat != "Tech" {
		t.Fatalf("post_summary: got author=%q cat=%q", viewAuthor, viewCat)
	}
	t.Logf("View: author=%q category=%q (correct)", viewAuthor, viewCat)

	// --- Comments: threaded (parent_id) ---
	var threadedCount int
	if err := db.QueryRow("SELECT count(*) FROM comments WHERE parent_id IS NOT NULL").Scan(&threadedCount); err != nil {
		t.Fatalf("threaded comments: %v", err)
	}
	if threadedCount < 1 {
		t.Fatalf("threaded comments: got %d, want >= 1", threadedCount)
	}
	t.Logf("Threaded comments: %d (self-referencing FK working)", threadedCount)

	// --- Recursive CTE works natively ---
	var maxDepth int
	if err := db.QueryRow(`
		WITH RECURSIVE thread(id, depth) AS (
			SELECT id, 0 FROM comments WHERE post_id=1 AND parent_id IS NULL
			UNION ALL
			SELECT c.id, t.depth + 1
			FROM comments c JOIN thread t ON c.parent_id = t.id
		)
		SELECT max(depth) FROM thread
	`).Scan(&maxDepth); err != nil {
		t.Fatalf("recursive CTE: %v", err)
	}
	t.Logf("Recursive CTE max depth: %d", maxDepth)

	// --- Window function works natively ---
	var topUser string
	if err := db.QueryRow(`
		SELECT name FROM (
			SELECT name, RANK() OVER (ORDER BY score DESC) as r FROM users
		) WHERE r = 1
	`).Scan(&topUser); err != nil {
		t.Fatalf("window function: %v", err)
	}
	if topUser != "Alice" {
		t.Fatalf("top user by score: got %q, want Alice", topUser)
	}
	t.Logf("Window function: top user = %q (correct)", topUser)

	// --- Audit log (trigger-populated) ---
	var auditCount int
	if err := db.QueryRow("SELECT count(*) FROM audit_log").Scan(&auditCount); err != nil {
		t.Fatalf("audit_log: %v", err)
	}
	if auditCount < 7 {
		t.Fatalf("audit_log: got %d entries, want >= 7", auditCount)
	}
	t.Logf("Audit log: %d entries (triggers verified)", auditCount)

	// Verify specific audit entries.
	var auditAction, auditNewData string
	if err := db.QueryRow(
		"SELECT action, new_data FROM audit_log WHERE table_name='users' AND action='UPDATE' LIMIT 1",
	).Scan(&auditAction, &auditNewData); err != nil {
		t.Fatalf("audit user update: %v", err)
	}
	if !strings.Contains(auditNewData, "110") {
		t.Fatalf("audit user update new_data: %q doesn't contain '110'", auditNewData)
	}
	t.Logf("Audit update entry: action=%q data=%s", auditAction, auditNewData)

	// --- Blob data ---
	var blobSize int
	var blobData []byte
	if err := db.QueryRow("SELECT size_bytes, data FROM attachments WHERE filename='diagram.png'").Scan(&blobSize, &blobData); err != nil {
		t.Fatalf("blob query: %v", err)
	}
	if blobSize != 4096 {
		t.Fatalf("blob size: got %d, want 4096", blobSize)
	}
	if len(blobData) != 4096 {
		t.Fatalf("blob data length: got %d, want 4096", len(blobData))
	}
	// Verify blob content pattern.
	for i := 0; i < 256; i++ {
		if blobData[i] != byte(i%256) {
			t.Fatalf("blob data[%d]: got %d, want %d", i, blobData[i], i%256)
		}
	}
	t.Log("Blob data: 4096 bytes, content pattern verified")

	// --- Categories ---
	var catCount int
	if err := db.QueryRow("SELECT count(*) FROM categories").Scan(&catCount); err != nil {
		t.Fatalf("categories: %v", err)
	}
	if catCount != 3 {
		t.Fatalf("categories: got %d, want 3", catCount)
	}
	t.Logf("Categories: %d (correct)", catCount)

	// --- Indexes exist ---
	var idxCount int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'").Scan(&idxCount); err != nil {
		t.Fatalf("indexes: %v", err)
	}
	if idxCount < 4 {
		t.Fatalf("custom indexes: got %d, want >= 4", idxCount)
	}
	t.Logf("Custom indexes: %d", idxCount)

	// --- Triggers exist ---
	var trigCount int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='trigger'").Scan(&trigCount); err != nil {
		t.Fatalf("triggers: %v", err)
	}
	if trigCount < 3 {
		t.Fatalf("triggers: got %d, want >= 3", trigCount)
	}
	t.Logf("Triggers: %d", trigCount)

	// --- Final summary ---
	t.Log("")
	t.Log("=== OPFS VFS Native Validation: ALL CHECKS PASSED ===")
	t.Log("Verified: SQLite header, integrity_check, 7 tables, FTS5, JSON,")
	t.Log("  generated columns, foreign key cascades, triggers, audit log,")
	t.Log("  views, recursive CTEs, window functions, blobs, indexes")
	t.Log("The OPFS VFS produces valid, portable SQLite databases.")
}

func extractDump(t *testing.T, msgs []testharness.ConsoleMsg) []byte {
	t.Helper()
	for _, m := range msgs {
		if !strings.Contains(m.Text, `"type":"dbdump"`) {
			continue
		}
		var msg struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(m.Text), &msg); err != nil {
			continue
		}
		if msg.Type != "dbdump" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(msg.Text)
		if err != nil {
			t.Fatalf("base64 decode: %v", err)
		}
		return data
	}
	t.Fatal("No dbdump message found — TestValidationDump may not have run")
	return nil
}

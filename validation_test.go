//go:build js

// validation_test.go creates a comprehensive SQLite database exercising every
// feature someone might use in a browser app: FTS5, foreign keys, triggers,
// indexes, views, JSON functions, CTEs, window functions, generated columns,
// blobs, and multi-table joins. It then dumps the raw bytes for native validation.

package opfsvfs

import (
	"database/sql"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/danmestas/go-sqlite3-opfs/testharness"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// TestValidationDump creates a feature-rich database and dumps raw bytes.
// This test MUST run last (alphabetically "V" > all other test prefixes).
func TestValidationDump(t *testing.T) {
	truncateAll(t)

	db, err := sql.Open("sqlite3", "file:test.db?vfs=opfs")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Enable foreign keys (off by default in SQLite).
	exec(t, db, "PRAGMA foreign_keys = ON")

	// --- Schema: users with generated column ---
	exec(t, db, `CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL UNIQUE,
		score INTEGER NOT NULL DEFAULT 0,
		profile_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		display_name TEXT GENERATED ALWAYS AS (name || ' <' || email || '>') STORED
	)`)

	// --- Schema: categories + posts with foreign keys ---
	exec(t, db, `CREATE TABLE categories (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT
	)`)

	exec(t, db, `CREATE TABLE posts (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		tags_json TEXT NOT NULL DEFAULT '[]',
		view_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	// --- Schema: comments with self-referencing FK (threaded) ---
	exec(t, db, `CREATE TABLE comments (
		id INTEGER PRIMARY KEY,
		post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		parent_id INTEGER REFERENCES comments(id) ON DELETE CASCADE,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	// --- Schema: audit log populated by triggers ---
	exec(t, db, `CREATE TABLE audit_log (
		id INTEGER PRIMARY KEY,
		table_name TEXT NOT NULL,
		action TEXT NOT NULL,
		row_id INTEGER NOT NULL,
		old_data TEXT,
		new_data TEXT,
		timestamp TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	// --- Schema: blob storage ---
	exec(t, db, `CREATE TABLE attachments (
		id INTEGER PRIMARY KEY,
		post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
		filename TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		data BLOB NOT NULL,
		size_bytes INTEGER NOT NULL
	)`)

	// --- Indexes ---
	exec(t, db, "CREATE INDEX idx_posts_user ON posts(user_id)")
	exec(t, db, "CREATE INDEX idx_posts_category ON posts(category_id)")
	exec(t, db, "CREATE INDEX idx_comments_post ON comments(post_id)")
	exec(t, db, "CREATE INDEX idx_comments_parent ON comments(parent_id)")

	// --- FTS5 full-text search ---
	exec(t, db, `CREATE VIRTUAL TABLE posts_fts USING fts5(
		title, body, content=posts, content_rowid=id
	)`)

	// --- Triggers: audit log + FTS sync ---
	exec(t, db, `CREATE TRIGGER posts_ai AFTER INSERT ON posts BEGIN
		INSERT INTO audit_log(table_name, action, row_id, new_data)
		VALUES('posts', 'INSERT', NEW.id, json_object('title', NEW.title));
		INSERT INTO posts_fts(rowid, title, body) VALUES(NEW.id, NEW.title, NEW.body);
	END`)

	exec(t, db, `CREATE TRIGGER posts_au AFTER UPDATE ON posts BEGIN
		INSERT INTO audit_log(table_name, action, row_id, old_data, new_data)
		VALUES('posts', 'UPDATE', NEW.id,
			json_object('title', OLD.title),
			json_object('title', NEW.title));
		INSERT INTO posts_fts(posts_fts, rowid, title, body) VALUES('delete', OLD.id, OLD.title, OLD.body);
		INSERT INTO posts_fts(rowid, title, body) VALUES(NEW.id, NEW.title, NEW.body);
	END`)

	exec(t, db, `CREATE TRIGGER users_au AFTER UPDATE ON users BEGIN
		INSERT INTO audit_log(table_name, action, row_id, old_data, new_data)
		VALUES('users', 'UPDATE', NEW.id,
			json_object('score', OLD.score),
			json_object('score', NEW.score));
	END`)

	// --- View: post summaries with join ---
	exec(t, db, `CREATE VIEW post_summary AS
		SELECT
			p.id, p.title, u.name AS author, c.name AS category,
			p.view_count, p.tags_json,
			(SELECT count(*) FROM comments cm WHERE cm.post_id = p.id) AS comment_count
		FROM posts p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN categories c ON c.id = p.category_id
	`)

	// --- Insert data ---
	users := []struct {
		name, email, profile string
		score                int
	}{
		{"Alice", "alice@example.com", `{"role":"admin","theme":"dark"}`, 100},
		{"Bob", "bob@example.com", `{"role":"editor","theme":"light"}`, 85},
		{"Charlie", "charlie@example.com", `{"role":"viewer","lang":"es"}`, 92},
		{"Diana", "diana@example.com", `{"role":"editor","notifications":true}`, 78},
		{"Eve", "eve@example.com", `{"role":"admin","2fa":true}`, 95},
	}
	for _, u := range users {
		exec(t, db, "INSERT INTO users(name, email, score, profile_json) VALUES(?, ?, ?, ?)",
			u.name, u.email, u.score, u.profile)
	}

	categories := []struct{ name, desc string }{
		{"Tech", "Technology and programming"},
		{"Science", "Scientific discoveries"},
		{"Art", "Creative works"},
	}
	for _, c := range categories {
		exec(t, db, "INSERT INTO categories(name, description) VALUES(?, ?)", c.name, c.desc)
	}

	posts := []struct {
		userID, catID int
		title, body   string
		tags          string
	}{
		{1, 1, "Getting Started with OPFS", "The Origin Private File System...", `["opfs","wasm","browser"]`},
		{2, 1, "SQLite in the Browser", "SQLite can now run natively...", `["sqlite","wasm"]`},
		{1, 2, "Quantum Computing Update", "Recent advances in quantum...", `["quantum","physics"]`},
		{3, 3, "Digital Art Techniques", "Modern approaches to digital...", `["art","digital"]`},
		{4, 1, "WebAssembly Performance", "Benchmarking WASM vs native...", `["wasm","performance"]`},
	}
	for _, p := range posts {
		exec(t, db, "INSERT INTO posts(user_id, category_id, title, body, tags_json) VALUES(?, ?, ?, ?, ?)",
			p.userID, p.catID, p.title, p.body, p.tags)
	}

	// Comments with threading (parent_id).
	exec(t, db, "INSERT INTO comments(post_id, user_id, body) VALUES(1, 2, 'Great article!')")
	exec(t, db, "INSERT INTO comments(post_id, user_id, body) VALUES(1, 3, 'Very informative')")
	exec(t, db, "INSERT INTO comments(post_id, user_id, parent_id, body) VALUES(1, 1, 1, 'Thanks Bob!')")
	exec(t, db, "INSERT INTO comments(post_id, user_id, body) VALUES(2, 4, 'Exciting times')")
	exec(t, db, "INSERT INTO comments(post_id, user_id, parent_id, body) VALUES(2, 2, 4, 'Indeed!')")

	// Binary attachments.
	blob := make([]byte, 4096)
	for i := range blob {
		blob[i] = byte(i % 256)
	}
	exec(t, db, "INSERT INTO attachments(post_id, filename, mime_type, data, size_bytes) VALUES(1, 'diagram.png', 'image/png', ?, ?)",
		blob, len(blob))
	exec(t, db, "INSERT INTO attachments(post_id, filename, mime_type, data, size_bytes) VALUES(2, 'benchmark.csv', 'text/csv', ?, ?)",
		[]byte("test,value\na,1\nb,2"), 18)

	// Update to trigger audit log.
	exec(t, db, "UPDATE posts SET view_count = 42 WHERE id = 1")
	exec(t, db, "UPDATE users SET score = 110 WHERE id = 1")

	// --- Verify features work in WASM before dumping ---

	// FTS5 search.
	var ftsTitle string
	if err := db.QueryRow("SELECT title FROM posts_fts WHERE posts_fts MATCH 'browser'").Scan(&ftsTitle); err != nil {
		t.Fatalf("FTS5 search: %v", err)
	}
	if ftsTitle != "SQLite in the Browser" {
		t.Fatalf("FTS5: got %q, want %q", ftsTitle, "SQLite in the Browser")
	}
	t.Log("FTS5 search: ok")

	// JSON functions.
	var role string
	if err := db.QueryRow("SELECT json_extract(profile_json, '$.role') FROM users WHERE id=1").Scan(&role); err != nil {
		t.Fatalf("json_extract: %v", err)
	}
	if role != "admin" {
		t.Fatalf("json_extract: got %q, want %q", role, "admin")
	}
	t.Log("JSON extract: ok")

	// Generated column.
	var display string
	if err := db.QueryRow("SELECT display_name FROM users WHERE id=1").Scan(&display); err != nil {
		t.Fatalf("generated column: %v", err)
	}
	if display != "Alice <alice@example.com>" {
		t.Fatalf("display_name: got %q", display)
	}
	t.Log("Generated column: ok")

	// View with join.
	var author, category string
	var commentCount int
	if err := db.QueryRow("SELECT author, category, comment_count FROM post_summary WHERE id=1").Scan(&author, &category, &commentCount); err != nil {
		t.Fatalf("view query: %v", err)
	}
	if author != "Alice" || category != "Tech" || commentCount != 3 {
		t.Fatalf("post_summary: got author=%q cat=%q comments=%d", author, category, commentCount)
	}
	t.Log("View with join: ok")

	// CTE: recursive comment thread.
	var threadDepth int
	if err := db.QueryRow(`
		WITH RECURSIVE thread(id, depth) AS (
			SELECT id, 0 FROM comments WHERE post_id=1 AND parent_id IS NULL
			UNION ALL
			SELECT c.id, t.depth + 1
			FROM comments c JOIN thread t ON c.parent_id = t.id
		)
		SELECT max(depth) FROM thread
	`).Scan(&threadDepth); err != nil {
		t.Fatalf("recursive CTE: %v", err)
	}
	if threadDepth != 1 {
		t.Fatalf("thread depth: got %d, want 1", threadDepth)
	}
	t.Log("Recursive CTE: ok")

	// Window function.
	var rank int
	if err := db.QueryRow(`
		SELECT rank FROM (
			SELECT id, RANK() OVER (ORDER BY score DESC) as rank FROM users
		) WHERE id = 1
	`).Scan(&rank); err != nil {
		t.Fatalf("window function: %v", err)
	}
	if rank != 1 {
		t.Fatalf("Alice rank: got %d, want 1 (highest score)", rank)
	}
	t.Log("Window function: ok")

	// Audit log (populated by triggers).
	var auditCount int
	if err := db.QueryRow("SELECT count(*) FROM audit_log").Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount < 7 {
		// 5 post inserts + 1 post update + 1 user update = 7 minimum
		t.Fatalf("audit_log count: got %d, want >= 7", auditCount)
	}
	t.Logf("Audit log: %d entries (triggers working)", auditCount)

	// Foreign key cascade: delete a user, verify posts and comments are gone.
	exec(t, db, "DELETE FROM users WHERE id = 3") // Charlie
	var charliePostCount int
	if err := db.QueryRow("SELECT count(*) FROM posts WHERE user_id = 3").Scan(&charliePostCount); err != nil {
		t.Fatalf("cascade check: %v", err)
	}
	if charliePostCount != 0 {
		t.Fatalf("FK cascade: Charlie's posts not deleted, count=%d", charliePostCount)
	}
	t.Log("Foreign key cascade: ok")

	// Integrity check before dump.
	var integrityResult string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if integrityResult != "ok" {
		t.Fatalf("integrity_check: %s", integrityResult)
	}
	t.Log("PRAGMA integrity_check: ok")

	// Close to flush everything.
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read raw bytes from OPFS handle and dump.
	h, ok := globalVFS.handles["test.db"]
	if !ok {
		t.Fatal("test.db handle not registered")
	}
	size, err := h.GetSize()
	if err != nil {
		t.Fatalf("GetSize: %v", err)
	}
	if size == 0 {
		t.Fatal("test.db is empty after writing data")
	}

	buf := make([]byte, size)
	n, err := h.Read(buf, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if int64(n) != size {
		t.Fatalf("Read: got %d bytes, want %d", n, size)
	}

	// Verify SQLite header magic.
	if string(buf[:16]) != "SQLite format 3\x00" {
		t.Fatalf("Invalid SQLite header: %q", buf[:16])
	}

	encoded := base64.StdEncoding.EncodeToString(buf[:n])
	testharness.PostMessage("dbdump", encoded)
	t.Logf("Dumped %d bytes for native validation (FTS5, FK, triggers, JSON, views, blobs, CTE, window functions)", n)
}

// exec is a helper that fatals on error.
func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", strings.TrimSpace(query[:min(len(query), 60)]), err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

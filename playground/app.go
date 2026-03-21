//go:build js

// Playground WASM app: creates a feature-rich demo database via the OPFS VFS,
// then exposes _query(sql) and _schema() JS callbacks for the interactive UI.

package main

import (
	"database/sql"
	"encoding/json"
	"syscall/js"

	_ "github.com/danmestas/go-sqlite3-opfs"
	_ "github.com/ncruces/go-sqlite3/driver"

)

var db *sql.DB

func main() {
	// Block until OPFS handles are registered by the Worker.
	// The _opfs_init callback (from opfsvfs) signals readiness.
	// We can't open the DB until then — wait for a JS signal.
	ready := make(chan struct{})

	js.Global().Set("_app_init", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		close(ready)
		return nil
	}))

	<-ready

	var err error
	db, err = sql.Open("sqlite3", "file:playground.db?vfs=opfs")
	if err != nil {
		js.Global().Call("_app_error", err.Error())
		select {}
	}
	db.SetMaxOpenConns(1)

	if err := createDemoSchema(); err != nil {
		js.Global().Call("_app_error", err.Error())
		select {}
	}

	// Register query callback.
	js.Global().Set("_query", js.FuncOf(queryCallback))
	js.Global().Set("_schema", js.FuncOf(schemaCallback))

	// Signal UI that app is ready.
	js.Global().Call("_app_ready", nil)

	// Keep alive.
	select {}
}

func queryCallback(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("_query requires 1 argument (SQL string)")
	}
	query := args[0].String()

	rows, err := db.Query(query)
	if err != nil {
		return errorResult(err.Error())
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return errorResult(err.Error())
	}

	var results []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return errorResult(err.Error())
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return errorResult(err.Error())
	}

	out, _ := json.Marshal(map[string]any{
		"columns": cols,
		"rows":    results,
		"count":   len(results),
	})
	return string(out)
}

func schemaCallback(_ js.Value, _ []js.Value) any {
	rows, err := db.Query("SELECT type, name, sql FROM sqlite_master ORDER BY type, name")
	if err != nil {
		return errorResult(err.Error())
	}
	defer rows.Close()

	var objects []map[string]string
	for rows.Next() {
		var typ, name string
		var ddl sql.NullString
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			continue
		}
		objects = append(objects, map[string]string{
			"type": typ,
			"name": name,
			"sql":  ddl.String,
		})
	}
	out, _ := json.Marshal(objects)
	return string(out)
}

func errorResult(msg string) string {
	out, _ := json.Marshal(map[string]any{"error": msg})
	return string(out)
}

func createDemoSchema() error {
	stmts := []string{
		"PRAGMA foreign_keys = ON",

		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			score INTEGER NOT NULL DEFAULT 0,
			profile_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			display_name TEXT GENERATED ALWAYS AS (name || ' <' || email || '>') STORED
		)`,

		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			tags_json TEXT NOT NULL DEFAULT '[]',
			view_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY,
			post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			parent_id INTEGER REFERENCES comments(id) ON DELETE CASCADE,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY,
			table_name TEXT NOT NULL,
			action TEXT NOT NULL,
			row_id INTEGER NOT NULL,
			old_data TEXT,
			new_data TEXT,
			timestamp TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		"CREATE INDEX IF NOT EXISTS idx_posts_user ON posts(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_posts_category ON posts(category_id)",
		"CREATE INDEX IF NOT EXISTS idx_comments_post ON comments(post_id)",

		`CREATE VIRTUAL TABLE IF NOT EXISTS posts_fts USING fts5(
			title, body, content=posts, content_rowid=id
		)`,

		`CREATE TRIGGER IF NOT EXISTS posts_ai AFTER INSERT ON posts BEGIN
			INSERT INTO audit_log(table_name, action, row_id, new_data)
			VALUES('posts', 'INSERT', NEW.id, json_object('title', NEW.title));
			INSERT INTO posts_fts(rowid, title, body) VALUES(NEW.id, NEW.title, NEW.body);
		END`,

		`CREATE TRIGGER IF NOT EXISTS posts_au AFTER UPDATE ON posts BEGIN
			INSERT INTO audit_log(table_name, action, row_id, old_data, new_data)
			VALUES('posts', 'UPDATE', NEW.id,
				json_object('title', OLD.title), json_object('title', NEW.title));
		END`,

		`CREATE TRIGGER IF NOT EXISTS users_au AFTER UPDATE ON users BEGIN
			INSERT INTO audit_log(table_name, action, row_id, old_data, new_data)
			VALUES('users', 'UPDATE', NEW.id,
				json_object('score', OLD.score), json_object('score', NEW.score));
		END`,

		`CREATE VIEW IF NOT EXISTS post_summary AS
			SELECT p.id, p.title, u.name AS author, c.name AS category,
				p.view_count, p.tags_json,
				(SELECT count(*) FROM comments cm WHERE cm.post_id = p.id) AS comment_count
			FROM posts p
			JOIN users u ON u.id = p.user_id
			LEFT JOIN categories c ON c.id = p.category_id`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	// Seed data (skip if already populated).
	var count int
	db.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if count > 0 {
		return nil
	}

	data := []string{
		`INSERT INTO users(name, email, score, profile_json) VALUES
			('Alice', 'alice@example.com', 100, '{"role":"admin","theme":"dark"}'),
			('Bob', 'bob@example.com', 85, '{"role":"editor","theme":"light"}'),
			('Charlie', 'charlie@example.com', 92, '{"role":"viewer","lang":"es"}'),
			('Diana', 'diana@example.com', 78, '{"role":"editor","notifications":true}'),
			('Eve', 'eve@example.com', 95, '{"role":"admin","2fa":true}')`,

		`INSERT INTO categories(name, description) VALUES
			('Tech', 'Technology and programming'),
			('Science', 'Scientific discoveries'),
			('Art', 'Creative works')`,

		`INSERT INTO posts(user_id, category_id, title, body, tags_json) VALUES
			(1, 1, 'Getting Started with OPFS', 'The Origin Private File System enables persistent storage in browser workers using synchronous file access handles.', '["opfs","wasm","browser"]'),
			(2, 1, 'SQLite in the Browser', 'SQLite can now run natively in the browser via WebAssembly, enabling full relational database capabilities.', '["sqlite","wasm"]'),
			(1, 2, 'Quantum Computing Update', 'Recent advances in quantum error correction have brought us closer to practical quantum computing.', '["quantum","physics"]'),
			(3, 3, 'Digital Art Techniques', 'Modern approaches to digital art combine traditional skills with computational tools.', '["art","digital"]'),
			(4, 1, 'WebAssembly Performance', 'Benchmarking WebAssembly against native code shows near-native performance for compute-heavy workloads.', '["wasm","performance"]')`,

		`INSERT INTO comments(post_id, user_id, body) VALUES
			(1, 2, 'Great article! OPFS is a game changer.'),
			(1, 3, 'Very informative, thanks for writing this.'),
			(2, 4, 'Exciting times for web development!')`,
		`INSERT INTO comments(post_id, user_id, parent_id, body) VALUES
			(1, 1, 1, 'Thanks Bob! Glad you found it useful.'),
			(2, 2, 3, 'Indeed! The possibilities are endless.')`,

		"UPDATE posts SET view_count = 142 WHERE id = 1",
		"UPDATE posts SET view_count = 89 WHERE id = 2",
		"UPDATE posts SET view_count = 56 WHERE id = 3",
	}

	for _, s := range data {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	return nil
}

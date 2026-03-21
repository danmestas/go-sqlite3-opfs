//go:build js

package opfsvfs

import (
	"database/sql"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	truncateAll(t)
	db, err := sql.Open("sqlite3", "file:test.db?vfs=opfs")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestIntegrationBasicCRUD(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t(name) VALUES(?)", "alice"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	var name string
	if err := db.QueryRow("SELECT name FROM t WHERE id=1").Scan(&name); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if name != "alice" {
		t.Fatalf("SELECT: got %q, want %q", name, "alice")
	}
}

func TestIntegrationPersistence(t *testing.T) {
	truncateAll(t)
	// Open, write, close
	db, err := sql.Open("sqlite3", "file:test.db?vfs=opfs")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE t(v TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t(v) VALUES('persistent')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	db.Close()

	// Reopen, read
	db2, err := sql.Open("sqlite3", "file:test.db?vfs=opfs")
	if err != nil {
		t.Fatalf("sql.Open reopen: %v", err)
	}
	db2.SetMaxOpenConns(1)
	defer db2.Close()
	var v string
	if err := db2.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatalf("SELECT after reopen: %v", err)
	}
	if v != "persistent" {
		t.Fatalf("SELECT: got %q, want %q", v, "persistent")
	}
}

func TestIntegrationBulkInsert(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v INTEGER)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	for i := 0; i < 100; i++ {
		if _, err := db.Exec("INSERT INTO t(v) VALUES(?)", i*2); err != nil {
			t.Fatalf("INSERT %d: %v", i, err)
		}
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("SELECT count: %v", err)
	}
	if count != 100 {
		t.Fatalf("count: got %d, want 100", count)
	}
}

func TestIntegrationTransactionRollback(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(v TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(v) VALUES('rolled back')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("SELECT count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count after rollback: got %d, want 0", count)
	}
}

func TestIntegrationTransactionCommit(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(v TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(v) VALUES('committed')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	var v string
	if err := db.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if v != "committed" {
		t.Fatalf("SELECT: got %q, want %q", v, "committed")
	}
}

func TestIntegrationWALMode(t *testing.T) {
	db := openTestDB(t)
	// WAL requires exclusive locking when SharedMemory is nil
	if _, err := db.Exec("PRAGMA locking_mode=EXCLUSIVE"); err != nil {
		t.Fatalf("PRAGMA locking_mode: %v", err)
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode: got %q, want %q", mode, "wal")
	}
	// Basic CRUD in WAL mode
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE in WAL: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t(v) VALUES('wal-data')"); err != nil {
		t.Fatalf("INSERT in WAL: %v", err)
	}
	var v string
	if err := db.QueryRow("SELECT v FROM t WHERE id=1").Scan(&v); err != nil {
		t.Fatalf("SELECT in WAL: %v", err)
	}
	if v != "wal-data" {
		t.Fatalf("SELECT: got %q, want %q", v, "wal-data")
	}
	t.Logf("WAL mode CRUD successful")
}

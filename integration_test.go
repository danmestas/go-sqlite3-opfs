//go:build js

package opfsvfs

import (
	"database/sql"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// TestIntegrationBasicRoundTrip tests opening a database, creating a table,
// inserting data, and selecting it back using database/sql.
func TestIntegrationBasicRoundTrip(t *testing.T) {
	dbName := "file:test-integration-basic.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-integration-basic.db", false)
	})

	// CREATE TABLE
	_, err = db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// INSERT
	_, err = db.Exec("INSERT INTO users (name) VALUES (?)", "Alice")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// SELECT
	var id int
	var name string
	err = db.QueryRow("SELECT id, name FROM users WHERE name = ?", "Alice").Scan(&id, &name)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if id != 1 {
		t.Errorf("SELECT: got id=%d, want 1", id)
	}
	if name != "Alice" {
		t.Errorf("SELECT: got name=%q, want %q", name, "Alice")
	}
}

// TestIntegrationPersistence tests that data persists after closing and
// reopening the same database.
func TestIntegrationPersistence(t *testing.T) {
	dbName := "file:test-integration-persist.db?vfs=opfs&nolock=1"

	// First connection: create table and insert data
	db1, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open (first): %v", err)
	}

	_, err = db1.Exec("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = db1.Exec("INSERT INTO products (name, price) VALUES (?, ?)", "Widget", 9.99)
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	err = db1.Close()
	if err != nil {
		t.Fatalf("Close (first): %v", err)
	}

	// Second connection: reopen and verify data persists
	db2, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open (second): %v", err)
	}

	t.Cleanup(func() {
		db2.Close()
		globalVFS.Delete("test-integration-persist.db", false)
	})

	var id int
	var name string
	var price float64
	err = db2.QueryRow("SELECT id, name, price FROM products WHERE name = ?", "Widget").Scan(&id, &name, &price)
	if err != nil {
		t.Fatalf("SELECT after reopen: %v", err)
	}

	if id != 1 {
		t.Errorf("After reopen: got id=%d, want 1", id)
	}
	if name != "Widget" {
		t.Errorf("After reopen: got name=%q, want %q", name, "Widget")
	}
	if price != 9.99 {
		t.Errorf("After reopen: got price=%f, want 9.99", price)
	}
}

// TestIntegrationMultipleDatabases tests opening two databases simultaneously
// with different names (different slots).
func TestIntegrationMultipleDatabases(t *testing.T) {
	dbName1 := "file:test-integration-multi-1.db?vfs=opfs&nolock=1"
	dbName2 := "file:test-integration-multi-2.db?vfs=opfs&nolock=1"

	db1, err := sql.Open("sqlite3", dbName1)
	if err != nil {
		t.Fatalf("sql.Open (db1): %v", err)
	}

	db2, err := sql.Open("sqlite3", dbName2)
	if err != nil {
		t.Fatalf("sql.Open (db2): %v", err)
	}

	t.Cleanup(func() {
		db1.Close()
		db2.Close()
		globalVFS.Delete("test-integration-multi-1.db", false)
		globalVFS.Delete("test-integration-multi-2.db", false)
	})

	// Create tables in both databases
	_, err = db1.Exec("CREATE TABLE data1 (value TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE (db1): %v", err)
	}

	_, err = db2.Exec("CREATE TABLE data2 (value TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE (db2): %v", err)
	}

	// Insert different data in each database
	_, err = db1.Exec("INSERT INTO data1 (value) VALUES (?)", "database-one")
	if err != nil {
		t.Fatalf("INSERT (db1): %v", err)
	}

	_, err = db2.Exec("INSERT INTO data2 (value) VALUES (?)", "database-two")
	if err != nil {
		t.Fatalf("INSERT (db2): %v", err)
	}

	// Verify data in db1
	var value1 string
	err = db1.QueryRow("SELECT value FROM data1").Scan(&value1)
	if err != nil {
		t.Fatalf("SELECT (db1): %v", err)
	}
	if value1 != "database-one" {
		t.Errorf("db1: got value=%q, want %q", value1, "database-one")
	}

	// Verify data in db2
	var value2 string
	err = db2.QueryRow("SELECT value FROM data2").Scan(&value2)
	if err != nil {
		t.Fatalf("SELECT (db2): %v", err)
	}
	if value2 != "database-two" {
		t.Errorf("db2: got value=%q, want %q", value2, "database-two")
	}
}

// TestIntegrationBulkInsertAndCount tests inserting 100 rows and verifying
// the count matches.
func TestIntegrationBulkInsertAndCount(t *testing.T) {
	dbName := "file:test-integration-bulk.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-integration-bulk.db", false)
	})

	// CREATE TABLE
	_, err = db.Exec("CREATE TABLE numbers (id INTEGER PRIMARY KEY, value INTEGER)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// INSERT 100 rows
	const rowCount = 100
	for i := 1; i <= rowCount; i++ {
		_, err = db.Exec("INSERT INTO numbers (value) VALUES (?)", i*10)
		if err != nil {
			t.Fatalf("INSERT row %d: %v", i, err)
		}
	}

	// SELECT count
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM numbers").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT COUNT: %v", err)
	}

	if count != rowCount {
		t.Errorf("SELECT COUNT: got %d, want %d", count, rowCount)
	}

	// Verify a few specific values
	var value int
	err = db.QueryRow("SELECT value FROM numbers WHERE id = ?", 50).Scan(&value)
	if err != nil {
		t.Fatalf("SELECT value for id=50: %v", err)
	}
	if value != 500 {
		t.Errorf("SELECT value for id=50: got %d, want 500", value)
	}
}

// TestIntegrationTransactionRollback tests that a ROLLBACK discards changes.
func TestIntegrationTransactionRollback(t *testing.T) {
	dbName := "file:test-integration-rollback.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-integration-rollback.db", false)
	})

	// CREATE TABLE
	_, err = db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// BEGIN transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("BEGIN: %v", err)
	}

	// INSERT within transaction
	_, err = tx.Exec("INSERT INTO items (name) VALUES (?)", "temp-item")
	if err != nil {
		t.Fatalf("INSERT in transaction: %v", err)
	}

	// ROLLBACK
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}

	// Verify item was NOT inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM items WHERE name = ?", "temp-item").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT COUNT after rollback: %v", err)
	}

	if count != 0 {
		t.Errorf("After ROLLBACK: found %d rows, want 0", count)
	}
}

// TestIntegrationTransactionCommit tests that a COMMIT persists changes.
func TestIntegrationTransactionCommit(t *testing.T) {
	dbName := "file:test-integration-commit.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-integration-commit.db", false)
	})

	// CREATE TABLE
	_, err = db.Exec("CREATE TABLE orders (id INTEGER PRIMARY KEY, item TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// BEGIN transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("BEGIN: %v", err)
	}

	// INSERT within transaction
	_, err = tx.Exec("INSERT INTO orders (item) VALUES (?)", "laptop")
	if err != nil {
		t.Fatalf("INSERT in transaction: %v", err)
	}

	// COMMIT
	err = tx.Commit()
	if err != nil {
		t.Fatalf("COMMIT: %v", err)
	}

	// Verify item WAS inserted
	var item string
	err = db.QueryRow("SELECT item FROM orders WHERE id = ?", 1).Scan(&item)
	if err != nil {
		t.Fatalf("SELECT after commit: %v", err)
	}

	if item != "laptop" {
		t.Errorf("After COMMIT: got item=%q, want %q", item, "laptop")
	}
}

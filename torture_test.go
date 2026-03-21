//go:build js

package opfsvfs

import (
	"database/sql"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// TestTortureLargeDB creates a 10MB database by inserting many large blobs.
// Verifies SQLite can handle large databases in OPFS.
func TestTortureLargeDB(t *testing.T) {
	dbName := "file:test-torture-large.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-torture-large.db", false)
	})

	// CREATE TABLE to store blobs
	_, err = db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, data BLOB)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// Insert ~1000 rows of 10KB blobs to reach ~10MB
	const blobSize = 10 * 1024 // 10KB
	const numBlobs = 1000
	blob := make([]byte, blobSize)
	for i := 0; i < blobSize; i++ {
		blob[i] = byte(i % 256)
	}

	t.Logf("Inserting %d blobs of %d bytes each...", numBlobs, blobSize)

	for i := 1; i <= numBlobs; i++ {
		_, err = db.Exec("INSERT INTO t (data) VALUES (?)", blob)
		if err != nil {
			t.Fatalf("INSERT row %d: %v", i, err)
		}
		if i%100 == 0 {
			t.Logf("  inserted %d rows...", i)
		}
	}

	// Verify SELECT count
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM t").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT COUNT: %v", err)
	}

	if count != numBlobs {
		t.Errorf("SELECT COUNT: got %d, want %d", count, numBlobs)
	}

	t.Logf("Successfully inserted and verified %d blobs", count)
}

// TestTortureRapidOpenClose performs rapid open/close cycles.
// Tests handle pooling and persistence under stress.
func TestTortureRapidOpenClose(t *testing.T) {
	dbName := "file:test-torture-rapid.db?vfs=opfs&nolock=1"
	const iterations = 50

	t.Logf("Running %d rapid open/close cycles...", iterations)

	for i := 1; i <= iterations; i++ {
		// Open database
		db, err := sql.Open("sqlite3", dbName)
		if err != nil {
			t.Fatalf("sql.Open (iteration %d): %v", i, err)
		}

		// Create table on first iteration
		if i == 1 {
			_, err = db.Exec("CREATE TABLE rapid (id INTEGER PRIMARY KEY, value INTEGER)")
			if err != nil {
				db.Close()
				t.Fatalf("CREATE TABLE (iteration %d): %v", i, err)
			}
		}

		// Insert a row
		_, err = db.Exec("INSERT INTO rapid (value) VALUES (?)", i*10)
		if err != nil {
			db.Close()
			t.Fatalf("INSERT (iteration %d): %v", i, err)
		}

		// Verify the row exists
		var value int
		err = db.QueryRow("SELECT value FROM rapid WHERE id = ?", i).Scan(&value)
		if err != nil {
			db.Close()
			t.Fatalf("SELECT (iteration %d): %v", i, err)
		}
		if value != i*10 {
			db.Close()
			t.Fatalf("SELECT (iteration %d): got value=%d, want %d", i, value, i*10)
		}

		// Close database
		err = db.Close()
		if err != nil {
			t.Fatalf("Close (iteration %d): %v", i, err)
		}

		if i%10 == 0 {
			t.Logf("  completed %d iterations...", i)
		}
	}

	// Final verification: reopen and verify all rows exist
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open (final): %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-torture-rapid.db", false)
	})

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM rapid").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT COUNT (final): %v", err)
	}

	if count != iterations {
		t.Errorf("Final count: got %d, want %d", count, iterations)
	}

	t.Logf("Successfully completed %d rapid open/close cycles", iterations)
}

// TestTortureBulkRows inserts 10,000 rows and verifies integrity.
// Tests large row counts and spot checks data integrity.
func TestTortureBulkRows(t *testing.T) {
	dbName := "file:test-torture-bulk.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-torture-bulk.db", false)
	})

	// CREATE TABLE
	_, err = db.Exec("CREATE TABLE bulk (id INTEGER PRIMARY KEY, value INTEGER, text TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// INSERT 10,000 rows
	const rowCount = 10000
	t.Logf("Inserting %d rows...", rowCount)

	for i := 1; i <= rowCount; i++ {
		_, err = db.Exec("INSERT INTO bulk (value, text) VALUES (?, ?)", i*2, "row-"+string(rune('A'+(i%26))))
		if err != nil {
			t.Fatalf("INSERT row %d: %v", i, err)
		}
		if i%1000 == 0 {
			t.Logf("  inserted %d rows...", i)
		}
	}

	// SELECT all and verify count
	rows, err := db.Query("SELECT id, value, text FROM bulk ORDER BY id")
	if err != nil {
		t.Fatalf("SELECT all: %v", err)
	}

	count := 0
	for rows.Next() {
		var id, value int
		var text string
		if err := rows.Scan(&id, &value, &text); err != nil {
			rows.Close()
			t.Fatalf("Scan row %d: %v", count+1, err)
		}

		count++

		// Spot check every 1000th row
		if count%1000 == 0 {
			expectedValue := count * 2
			if value != expectedValue {
				rows.Close()
				t.Errorf("Row %d: got value=%d, want %d", count, value, expectedValue)
			}
			t.Logf("  verified row %d: id=%d, value=%d, text=%s", count, id, value, text)
		}
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("Rows iteration error: %v", err)
	}
	rows.Close()

	if count != rowCount {
		t.Errorf("Row count: got %d, want %d", count, rowCount)
	}

	t.Logf("Successfully inserted and verified %d rows", count)
}

// TestTortureJournalRollback tests transaction rollback integrity.
// Writes data, starts a transaction with conflicting data, rolls back,
// and verifies original data is intact.
func TestTortureJournalRollback(t *testing.T) {
	dbName := "file:test-torture-rollback.db?vfs=opfs&nolock=1"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		globalVFS.Delete("test-torture-rollback.db", false)
	})

	// CREATE TABLE
	_, err = db.Exec("CREATE TABLE journal_test (id INTEGER PRIMARY KEY, status TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	// INSERT initial data
	_, err = db.Exec("INSERT INTO journal_test (id, status) VALUES (1, 'original')")
	if err != nil {
		t.Fatalf("INSERT initial data: %v", err)
	}

	_, err = db.Exec("INSERT INTO journal_test (id, status) VALUES (2, 'original')")
	if err != nil {
		t.Fatalf("INSERT initial data: %v", err)
	}

	// Verify initial data
	var status1, status2 string
	err = db.QueryRow("SELECT status FROM journal_test WHERE id = 1").Scan(&status1)
	if err != nil {
		t.Fatalf("SELECT initial status1: %v", err)
	}
	err = db.QueryRow("SELECT status FROM journal_test WHERE id = 2").Scan(&status2)
	if err != nil {
		t.Fatalf("SELECT initial status2: %v", err)
	}

	if status1 != "original" || status2 != "original" {
		t.Fatalf("Initial data incorrect: status1=%q, status2=%q", status1, status2)
	}

	t.Logf("Initial data verified: status1=%q, status2=%q", status1, status2)

	// BEGIN transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("BEGIN: %v", err)
	}

	// UPDATE data within transaction
	_, err = tx.Exec("UPDATE journal_test SET status = 'modified' WHERE id = 1")
	if err != nil {
		tx.Rollback()
		t.Fatalf("UPDATE in transaction: %v", err)
	}

	_, err = tx.Exec("UPDATE journal_test SET status = 'modified' WHERE id = 2")
	if err != nil {
		tx.Rollback()
		t.Fatalf("UPDATE in transaction: %v", err)
	}

	// INSERT new conflicting data
	_, err = tx.Exec("INSERT INTO journal_test (id, status) VALUES (3, 'temp')")
	if err != nil {
		tx.Rollback()
		t.Fatalf("INSERT in transaction: %v", err)
	}

	t.Logf("Modified data in transaction, now rolling back...")

	// ROLLBACK
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}

	// Verify original data is intact
	err = db.QueryRow("SELECT status FROM journal_test WHERE id = 1").Scan(&status1)
	if err != nil {
		t.Fatalf("SELECT status1 after rollback: %v", err)
	}
	err = db.QueryRow("SELECT status FROM journal_test WHERE id = 2").Scan(&status2)
	if err != nil {
		t.Fatalf("SELECT status2 after rollback: %v", err)
	}

	if status1 != "original" {
		t.Errorf("After rollback: status1=%q, want 'original'", status1)
	}
	if status2 != "original" {
		t.Errorf("After rollback: status2=%q, want 'original'", status2)
	}

	// Verify row 3 was NOT inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM journal_test WHERE id = 3").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT COUNT for id=3 after rollback: %v", err)
	}

	if count != 0 {
		t.Errorf("After rollback: found %d rows with id=3, want 0", count)
	}

	t.Logf("Rollback successful: original data intact, temp data discarded")
}

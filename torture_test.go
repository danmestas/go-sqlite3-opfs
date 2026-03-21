//go:build js

package opfsvfs

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"

)

func TestTortureLargeDB(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	blob := make([]byte, 10240) // 10KB
	for i := range blob {
		blob[i] = byte(i % 256)
	}
	t.Logf("Inserting 1000 blobs of %d bytes each...", len(blob))
	for i := 0; i < 1000; i++ {
		if _, err := db.Exec("INSERT INTO t(data) VALUES(?)", blob); err != nil {
			t.Fatalf("INSERT %d: %v", i, err)
		}
		if (i+1)%100 == 0 {
			t.Logf("  inserted %d rows...", i+1)
		}
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("SELECT count: %v", err)
	}
	if count != 1000 {
		t.Fatalf("count: got %d, want 1000", count)
	}
	t.Logf("Successfully inserted and verified 1000 blobs")
}

func TestTortureRapidOpenClose(t *testing.T) {
	truncateAll(t)
	t.Logf("Running 50 rapid open/close cycles...")
	for i := 0; i < 50; i++ {
		db, err := sql.Open("sqlite3", "file:test.db?vfs=opfs")
		if err != nil {
			t.Fatalf("Cycle %d Open: %v", i, err)
		}
		db.SetMaxOpenConns(1)
		if i == 0 {
			if _, err := db.Exec("CREATE TABLE IF NOT EXISTS t(v INTEGER)"); err != nil {
				t.Fatalf("CREATE TABLE: %v", err)
			}
		}
		if _, err := db.Exec("INSERT INTO t(v) VALUES(?)", i); err != nil {
			t.Fatalf("Cycle %d INSERT: %v", i, err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Cycle %d Close: %v", i, err)
		}
		if (i+1)%10 == 0 {
			t.Logf("  completed %d iterations...", i+1)
		}
	}
	db, err := sql.Open("sqlite3", "file:test.db?vfs=opfs")
	if err != nil {
		t.Fatalf("Final Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("Final Close: %v", err)
		}
	}()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("Final SELECT count: %v", err)
	}
	if count != 50 {
		t.Fatalf("count after 50 cycles: got %d, want 50", count)
	}
	t.Logf("Successfully completed 50 rapid open/close cycles")
}

func TestTortureBulkRows(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v INTEGER, label TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Logf("Inserting 10000 rows...")
	for i := 1; i <= 10000; i++ {
		label := fmt.Sprintf("row-%c", 'A'+byte(i%26))
		if _, err := db.Exec("INSERT INTO t(v, label) VALUES(?, ?)", i*2, label); err != nil {
			t.Fatalf("INSERT %d: %v", i, err)
		}
		if i%1000 == 0 {
			t.Logf("  inserted %d rows...", i)
		}
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatalf("SELECT count: %v", err)
	}
	if count != 10000 {
		t.Fatalf("count: got %d, want 10000", count)
	}
	t.Logf("Successfully inserted and verified 10000 rows")
}

func TestTortureJournalRollback(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, status TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t(status) VALUES('original')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.Exec("UPDATE t SET status='modified' WHERE id=1"); err != nil {
		t.Fatalf("UPDATE: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM t WHERE id=1").Scan(&status); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if status != "original" {
		t.Fatalf("After rollback: got %q, want %q", status, "original")
	}
	t.Logf("Rollback successful: original data intact")
}

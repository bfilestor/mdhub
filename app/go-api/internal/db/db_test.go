package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSchemaCreatesTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "db", "app.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer database.Close()

	if err := InitSchema(database); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}

	tables := []string{"files", "sync_logs"}
	for _, table := range tables {
		if !tableExists(t, database, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	if !columnExists(t, database, "files", "uuid") {
		t.Fatalf("expected files.uuid to exist")
	}
	if !columnExists(t, database, "files", "deleted") {
		t.Fatalf("expected files.deleted to exist")
	}

	if !hasUniqueIndexOn(t, database, "files", "uuid") {
		t.Fatalf("expected unique index on files.uuid")
	}
}

func TestInitSchemaIsIdempotent(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "app.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer database.Close()

	if err := InitSchema(database); err != nil {
		t.Fatalf("first init failed: %v", err)
	}
	if err := InitSchema(database); err != nil {
		t.Fatalf("second init failed: %v", err)
	}
}

func TestOpenCreatesParentDirectory(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "nested", "db", "app.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer database.Close()

	if err := InitSchema(database); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}
}

func TestOpenReadonlyDBReturnsErrorOnInit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "readonly.db")

	if err := os.WriteFile(dbPath, []byte{}, 0o644); err != nil {
		t.Fatalf("create empty db file failed: %v", err)
	}

	databaseRO, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("readonly open failed: %v", err)
	}
	defer databaseRO.Close()

	err = InitSchema(databaseRO)
	if err == nil {
		t.Fatalf("expected init to fail on readonly db")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "readonly") {
		t.Fatalf("expected readonly error, got: %v", err)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&got)
	return err == nil && got == name
}

func columnExists(t *testing.T, db *sql.DB, table string, col string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma table_info failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row failed: %v", err)
		}
		if name == col {
			return true
		}
	}
	return false
}

func hasUniqueIndexOn(t *testing.T, db *sql.DB, table, col string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA index_list(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma index_list failed: %v", err)
	}

	uniqueIndexes := make([]string, 0)
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin, partial interface{}
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			t.Fatalf("scan index_list failed: %v", err)
		}
		if unique == 1 {
			uniqueIndexes = append(uniqueIndexes, name)
		}
	}
	rows.Close()

	for _, name := range uniqueIndexes {
		idxRows, err := db.Query(`PRAGMA index_info(` + name + `)`)
		if err != nil {
			t.Fatalf("pragma index_info failed: %v", err)
		}
		for idxRows.Next() {
			var seqno, cid int
			var cname string
			if err := idxRows.Scan(&seqno, &cid, &cname); err != nil {
				idxRows.Close()
				t.Fatalf("scan index_info failed: %v", err)
			}
			if cname == col {
				idxRows.Close()
				return true
			}
		}
		idxRows.Close()
	}
	return false
}

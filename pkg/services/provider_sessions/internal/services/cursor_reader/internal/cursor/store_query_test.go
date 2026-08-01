package cursor

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSelectKeyValueQuery_PrefersKeyValueThenIDDataThenFirstTwo(t *testing.T) {
	t.Parallel()

	query, ok := selectKeyValueQuery("blobs", []string{"key", "value", "extra"})
	if !ok || query != "SELECT key, value FROM blobs WHERE value IS NOT NULL ORDER BY key" {
		t.Fatalf("key/value query = %q ok=%v", query, ok)
	}

	query, ok = selectKeyValueQuery("blobs", []string{"id", "data"})
	if !ok || query != "SELECT id, data FROM blobs WHERE data IS NOT NULL ORDER BY id" {
		t.Fatalf("id/data query = %q ok=%v", query, ok)
	}

	query, ok = selectKeyValueQuery("blobs", []string{"alpha", "beta", "gamma"})
	wantFallback := "SELECT alpha, beta FROM blobs WHERE beta IS NOT NULL ORDER BY alpha"
	if !ok || query != wantFallback {
		t.Fatalf("fallback query = %q ok=%v, want %q", query, ok, wantFallback)
	}

	if _, ok := selectKeyValueQuery("blobs", nil); ok {
		t.Fatal("empty columns unexpectedly produced a query")
	}
	if _, ok := selectKeyValueQuery("blobs", []string{"only"}); ok {
		t.Fatal("single column unexpectedly produced a query")
	}
}

func TestQueryBlobsTable_MissingAndUnusableSchemaReturnNoEntries(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	ins := newInspection(context.Background())
	entries, err := QueryBlobsTable(ins, db)
	if err != nil {
		t.Fatalf("missing table QueryBlobsTable: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("missing table entries = %#v, want empty", entries)
	}

	if _, err := db.Exec(`CREATE TABLE blobs (solo TEXT)`); err != nil {
		t.Fatalf("create single-column blobs: %v", err)
	}
	entries, err = QueryBlobsTable(ins, db)
	if err != nil {
		t.Fatalf("single-column QueryBlobsTable: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("single-column entries = %#v, want empty", entries)
	}
}

func TestQueryBlobsTable_IDDataAndFallbackColumnPatterns(t *testing.T) {
	t.Parallel()

	t.Run("id_data", func(t *testing.T) {
		db := openTempSQLite(t)
		if _, err := db.Exec(`CREATE TABLE blobs (id TEXT PRIMARY KEY, data TEXT)`); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO blobs (id, data) VALUES (?, ?)`, "row-1", "payload"); err != nil {
			t.Fatalf("insert: %v", err)
		}
		entries, err := QueryBlobsTable(newInspection(context.Background()), db)
		if err != nil {
			t.Fatalf("QueryBlobsTable: %v", err)
		}
		if len(entries) != 1 || entries[0].Key != "row-1" || entries[0].Value != "payload" {
			t.Fatalf("entries = %#v, want id/data payload", entries)
		}
	})

	t.Run("first_two_columns", func(t *testing.T) {
		db := openTempSQLite(t)
		if _, err := db.Exec(`CREATE TABLE blobs (alpha TEXT, beta TEXT)`); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO blobs (alpha, beta) VALUES (?, ?)`, "k", "v"); err != nil {
			t.Fatalf("insert: %v", err)
		}
		entries, err := QueryBlobsTable(newInspection(context.Background()), db)
		if err != nil {
			t.Fatalf("QueryBlobsTable: %v", err)
		}
		if len(entries) != 1 || entries[0].Key != "k" || entries[0].Value != "v" {
			t.Fatalf("entries = %#v, want first-two-column fallback", entries)
		}
	})
}

func TestQueryBlobsTable_IncludesValidRowsAndStopsOnResourceLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create blobs: %v", err)
	}
	for _, row := range []struct {
		key   string
		value any
	}{
		{"a", "one"},
		{"b", nil}, // filtered by WHERE value IS NOT NULL
		{"c", "three"},
		{"d", "four"},
		{"e", "five"},
	} {
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, row.key, row.value); err != nil {
			t.Fatalf("insert %q: %v", row.key, err)
		}
	}

	withTestLimits(t, func() { testLimitOverrides.queriedRows = 3 })
	ins := newInspection(context.Background())
	entries, err := QueryBlobsTable(ins, db)
	if err != nil {
		t.Fatalf("QueryBlobsTable: %v", err)
	}
	if got, want := len(entries), 3; got != want {
		t.Fatalf("entries len = %d, want %d (%#v)", got, want, entries)
	}
	if entries[0].Key != "a" || entries[0].Value != "one" {
		t.Fatalf("first entry = %#v, want key=a value=one", entries[0])
	}
	if entries[1].Key != "c" || entries[1].Value != "three" {
		t.Fatalf("second entry = %#v, want key=c value=three", entries[1])
	}
	if entries[2].Key != "d" || entries[2].Value != "four" {
		t.Fatalf("third entry = %#v, want key=d value=four", entries[2])
	}
	if ins.exhaustedLimit != LimitQueriedRows {
		t.Fatalf("exhaustedLimit = %q, want %q", ins.exhaustedLimit, LimitQueriedRows)
	}
}

func openTempSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

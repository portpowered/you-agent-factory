package cursorstorage

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens a SQLite database in read-only mode
func OpenDatabase(path string) (*sql.DB, error) {
	// Check if file exists when opening in read-only mode
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file does not exist: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return db, nil
}

// QueryCursorDiskKV queries the cursorDiskKV table with a LIKE pattern
func QueryCursorDiskKV(db *sql.DB, pattern string) ([]KeyValuePair, error) {
	query := "SELECT key, value FROM cursorDiskKV WHERE key LIKE ? AND value IS NOT NULL"
	rows, err := db.Query(query, pattern)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pairs []KeyValuePair
	for rows.Next() {
		var pair KeyValuePair
		var value sql.NullString
		if err := rows.Scan(&pair.Key, &value); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		if value.Valid {
			pair.Value = value.String
			pairs = append(pairs, pair)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return pairs, nil
}

// KeyValuePair represents a key-value pair from cursorDiskKV
type KeyValuePair struct {
	Key   string
	Value string
}

// Logging helpers ported from iksnae/cursor-session; disabled in-process for server use.

func LogError(string, ...any)  {}
func LogWarn(string, ...any)   {}
func LogInfo(string, ...any)   {}
func LogDebug(string, ...any)  {}

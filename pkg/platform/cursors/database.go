package cursors

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

// Logging helpers ported from iksnae/cursor-session; disabled in-process for server use.

func LogWarn(string, ...any)  {}
func LogInfo(string, ...any)  {}
func LogDebug(string, ...any) {}

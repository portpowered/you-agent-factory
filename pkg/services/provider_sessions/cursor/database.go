package cursor

import (
	"database/sql"
	"fmt"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens a SQLite database in read-only mode
func OpenDatabase(files providersessions.FileSystem, openSQLDatabase providersessions.CursorOpenSQLDatabase, path string) (*sql.DB, error) {
	if openSQLDatabase == nil {
		return nil, fmt.Errorf("cursor SQL database opener is required")
	}
	// Check if file exists when opening in read-only mode
	if _, err := files.Stat(path); err != nil {
		return nil, fmt.Errorf("database file does not exist: %w", err)
	}

	db, err := openSQLDatabase("sqlite", path+"?mode=ro")
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

func LogWarn(string, ...any) {}
func LogInfo(string, ...any) {}

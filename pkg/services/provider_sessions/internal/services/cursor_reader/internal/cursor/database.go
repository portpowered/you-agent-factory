package cursor

import (
	"database/sql"
	"fmt"

	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens a SQLite database in read-only mode
func OpenDatabase(files providersessionsinternal.FileSystem, openSQLDatabase providersessionsinternal.CursorOpenSQLDatabase, path string) (*sql.DB, error) {
	if openSQLDatabase == nil {
		return nil, fmt.Errorf("cursor SQL database opener is required")
	}
	// Check if file exists when opening in read-only mode
	if _, err := files.Stat(path); err != nil {
		return nil, fmt.Errorf("cursor session store could not be opened: %w", err)
	}

	db, err := openSQLDatabase("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("cursor session store could not be opened")
	}

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cursor session store could not be opened")
	}

	return db, nil
}

// Logging helpers ported from iksnae/cursor-session; disabled in-process for server use.

func LogWarn(string, ...any) {}
func LogInfo(string, ...any) {}

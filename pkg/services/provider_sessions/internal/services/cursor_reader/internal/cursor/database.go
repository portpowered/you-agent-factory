package cursor

import (
	"context"
	"database/sql"
	"fmt"

	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens a SQLite database in read-only mode
func OpenDatabase(files providersessionsinternal.FileSystem, openSQLDatabase providersessionsinternal.CursorOpenSQLDatabase, path string) (*sql.DB, error) {
	return openDatabase(context.Background(), files, openSQLDatabase, path)
}

func openDatabase(ctx context.Context, files providersessionsinternal.FileSystem, openSQLDatabase providersessionsinternal.CursorOpenSQLDatabase, path string) (*sql.DB, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if openSQLDatabase == nil {
		return nil, fmt.Errorf("cursor SQL database opener is required")
	}
	// Check if file exists when opening in read-only mode
	if _, err := files.Stat(path); err != nil {
		return nil, fmt.Errorf("cursor session store could not be opened: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := openSQLDatabase("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("cursor session store could not be opened")
	}
	if err := ctx.Err(); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("cursor session store could not be opened")
	}

	return db, nil
}

// Logging helpers ported from iksnae/cursor-session; disabled in-process for server use.

func LogWarn(string, ...any) {}
func LogInfo(string, ...any) {}

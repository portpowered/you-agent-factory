package cursor

import (
	"database/sql"
	"errors"
	"fmt"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// QueryBlobsTable queries the blobs table from a store.db file
func QueryBlobsTable(ins *inspection, db *sql.DB) ([]BlobEntry, error) {
	exists, err := sqliteTableExists(ins, db, "blobs")
	if err != nil {
		return nil, fmt.Errorf("failed to check for blobs table: %w", err)
	}
	if !exists {
		return []BlobEntry{}, nil
	}

	columns, err := readSQLiteTableColumns(ins, db, "blobs")
	if err != nil {
		return nil, fmt.Errorf("failed to get blobs table info: %w", err)
	}
	if len(columns) == 0 {
		return []BlobEntry{}, nil
	}

	query, ok := selectKeyValueQuery("blobs", columns)
	if !ok {
		return []BlobEntry{}, nil
	}

	rows, err := db.QueryContext(ins.context(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to query blobs table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanBlobEntries(ins, rows)
}

// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session store schema probing stays grouped until extraction refactor.
// QueryMetaTable queries the meta table from a store.db file
func QueryMetaTable(ins *inspection, db *sql.DB) ([]MetaEntry, error) {
	// Check if meta table exists
	var tableExists bool
	err := db.QueryRowContext(ins.context(), `
		SELECT EXISTS (
			SELECT name FROM sqlite_master
			WHERE type='table' AND name='meta'
		)
	`).Scan(&tableExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for meta table: %w", err)
	}

	if !tableExists {
		return []MetaEntry{}, nil
	}

	// Query meta table - similar flexible approach
	columns, err := readSQLiteTableColumns(ins, db, "meta")
	if err != nil {
		return nil, fmt.Errorf("failed to get meta table info: %w", err)
	}

	if len(columns) == 0 {
		return []MetaEntry{}, nil
	}

	var query string
	if containsString(columns, "key") && containsString(columns, "value") {
		query = "SELECT key, value FROM meta WHERE value IS NOT NULL ORDER BY key"
	} else if containsString(columns, "id") && containsString(columns, "data") {
		query = "SELECT id, data FROM meta WHERE data IS NOT NULL ORDER BY id"
	} else if len(columns) >= 2 {
		query = fmt.Sprintf("SELECT %s, %s FROM meta WHERE %s IS NOT NULL ORDER BY %s", columns[0], columns[1], columns[1], columns[0])
	} else {
		return []MetaEntry{}, nil
	}

	rows, err := db.QueryContext(ins.context(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to query meta table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []MetaEntry
	rowCount := 0
	for rows.Next() {
		if err := ins.recordRow(); err != nil {
			if errors.Is(err, providersessions.ErrResourceLimitExceeded) {
				break
			}
			return entries, err
		}
		rowCount++
		var entry MetaEntry
		var value sql.NullString
		if err := rows.Scan(&entry.Key, &value); err != nil {
			LogWarn("Failed to scan meta row %d: %v", rowCount, err)
			continue
		}
		if value.Valid {
			entry.Value = value.String
			entries = append(entries, entry)
			// Log first few entries for diagnostics
			if rowCount <= 3 {
				valuePreview := entry.Value
				if len(valuePreview) > 200 {
					valuePreview = valuePreview[:200] + "..."
				}
				LogInfo("Meta entry %d: key='%s', value_preview='%s'", rowCount, entry.Key, valuePreview)
			}
		} else {
			LogWarn("Meta row %d has NULL value: key='%s'", rowCount, entry.Key)
		}
	}

	LogInfo("QueryMetaTable: queried %d rows, returned %d valid entries", rowCount, len(entries))

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return entries, nil
}

// BlobEntry represents an entry from the blobs table
type BlobEntry struct {
	Key   string
	Value string
}

// MetaEntry represents an entry from the meta table
type MetaEntry struct {
	Key   string
	Value string
}

func sqliteTableExists(ins *inspection, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ins.context(), `
		SELECT EXISTS (
			SELECT name FROM sqlite_master
			WHERE type='table' AND name=?
		)
	`, table).Scan(&exists)
	return exists, err
}

func readSQLiteTableColumns(ins *inspection, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ins.context(), fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		if err := ins.checkCanceled(); err != nil {
			return nil, err
		}
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			continue
		}
		columns = append(columns, name)
	}
	return columns, nil
}

// selectKeyValueQuery builds a two-column SELECT for Cursor store tables from
// probed column names. It prefers key/value, then id/data, then the first two
// columns. ok is false when no usable pattern exists.
func selectKeyValueQuery(table string, columns []string) (string, bool) {
	if containsString(columns, "key") && containsString(columns, "value") {
		return fmt.Sprintf("SELECT key, value FROM %s WHERE value IS NOT NULL ORDER BY key", table), true
	}
	if containsString(columns, "id") && containsString(columns, "data") {
		return fmt.Sprintf("SELECT id, data FROM %s WHERE data IS NOT NULL ORDER BY id", table), true
	}
	if len(columns) >= 2 {
		return fmt.Sprintf(
			"SELECT %s, %s FROM %s WHERE %s IS NOT NULL ORDER BY %s",
			columns[0], columns[1], table, columns[1], columns[0],
		), true
	}
	return "", false
}

func scanBlobEntries(ins *inspection, rows *sql.Rows) ([]BlobEntry, error) {
	var entries []BlobEntry
	rowCount := 0
	for rows.Next() {
		if err := ins.recordRow(); err != nil {
			if errors.Is(err, providersessions.ErrResourceLimitExceeded) {
				break
			}
			return entries, err
		}
		rowCount++
		var entry BlobEntry
		var value sql.NullString
		if err := rows.Scan(&entry.Key, &value); err != nil {
			LogWarn("Failed to scan blob row %d: %v", rowCount, err)
			continue
		}
		if value.Valid {
			entry.Value = value.String
			entries = append(entries, entry)
			if rowCount <= 3 {
				valuePreview := entry.Value
				if len(valuePreview) > 200 {
					valuePreview = valuePreview[:200] + "..."
				}
				LogInfo("Blob entry %d: key='%s', value_preview='%s'", rowCount, entry.Key, valuePreview)
			}
		} else {
			LogWarn("Blob row %d has NULL value: key='%s'", rowCount, entry.Key)
		}
	}

	LogInfo("QueryBlobsTable: queried %d rows, returned %d valid entries", rowCount, len(entries))

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return entries, nil
}

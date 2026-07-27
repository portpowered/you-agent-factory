package cursor

import (
	"database/sql"
	"fmt"
)

// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session store schema probing stays grouped until extraction refactor.
// QueryBlobsTable queries the blobs table from a store.db file
func QueryBlobsTable(db *sql.DB) ([]BlobEntry, error) {
	// Check if blobs table exists
	var tableExists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master
			WHERE type='table' AND name='blobs'
		)
	`).Scan(&tableExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for blobs table: %w", err)
	}

	if !tableExists {
		return []BlobEntry{}, nil
	}

	// Query all blobs - we'll need to inspect the schema
	// Common patterns: key-value, id-data, etc.
	// Try to get column names first
	rows, err := db.Query("PRAGMA table_info(blobs)")
	if err != nil {
		return nil, fmt.Errorf("failed to get blobs table info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
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

	if len(columns) == 0 {
		return []BlobEntry{}, nil
	}

	// Build query based on common column patterns
	// Try key-value pattern first (most common for session storage)
	var query string
	if containsString(columns, "key") && containsString(columns, "value") {
		query = "SELECT key, value FROM blobs WHERE value IS NOT NULL ORDER BY key"
	} else if containsString(columns, "id") && containsString(columns, "data") {
		query = "SELECT id, data FROM blobs WHERE data IS NOT NULL ORDER BY id"
	} else if len(columns) >= 2 {
		// Use first two columns
		query = fmt.Sprintf("SELECT %s, %s FROM blobs WHERE %s IS NOT NULL ORDER BY %s", columns[0], columns[1], columns[1], columns[0])
	} else {
		return []BlobEntry{}, nil
	}

	rows, err = db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query blobs table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []BlobEntry
	rowCount := 0
	for rows.Next() {
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
			// Log first few entries for diagnostics
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

// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session store schema probing stays grouped until extraction refactor.
// QueryMetaTable queries the meta table from a store.db file
func QueryMetaTable(db *sql.DB) ([]MetaEntry, error) {
	// Check if meta table exists
	var tableExists bool
	err := db.QueryRow(`
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
	rows, err := db.Query("PRAGMA table_info(meta)")
	if err != nil {
		return nil, fmt.Errorf("failed to get meta table info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
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

	rows, err = db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query meta table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []MetaEntry
	rowCount := 0
	for rows.Next() {
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

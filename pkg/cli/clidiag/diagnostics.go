// Package clidiag owns shared helpers for customer-facing CLI diagnostics.
package clidiag

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const DefaultSessionID = "~default"

// Printf writes one verbose diagnostic line when diagnostics are enabled.
func Printf(output io.Writer, enabled bool, format string, args ...any) {
	if !enabled || output == nil {
		return
	}
	_, _ = fmt.Fprintf(output, format+"\n", args...)
}

// SessionLabel returns the user-facing session label used in diagnostics.
func SessionLabel(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return DefaultSessionID
	}
	return sessionID
}

// PayloadType classifies a submitted file without reading or logging content.
func PayloadType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".md", ".markdown":
		return "markdown"
	default:
		return "file"
	}
}

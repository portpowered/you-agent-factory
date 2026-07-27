package cursor

import (
	"fmt"
	"strings"
)

func sanitizeDiagnosticMessage(class string, position int, extra ...string) string {
	class = strings.TrimSpace(class)
	if class == "" {
		class = "cursor_record_error"
	}
	message := class
	if position > 0 {
		message = fmt.Sprintf("%s at row %d", class, position)
	}
	if len(extra) > 0 && strings.TrimSpace(extra[0]) != "" {
		message = fmt.Sprintf("%s (%s)", message, strings.TrimSpace(extra[0]))
	}
	return truncateDiagnosticMessage(message)
}

func truncateDiagnosticMessage(message string) string {
	if len(message) <= maxDiagnosticMessage {
		return message
	}
	if maxDiagnosticMessage <= 3 {
		return message[:maxDiagnosticMessage]
	}
	return message[:maxDiagnosticMessage-3] + "..."
}

func sanitizeStructuralError(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "cursor session store could not be read"
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "select "),
		strings.Contains(lower, "pragma "),
		strings.Contains(lower, "sqlite"),
		strings.Contains(lower, "\\"),
		strings.Contains(lower, "/"),
		strings.Contains(lower, ":"):
		return "cursor session store could not be read"
	default:
		return truncateDiagnosticMessage(trimmed)
	}
}

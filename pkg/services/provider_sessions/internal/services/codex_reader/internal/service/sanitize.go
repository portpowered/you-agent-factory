package service

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	unsafeDiagnosticPathPattern  = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\|/|\\|\.\./)`)
	unsafeDiagnosticTokenPattern = regexp.MustCompile(`(?i)(?:api[_-]?key|authorization|bearer\s+|password|secret|token)`)
)

func sanitizeDiagnosticMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return diagnosticInvalidJSONEvent
	}
	if !isAllowlistedDiagnosticMessage(trimmed) {
		return diagnosticInvalidJSONEvent
	}
	return truncateDiagnosticMessage(trimmed)
}

func isAllowlistedDiagnosticMessage(message string) bool {
	switch message {
	case diagnosticInvalidJSONEvent,
		diagnosticTruncatedJSONEvent,
		diagnosticInspectionLineLimit,
		diagnosticInspectionByteLimit,
		diagnosticInspectionRecordLimit,
		diagnosticInspectionTranscriptLimit,
		diagnosticInspectionRetainedTextLimit,
		diagnosticInspectionDiagnosticLimit:
		return true
	default:
		return false
	}
}

func truncateDiagnosticMessage(message string) string {
	if len(message) <= maxCodexDiagnosticMessageLength {
		return message
	}
	return message[:maxCodexDiagnosticMessageLength]
}

func truncateCodexText(value string, maxBytes int64) string {
	if maxBytes <= 0 {
		return ""
	}
	if int64(len(value)) <= maxBytes {
		return value
	}
	bounded := value[:maxBytes]
	for len(bounded) > 0 && bounded[len(bounded)-1]&0xc0 == 0x80 {
		bounded = bounded[:len(bounded)-1]
	}
	return bounded
}

func sanitizeUnknownEventLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if unsafeDiagnosticPathPattern.MatchString(trimmed) ||
		unsafeDiagnosticTokenPattern.MatchString(trimmed) ||
		strings.Contains(trimmed, string(filepath.Separator)) {
		return "redacted"
	}
	builder := strings.Builder{}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
		}
		if builder.Len() >= maxCodexUnknownEventLabelLength {
			break
		}
	}
	sanitized := builder.String()
	if sanitized == "" {
		return "redacted"
	}
	return sanitized
}

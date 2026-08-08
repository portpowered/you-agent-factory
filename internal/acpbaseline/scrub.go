package acpbaseline

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// secretPatterns are deterministic, ordered, idempotent redactions applied
// when producing a scrubbed transcript.
//
// Scrubbing is a mitigation, not a boundary: pattern matching cannot promise a
// transcript is safe, which is exactly why a scrubbed transcript is still
// never committed. Only the structural digest is.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{8,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

// sensitiveKeyPattern matches JSON keys whose value is redacted wholesale,
// regardless of the value's shape.
var sensitiveKeyPattern = regexp.MustCompile(
	`(?i)"([^"]*(token|secret|api[_\-]?key|password|authorization|credential|cookie)[^"]*)"\s*:\s*"[^"]*"`)

// Scrub applies the redaction rules to one line.
func Scrub(line string, environmentLiterals []string) string {
	scrubbed := line
	// Longest first, so a nested path is not partially replaced by its parent.
	literals := append([]string(nil), environmentLiterals...)
	sort.Slice(literals, func(i, j int) bool { return len(literals[i]) > len(literals[j]) })
	for _, literal := range literals {
		if len(literal) < 4 {
			continue
		}
		scrubbed = strings.ReplaceAll(scrubbed, literal, "<redacted>")
	}
	for _, pattern := range secretPatterns {
		scrubbed = pattern.ReplaceAllString(scrubbed, "<redacted>")
	}
	scrubbed = sensitiveKeyPattern.ReplaceAllString(scrubbed, `"$1":"<redacted>"`)
	return scrubbed
}

// EnvironmentLiterals returns machine-specific strings worth redacting before
// a transcript leaves the machine that produced it.
func EnvironmentLiterals() []string {
	var literals []string
	for _, key := range []string{"HOME", "USER"} {
		if value := os.Getenv(key); value != "" {
			literals = append(literals, value)
		}
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		literals = append(literals, host)
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		literals = append(literals, cwd)
	}
	return literals
}

package providerparity

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	forbiddenSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)sk-[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|bearer)\s*[:=]`),
	}
	forbiddenPathPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)[A-Za-z]:\\Users\\`),
		regexp.MustCompile(`(?i)/Users/[^/\s]+/`),
		regexp.MustCompile(`(?i)/home/[^/\s]+/`),
	}
)

// DefaultForbiddenTokens are substrings parity fixtures must never require for assertions.
func DefaultForbiddenTokens() []string {
	return []string{
		"sk-parity-secret-token",
		"private submitted prompt",
		"cursor-secret",
		"secret-value",
		"PRIVATE.md",
		"private result",
	}
}

// ValidateSanitized rejects fixture bytes that still carry secrets or host-specific paths.
func ValidateSanitized(contents []byte, extraForbidden ...string) error {
	text := string(contents)
	for _, forbidden := range append(DefaultForbiddenTokens(), extraForbidden...) {
		if forbidden == "" {
			continue
		}
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("fixture contains forbidden token %q", forbidden)
		}
	}
	for _, pattern := range forbiddenSecretPatterns {
		if pattern.FindStringIndex(text) != nil {
			return fmt.Errorf("fixture matches forbidden secret pattern %q", pattern.String())
		}
	}
	for _, pattern := range forbiddenPathPatterns {
		if pattern.FindStringIndex(text) != nil {
			return fmt.Errorf("fixture matches forbidden path pattern %q", pattern.String())
		}
	}
	if filepath.IsAbs(text) {
		return fmt.Errorf("fixture must not be an absolute path")
	}
	return nil
}

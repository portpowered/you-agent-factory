package baseline

import (
	"os"
	"strings"
)

// NormalizeFixtureText canonicalizes committed baseline fixture text so compares
// stay stable when Git checks out files with CRLF on Windows.
func NormalizeFixtureText(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return normalized
}

// ReadFixtureText reads a committed baseline fixture and normalizes line endings.
func ReadFixtureText(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return NormalizeFixtureText(string(raw)), nil
}

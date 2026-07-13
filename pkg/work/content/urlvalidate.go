package content

import (
	"fmt"
	"net/url"
	"strings"
)

var supportedContentURLSchemes = map[string]struct{}{
	"file":  {},
	"http":  {},
	"https": {},
	"data":  {},
}

// ValidateContentURL reports whether rawURL is a non-empty supported content URL.
func ValidateContentURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return fmt.Errorf("url must be a non-empty string")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("url must be a valid URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if _, ok := supportedContentURLSchemes[scheme]; !ok {
		return fmt.Errorf("url scheme must be one of file, http, https, or data")
	}
	return nil
}

// ContentURLAndFileConflictError is returned when both url and file are set on one part.
func ContentURLAndFileConflictError() error {
	return fmt.Errorf("url and file cannot both be set on the same content part")
}

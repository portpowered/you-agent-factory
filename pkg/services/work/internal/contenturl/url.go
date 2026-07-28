package contenturl

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

var supportedContentURLSchemes = map[string]struct{}{
	"file":  {},
	"http":  {},
	"https": {},
	"data":  {},
}

// Validate reports whether rawURL is a non-empty supported Work content URL.
func Validate(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return fmt.Errorf("url must be a non-empty string")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("url must be a valid URL")
	}
	if _, ok := supportedContentURLSchemes[strings.ToLower(parsed.Scheme)]; !ok {
		return fmt.Errorf("url scheme must be one of file, http, https, or data")
	}
	return nil
}

// URLAndFileConflictError reports use of both canonical URL and legacy file
// fields on one content part.
func URLAndFileConflictError() error {
	return fmt.Errorf("url and file cannot both be set on the same content part")
}

// FilesystemPathToURL maps a host filesystem path to a canonical file content URL.
func FilesystemPathToURL(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("file path must be a non-empty string")
	}
	cleaned := filepath.Clean(trimmed)
	if filepath.IsAbs(cleaned) {
		urlPath := filepath.ToSlash(cleaned)
		if volume := filepath.VolumeName(cleaned); volume != "" && !strings.HasPrefix(urlPath, "/") {
			urlPath = "/" + urlPath
		}
		return (&url.URL{Scheme: "file", Path: urlPath}).String(), nil
	}
	return "file://" + filepath.ToSlash(cleaned), nil
}

// ResolveDispatchURL resolves relative file content URLs against a dispatch
// working directory.
func ResolveDispatchURL(workingDirectory, rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.ToLower(parsed.Scheme) != "file" {
		return trimmed, nil
	}
	path := fileContentURLPath(parsed)
	if path == "" || workingDirectory == "" || filepath.IsAbs(filepath.FromSlash(path)) {
		return trimmed, nil
	}
	return FilesystemPathToURL(filepath.Join(workingDirectory, filepath.FromSlash(path)))
}

func fileContentURLPath(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	if parsed.Host != "" {
		return parsed.Host + parsed.Path
	}
	if path := parsed.Path; path != "" {
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			return path[1:]
		}
		return path
	}
	return parsed.Opaque
}

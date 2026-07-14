package content

import (
	"net/url"
	"path/filepath"
	"strings"
)

// fileContentURLPath returns the filesystem path encoded in a file:// content URL.
// Legacy relative URLs use the shape file://relative/path (host + path segments).
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

// ResolveDispatchContentURL resolves relative file:// URLs against workingDirectory
// so dispatch-time materialization can stat the correct host path.
func ResolveDispatchContentURL(workingDirectory, rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || strings.ToLower(parsed.Scheme) != "file" {
		return trimmed, nil
	}

	path := fileContentURLPath(parsed)
	if path == "" {
		return trimmed, nil
	}

	if workingDirectory == "" || filepath.IsAbs(filepath.FromSlash(path)) {
		return trimmed, nil
	}

	absPath := filepath.Join(workingDirectory, filepath.FromSlash(path))
	return FilesystemPathToContentURL(absPath)
}

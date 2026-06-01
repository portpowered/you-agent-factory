package materialize

import (
	"fmt"
	"net/url"
	"os"
)

func materializeFileURL(rawURL string, parsed *url.URL) (string, CleanupFunc, error) {
	path := parsed.Path
	if path == "" && parsed.Opaque != "" {
		path = parsed.Opaque
	}
	if path == "" {
		return "", noopCleanup, notReadableError(rawURL, "empty file path")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", noopCleanup, notReadableError(rawURL, "file does not exist")
		}
		return "", noopCleanup, notReadableError(rawURL, err.Error())
	}
	if info.IsDir() {
		return "", noopCleanup, notReadableError(rawURL, "path is a directory")
	}

	return path, noopCleanup, nil
}

func notReadableError(rawURL, reason string) error {
	return fmt.Errorf("media url not readable: %s (%s)", rawURL, reason)
}

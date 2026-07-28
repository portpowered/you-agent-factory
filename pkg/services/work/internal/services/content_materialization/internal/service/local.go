package service

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

func materializeFileURL(
	rawURL string,
	parsed *url.URL,
	opts *Options,
) (string, CleanupFunc, error) {
	path, err := localFilePath(parsed, opts.HostPlatform)
	if err != nil {
		return "", noopCleanup, notReadableError(rawURL, err.Error())
	}

	if opts.InspectPath == nil {
		return "", noopCleanup, notReadableError(rawURL, "inspect path dependency is required")
	}
	info, err := opts.InspectPath(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", noopCleanup, notReadableError(rawURL, "file does not exist")
		}
		return "", noopCleanup, notReadableError(rawURL, err.Error())
	}
	if info.IsDir() {
		return "", noopCleanup, notReadableError(rawURL, "path is a directory")
	}

	return path, noopCleanup, nil
}

func localFilePath(parsed *url.URL, hostPlatform work.ContentHostPlatform) (string, error) {
	path := parsed.Path
	if path == "" && parsed.Opaque != "" {
		path = parsed.Opaque
	}
	if path == "" {
		return "", fmt.Errorf("empty file path")
	}
	if strings.EqualFold(string(hostPlatform), "windows") && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), nil
}

func notReadableError(rawURL, reason string) error {
	return fmt.Errorf("media url not readable: %s (%s)", rawURL, reason)
}

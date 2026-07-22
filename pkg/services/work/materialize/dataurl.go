package materialize

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

func materializeDataURL(rawURL string, parsed *url.URL, opts *Options) (string, CleanupFunc, error) {
	mediaType, data, err := parseDataURL(parsed)
	if err != nil {
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}

	maxBytes := opts.maxBytes()
	if int64(len(data)) > maxBytes {
		return "", noopCleanup, inaccessibleError(rawURL, "data exceeds size limit")
	}

	ext := extensionForMediaType(mediaType)
	path, cleanup, err := createTempFile(opts, ext)
	if err != nil {
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}

	if opts == nil || opts.WriteFile == nil {
		cleanup()
		return "", noopCleanup, inaccessibleError(rawURL, "write file dependency is required")
	}
	if err := opts.WriteFile(path, data, 0o600); err != nil {
		cleanup()
		return "", noopCleanup, inaccessibleError(rawURL, err.Error())
	}
	return path, cleanup, nil
}

func parseDataURL(parsed *url.URL) (mediaType string, payload []byte, err error) {
	// data:[<mediatype>][;base64],<data>
	spec := parsed.Opaque
	if spec == "" {
		spec = strings.TrimPrefix(parsed.Path, "/")
	}
	if spec == "" {
		return "", nil, fmt.Errorf("empty data payload")
	}

	meta, encoded, found := strings.Cut(spec, ",")
	if !found {
		return "", nil, fmt.Errorf("invalid data url")
	}

	mediaType = meta
	isBase64 := strings.HasSuffix(strings.ToLower(meta), ";base64")
	if idx := strings.Index(meta, ";"); idx >= 0 {
		mediaType = meta[:idx]
	}
	mediaType = strings.TrimSpace(mediaType)

	if isBase64 {
		payload, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", nil, fmt.Errorf("invalid base64 data")
		}
		return mediaType, payload, nil
	}

	// Percent-decoded per RFC; data URLs in tests use base64 for binary.
	unescaped, err := url.PathUnescape(encoded)
	if err != nil {
		return mediaType, []byte(encoded), nil
	}
	return mediaType, []byte(unescaped), nil
}

func extensionForMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func createTempFile(opts *Options, ext string) (path string, cleanup CleanupFunc, err error) {
	dir := ""
	if opts != nil {
		dir = opts.TempDir
	}
	if opts == nil || opts.CreateTempFile == nil {
		return "", noopCleanup, fmt.Errorf("create temporary file dependency is required")
	}
	if opts.RemovePath == nil {
		return "", noopCleanup, fmt.Errorf("remove path dependency is required")
	}
	f, err := opts.CreateTempFile(dir, "workcontent-*"+ext)
	if err != nil {
		return "", noopCleanup, err
	}
	path = f.Name()
	_ = f.Close()
	return path, func() { _ = opts.RemovePath(path) }, nil
}

package work

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

// ValidateContentURL reports whether rawURL is a non-empty supported Work
// content URL.
func ValidateContentURL(rawURL string) error {
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

// ContentURLAndFileConflictError reports use of both canonical URL and legacy
// file fields on one content part.
func ContentURLAndFileConflictError() error {
	return fmt.Errorf("url and file cannot both be set on the same content part")
}

// FilesystemPathToContentURL maps a host filesystem path to a canonical file
// content URL.
func FilesystemPathToContentURL(path string) (string, error) {
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

// ResolveDispatchContentURL resolves relative file content URLs against a
// dispatch working directory.
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
	if path == "" || workingDirectory == "" || filepath.IsAbs(filepath.FromSlash(path)) {
		return trimmed, nil
	}
	return FilesystemPathToContentURL(filepath.Join(workingDirectory, filepath.FromSlash(path)))
}

// NormalizeFileBackedContentPart maps legacy file-only content onto its
// canonical URL representation.
func NormalizeFileBackedContentPart(part WorkContentPart) (WorkContentPart, error) {
	switch part.Type.Normalized() {
	case WorkContentPartTypeImage, WorkContentPartTypeAudio, WorkContentPartTypeBinary:
	default:
		return part, nil
	}
	hasURL := strings.TrimSpace(part.URL) != ""
	hasFile := strings.TrimSpace(part.File) != ""
	if hasURL && hasFile {
		return WorkContentPart{}, ContentURLAndFileConflictError()
	}
	if !hasURL && !hasFile {
		return WorkContentPart{}, fmt.Errorf("url must be a non-empty string")
	}
	if hasURL {
		part.File = ""
		return part, nil
	}
	contentURL, err := FilesystemPathToContentURL(part.File)
	if err != nil {
		return WorkContentPart{}, err
	}
	if err := ValidateContentURL(contentURL); err != nil {
		return WorkContentPart{}, err
	}
	part.URL = contentURL
	part.File = ""
	return part, nil
}

// NormalizeFileBackedContent normalizes every content part in order.
func NormalizeFileBackedContent(content []WorkContentPart) ([]WorkContentPart, error) {
	if len(content) == 0 {
		return content, nil
	}
	normalized := make([]WorkContentPart, len(content))
	for i, part := range content {
		var err error
		normalized[i], err = NormalizeFileBackedContentPart(part)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
	}
	return normalized, nil
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

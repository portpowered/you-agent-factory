package content

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"
)

// FilesystemPathToContentURL maps a host filesystem path to a canonical file:// content URL.
// Absolute paths use a proper file URL path; relative paths keep the legacy file://<relative> shape
// resolved later at dispatch-time materialization.
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

// NormalizeFileBackedContentPart maps legacy file-only parts to url and clears bare file
// so canonical content persists url only.
func NormalizeFileBackedContentPart(part work.WorkContentPart) (work.WorkContentPart, error) {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeImage,
		work.WorkContentPartTypeAudio,
		work.WorkContentPartTypeBinary:
	default:
		return part, nil
	}

	hasURL := strings.TrimSpace(part.URL) != ""
	hasFile := strings.TrimSpace(part.File) != ""
	if hasURL && hasFile {
		return work.WorkContentPart{}, ContentURLAndFileConflictError()
	}
	if !hasURL && !hasFile {
		return work.WorkContentPart{}, fmt.Errorf("url must be a non-empty string")
	}
	if hasURL {
		part.File = ""
		return part, nil
	}

	contentURL, err := FilesystemPathToContentURL(part.File)
	if err != nil {
		return work.WorkContentPart{}, err
	}
	if err := ValidateContentURL(contentURL); err != nil {
		return work.WorkContentPart{}, err
	}
	part.URL = contentURL
	part.File = ""
	return part, nil
}

// NormalizeFileBackedContent applies NormalizeFileBackedContentPart to each part in order.
func NormalizeFileBackedContent(content []work.WorkContentPart) ([]work.WorkContentPart, error) {
	if len(content) == 0 {
		return content, nil
	}
	normalized := make([]work.WorkContentPart, len(content))
	for i, part := range content {
		var err error
		normalized[i], err = NormalizeFileBackedContentPart(part)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
	}
	return normalized, nil
}

package requestadmission

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/contenturl"
)

func normalizeFileBackedContentPart(part ContentPart) (ContentPart, error) {
	switch part.Type.Normalized() {
	case ContentPartTypeImage, ContentPartTypeAudio, ContentPartTypeBinary:
	default:
		return part, nil
	}
	hasURL := strings.TrimSpace(part.URL) != ""
	hasFile := strings.TrimSpace(part.File) != ""
	if hasURL && hasFile {
		return ContentPart{}, contenturl.URLAndFileConflictError()
	}
	if !hasURL && !hasFile {
		return ContentPart{}, fmt.Errorf("url must be a non-empty string")
	}
	if hasURL {
		part.File = ""
		return part, nil
	}
	contentURL, err := contenturl.FilesystemPathToURL(part.File)
	if err != nil {
		return ContentPart{}, err
	}
	if err := contenturl.Validate(contentURL); err != nil {
		return ContentPart{}, err
	}
	part.URL = contentURL
	part.File = ""
	return part, nil
}

func normalizeFileBackedContent(content []ContentPart) ([]ContentPart, error) {
	if len(content) == 0 {
		return content, nil
	}
	normalized := make([]ContentPart, len(content))
	for i, part := range content {
		var err error
		normalized[i], err = normalizeFileBackedContentPart(part)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
	}
	return normalized, nil
}

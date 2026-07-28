package work

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/contenturl"
)

// ContentHostPlatform identifies the operating-system path convention used
// when resolving local Work content URLs.
type ContentHostPlatform string

// ContentCleanup releases resources created while materializing Work content.
type ContentCleanup func()

// ContentTemporaryFile is the exact temporary-file handle used while
// materializing remote and inline Work content.
type ContentTemporaryFile interface {
	Name() string
	Close() error
}

// ContentInspectPath inspects a local Work content path.
type ContentInspectPath func(string) (fs.FileInfo, error)

// ContentCreateTemporaryFile reserves a materialized Work content path.
type ContentCreateTemporaryFile func(string, string) (ContentTemporaryFile, error)

// ContentRemovePath removes a temporary materialized Work content path.
type ContentRemovePath func(string) error

// ContentWriteFile writes decoded inline Work content.
type ContentWriteFile func(string, []byte, fs.FileMode) error

// ContentOpenFile opens a temporary path for bounded remote-content writes.
type ContentOpenFile func(string) (io.WriteCloser, error)

// ContentHTTPDoer performs the exact outbound HTTP effect used to retrieve
// remote Work content.
type ContentHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// ContentMaterializer is the focused Work materialization role. The published
// peer root exposes MaterializeContentURL on Service; workers may still inject
// this narrower role until nested IMP-WORK cuts fold injection onto the root.
type ContentMaterializer interface {
	MaterializeContentURL(context.Context, string) (string, ContentCleanup, error)
}

// ContentMaterializeFunc adapts a function to ContentMaterializer.
type ContentMaterializeFunc func(context.Context, string) (string, ContentCleanup, error)

func (f ContentMaterializeFunc) MaterializeContentURL(
	ctx context.Context,
	rawURL string,
) (string, ContentCleanup, error) {
	return f(ctx, rawURL)
}

// ValidateContentURL reports whether rawURL is a non-empty supported Work
// content URL. Implementation lives in the content_materialization subservice.
func ValidateContentURL(rawURL string) error {
	return contenturl.Validate(rawURL)
}

// ContentURLAndFileConflictError reports use of both canonical URL and legacy
// file fields on one content part.
func ContentURLAndFileConflictError() error {
	return contenturl.URLAndFileConflictError()
}

// FilesystemPathToContentURL maps a host filesystem path to a canonical file
// content URL.
func FilesystemPathToContentURL(path string) (string, error) {
	return contenturl.FilesystemPathToURL(path)
}

// ResolveDispatchContentURL resolves relative file content URLs against a
// dispatch working directory.
func ResolveDispatchContentURL(workingDirectory, rawURL string) (string, error) {
	return contenturl.ResolveDispatchURL(workingDirectory, rawURL)
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

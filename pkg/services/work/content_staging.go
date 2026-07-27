package work

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

var (
	ErrInvalidStagedContentRef = errors.New("stagedFileRef must be a backend-issued staged file reference")
	ErrStagedContentNotFound   = errors.New("stagedFileRef must reference an existing staged submit-work file")
	ErrStagedContentExpired    = errors.New("stagedFileRef has expired")
)

// ContentStagingFileSystem is the exact filesystem effect used to own staged
// submit-Work content.
type ContentStagingFileSystem interface {
	MkdirTemp(string, string) (string, error)
	WriteFile(string, []byte, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
	RemoveAll(string) error
}

// ContentStagingRandom supplies cryptographic entropy for signing keys and
// fallback file names.
type ContentStagingRandom interface {
	Read([]byte) (int, error)
}

// ContentStagingClock supplies issuance and expiry time.
type ContentStagingClock interface {
	Now() time.Time
}

// ContentStagingService is the focused Work staging role. The published peer
// root exposes the same operations on Service; transports may still inject this
// narrower role until nested IMP-WORK cuts fold injection onto the root.
// Construction is owned by work/internal/services/content_staging/wire.
type ContentStagingService interface {
	StageContent(context.Context, StageContentRequest) (StageContentResult, error)
	PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error)
	ResolveContent(context.Context, string) (ResolvedStagedContent, error)
	CleanupContent(context.Context, string) error
}

// StageContentRequest is the plain Work-owned staging request contract.
type StageContentRequest struct {
	ItemType  string
	FileName  string
	MediaType string
	Content   []byte
}

// StageContentResult is the plain Work-owned staging result. StagedFileRef is an
// opaque reference peers pass back through prepare/resolve/cleanup without
// observing staging implementation types.
type StageContentResult struct {
	StagedFileRef string
	FileName      string
	MediaType     string
	URL           string
}

// StagedSubmissionItem is one prepared submission item for PrepareContent.
type StagedSubmissionItem struct {
	ItemType      string
	Text          string
	StagedFileRef string
	FileName      string
	MediaType     string
}

// ResolvedStagedContent is the local path and URL for a staged reference.
type ResolvedStagedContent struct {
	Path      string
	URL       string
	ExpiresAt time.Time
}

// ContentStagingError is a customer-safe Work validation failure.
type ContentStagingError struct {
	Message string
	Cause   error
}

func (e *ContentStagingError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *ContentStagingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Package content_staging owns Work secure staged-reference issuance, resolve,
// prepare, expiry, and cleanup behind the published CTR-WORK staging seam.
// Peers and transports consume Work root staging contracts; this package is the
// parent-private nested owner for the staging effect.
package content_staging

import (
	"context"
	"io/fs"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the singular content_staging subservice contract.
type Service interface {
	StageContent(context.Context, work.StageContentRequest) (work.StageContentResult, error)
	PrepareContent(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error)
	ResolveContent(context.Context, string) (work.ResolvedStagedContent, error)
	CleanupContent(context.Context, string) error
}

// FileSystem is the exact filesystem effect used to own staged submit-Work content.
type FileSystem interface {
	MkdirTemp(string, string) (string, error)
	WriteFile(string, []byte, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
	RemoveAll(string) error
}

// Random supplies cryptographic entropy for signing keys and fallback file names.
type Random interface {
	Read([]byte) (int, error)
}

// Clock supplies issuance and expiry time.
type Clock interface {
	Now() time.Time
}

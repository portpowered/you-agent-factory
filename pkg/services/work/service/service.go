// Package service is a transitional compile shim for DEL-WORK. Production
// construction should use work/wire; the composed root lives in
// work/internal/service.
package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/service"
)

// RuntimeResolver resolves live Factory Session runtimes for transitional shim callers.
type RuntimeResolver = internalservice.RuntimeResolver

// Service is the transitional shim type for session-scoped Work operations.
type Service = internalservice.Service

// New delegates to the owner-private composed root in internal/service.
func New(sessions RuntimeResolver) *Service {
	return internalservice.New(sessions)
}

// NewService delegates to the owner-private composed root in internal/service.
func NewService(
	runtimes work.RuntimeResolver,
	readSubmittedFile work.SubmittedFileReader,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) work.FileSubmissionService {
	return internalservice.NewService(
		runtimes,
		readSubmittedFile,
		contentStaging,
		contentMaterializer,
	)
}

// SubmitFile delegates to the owner-private composed root in internal/service.
func SubmitFile(
	ctx context.Context,
	path string,
	target internalservice.SubmitTarget,
	readFile work.SubmittedFileReader,
) error {
	return internalservice.SubmitFile(ctx, path, target, readFile)
}

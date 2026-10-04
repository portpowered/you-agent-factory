package internal_test

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

func newTestWorkService(runtimes work.RuntimeResolver, readFile work.SubmittedFileReader, inspectPath work.SubmittedFilePathInspector, staging work.ContentStagingService, materializer work.ContentMaterializer) work.FileSubmissionService {
	if staging == nil {
		staging = admissionOnlyContentStaging{}
	}
	if materializer == nil {
		materializer = admissionOnlyContentMaterializer{}
	}
	return internalservice.NewService(runtimes, readFile, inspectPath, staging, materializer,
		completedStateAccess{},
		fakeRequestPreparation(func(context.Context, work.WorkRequestPreparation) (work.WorkRequest, error) {
			panic("unexpected request preparation")
		}),
		fakeInvocationPreparation(func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
			panic("unexpected invocation preparation")
		}))
}

var _ stateaccess.Service = completedStateAccess{}

// admissionOnlyContentStaging keeps unsupported content operations explicit.
type admissionOnlyContentStaging struct{}

func (admissionOnlyContentStaging) StageContent(context.Context, work.StageContentRequest) (work.StageContentResult, error) {
	return work.StageContentResult{}, errors.New("Work content staging is required")
}
func (admissionOnlyContentStaging) PrepareContent(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
	return nil, errors.New("Work content staging is required")
}
func (admissionOnlyContentStaging) ResolveContent(context.Context, string) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, errors.New("Work content staging is required")
}
func (admissionOnlyContentStaging) CleanupContent(context.Context, string) error {
	return errors.New("Work content staging is required")
}

// admissionOnlyContentMaterializer keeps unsupported materialization explicit.
type admissionOnlyContentMaterializer struct{}

func (admissionOnlyContentMaterializer) MaterializeContentURL(context.Context, string) (string, work.ContentCleanup, error) {
	return "", nil, errors.New("Work content materializer is required")
}

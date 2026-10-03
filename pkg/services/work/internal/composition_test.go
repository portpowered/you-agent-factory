package internal_test

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

func newTestWorkService(runtimes work.RuntimeResolver, readFile work.SubmittedFileReader, inspectPath work.SubmittedFilePathInspector, staging work.ContentStagingService, materializer work.ContentMaterializer) work.FileSubmissionService {
	if staging == nil {
		staging = admissionOnlyContentStaging{}
	}
	if materializer == nil {
		materializer = admissionOnlyContentMaterializer{}
	}
	state := workwire.NewStateAccess(workwire.NewRuntimeSessionResolver(runtimes), nil, testDurability{})
	content := workwire.NewContentPreparation(workwire.NewContentPolicy())
	prep := workwire.NewRequestPreparationService(workwire.NewRequestPolicy(workwire.NewRequestContentBridge(content)))
	invocation := workwire.NewInvocationInputAdapter(workwire.NewInvocationInputPolicy(readFile, inspectPath))
	return internalservice.NewService(runtimes, readFile, inspectPath, staging, materializer, state, prep, invocation)
}

type testDurability struct{}

func (testDurability) CompletedFlushSequence(string) (int64, bool) { return 0, false }

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

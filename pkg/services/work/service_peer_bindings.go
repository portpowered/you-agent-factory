package work

import (
	"context"
	"fmt"
)

// MaterializationService projects a published MaterializeContentURL capability
// onto the Work service root for peers that must depend on Service rather than
// the narrower ContentMaterializer role. Wire and composition edges may still
// construct the focused role; internal consumers should depend only on Service.
func MaterializationService(materializer ContentMaterializer) Service {
	if materializer == nil {
		return nil
	}
	return materializationService{materializer: materializer}
}

type materializationService struct {
	materializer ContentMaterializer
}

func (materializationService) SubmitWorkRequestForSession(
	context.Context,
	string,
	WorkRequest,
) (WorkRequestSubmitResult, error) {
	return WorkRequestSubmitResult{}, fmt.Errorf("Work materialization service does not support admission")
}

func (materializationService) PrepareWorkRequest(
	context.Context,
	WorkRequestPreparation,
) (WorkRequest, error) {
	return WorkRequest{}, fmt.Errorf("Work materialization service does not support admission prep")
}

func (materializationService) MoveWorkForSession(
	context.Context,
	string,
	string,
	string,
	string,
) (OperatorMoveResult, error) {
	return OperatorMoveResult{}, fmt.Errorf("Work materialization service does not support state access")
}

func (materializationService) ListWork(context.Context, string, ListOptions) (ListResult, error) {
	return ListResult{}, fmt.Errorf("Work materialization service does not support state access")
}

func (materializationService) GetWork(context.Context, string, string) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work materialization service does not support state access")
}

func (materializationService) MoveWorkAndRead(
	context.Context,
	string,
	string,
	string,
	string,
) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work materialization service does not support state access")
}

func (materializationService) StageContent(context.Context, StageContentRequest) (StageContentResult, error) {
	return StageContentResult{}, fmt.Errorf("Work materialization service does not support content staging")
}

func (materializationService) PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error) {
	return nil, fmt.Errorf("Work materialization service does not support content staging")
}

func (materializationService) ResolveContent(context.Context, string) (ResolvedStagedContent, error) {
	return ResolvedStagedContent{}, fmt.Errorf("Work materialization service does not support content staging")
}

func (materializationService) CleanupContent(context.Context, string) error {
	return fmt.Errorf("Work materialization service does not support content staging")
}

func (s materializationService) MaterializeContentURL(
	ctx context.Context,
	rawURL string,
) (string, ContentCleanup, error) {
	return s.materializer.MaterializeContentURL(ctx, rawURL)
}

func (materializationService) MaterializeWorkerOutput(
	context.Context,
	MaterializeWorkerOutputRequest,
) (MaterializeWorkerOutputResult, error) {
	return MaterializeWorkerOutputResult{}, fmt.Errorf("Work materialization service does not support worker-output materialization")
}

func (materializationService) PrepareInvocationInput(
	context.Context,
	InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	return PreparedInvocationInput{}, fmt.Errorf("Work materialization service does not support invocation policy")
}

func (materializationService) ResolvePrimaryResult(
	context.Context,
	PrimaryResultSelectionInput,
) (PrimaryResultSelection, error) {
	return PrimaryResultSelection{}, fmt.Errorf("Work materialization service does not support invocation policy")
}

// AdmissionContentService projects admission content staging and Work Request
// preparation onto the Work service root for peers that must depend on Service
// rather than narrower Work roles. Wire and composition edges may still
// construct the focused roles; internal consumers should depend only on Service.
func AdmissionContentService(
	staging ContentStagingService,
	preparation RequestPreparationService,
) Service {
	if staging == nil || preparation == nil {
		return nil
	}
	return admissionContentService{staging: staging, preparation: preparation}
}

type admissionContentService struct {
	staging     ContentStagingService
	preparation RequestPreparationService
}

func (admissionContentService) SubmitWorkRequestForSession(
	context.Context,
	string,
	WorkRequest,
) (WorkRequestSubmitResult, error) {
	return WorkRequestSubmitResult{}, fmt.Errorf("Work admission content service does not support admission")
}

func (s admissionContentService) PrepareWorkRequest(
	ctx context.Context,
	input WorkRequestPreparation,
) (WorkRequest, error) {
	return s.preparation.PrepareWorkRequest(ctx, input)
}

func (admissionContentService) MoveWorkForSession(
	context.Context,
	string,
	string,
	string,
	string,
) (OperatorMoveResult, error) {
	return OperatorMoveResult{}, fmt.Errorf("Work admission content service does not support state access")
}

func (admissionContentService) ListWork(context.Context, string, ListOptions) (ListResult, error) {
	return ListResult{}, fmt.Errorf("Work admission content service does not support state access")
}

func (admissionContentService) GetWork(context.Context, string, string) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work admission content service does not support state access")
}

func (admissionContentService) MoveWorkAndRead(
	context.Context,
	string,
	string,
	string,
	string,
) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work admission content service does not support state access")
}

func (s admissionContentService) StageContent(
	ctx context.Context,
	request StageContentRequest,
) (StageContentResult, error) {
	return s.staging.StageContent(ctx, request)
}

func (s admissionContentService) PrepareContent(
	ctx context.Context,
	items []StagedSubmissionItem,
) ([]WorkContentPart, error) {
	return s.staging.PrepareContent(ctx, items)
}

func (s admissionContentService) ResolveContent(
	ctx context.Context,
	ref string,
) (ResolvedStagedContent, error) {
	return s.staging.ResolveContent(ctx, ref)
}

func (s admissionContentService) CleanupContent(ctx context.Context, ref string) error {
	return s.staging.CleanupContent(ctx, ref)
}

func (admissionContentService) MaterializeContentURL(
	context.Context,
	string,
) (string, ContentCleanup, error) {
	return "", nil, fmt.Errorf("Work admission content service does not support content materialization")
}

func (admissionContentService) MaterializeWorkerOutput(
	context.Context,
	MaterializeWorkerOutputRequest,
) (MaterializeWorkerOutputResult, error) {
	return MaterializeWorkerOutputResult{}, fmt.Errorf("Work admission content service does not support worker-output materialization")
}

func (admissionContentService) PrepareInvocationInput(
	context.Context,
	InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	return PreparedInvocationInput{}, fmt.Errorf("Work admission content service does not support invocation policy")
}

func (admissionContentService) ResolvePrimaryResult(
	context.Context,
	PrimaryResultSelectionInput,
) (PrimaryResultSelection, error) {
	return PrimaryResultSelection{}, fmt.Errorf("Work admission content service does not support invocation policy")
}

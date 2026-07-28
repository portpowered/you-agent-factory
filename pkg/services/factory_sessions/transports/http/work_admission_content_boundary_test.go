package http

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var errUnsupportedAdmissionWorkServiceMethod = errors.New("unsupported admission work service method")

type recordingAdmissionWorkService struct {
	stageCalls   int
	prepareCalls int
	prepCalls    int
	lastStage    work.StageContentRequest
	lastItems    []work.StagedSubmissionItem
	lastPrep     work.WorkRequestPreparation
}

func (f *recordingAdmissionWorkService) SubmitWorkRequestForSession(
	context.Context,
	string,
	work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) PrepareWorkRequest(
	_ context.Context,
	input work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	f.prepCalls++
	f.lastPrep = input
	request := input.Request
	request.Works = append([]work.Work(nil), request.Works...)
	return request, nil
}

func (f *recordingAdmissionWorkService) MoveWorkForSession(
	context.Context,
	string,
	string,
	string,
	string,
) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) ListWork(context.Context, string, work.ListOptions) (work.ListResult, error) {
	return work.ListResult{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) GetWork(context.Context, string, string) (work.ReadModel, error) {
	return work.ReadModel{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) MoveWorkAndRead(
	context.Context,
	string,
	string,
	string,
	string,
) (work.ReadModel, error) {
	return work.ReadModel{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) StageContent(
	_ context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	f.stageCalls++
	f.lastStage = request
	return work.StageContentResult{
		FileName:      request.FileName,
		MediaType:     request.MediaType,
		StagedFileRef: "submit-work-stage:v1:test",
		URL:           "file:///tmp/test",
	}, nil
}

func (f *recordingAdmissionWorkService) PrepareContent(
	_ context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	f.prepareCalls++
	f.lastItems = append([]work.StagedSubmissionItem(nil), items...)
	return []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}}, nil
}

func (f *recordingAdmissionWorkService) ResolveContent(context.Context, string) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) CleanupContent(context.Context, string) error {
	return errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) MaterializeContentURL(
	context.Context,
	string,
) (string, work.ContentCleanup, error) {
	return "", nil, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) PrepareInvocationInput(
	context.Context,
	work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	return work.PreparedInvocationInput{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) ResolvePrimaryResult(
	context.Context,
	work.PrimaryResultSelectionInput,
) (work.PrimaryResultSelection, error) {
	return work.PrimaryResultSelection{}, errUnsupportedAdmissionWorkServiceMethod
}

// TestWorkAdmissionContentBoundary_StagesContentThroughWorkService proves Factory
// Sessions HTTP staging reaches Work only through the published work.Service
// StageContent contract.
func TestWorkAdmissionContentBoundary_StagesContentThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	request := httptest.NewRequest(
		"POST",
		"/factory-sessions/session-1/work/staged-files",
		strings.NewReader(`{"contentBase64":"aGVsbG8=","fileName":"note.txt","itemType":"document","mediaType":"text/plain"}`),
	)
	response, err := server.stageSubmitWorkFileRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("stageSubmitWorkFile: %v", err)
	}
	if recording.stageCalls != 1 {
		t.Fatalf("StageContent calls = %d, want 1", recording.stageCalls)
	}
	if recording.lastStage.FileName != "note.txt" {
		t.Fatalf("last stage request = %#v, want note.txt", recording.lastStage)
	}
	if response.StagedFileRef == "" {
		t.Fatalf("staged file ref = %q, want non-empty", response.StagedFileRef)
	}
}

// TestWorkAdmissionContentBoundary_PreparesWorkRequestThroughWorkService proves
// Factory Sessions HTTP admission prep reaches Work only through the published
// work.Service PrepareWorkRequest contract.
func TestWorkAdmissionContentBoundary_PreparesWorkRequestThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	request := work.WorkRequest{
		Works: []work.Work{{
			Name: "task", WorkTypeID: "prd",
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
		}},
	}
	prepared, err := server.prepareWorkRequest(context.Background(), request, nil)
	if err != nil {
		t.Fatalf("prepareWorkRequest: %v", err)
	}
	if recording.prepCalls != 1 {
		t.Fatalf("PrepareWorkRequest calls = %d, want 1", recording.prepCalls)
	}
	if len(prepared.Works) != 1 || prepared.Works[0].Name != "task" {
		t.Fatalf("prepared request = %#v, want task work", prepared)
	}
}

// TestWorkAdmissionContentBoundary_PrepareContentThroughWorkService proves
// Factory Sessions structured submit content resolution reaches Work only through
// the published work.Service PrepareContent contract.
func TestWorkAdmissionContentBoundary_PrepareContentThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	var item factoryapi.SubmitWorkItem
	if err := item.FromSubmitWorkTextItem(factoryapi.SubmitWorkTextItem{
		Type: factoryapi.SubmitWorkItemTypeText,
		Text: "hello",
	}); err != nil {
		t.Fatalf("FromSubmitWorkTextItem: %v", err)
	}
	items := []factoryapi.SubmitWorkItem{item}

	content, err := server.submitWorkContent(context.Background(), factoryapi.SubmitWorkRequest{
		Items: &items,
	})
	if err != nil {
		t.Fatalf("submitWorkContent: %v", err)
	}
	if recording.prepareCalls != 1 {
		t.Fatalf("PrepareContent calls = %d, want 1", recording.prepareCalls)
	}
	if len(recording.lastItems) != 1 || recording.lastItems[0].Text != "hello" {
		t.Fatalf("last prepare items = %#v, want hello text item", recording.lastItems)
	}
	if len(content) != 1 || content[0].Text != "hello" {
		t.Fatalf("prepared content = %#v, want hello", content)
	}
}

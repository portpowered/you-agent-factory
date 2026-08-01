package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var errUnsupportedAdmissionWorkServiceMethod = errors.New("unsupported admission work service method")

type allowingFactoryDefinitions struct{}

func (allowingFactoryDefinitions) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (allowingFactoryDefinitions) SaveFactoryForSession(
	context.Context,
	string,
	factoryapi.FactorySaveMode,
	factoryapi.Factory,
) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (allowingFactoryDefinitions) SaveCurrentFactoryForSession(
	context.Context,
	string,
	factoryapi.Factory,
) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

type recordingAdmissionWorkService struct {
	stageCalls        int
	prepareCalls      int
	prepCalls         int
	submitCalls       int
	lastStage         work.StageContentRequest
	lastItems         []work.StagedSubmissionItem
	lastPrep          work.WorkRequestPreparation
	lastSubmitSession string
	lastSubmitRequest work.WorkRequest

	prepareRequestErr error
	submitResult      work.WorkRequestSubmitResult
	submitErr         error
}

func (f *recordingAdmissionWorkService) SubmitWorkRequestForSession(
	_ context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	f.submitCalls++
	f.lastSubmitSession = sessionID
	f.lastSubmitRequest = request
	if f.submitErr != nil {
		return work.WorkRequestSubmitResult{}, f.submitErr
	}
	if f.submitResult.RequestID != "" || f.submitResult.TraceID != "" || f.submitResult.Accepted {
		return f.submitResult, nil
	}
	return work.WorkRequestSubmitResult{}, errUnsupportedAdmissionWorkServiceMethod
}

func (f *recordingAdmissionWorkService) PrepareWorkRequest(
	_ context.Context,
	input work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	f.prepCalls++
	f.lastPrep = input
	if f.prepareRequestErr != nil {
		return work.WorkRequest{}, f.prepareRequestErr
	}
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

func (f *recordingAdmissionWorkService) MaterializeWorkerOutput(
	context.Context,
	work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	return work.MaterializeWorkerOutputResult{}, errUnsupportedAdmissionWorkServiceMethod
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
	server := NewHandler(Dependencies{
		FactoryDefinitions: allowingFactoryDefinitions{},
		WorkService:        recording,
	}, nil)

	request := httptest.NewRequest(
		"POST",
		"/factory-sessions/session-1/work/staged-files",
		strings.NewReader(`{"contentBase64":"aGVsbG8=","fileName":"note.txt","itemType":"document","mediaType":"text/plain"}`),
	)
	response := httptest.NewRecorder()
	server.StageSubmitWorkFileBySessionId(response, request, "session-1")
	if response.Code != http.StatusCreated {
		t.Fatalf("stageSubmitWorkFile status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if recording.stageCalls != 1 {
		t.Fatalf("StageContent calls = %d, want 1", recording.stageCalls)
	}
	if recording.lastStage.FileName != "note.txt" {
		t.Fatalf("last stage request = %#v, want note.txt", recording.lastStage)
	}
	var body factoryapi.StageSubmitWorkFileResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode stage response: %v", err)
	}
	if body.StagedFileRef == "" {
		t.Fatalf("staged file ref = %q, want non-empty", body.StagedFileRef)
	}
}

// TestWorkAdmissionContentBoundary_PreparesWorkRequestThroughWorkService proves
// Factory Sessions HTTP admission prep reaches Work only through the published
// work.Service PrepareWorkRequest contract.
func TestWorkAdmissionContentBoundary_PreparesWorkRequestThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{
		submitResult: work.WorkRequestSubmitResult{Accepted: true},
	}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-sessions/session-1/work",
		strings.NewReader(`{"name":"task","workTypeName":"prd"}`),
	)
	response := httptest.NewRecorder()
	server.SubmitWorkBySessionId(response, request, "session-1")
	if response.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if recording.prepCalls != 1 {
		t.Fatalf("PrepareWorkRequest calls = %d, want 1", recording.prepCalls)
	}
	if len(recording.lastPrep.Request.Works) != 1 || recording.lastPrep.Request.Works[0].Name != "task" {
		t.Fatalf("prepared request = %#v, want task work", recording.lastPrep.Request)
	}
	if string(recording.lastPrep.CanonicalJSON) != `{"name":"task","workTypeName":"prd"}` {
		t.Fatalf("canonical JSON = %q, want original submit body", recording.lastPrep.CanonicalJSON)
	}
}

// TestWorkAdmissionContentBoundary_PrepareContentThroughWorkService proves
// Factory Sessions structured submit content resolution reaches Work only through
// the published work.Service PrepareContent contract.
func TestWorkAdmissionContentBoundary_PrepareContentThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{
		submitResult: work.WorkRequestSubmitResult{Accepted: true},
	}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-sessions/session-1/work",
		strings.NewReader(`{"items":[{"type":"text","text":"hello"}]}`),
	)
	response := httptest.NewRecorder()
	server.SubmitWorkBySessionId(response, request, "session-1")
	if response.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if recording.prepareCalls != 1 {
		t.Fatalf("PrepareContent calls = %d, want 1", recording.prepareCalls)
	}
	if len(recording.lastItems) != 1 || recording.lastItems[0].Text != "hello" {
		t.Fatalf("last prepare items = %#v, want hello text item", recording.lastItems)
	}
}

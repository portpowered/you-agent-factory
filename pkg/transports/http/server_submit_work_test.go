package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type workAPIObservations struct {
	WorkRequests  []work.WorkRequest
	ListWorkCalls int
	GetWorkCalls  int
	ReadItems     []work.ReadModel
	accepted      map[string]work.WorkRequestSubmitResult
}

func newRecordingWorkRole() (*workAPIObservations, strictWorkAPIFake) {
	observed := &workAPIObservations{
		accepted: make(map[string]work.WorkRequestSubmitResult),
	}
	return observed, strictWorkAPIFake{
		submit: func(_ context.Context, _ string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			result := acceptProgrammedHTTPWorkRequest(request)
			if previous, ok := observed.accepted[result.RequestID]; ok {
				previous.Accepted = false
				return previous, nil
			}
			observed.WorkRequests = append(observed.WorkRequests, request)
			observed.accepted[result.RequestID] = result
			return result, nil
		},
		list: func(_ context.Context, _ string, _ work.ListOptions) (work.ListResult, error) {
			observed.ListWorkCalls++
			return work.ListResult{Results: append([]work.ReadModel(nil), observed.ReadItems...), MaxResults: work.DefaultListMaxResults}, nil
		},
		getWork: func(_ context.Context, _ string, id string) (work.ReadModel, error) {
			observed.GetWorkCalls++
			for _, item := range observed.ReadItems {
				if item.CursorID == id || item.WorkID == id {
					return item, nil
				}
			}
			return work.ReadModel{}, work.ErrWorkNotFound
		},
	}
}

func TestSubmitWork(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"draft-prd","workTypeName":"prd","traceId":"test-trace-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	assertSubmitWorkIdentityResponse(t, resp, submitWorkIdentityExpectation{
		name:         "draft-prd",
		workTypeName: "prd",
		traceId:      "test-trace-1",
		accepted:     true,
	})
	if resp.RequestId == "" {
		t.Fatalf("requestId = %q, want non-empty normalized request id", resp.RequestId)
	}
	if stringValue(resp.WorkId) != "batch-"+resp.RequestId+"-draft-prd" {
		t.Fatalf("workId = %q, want batch-%s-draft-prd", stringValue(resp.WorkId), resp.RequestId)
	}
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(mf.WorkRequests))
	}
	if mf.WorkRequests[0].Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want FACTORY_REQUEST_BATCH", mf.WorkRequests[0].Type)
	}
	if len(mf.WorkRequests[0].Works) != 1 || mf.WorkRequests[0].Works[0].WorkTypeID != "prd" {
		t.Fatalf("received Works = %#v, want one prd Work", mf.WorkRequests[0].Works)
	}
	payload, ok := mf.WorkRequests[0].Works[0].Payload.([]byte)
	if !ok || string(payload) != `{"title":"Draft PRD"}` {
		t.Errorf("payload = %#v, want encoded JSON object payload", mf.WorkRequests[0].Works[0].Payload)
	}
}

func TestSubmitWork_ReturnsAcceptedWorkIdentifiers(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"draft-prd","workTypeName":"prd","traceId":"test-trace-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	assertSubmitWorkResponseIdentifiers(t, resp, submitWorkResponseExpectation{
		traceID:      "test-trace-1",
		name:         "draft-prd",
		workTypeName: "prd",
		sessionID:    factorysessions.DefaultSessionID,
		workIDSuffix: "-draft-prd",
	})
	if resp.RequestId == "" {
		t.Fatal("expected non-empty requestId")
	}
	if !strings.HasPrefix(stringValue(resp.WorkId), "batch-"+resp.RequestId) {
		t.Fatalf("workId = %q, want batch-<requestId>-draft-prd prefix", stringValue(resp.WorkId))
	}
}

type submitWorkResponseExpectation struct {
	traceID      string
	name         string
	workTypeName string
	sessionID    string
	workIDSuffix string
}

func assertSubmitWorkResponseIdentifiers(t *testing.T, resp factoryapi.SubmitWorkResponse, want submitWorkResponseExpectation) {
	t.Helper()

	if resp.TraceId != want.traceID {
		t.Fatalf("traceId = %q, want %q", resp.TraceId, want.traceID)
	}
	if !resp.Accepted {
		t.Fatal("accepted = false, want true")
	}
	if stringValue(resp.Name) != want.name {
		t.Fatalf("name = %q, want %q", stringValue(resp.Name), want.name)
	}
	if stringValue(resp.WorkTypeName) != want.workTypeName {
		t.Fatalf("workTypeName = %q, want %q", stringValue(resp.WorkTypeName), want.workTypeName)
	}
	if stringValue(resp.SessionId) != want.sessionID {
		t.Fatalf("sessionId = %q, want %q", stringValue(resp.SessionId), want.sessionID)
	}
	workID := stringValue(resp.WorkId)
	if workID == "" {
		t.Fatal("workId is empty, want normalized batch id")
	}
	if want.workIDSuffix != "" && !strings.HasSuffix(workID, want.workIDSuffix) {
		t.Fatalf("workId = %q, want suffix %q", workID, want.workIDSuffix)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this contract-heavy submit boundary test keeps the full canonical request and response shape in one reviewer-readable flow.
func TestSubmitWork_AcceptsCanonicalContent(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"ui-review","workTypeName":"prd","content":[{"type":"text","text":"Review this UI."},{"type":"image","url":"file://fixtures/ui.png"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if len(mf.WorkRequests[0].Works[0].Content) != 2 {
		t.Fatalf("submitted work request content count = %d, want 2", len(mf.WorkRequests[0].Works[0].Content))
	}
	if mf.WorkRequests[0].Works[0].Content[0].Type != work.WorkContentPartTypeText || mf.WorkRequests[0].Works[0].Content[0].Text != "Review this UI." {
		t.Fatalf("submitted work request content[0] = %#v, want canonical text content", mf.WorkRequests[0].Works[0].Content[0])
	}
	if mf.WorkRequests[0].Works[0].Content[1].Type != work.WorkContentPartTypeImage || mf.WorkRequests[0].Works[0].Content[1].URL != "file://fixtures/ui.png" {
		t.Fatalf("submitted work request content[1] = %#v, want canonical image content", mf.WorkRequests[0].Works[0].Content[1])
	}
}

func TestSubmitWork_AcceptsStructuredItems(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	staged := stageSubmitWorkTestFile(t, srv, "image", "ui.png", "image/png", []byte("png-bytes"))

	rec := submitWorkRequest(t, srv, `{"name":"ui-review","workTypeName":"prd","items":[{"type":"text","text":"Review this UI."},{"type":"image","url":"file://staged/ui.png","stagedFileRef":"`+staged.StagedFileRef+`","fileName":"ui.png","mediaType":"image/png"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	assertStructuredSubmitWorkSubmission(t, mf, staged)
}

func assertStructuredSubmitWorkSubmission(t *testing.T, mf *workAPIObservations, staged factoryapi.StageSubmitWorkFileResponse) {
	t.Helper()
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("Work requests = %#v, want one Work", mf.WorkRequests)
	}
	content := mf.WorkRequests[0].Works[0].Content
	if len(content) != 2 {
		t.Fatalf("content count = %d, want 2", len(content))
	}
	assertStructuredSubmitWorkTextPart(t, content[0])
	assertStructuredSubmitWorkStagedImagePart(t, content[1], staged)
}

func assertStructuredSubmitWorkTextPart(t *testing.T, part work.WorkContentPart) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeText || part.Text != "Review this UI." {
		t.Fatalf("submitted content[0] = %#v, want canonical text content", part)
	}
}

func assertStructuredSubmitWorkStagedImagePart(
	t *testing.T,
	part work.WorkContentPart,
	staged factoryapi.StageSubmitWorkFileResponse,
) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeImage || part.ContentType != "image/png" {
		t.Fatalf("submitted content[1] = %#v, want canonical staged image content", part)
	}
	if part.File != "" {
		t.Fatalf("submitted content[1].file = %q, want empty canonical file field", part.File)
	}
	if part.URL != string(staged.Url) {
		t.Fatalf("submitted content[1].url = %q, want staged response URL %q", part.URL, staged.Url)
	}
	if part.Metadata[submitWorkItemTypeMetadataKey] != "image" || part.Metadata[submitWorkFileNameMetadataKey] != "ui.png" {
		t.Fatalf("submitted content[1].metadata = %#v, want item type and file name metadata", part.Metadata)
	}
}

func TestSubmitWork_AcceptsUppercaseAndExtendedCanonicalContent(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationResult(srv, func(input work.WorkRequestPreparation) work.WorkRequest {
		prepared := input.Request
		if got := prepared.Works[0].Content[0].Type; got != work.WorkContentPartType("TEXT") {
			t.Fatalf("preparation role input type = %q, want representation-preserved TEXT", got)
		}
		prepared.Works[0].Content[0].Type = work.WorkContentPartTypeText
		return prepared
	})

	rec := submitWorkRequest(t, srv, `{
		"name":"tts-request",
		"workTypeName":"prd",
		"content":[
			{"type":"TEXT","text":"Synthesize this","label":"prompt"},
			{"type":"AUDIO","url":"file://artifacts/output.wav","contentType":"audio/wav","artifactId":"artifact-audio-1","metadata":{"voice":"alloy"}},
			{"type":"JSON","json":{"voice":"alloy","speed":1}}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	assertUppercaseExtendedCanonicalSubmission(t, mf)
}

func assertUppercaseExtendedCanonicalSubmission(t *testing.T, mf *workAPIObservations) {
	t.Helper()
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 || len(mf.WorkRequests[0].Works[0].Content) != 3 {
		t.Fatalf("received Work content = %#v, want 3 canonical parts", mf.WorkRequests)
	}
	content := mf.WorkRequests[0].Works[0].Content
	assertUppercaseExtendedTextPart(t, content[0])
	assertUppercaseExtendedAudioPart(t, content[1])
	assertUppercaseExtendedJSONPart(t, content[2])
}

func assertUppercaseExtendedTextPart(t *testing.T, part work.WorkContentPart) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeText || part.Label != "prompt" {
		t.Fatalf("submitted content[0] = %#v, want normalized text part with label", part)
	}
}

func assertUppercaseExtendedAudioPart(t *testing.T, part work.WorkContentPart) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeAudio || part.URL != "file://artifacts/output.wav" || part.ContentType != "audio/wav" || part.ArtifactID != "artifact-audio-1" {
		t.Fatalf("submitted content[1] = %#v, want canonical audio content", part)
	}
	audioMetadata, _ := json.Marshal(part.Metadata)
	if string(audioMetadata) != `{"voice":"alloy"}` {
		t.Fatalf("audio metadata = %s, want voice metadata", audioMetadata)
	}
}

func assertUppercaseExtendedJSONPart(t *testing.T, part work.WorkContentPart) {
	t.Helper()
	jsonValue := map[string]any{}
	if err := json.Unmarshal(part.JSON, &jsonValue); err != nil {
		t.Fatalf("decode json content: %v", err)
	}
	if part.Type != work.WorkContentPartTypeJSON || jsonValue["voice"] != "alloy" || jsonValue["speed"] != float64(1) {
		t.Fatalf("submitted content[2] = %#v, want canonical json content", part)
	}
}

func TestSubmitWork_RejectsConflictingContentAndPayload(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, `work_request: works[0] ("conflicting-content") has invalid content/payload: payload conflicts with explicit content`)
	rec := submitWorkRequest(t, srv, `{"name":"conflicting-content","workTypeName":"prd","content":[{"type":"text","text":"canonical"}],"payload":"different"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("conflicting-content") has invalid content/payload: payload conflicts with explicit content`)
}

func TestSubmitWork_RejectsStructuredItemsCombinedWithPayload(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "items cannot be combined with payload")
	rec := submitWorkRequest(t, srv, `{"name":"conflicting-items","workTypeName":"prd","items":[{"type":"text","text":"canonical"}],"payload":"different"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items cannot be combined with payload")
}

func TestSubmitWork_RejectsStructuredItemsCombinedWithContent(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "items cannot be combined with content")
	rec := submitWorkRequest(t, srv, `{"name":"conflicting-items-content","workTypeName":"prd","items":[{"type":"text","text":"structured"}],"content":[{"type":"text","text":"canonical"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items cannot be combined with content")
}

func TestSubmitWork_AcceptsHeaderOnlyStructuredSubmitWork(t *testing.T) {
	// Dashboard submit-work sends name, workTypeName, and items: [] when optional text
	// inputs are blank. Header/type-only submissions carry no structured work.
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	rec := submitWorkRequest(t, srv, `{"name":"header-only-request","workTypeName":"prd","items":[]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("header-only structured submit-work: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId == "" {
		t.Fatalf("expected non-empty trace_id, got %q", resp.TraceId)
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if item := mf.WorkRequests[0].Works[0]; item.Name != "header-only-request" || item.WorkTypeID != "prd" {
		t.Fatalf("received Work = %#v, want header-only-request/prd", item)
	}
	if len(mf.WorkRequests[0].Works[0].Content) != 0 {
		t.Fatalf("submitted work request content count = %d, want empty structured content", len(mf.WorkRequests[0].Works[0].Content))
	}
}

func TestSubmitWork_RejectsBlankOnlyStructuredItems(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	staging := newContentStagingFake()
	staging.prepareContent = func(
		context.Context,
		[]work.StagedSubmissionItem,
	) ([]work.WorkContentPart, error) {
		return nil, &work.ContentStagingError{
			Message: "items must contain at least one non-empty item",
		}
	}
	srv.Adapter = srv.Adapter.WithContentStaging(staging)
	rec := submitWorkRequest(t, srv, `{"name":"blank-items","workTypeName":"prd","items":[{"type":"text","text":"   \t"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items must contain at least one non-empty item")
}

func TestSubmitWork_RejectsStructuredFileItemWithoutStagedReference(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	rec := submitWorkRequest(t, srv, `{"name":"missing-staged-ref","workTypeName":"prd","items":[{"type":"document","url":"file://staged/spec.pdf","stagedFileRef":"","fileName":"spec.pdf","mediaType":"application/pdf"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items[0].stagedFileRef must be a non-empty string")
}

func TestSubmitWork_RejectsForgedStructuredFileReference(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"forged-staged-ref","workTypeName":"prd","items":[{"type":"image","url":"file://staged/ui.png","stagedFileRef":"staged://forged-ui.png","fileName":"ui.png","mediaType":"image/png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items[0]: stagedFileRef must be a backend-issued staged file reference")
}

func TestStageSubmitWorkFile(t *testing.T) {
	srv := newWorkTransportTestServer(nil)

	response := stageSubmitWorkTestFile(t, srv, "image", "ui.png", "image/png", []byte("png-bytes"))
	if response.FileName != "ui.png" || response.MediaType != "image/png" {
		t.Fatalf("stage response = %#v, want identifying metadata", response)
	}
	if response.StagedFileRef == "" {
		t.Fatalf("stagedFileRef must be non-empty")
	}
	if response.Url == "" {
		t.Fatalf("url must be non-empty")
	}
	if string(response.Url) != "file://staged/ui.png" {
		t.Fatalf("stage url = %q, want fake Work service URL", response.Url)
	}
}

func stageSubmitWorkTestFile(
	t *testing.T,
	srv *Server,
	itemType string,
	fileName string,
	mediaType string,
	content []byte,
) factoryapi.StageSubmitWorkFileResponse {
	t.Helper()

	rec := submitWorkStageFileRequest(t, srv, "/factory-sessions/~default/work/staged-files", `{
		"itemType":"`+itemType+`",
		"fileName":"`+fileName+`",
		"mediaType":"`+mediaType+`",
		"contentBase64":"`+base64.StdEncoding.EncodeToString(content)+`"
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	return decodeJSONResponse[factoryapi.StageSubmitWorkFileResponse](t, rec)
}

func TestStageSubmitWorkFile_RejectsTextItemType(t *testing.T) {
	srv := newWorkTransportTestServer(nil)

	rec := submitWorkStageFileRequest(t, srv, "/factory-sessions/~default/work/staged-files", `{
		"itemType":"text",
		"fileName":"notes.txt",
		"mediaType":"text/plain",
		"contentBase64":"`+base64.StdEncoding.EncodeToString([]byte("text"))+`"
	}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "itemType must be one of image, video, audio, or document")
}

func TestStageSubmitWorkFileBySessionId_NotFound(t *testing.T) {
	srv := newWorkTransportTestServerWithRoles(nil, nil, factoryReadFake(factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound))

	rec := submitWorkStageFileRequest(t, srv, "/factory-sessions/session-missing/work/staged-files", `{
		"itemType":"document",
		"fileName":"spec.pdf",
		"mediaType":"application/pdf",
		"contentBase64":"`+base64.StdEncoding.EncodeToString([]byte("pdf"))+`"
	}`)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestSubmitWork_RejectsInvalidContentPartShape(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	rec := submitWorkRequest(t, srv, `{"workTypeName":"prd","content":[{"type":"image","text":"wrong-field"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].text is not supported")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work requests = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWork_RejectsInvalidExtendedContentMetadata(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	rec := submitWorkRequest(t, srv, `{"workTypeName":"prd","content":[{"type":"AUDIO","url":"file://voice.wav","metadata":"wrong"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].metadata must be an object")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work requests = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWork_CurrentChainingTraceIDPreservesRuntimeBoundary(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"chain-submit","workTypeName":"prd","currentChainingTraceId":"chain-submit-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if mf.WorkRequests[0].CurrentChainingTraceID != "chain-submit-1" || mf.WorkRequests[0].Works[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("current chaining trace IDs = %#v", mf.WorkRequests[0])
	}
}

func TestSubmitWork_CopiesTagMapBeforeRuntimeSubmission(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"tag-copy","workTypeName":"prd","payload":{"title":"Draft PRD"},"tags":{"priority":"high","team":"api"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("received Work requests = %#v, want one", mf.WorkRequests)
	}
	tags := mf.WorkRequests[0].Works[0].Tags
	if tags["priority"] != "high" || tags["team"] != "api" {
		t.Fatalf("received Work tags = %#v, want priority=high and team=api", tags)
	}
}

func TestSubmitWork_MatchingTraceAliasesNormalizeAtBoundary(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"chain-submit","workTypeName":"prd","currentChainingTraceId":"chain-submit-1","traceId":"chain-submit-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 || mf.WorkRequests[0].Works[0].TraceID != "chain-submit-1" {
		t.Fatalf("received Work request = %#v, want matching aliases", mf.WorkRequests)
	}
}

func TestSubmitWork_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "currentChainingTraceId and traceId must match when both are provided")

	rec := submitWorkRequest(t, srv, `{"workTypeName":"prd","currentChainingTraceId":"chain-submit-1","traceId":"trace-submit-1","payload":{"title":"Draft PRD"}}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "currentChainingTraceId and traceId must match when both are provided")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work request count = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWork_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "work_type_id is not supported; use workTypeName")

	rec := submitWorkRequest(t, srv, `{"work_type_id":"legacy-task","traceId":"test-trace-legacy","payload":{"title":"Legacy"}}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "work_type_id is not supported; use workTypeName")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work request count = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWork_TargetStateReturnsBadRequest(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "target_state is not supported; use state")

	rec := submitWorkRequest(t, srv, `{"name":"draft","workTypeName":"prd","target_state":"queued","payload":{"title":"Draft PRD"}}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "target_state is not supported; use state")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work request count = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWork_PreservesRuntimeRelations(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"runtime-relations","workTypeName":"prd","payload":{"title":"Draft PRD"},"relations":[{"type":"DEPENDS_ON","targetWorkId":"review-work","requiredState":"complete"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 || len(mf.WorkRequests[0].Works[0].RuntimeRelations) != 1 {
		t.Fatalf("received runtime relations = %#v, want one", mf.WorkRequests)
	}
	relation := mf.WorkRequests[0].Works[0].RuntimeRelations[0]
	if relation.Type != work.RelationDependsOn || relation.TargetWorkID != "review-work" || relation.RequiredState != "complete" {
		t.Fatalf("submitted relation = %#v, want dependency on review-work at complete", relation)
	}
}

func TestSubmitWork_WorkTypeNameWithWorkTypeIDReturnsBadRequest(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "work_type_id is not supported; use workTypeName")

	rec := submitWorkRequest(t, srv, `{"workTypeName":"tasks","work_type_id":"legacy-task","payload":"fix lint"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "work_type_id is not supported; use workTypeName")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work request count = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWorkMissingWorkType(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "workTypeName is required")

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/work", bytes.NewBufferString(`{"name":"missing-work-type","traceId":"test-trace-1"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "workTypeName is required")
}

func TestSubmitWorkMissingName(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "name is required")

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/work", bytes.NewBufferString(`{"workTypeName":"task","traceId":"test-trace-1","payload":{"title":"unnamed"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "name is required")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work request count = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWorkBlankName(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "name is required")

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/work", bytes.NewBufferString("{\"name\":\"   \\t\\n \",\"workTypeName\":\"task\",\"traceId\":\"test-trace-1\",\"payload\":{\"title\":\"blank\"}}"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "name is required")
	if len(mf.WorkRequests) != 0 {
		t.Fatalf("Work request count = %d, want 0", len(mf.WorkRequests))
	}
}

func TestSubmitWorkMarkdownPayload(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"markdown-fix","workTypeName":"tasks","traceId":"trace-markdown","payload":"# Fix lint\n\nRun gofmt."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("received Work requests = %#v, want one", mf.WorkRequests)
	}
	item := mf.WorkRequests[0].Works[0]
	payload, ok := item.Payload.([]byte)
	if item.WorkTypeID != "tasks" || !ok || string(payload) != `"# Fix lint\n\nRun gofmt."` {
		t.Fatalf("received Work = %#v, want tasks with markdown payload", item)
	}
}

func TestSubmitWorkInvalidPayload_ReturnsDocumentedBadRequest(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/work", bytes.NewBufferString(`{"workTypeName":`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestSubmitWorkUnknownWorkTypeReturnsBadRequest(t *testing.T) {
	srv := newWorkTransportTestServer(strictWorkAPIFake{submit: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
		return work.WorkRequestSubmitResult{}, errors.New(`work_request: works[0] ("unknown-work") references unknown work type "unknown"`)
	}})

	rec := submitWorkRequest(t, srv, `{"name":"unknown-work","workTypeName":"unknown","payload":"fix lint"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("unknown-work") references unknown work type name "unknown"`)
}

func TestSubmitWorkBySessionId_ReturnsWorkIdentityFields(t *testing.T) {
	sessionFactory, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-alpha/work", bytes.NewBufferString(`{"name":"scoped-draft","workTypeName":"task","traceId":"trace-scoped-submit","payload":{"title":"Scoped"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	assertSubmitWorkIdentityResponse(t, resp, submitWorkIdentityExpectation{
		name:         "scoped-draft",
		workTypeName: "task",
		traceId:      "trace-scoped-submit",
		accepted:     true,
	})
	if len(sessionFactory.WorkRequests) != 1 {
		t.Fatalf("session submitted work requests = %d, want 1", len(sessionFactory.WorkRequests))
	}
	if resp.RequestId == "" {
		t.Fatalf("requestId = %q, want non-empty normalized request id", resp.RequestId)
	}
	if stringValue(resp.WorkId) != "batch-"+resp.RequestId+"-scoped-draft" {
		t.Fatalf("workId = %q, want batch-%s-scoped-draft", stringValue(resp.WorkId), resp.RequestId)
	}
	if stringValue(resp.SessionId) != "session-alpha" {
		t.Fatalf("sessionId = %q, want session-alpha", stringValue(resp.SessionId))
	}
}

type submitWorkIdentityExpectation struct {
	name         string
	workTypeName string
	traceId      string
	accepted     bool
}

func assertSubmitWorkIdentityResponse(t *testing.T, resp factoryapi.SubmitWorkResponse, want submitWorkIdentityExpectation) {
	t.Helper()
	if resp.TraceId != want.traceId {
		t.Fatalf("traceId = %q, want %q", resp.TraceId, want.traceId)
	}
	if resp.Accepted != want.accepted {
		t.Fatalf("accepted = %v, want %v", resp.Accepted, want.accepted)
	}
	if stringValue(resp.Name) != want.name {
		t.Fatalf("name = %q, want %q", stringValue(resp.Name), want.name)
	}
	if stringValue(resp.WorkTypeName) != want.workTypeName {
		t.Fatalf("workTypeName = %q, want %q", stringValue(resp.WorkTypeName), want.workTypeName)
	}
	if want.name != "" && stringValue(resp.WorkId) == "" {
		t.Fatalf("workId = %q, want non-empty work id for named submit", stringValue(resp.WorkId))
	}
}

func TestSubmitWorkAutoTraceID(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/work", bytes.NewBufferString(`{"name":"auto-trace","workTypeName":"prd"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId == "" {
		t.Error("expected auto-generated trace_id, got empty")
	}
}

func TestServer_APISurfaceSmokePreservesEmbeddedFactoryContract(t *testing.T) {
	eventTime := testSubmitSurfaceSmokeEventTime()
	liveEvents := make(chan interfaces.FactoryEvent, 1)
	srv, mf := newSubmitSurfaceSmokeServer(t, eventTime, liveEvents)

	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	assertSubmitSurfaceSmokeSubmitAndList(t, server.URL, mf)
	assertSubmitSurfaceSmokeCurrentFactory(t, server.URL)
	assertSubmitSurfaceSmokeEvents(t, server.URL)
}

func testSubmitSurfaceSmokeEventTime() time.Time {
	return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
}

func newSubmitSurfaceSmokeServer(t *testing.T, eventTime time.Time, liveEvents chan interfaces.FactoryEvent) (*Server, *workAPIObservations) {
	t.Helper()

	currentFactoryID := "beta"
	observed, workRole := newRecordingWorkRole()
	observed.ReadItems = []work.ReadModel{{CursorID: "tok-api-surface-1", WorkID: "work-api-surface-1", Name: "api-surface-smoke", WorkTypeName: "task", State: &work.State{Name: "init", Type: work.StateTypeInitial}}}
	workRole.subscribe = func(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
		return &interfaces.FactoryEventStream{
			History: []interfaces.FactoryEvent{
				canonicalFactoryEventForHTTPTest(t, testFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "factory-event/work-request/api-surface-history", factoryapi.FactoryEventContext{
					Tick:      1,
					EventTime: eventTime,
					RequestId: stringPointerForAPITest("request-api-surface"),
				}, factoryapi.WorkRequestEventPayload{Type: factoryapi.WorkRequestTypeFactoryRequestBatch})),
			},
			Events: liveEvents,
		}, nil
	}
	definitions := factoryReadFake(factoryapi.Factory{Name: "beta", Id: &currentFactoryID}, nil)
	return newWorkTransportTestServerWithRoles(nil, workRole, definitions), observed
}

func assertSubmitSurfaceSmokeSubmitAndList(t *testing.T, serverURL string, mf *workAPIObservations) {
	t.Helper()

	submitResp, err := http.Post(serverURL+"/factory-sessions/~default/work", "application/json", bytes.NewBufferString(`{"name":"api-surface-smoke","workTypeName":"task","traceId":"trace-api-surface-smoke","payload":{"title":"API surface smoke"}}`))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer submitResp.Body.Close()
	if submitResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(submitResp.Body)
		t.Fatalf("POST /work status = %d, want 201: %s", submitResp.StatusCode, string(body))
	}

	var submitBody factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(submitResp.Body).Decode(&submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if submitBody.TraceId != "trace-api-surface-smoke" {
		t.Fatalf("submit trace_id = %q, want trace-api-surface-smoke", submitBody.TraceId)
	}
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("submitted work requests = %d, want 1", len(mf.WorkRequests))
	}

	listResp, err := http.Get(serverURL + "/factory-sessions/~default/work")
	if err != nil {
		t.Fatalf("GET /work: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		t.Fatalf("GET /work status = %d, want 200: %s", listResp.StatusCode, string(body))
	}
	var listBody factoryapi.ListWorkResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list work response: %v", err)
	}
	if len(listBody.Results) != 1 || stringValue(listBody.Results[0].WorkId) != "work-api-surface-1" {
		t.Fatalf("GET /work results = %#v, want work-api-surface-1", listBody.Results)
	}
	if mf.ListWorkCalls == 0 {
		t.Fatal("expected GET /work to call the Work read role")
	}
}

func assertSubmitSurfaceSmokeCurrentFactory(t *testing.T, serverURL string) {
	t.Helper()

	currentResp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	defer currentResp.Body.Close()
	if currentResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(currentResp.Body)
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200: %s", currentResp.StatusCode, string(body))
	}
	var currentBody factoryapi.Factory
	if err := json.NewDecoder(currentResp.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode current factory response: %v", err)
	}
	if currentBody.Name != "beta" {
		t.Fatalf("current factory name = %q, want beta", currentBody.Name)
	}
}

func assertSubmitSurfaceSmokeEvents(t *testing.T, serverURL string) {
	t.Helper()

	eventsReq, err := http.NewRequest(
		http.MethodGet,
		serverURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/events",
		nil,
	)
	if err != nil {
		t.Fatalf("new session-scoped /events request: %v", err)
	}
	eventsResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET session-scoped /events: %v", err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(eventsResp.Body)
		t.Fatalf("GET session-scoped /events status = %d, want 200: %s", eventsResp.StatusCode, string(body))
	}

	streamed := readSSEFactoryEvent(t, bufio.NewReader(eventsResp.Body))
	if streamed.Id != "factory-event/work-request/api-surface-history" {
		t.Fatalf("streamed event id = %q, want factory-event/work-request/api-surface-history", streamed.Id)
	}
}

func TestSubmitWorkResponseFromResult_IdempotentReplayPreservesWorkIdentity(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	request := work.WorkRequest{
		RequestID: "request-idem-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "draft-prd",
			WorkTypeID: "prd",
			TraceID:    "trace-idem-1",
		}},
	}

	first, err := workRole.SubmitWorkRequestForSession(context.Background(), factorysessions.DefaultSessionID, request)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second, err := workRole.SubmitWorkRequestForSession(context.Background(), factorysessions.DefaultSessionID, request)
	if err != nil {
		t.Fatalf("duplicate submit: %v", err)
	}

	resp1 := submitWorkResponseFromResult(first, "")
	resp2 := submitWorkResponseFromResult(second, "")
	if !resp1.Accepted || resp2.Accepted {
		t.Fatalf("accepted flags = %v/%v, want true then false", resp1.Accepted, resp2.Accepted)
	}
	if resp1.RequestId != "request-idem-1" || resp2.RequestId != resp1.RequestId {
		t.Fatalf("requestId = %q/%q, want request-idem-1", resp1.RequestId, resp2.RequestId)
	}
	if resp1.TraceId != "trace-idem-1" || resp2.TraceId != resp1.TraceId {
		t.Fatalf("traceId = %q/%q, want stable trace-idem-1", resp1.TraceId, resp2.TraceId)
	}
	if stringValue(resp1.WorkId) != "batch-request-idem-1-draft-prd" || stringValue(resp2.WorkId) != stringValue(resp1.WorkId) {
		t.Fatalf("workId = %q/%q, want batch-request-idem-1-draft-prd", stringValue(resp1.WorkId), stringValue(resp2.WorkId))
	}
	if stringValue(resp1.Name) != "draft-prd" || stringValue(resp2.Name) != "draft-prd" {
		t.Fatalf("name = %q/%q, want draft-prd", stringValue(resp1.Name), stringValue(resp2.Name))
	}
	if stringValue(resp1.WorkTypeName) != "prd" || stringValue(resp2.WorkTypeName) != "prd" {
		t.Fatalf("workTypeName = %q/%q, want prd", stringValue(resp1.WorkTypeName), stringValue(resp2.WorkTypeName))
	}
}
func TestSubmitWork_RejectsUnsupportedContentURLScheme(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "content[0].url url scheme must be one of file, http, https, or data")

	rec := submitWorkRequest(t, srv, `{"name":"bad-url","workTypeName":"prd","content":[{"type":"image","url":"ftp://example.com/ui.png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].url url scheme must be one of file, http, https, or data")
}

func TestSubmitWork_RejectsURLAndFileConflict(t *testing.T) {
	_, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)
	setWorkRequestPreparationError(srv, "content[0].url and file cannot both be set on the same content part")

	rec := submitWorkRequest(t, srv, `{"name":"conflict","workTypeName":"prd","content":[{"type":"image","url":"file://fixtures/ui.png","file":"fixtures/ui.png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].url and file cannot both be set on the same content part")
}

func TestSubmitWork_AcceptsLegacyFileOnlyContent(t *testing.T) {
	mf, workRole := newRecordingWorkRole()
	srv := newWorkTransportTestServer(workRole)

	rec := submitWorkRequest(t, srv, `{"name":"legacy-file","workTypeName":"prd","content":[{"type":"image","file":"fixtures/ui.png"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 || len(mf.WorkRequests[0].Works[0].Content) != 1 {
		t.Fatalf("received Work requests = %#v, want one image part", mf.WorkRequests)
	}
	part := mf.WorkRequests[0].Works[0].Content[0]
	if part.Type != work.WorkContentPartTypeImage || part.File != "fixtures/ui.png" {
		t.Fatalf("content[0] = %#v, want legacy image file preserved at the HTTP boundary", part)
	}
	if part.URL != "" {
		t.Fatalf("content[0].url = %q, want normalization deferred to Work", part.URL)
	}
}

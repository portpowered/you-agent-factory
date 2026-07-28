package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const defaultSessionWorkAPIPrefix = "/factory-sessions/" + factorysessions.DefaultSessionID

func scopeDefaultSessionWorkPath(path string) string {
	if path == "/work" || strings.HasPrefix(path, "/work?") || strings.HasPrefix(path, "/work/") {
		return defaultSessionWorkAPIPrefix + path
	}
	if strings.HasPrefix(path, "/work-requests/") {
		return defaultSessionWorkAPIPrefix + path
	}
	return path
}

type httpPromptTemplatesFake struct{}

func (httpPromptTemplatesFake) BuildPromptTemplateContract(
	inputCount int,
	docPaths []string,
) workerexecution.PromptTemplateContract {
	references := []workerexecution.PromptTemplateVariableReference{{
		Category: workerexecution.PromptTemplateVariableCategoryContext,
		Path:     ".Context.SessionID",
	}}
	for _, path := range docPaths {
		references = append(references, workerexecution.PromptTemplateVariableReference{
			Category: workerexecution.PromptTemplateVariableCategoryDoc,
			Path:     fmt.Sprintf(`.Docs[%q]`, path),
		})
	}
	return workerexecution.PromptTemplateContract{
		AvailableVariables: references,
		InputCount:         inputCount,
	}
}

func (httpPromptTemplatesFake) ValidatePromptTemplate(
	template string,
	inputCount int,
	_ []string,
) workerexecution.PromptTemplateValidationResult {
	if inputCount < 2 && strings.Contains(template, "(index .Inputs 1)") {
		return workerexecution.PromptTemplateValidationResult{
			Diagnostics: []workerexecution.PromptTemplateDiagnostic{{
				Kind: workerexecution.PromptTemplateDiagnosticKindUnavailableVariable,
				Path: ".Inputs[1]",
			}},
		}
	}
	return workerexecution.PromptTemplateValidationResult{Valid: true}
}

type workRequestPreparationFake struct {
	prepare func(context.Context, work.WorkRequestPreparation) (work.WorkRequest, error)
}

func (f *workRequestPreparationFake) PrepareWorkRequest(
	ctx context.Context,
	input work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	return f.prepare(ctx, input)
}

func setWorkRequestPreparationError(srv *Server, message string) {
	srv.Adapter = srv.Adapter.WithWorkService(work.AdmissionContentService(
		newContentStagingFake(),
		&workRequestPreparationFake{
			prepare: func(
				context.Context,
				work.WorkRequestPreparation,
			) (work.WorkRequest, error) {
				return work.WorkRequest{}, &work.RequestPreparationError{Message: message}
			},
		},
	))
}

func setWorkRequestPreparationResult(
	srv *Server,
	prepare func(work.WorkRequestPreparation) work.WorkRequest,
) {
	srv.Adapter = srv.Adapter.WithWorkService(work.AdmissionContentService(
		newContentStagingFake(),
		&workRequestPreparationFake{
			prepare: func(
				_ context.Context,
				input work.WorkRequestPreparation,
			) (work.WorkRequest, error) {
				return prepare(input), nil
			},
		},
	))
}

type contentStagingFake struct {
	stageContent   func(context.Context, work.StageContentRequest) (work.StageContentResult, error)
	prepareContent func(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error)
	resolveContent func(context.Context, string) (work.ResolvedStagedContent, error)
	cleanupContent func(context.Context, string) error
}

func newContentStagingFake() *contentStagingFake {
	staged := make(map[string]work.StageContentResult)
	return &contentStagingFake{
		stageContent: func(_ context.Context, request work.StageContentRequest) (work.StageContentResult, error) {
			switch request.ItemType {
			case "image", "video", "audio", "document":
			default:
				return work.StageContentResult{}, &work.ContentStagingError{
					Message: "itemType must be one of image, video, audio, or document",
				}
			}
			ref := "test-staged:" + request.FileName
			result := work.StageContentResult{
				StagedFileRef: ref,
				FileName:      request.FileName,
				MediaType:     request.MediaType,
				URL:           "file://staged/" + request.FileName,
			}
			staged[ref] = result
			return result, nil
		},
		prepareContent: func(_ context.Context, items []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
			content := make([]work.WorkContentPart, 0, len(items))
			for index, item := range items {
				if item.ItemType == "text" {
					content = append(content, work.WorkContentPart{
						Type: work.WorkContentPartTypeText,
						Text: item.Text,
					})
					continue
				}
				result, ok := staged[item.StagedFileRef]
				if !ok {
					return nil, &work.ContentStagingError{
						Message: fmt.Sprintf("items[%d]: stagedFileRef must be a backend-issued staged file reference", index),
					}
				}
				partType := work.WorkContentPartTypeBinary
				switch item.ItemType {
				case "image":
					partType = work.WorkContentPartTypeImage
				case "audio":
					partType = work.WorkContentPartTypeAudio
				}
				content = append(content, work.WorkContentPart{
					Type:        partType,
					URL:         result.URL,
					ContentType: item.MediaType,
					Metadata: map[string]any{
						submitWorkItemTypeMetadataKey: item.ItemType,
						submitWorkFileNameMetadataKey: item.FileName,
					},
				})
			}
			return content, nil
		},
		resolveContent: func(context.Context, string) (work.ResolvedStagedContent, error) {
			return work.ResolvedStagedContent{}, errors.New("unexpected ResolveContent call")
		},
		cleanupContent: func(context.Context, string) error {
			return errors.New("unexpected CleanupContent call")
		},
	}
}

func (f *contentStagingFake) StageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	return f.stageContent(ctx, request)
}

func (f *contentStagingFake) PrepareContent(
	ctx context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	return f.prepareContent(ctx, items)
}

func (f *contentStagingFake) ResolveContent(
	ctx context.Context,
	ref string,
) (work.ResolvedStagedContent, error) {
	return f.resolveContent(ctx, ref)
}

func (f *contentStagingFake) CleanupContent(ctx context.Context, ref string) error {
	return f.cleanupContent(ctx, ref)
}

type providerSessionCall struct {
	provider string
	kind     string
	id       string
	detail   providersessions.Detail
	err      error
}

func providerSessionSuccess(provider, id string, detail providersessions.Detail) providerSessionCall {
	return providerSessionCall{provider: provider, kind: providersessions.SessionIDKind, id: id, detail: detail}
}

func providerSessionFailure(provider, kind, id string, err error) providerSessionCall {
	return providerSessionCall{provider: provider, kind: kind, id: id, err: err}
}

type strictProviderSessionRole struct {
	t     *testing.T
	calls []providerSessionCall
	next  int
}

func (role *strictProviderSessionRole) Details(provider, kind, id string) (providersessions.Detail, error) {
	role.t.Helper()
	if role.next >= len(role.calls) {
		role.t.Fatalf("unexpected Provider Sessions Details(%q, %q, %q)", provider, kind, id)
	}
	want := role.calls[role.next]
	role.next++
	if provider != want.provider || kind != want.kind || id != want.id {
		role.t.Fatalf("Provider Sessions Details = (%q, %q, %q), want (%q, %q, %q)", provider, kind, id, want.provider, want.kind, want.id)
	}
	return want.detail, want.err
}

func (role *strictProviderSessionRole) Inspect(req providersessions.InspectRequest) (providersessions.InspectResult, error) {
	role.t.Helper()
	role.t.Fatalf("unexpected Provider Sessions Inspect(%#v)", req.Session)
	return providersessions.InspectResult{}, nil
}

func (role *strictProviderSessionRole) Project(req providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	role.t.Helper()
	role.t.Fatalf("unexpected Provider Sessions Project(%#v)", req.Session)
	return providersessions.ProjectResult{}, nil
}

func newTestServerWithProviderSessionCalls(t *testing.T, calls ...providerSessionCall) *Server {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	return newTestServerWithProviderSessionCallsAndLogger(t, logger, calls...)
}

func newTestServerWithProviderSessionCallsAndLogger(t *testing.T, logger *zap.Logger, calls ...providerSessionCall) *Server {
	t.Helper()
	role := &strictProviderSessionRole{t: t, calls: calls}
	t.Cleanup(func() {
		if role.next != len(role.calls) {
			t.Errorf("Provider Sessions Details calls = %d, want %d", role.next, len(role.calls))
		}
	})
	return newServerFromRoles(
		nil, nil, nil, nil, nil, nil, &modelshttp.Handler{},
		nil, httpFactoryValidator{}, nil,
		nil, nil, nil, nil, nil, nil, role, nil, nil, nil, nil, logger,
	)
}

func providerSessionLookupFailure(provider providersessions.Provider, root string, err error) error {
	return &providersessions.LookupError{Provider: provider, Root: root, Err: err}
}

func cursorProviderSessionDetail(id, relativePath string) providersessions.Detail {
	inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens, totalTokens := 100, 25, 40, 10, 175
	text := "Hello from API fixture"
	return providersessions.Detail{
		ProviderSession: providersessions.Ref{Provider: providersessions.ProviderCursor, Kind: providersessions.SessionIDKind, ID: id},
		Source:          providersessions.SourceMetadata{RelativePath: relativePath, SizeBytes: 1},
		Parse: providersessions.ParseSummary{EventCount: 1, LineCount: 1, TokenUsage: &providersessions.TokenUsage{
			InputTokens: &inputTokens, OutputTokens: &outputTokens, CachedInputTokens: &cacheReadTokens,
			CacheWriteTokens: &cacheWriteTokens, TotalTokens: &totalTokens,
		}},
		Transcript: []providersessions.TranscriptEntry{{Order: 1, Text: &text, Type: providersessions.TranscriptAssistantMessage}},
	}
}

func codexProviderSessionDetail(id, relativePath string, eventCount int) providersessions.Detail {
	reasoningSource := "reasoning"
	return providersessions.Detail{
		ProviderSession: providersessions.Ref{Provider: providersessions.ProviderCodex, Kind: providersessions.SessionIDKind, ID: id},
		Source:          providersessions.SourceMetadata{RelativePath: relativePath, SizeBytes: 1},
		Parse: providersessions.ParseSummary{
			EventCount: eventCount, LineCount: 4, MalformedLineCount: 1, UnknownEventCount: 1,
			ParseErrors: []providersessions.LineError{{LineNumber: 4}}, UnknownEvents: []providersessions.UnknownEvent{{LineNumber: 3}},
			Reasoning: []providersessions.ReasoningSummary{{SourceType: reasoningSource}}, Turns: []providersessions.TurnSummary{{ReasoningCount: 1}},
		},
		Transcript: []providersessions.TranscriptEntry{{Order: 1, Type: providersessions.TranscriptReasoning}},
	}
}

// customerCursorProviderSessionID mirrors the UUID-shaped session_id from the
// reported Windows provider-session detail failure mode.
const customerCursorProviderSessionID = "ed332681-38eb-485f-b3d3-d8b6df3a450b"

// customerCursorWorkspaceHash mirrors the workspace-hash directory layout under ~/.cursor/chats.
const customerCursorWorkspaceHash = "d2191e81bfe68d31807c1e354ea83571"

// providerSessionDetailURLFromEventRef builds the GET path the dashboard uses to
// load a provider session from an event-emitted LoadableProviderSessionRef.
func providerSessionDetailURLFromEventRef(ref factoryapi.LoadableProviderSessionRef) string {
	query := url.Values{}
	query.Set("provider", string(ref.Provider))
	query.Set("kind", string(ref.Kind))
	query.Set("id", ref.Id)
	return "/provider-sessions/detail?" + query.Encode()
}

func assertProviderSessionDetailLoadsFromEventRef(
	t *testing.T,
	srv *Server,
	ref factoryapi.LoadableProviderSessionRef,
	wantProvider factoryapi.LoadableProviderSessionProvider,
) factoryapi.ProviderSessionDetailResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, providerSessionDetailURLFromEventRef(ref), nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for event ref %#v: %s", rec.Code, ref, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != string(wantProvider) ||
		string(resp.ProviderSession.Kind) != string(ref.Kind) ||
		resp.ProviderSession.Id != ref.Id {
		t.Fatalf("provider session = %#v, want provider=%s kind=%s id=%s", resp.ProviderSession, wantProvider, ref.Kind, ref.Id)
	}
	if resp.Source.RelativePath == "" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted source metadata for event ref %#v", resp.Source, ref)
	}
	return resp
}

func readSSEFactoryEvent(t *testing.T, reader *bufio.Reader) factoryapi.FactoryEvent {
	t.Helper()

	var dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "event:") {
			t.Fatalf("factory event stream should use default SSE message event, got line %q", line)
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}

	if dataLine == "" {
		t.Fatal("expected SSE data payload")
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		t.Fatalf("decode SSE factory event: %v", err)
	}
	return event
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string, wantMessage string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp factoryapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(resp.Code) != wantCode {
		t.Fatalf("error code = %q, want %q", resp.Code, wantCode)
	}
	if resp.Message != wantMessage {
		t.Fatalf("error message = %q, want %q", resp.Message, wantMessage)
	}
}

func assertGeneratedWorkContentParts(t *testing.T, content *factoryapi.WorkContent, want []work.WorkContentPart) {
	t.Helper()
	if content == nil {
		t.Fatalf("content = nil, want %#v", want)
	}
	if len(*content) != len(want) {
		t.Fatalf("content count = %d, want %d", len(*content), len(want))
	}
	for i, wantPart := range want {
		assertGeneratedWorkContentPart(t, (*content)[i], i, wantPart)
	}
}

func extractInvocationRequestText(t *testing.T, request *factoryapi.InvocationRequest) string {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.Content == nil {
		t.Fatal("content = nil, want one text part")
	}
	if len(*request.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(*request.Content))
	}
	part, err := (*request.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("AsWorkTextContentPart: %v", err)
	}
	return part.Text
}

func assertGeneratedWorkContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	switch want.Type {
	case work.WorkContentPartTypeText:
		assertGeneratedTextContentPart(t, got, index, want)
	case work.WorkContentPartTypeImage:
		assertGeneratedImageContentPart(t, got, index, want)
	case work.WorkContentPartTypeAudio:
		assertGeneratedAudioContentPart(t, got, index, want)
	case work.WorkContentPartTypeJSON:
		assertGeneratedJSONContentPart(t, got, index, want)
	case work.WorkContentPartTypeBinary:
		assertGeneratedBinaryContentPart(t, got, index, want)
	default:
		t.Fatalf("unsupported expected content type %q", want.Type)
	}
}

func assertGeneratedTextContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode text: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeText || part.Text != want.Text {
		t.Fatalf("content[%d] = %#v, want text %q", index, part, want.Text)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedImageContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode image: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeImage || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want image url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedAudioContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode audio: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeAudio || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want audio url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedJSONContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode json: %v", index, err)
	}
	rawJSON, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("content[%d] encode json: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeJSON || string(rawJSON) != string(want.JSON) {
		t.Fatalf("content[%d] = %#v, want json %s", index, part, want.JSON)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedBinaryContentPart(t *testing.T, got factoryapi.WorkContentPart, index int, want work.WorkContentPart) {
	t.Helper()
	part, err := got.AsWorkBinaryContentPart()
	if err != nil {
		t.Fatalf("content[%d] decode binary: %v", index, err)
	}
	if part.Type != factoryapi.WorkContentPartTypeBinary || string(part.Url) != want.URL || deprecatedGeneratedFile(part.File) != want.File {
		t.Fatalf("content[%d] = %#v, want binary url %q", index, part, want.URL)
	}
	assertGeneratedPartSharedFields(t, index, part.Slot, part.Label, part.Role, part.ContentType, part.ArtifactId, part.Metadata, want)
}

func assertGeneratedPartSharedFields(t *testing.T, index int, slot *string, label *string, role *string, contentType *string, artifactID *string, metadata *factoryapi.WorkContentMetadata, want work.WorkContentPart) {
	t.Helper()

	if derefString(slot) != want.Slot ||
		derefString(label) != want.Label ||
		derefString(role) != want.Role ||
		derefString(contentType) != want.ContentType ||
		derefString(artifactID) != want.ArtifactID {
		t.Fatalf("content[%d] shared fields mismatch for %#v", index, want)
	}
	gotMetadata, _ := json.Marshal(metadata)
	wantMetadata, _ := json.Marshal(want.Metadata)
	if string(gotMetadata) != string(wantMetadata) {
		t.Fatalf("content[%d] metadata = %s, want %s", index, gotMetadata, wantMetadata)
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func deprecatedGeneratedFile(file *factoryapi.WorkContentDeprecatedFileProperty) string {
	if file == nil {
		return ""
	}
	return string(*file)
}

func testFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	t.Helper()
	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = eventPayload.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	default:
		t.Fatalf("unsupported test factory event payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode test factory event payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context:       context,
		Payload:       eventPayload,
	}
}

func stringPointerForAPITest(value string) *string {
	return &value
}

func validNamedFactoryBody(name, workType string) string {
	return fmt.Sprintf(`{"name":%q,%s`, name, strings.TrimPrefix(namedFactoryPayloadJSON(name, workType), "{"))
}

func saveFactoryForSessionRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func namedFactoryPayloadJSON(project, workType string) string {
	return fmt.Sprintf(`{
		"name": %q,
		"id": %q,
		"workTypes": [{
			"name": %q,
			"states": [
				{"name":"init","type":"INITIAL"},
				{"name":"done","type":"TERMINAL"},
				{"name":"failed","type":"FAILED"}
			]
		}],
		"workers": [{
			"name":"planner",
			"type":"MODEL_WORKER",
			"modelProvider":"CLAUDE",
			"executorProvider":"SCRIPT_WRAP",
			"model":"claude-sonnet-4-20250514"
		}],
		"workstations": [{
			"name":"plan-task",
			"behavior":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"planner",
			"inputs":[{"workType":%q,"state":"init"}],
			"outputs":[{"workType":%q,"state":"done"}]
		}]
	}`, project, project, workType, workType, workType)
}

func listedWorkByID(t *testing.T, works []factoryapi.Work, workID string) factoryapi.Work {
	t.Helper()
	for _, work := range works {
		if stringValue(work.WorkId) == workID {
			return work
		}
	}
	t.Fatalf("listed work %q not found in %#v", workID, works)
	return factoryapi.Work{}
}

func submitWorkRequest(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, scopeDefaultSessionWorkPath("/work"), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func submitWorkStageFileRequest(
	t *testing.T,
	srv *Server,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, scopeDefaultSessionWorkPath(path), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func upsertWorkRequest(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, scopeDefaultSessionWorkPath(path), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeJSONResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	return out
}

func encodeNextToken(token string) string {
	return base64.StdEncoding.EncodeToString([]byte(token))
}

func decodeListWorkPage(t *testing.T, srv *Server, path string) factoryapi.ListWorkResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, scopeDefaultSessionWorkPath(path), nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200: %s", path, rec.Code, rec.Body.String())
	}
	return decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
}

func assertListedWorkIDs(t *testing.T, works []factoryapi.Work, want []string) {
	t.Helper()
	if len(works) != len(want) {
		t.Fatalf("results = %d, want %d: %#v", len(works), len(want), works)
	}
	for i, wantWorkID := range want {
		if got := stringValue(works[i].WorkId); got != wantWorkID {
			t.Fatalf("result[%d].workId = %q, want %q: %#v", i, got, wantWorkID, works)
		}
	}
}

type upsertValidationFailureCase struct {
	name                 string
	path                 string
	body                 string
	submitError          string
	workPreparationError string
	wantMsg              string
}

func runUpsertValidationFailureCases(t *testing.T, cases []upsertValidationFailureCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observed, workRole := newRecordingWorkRole()
			if tc.submitError != "" {
				workRole.submit = func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
					return work.WorkRequestSubmitResult{}, errors.New(tc.submitError)
				}
			}
			sessions := strictLiveSessionAPIFake{get: func(_ context.Context, sessionID string) (factoryapi.FactorySession, error) {
				return factoryapi.FactorySession{Id: sessionID}, nil
			}}
			srv := newFactorySessionRolesTestServer(sessions, workRole, factoryReadFake(factoryapi.Factory{Name: "test-factory"}, nil), nil)
			if tc.workPreparationError != "" {
				setWorkRequestPreparationError(srv, tc.workPreparationError)
			}

			rec := upsertWorkRequest(t, srv, tc.path, tc.body)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", tc.wantMsg)
			if len(observed.WorkRequests) != 0 {
				t.Fatalf("Work request count = %d, want 0", len(observed.WorkRequests))
			}
		})
	}
}

type strictJSONTestPayload struct {
	Name string `json:"name"`
}

func TestDecodeStrictJSON_ValidObject(t *testing.T) {
	got, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha"}`))
	if err != nil {
		t.Fatalf("decodeStrictJSON() error = %v, want nil", err)
	}
	if got.Name != "alpha" {
		t.Fatalf("decoded name = %q, want alpha", got.Name)
	}
}

func TestDecodeStrictJSON_UnknownField(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha","extra":1}`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), `json: unknown field "extra"`) {
		t.Fatalf("decodeStrictJSON() error = %v, want unknown field message", err)
	}
}

func TestDecodeStrictJSON_MalformedJSON(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want malformed JSON error")
	}
}

func TestDecodeStrictJSON_EmptyBody(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(""))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("decodeStrictJSON() error = %v, want io.EOF", err)
	}
}

func TestDecodeStrictJSON_MultiObjectPayload(t *testing.T) {
	_, err := decodeStrictJSON[strictJSONTestPayload](strings.NewReader(`{"name":"alpha"}{}`))
	if err == nil {
		t.Fatal("decodeStrictJSON() error = nil, want single-object validation error")
	}
	message, ok := requestFieldValidationMessage(err)
	if !ok {
		t.Fatalf("decodeStrictJSON() error = %T(%v), want requestFieldValidationError", err, err)
	}
	if message != "request payload must contain one JSON object" {
		t.Fatalf("validation message = %q, want single-object payload message", message)
	}
}

func TestGetProviderSessionDetails_LoadsTimestampPrefixedCodexSessionFromConfiguredRoot(t *testing.T) {
	id := "019e44f4-580e-7f32-981e-1e54ec6907d6"
	srv := newTestServerWithProviderSessionCalls(t, providerSessionSuccess(
		"codex", id, codexProviderSessionDetail(id, "2026/05/18/rollout-2026-05-20T17-35-24-"+id+".jsonl", 2),
	))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=019e44f4-580e-7f32-981e-1e54ec6907d6", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl" || resp.ProviderSession.Id != "019e44f4-580e-7f32-981e-1e54ec6907d6" || resp.Parse.EventCount != 2 {
		t.Fatalf("provider session detail = %#v, want timestamp-prefixed session path", resp)
	}
}

func TestGetProviderSessionDetails_PrefersExactCodexSessionFileWhenSupportedLayoutsBothExist(t *testing.T) {
	srv := newTestServerWithProviderSessionCalls(t, providerSessionSuccess(
		"codex", "sess_123", codexProviderSessionDetail("sess_123", "2026/05/18/rollout-sess_123.jsonl", 3),
	))
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" {
		t.Fatalf("relative path = %q, want exact rollout basename", resp.Source.RelativePath)
	}
}

package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelcontract "github.com/portpowered/infinite-you/pkg/services/models"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func newAPITestServer(roles any) *api.Server {
	logger, _ := zap.NewDevelopment()
	modelsHandler := &modelshttp.Handler{}
	modelsService := apiTestRole[modelcontract.Service](roles)
	if modelsService != nil {
		modelsHandler = modelshttp.NewHandler(
			modelshttp.NewAdapter(modelsService, modelHTTPTestScope()),
			logger,
		)
	}
	return newAPIServerFromRoles(
		apiTestRole[apisurface.RuntimeAPI](roles),
		nil,
		apiTestRole[apisurface.LiveSessionAPI](roles),
		apiTestRole[apisurface.WorkAPI](roles),
		apiTestRole[apisurface.WorkReadAPI](roles),
		apiTestRole[apisurface.InvocationAPI](roles),
		modelsHandler,
		apiTestRole[apisurface.FactorySaveAPI](roles),
		programmableFactoryValidator{}, apiWorkflowDefinitionsFake(),
		apiTestRole[apisurface.DurableSessionExecutionAPI](roles),
		apiTestRole[apisurface.DurableSessionLifecycleAPI](roles),
		apiTestRole[apisurface.DurableSessionListingAPI](roles),
		apiTestRole[apisurface.DurableSessionProjectionAPI](roles),
		apiTestRole[api.DurableExecutionSessionLister](roles),
		nil, nil,
		apiPromptTemplatesFake{},
		nil, nil, nil,
		logger,
	)
}

func modelHTTPTestScope() modelcontract.RuntimeScopeRef {
	scope, err := (modelcontract.RuntimeScopeRef{}).Parse("factory-session:http-transport-test")
	if err != nil {
		panic(err)
	}
	return scope
}

func apiTestRole[T any](candidate any) T {
	var zero T
	if candidate == nil {
		return zero
	}
	role, ok := candidate.(T)
	if !ok {
		return zero
	}
	return role
}

type apiWorkflowDefinitionsScript struct {
	previewWorkflow func(factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error)
}

func (script apiWorkflowDefinitionsScript) PreviewWorkflow(_ context.Context, input factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error) {
	return script.previewWorkflow(input)
}

func apiWorkflowDefinitionsFake() factoryruntime.WorkflowPreviewOperation {
	return apiWorkflowDefinitionsScript{
		previewWorkflow: func(input factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error) {
			sourceRef := factoryruntime.WorkflowSourceProjectClaudeWorkflowsDir + "/" + input.Source.Value + ".js"
			preview := factoryruntime.WorkflowPreview{
				Valid: true,
				SourceResolution: factoryruntime.WorkflowSourceResolution{
					RequestKind:  input.Source.Kind,
					RequestValue: input.Source.Value,
					ResolvedKind: input.Source.Kind,
					SourceRef:    sourceRef,
					SourceHash:   "sha256:http-preview",
					Found:        true,
					ArtifactRoot: factoryruntime.WorkflowSourceArtifactRootDecision{Allowed: true},
				},
				PolicyPreview: factoryruntime.JavaScriptPolicyPreview{PolicyHash: "sha256:http-preview-policy"},
				ResultConstraints: factoryruntime.WorkflowResultConstraints{
					RequiresStructuredCloneableJSON: true,
					ArtifactURIScheme:               "you-artifact",
				},
			}
			if input.Source.Value == "unsafe" {
				preview.Valid = false
				preview.SourceValidationIssues = []factoryruntime.WorkflowPreviewSourceValidationIssue{{
					Code:    factoryruntime.WorkflowValidationCodeForbiddenHostAccess,
					Message: "host filesystem access is unavailable",
					Path:    sourceRef,
				}}
			}
			return preview, nil
		},
	}
}

type apiPromptTemplatesFake struct{}

type dashboardEventWorkAPI struct {
	apisurface.WorkAPI
	subscribe func(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	probe     func(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error
}

func (role dashboardEventWorkAPI) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return role.subscribe(ctx, sessionID, reconnect)
}

func (role dashboardEventWorkAPI) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) error {
	return role.probe(ctx, sessionID, reconnect)
}

func (apiPromptTemplatesFake) BuildPromptTemplateContract(
	inputCount int,
	_ []string,
) workers.PromptTemplateContract {
	return workers.PromptTemplateContract{InputCount: inputCount}
}

func (apiPromptTemplatesFake) ValidatePromptTemplate(
	string,
	int,
	[]string,
) workers.PromptTemplateValidationResult {
	return workers.PromptTemplateValidationResult{Valid: true}
}

func readAPISSEFactoryEvent(t *testing.T, reader *bufio.Reader) factoryapi.FactoryEvent {
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

func embeddedAPIDashboardAssetPath(t *testing.T, html string) string {
	t.Helper()

	pattern := regexp.MustCompile(`(?:src|href)="(/dashboard/ui/assets/[^"]+)"`)
	matches := pattern.FindStringSubmatch(html)
	if len(matches) != 2 {
		t.Fatalf("expected embedded dashboard asset path in html: %s", html)
	}
	return matches[1]
}

func testAPIFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
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

func stringPointerForAPIServerTest(value string) *string {
	return &value
}

func TestDeprecatedFactoryApiRoutesAreNotRegistered(t *testing.T) {
	srv := newAPITestServer(nil)
	for _, path := range []string{"/dashboard", "/dashboard/stream", "/events", "/state", "/traces/trace-id", "/work/token-1/trace", "/workflows", "/workflows/wf-1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestGetDashboardUI_ReturnsEmbeddedShell(t *testing.T) {
	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"<title>You Agent Factory Dashboard</title>", "Standalone live dashboard shell for You Agent Factory.", "You%20Agent%20Factory%20dashboard%20icon", "<div id=\"root\"></div>", "/dashboard/ui/assets/"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected embedded dashboard shell to contain %q, got body: %s", want, rec.Body.String())
		}
	}
	for _, retired := range []string{"Infinite You Dashboard", "Standalone live dashboard shell for Infinite You.", "Infinite%20You%20dashboard%20icon"} {
		if strings.Contains(rec.Body.String(), retired) {
			t.Fatalf("expected embedded dashboard shell to retire %q, got body: %s", retired, rec.Body.String())
		}
	}
}

func TestGetDashboardUI_ServesEmbeddedAsset(t *testing.T) {
	srv := newAPITestServer(nil)
	shellReq := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	shellRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(shellRec, shellReq)

	assetReq := httptest.NewRequest(http.MethodGet, embeddedAPIDashboardAssetPath(t, shellRec.Body.String()), nil)
	assetRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK || assetRec.Body.Len() == 0 {
		t.Fatalf("embedded asset response = status %d len %d", assetRec.Code, assetRec.Body.Len())
	}
}

func TestGetDashboardUI_FallbacksToIndexForClientRoutes(t *testing.T) {
	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/ui/workstations/live", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<div id=\"root\"></div>") {
		t.Fatalf("expected SPA fallback index shell, got status %d body: %s", rec.Code, rec.Body.String())
	}
}

func TestSessionScopedLiveGetEvents_JSONRecoveryProbeReturnsCursorStaleForLiveSession(t *testing.T) {
	sessionID := "session-alpha"
	workAPI := dashboardEventWorkAPI{
		probe: func(_ context.Context, gotSessionID string, cursor *factorydefinitions.FactoryEventReconnectCursor) error {
			if gotSessionID != sessionID || cursor == nil || cursor.AfterEventID != "missing-event-id" {
				t.Fatalf("reconnect request = (%q, %#v), want (%q, missing-event-id)", gotSessionID, cursor, sessionID)
			}
			return factorysessions.ErrReconnectCursorNotFound
		},
	}

	server := httptest.NewServer(newAPITestServer(workAPI).Handler())
	defer server.Close()

	eventPath := "/factory-sessions/" + sessionID + "/events?after_event_id=missing-event-id"
	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+eventPath,
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET recovery probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.NewDecoder(resp.Body).Decode(&recovery); err != nil {
		t.Fatalf("decode recovery response: %v", err)
	}
	if recovery.FactorySessionId != sessionID {
		t.Fatalf("factorySessionId = %q, want %q", recovery.FactorySessionId, sessionID)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcome("CURSOR_STALE") {
		t.Fatalf("outcome = %q, want CURSOR_STALE", recovery.Outcome)
	}
	if !recovery.Retry.OmitAfterEventId || !recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want both omit flags true", recovery.Retry)
	}
	assertSessionScopedFactoryEventsPath(t, eventPath, sessionID)
}

func assertSessionScopedFactoryEventsPath(t *testing.T, path, sessionID string) {
	t.Helper()
	wantPrefix := "/factory-sessions/" + sessionID + "/events"
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("event stream path = %q, want session-scoped prefix %q", path, wantPrefix)
	}
	if path == "/events" || strings.HasPrefix(path, "/events?") {
		t.Fatalf("event stream path = %q, must not use compatibility-only GET /events", path)
	}
}

func TestSessionScopedLiveGetEvents_JSONRecoveryProbeReturnsUnknownSessionForMissingLiveSession(t *testing.T) {
	server := httptest.NewServer(newAPITestServer(dashboardEventWorkAPI{
		probe: func(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error {
			return apisurface.ErrFactorySessionNotFound
		},
	}).Handler())
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+"/factory-sessions/session-missing/events?after_event_id=missing-event-id",
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET recovery probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.NewDecoder(resp.Body).Decode(&recovery); err != nil {
		t.Fatalf("decode recovery response: %v", err)
	}
	if recovery.FactorySessionId != "session-missing" {
		t.Fatalf("factorySessionId = %q, want session-missing", recovery.FactorySessionId)
	}
	if recovery.Outcome != factoryapi.FactorySessionEventStreamRecoveryOutcome("UNKNOWN_SESSION") {
		t.Fatalf("outcome = %q, want UNKNOWN_SESSION", recovery.Outcome)
	}
	if recovery.Retry.OmitAfterEventId || recovery.Retry.OmitAfterSequence {
		t.Fatalf("retry = %#v, want omit flags false", recovery.Retry)
	}
}

func TestSessionScopedLiveGetEvents_ValidReconnectCursorStillStreamsSSEForLiveSession(t *testing.T) {
	eventTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	sessionID := "session-alpha"
	historical := testHistoricalFactoryEvents(t, eventTime)
	domainHistory := apiFactoryEvents(t, historical)
	workAPI := dashboardEventWorkAPI{
		subscribe: func(_ context.Context, gotSessionID string, cursor *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
			if gotSessionID != sessionID || cursor == nil || cursor.AfterEventID != historical[1].Id {
				t.Fatalf("reconnect request = (%q, %#v), want (%q, %q)", gotSessionID, cursor, sessionID, historical[1].Id)
			}
			return &factorydefinitions.FactoryEventStream{
				FactorySessionID: sessionID,
				History:          append([]factorydefinitions.FactoryEvent(nil), domainHistory[2:]...),
				Events:           make(chan factorydefinitions.FactoryEvent),
			}, nil
		},
	}

	server := httptest.NewServer(newAPITestServer(workAPI).Handler())
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+"/factory-sessions/"+sessionID+"/events?after_event_id="+historical[1].Id,
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	event := readAPISSEFactoryEvent(t, reader)
	if event.Id != historical[2].Id {
		t.Fatalf("event id = %q, want %q", event.Id, historical[2].Id)
	}
}

func testHistoricalFactoryEvents(t *testing.T, eventTime time.Time) []factoryapi.FactoryEvent {
	t.Helper()

	runStartedFactory := factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL}}}},
	}
	return []factoryapi.FactoryEvent{
		testAPIFactoryEvent(t, factoryapi.FactoryEventTypeRunRequest, "factory-event/run-started", factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime}, factoryapi.RunRequestEventPayload{RecordedAt: eventTime, Factory: runStartedFactory}),
		testAPIFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-event/initial-structure/0", factoryapi.FactoryEventContext{Tick: 0, EventTime: eventTime}, factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{Name: "factory"}}),
		testAPIFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "factory-event/work-request/request-1", factoryapi.FactoryEventContext{Tick: 1, EventTime: time.Date(2026, 4, 8, 12, 0, 1, 0, time.UTC), RequestId: stringPointerForAPIServerTest("request-1")}, factoryapi.WorkRequestEventPayload{Type: factoryapi.WorkRequestTypeFactoryRequestBatch}),
	}
}

func apiFactoryEvents(t *testing.T, events []factoryapi.FactoryEvent) []factorydefinitions.FactoryEvent {
	t.Helper()
	converted := make([]factorydefinitions.FactoryEvent, 0, len(events))
	for index, event := range events {
		domainEvent, err := factorydefinitions.NewFactoryEvent(event)
		if err != nil {
			t.Fatalf("convert FactoryEvent[%d]: %v", index, err)
		}
		converted = append(converted, domainEvent)
	}
	return converted
}

func TestDashboardSnapshotRoutes_RemovedFromRouter(t *testing.T) {
	srv := newAPITestServer(nil)
	for _, path := range []string{"/dashboard", "/dashboard/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s status = %d, want route removed", path, rec.Code)
		}
	}
}

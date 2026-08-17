package recordingmcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

const missingRecordingID = "recording-mcp-missing-001"

func TestBind_QueryStatusSuccessReturnsStatusFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRecordingsRoot{
		invoked: &invoked,
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != testRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, testRecordingID)
			}
			return recordings.RecordingStatusResult{
				Status: recordings.RecordingStatusFacts{
					RecordingID: testRecordingID,
					State:       recordings.RecordingActive,
				},
			}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake recordings root was not invoked")
	}
	var response mcprecording.ToolResponse[recordings.RecordingStatusResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Status.RecordingID != testRecordingID {
		t.Fatalf("recordingId = %q, want %q", response.Result.Status.RecordingID, testRecordingID)
	}
	if response.Result.Status.State != recordings.RecordingActive {
		t.Fatalf("state = %q, want ACTIVE", response.Result.Status.State)
	}
}

func TestBind_QueryStatusMissingTargetReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != missingRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, missingRecordingID)
			}
			return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+missingRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.target.missing",
		false,
		missingRecordingID,
	)
	if envelope.Message != "recording target not found" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "recording target not found", envelope)
	}
}

func TestBind_QueryStatusInvalidScopeReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		queryStatus: func(_ recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{}, recordings.ErrInvalidRecordingScope
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.target.missing",
		false,
		testRecordingID,
	)
	if envelope.Details == nil || !strings.Contains(envelope.Details["reason"].(string), "invalid recording scope") {
		t.Fatalf("error.details = %#v, want invalid recording scope reason", envelope.Details)
	}
}

func TestBind_QueryStatusInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	assertBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake recordings root was invoked for invalid JSON decode")
	}
}

func TestBind_QueryStatusNilServiceReturnsUnavailableEnvelope(t *testing.T) {
	t.Parallel()

	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: nil})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.service.unavailable",
		false,
		"",
	)
	if envelope.Message != "recordings service is unavailable" {
		t.Fatalf("error.message = %q, want unavailable message; envelope = %#v", envelope.Message, envelope)
	}
}

func assertBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
}

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
	wantRecordingID string,
) *mcprecording.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage                `json:"result"`
		Error  *mcprecording.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if wantRecordingID != "" && response.Error.RecordingID != wantRecordingID {
		t.Fatalf("error.recordingId = %q, want %q; envelope = %#v", response.Error.RecordingID, wantRecordingID, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}

const inspectionSessionID = "dur-sess-recordings-mcp-001"

func TestFactorySessionInspection_ListDispatchesUsesHistoricalRecordingsQuery(t *testing.T) {
	t.Parallel()

	var statusCalled, historyCalled bool
	fake := inspectionRootFake{
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			statusCalled = true
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: "artifact-inspection-001", State: recordings.RecordingFinalized,
			}}, nil
		},
		queryHistory: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			historyCalled = true
			if request.Recording.Scope.FactorySessionID != inspectionSessionID {
				t.Fatalf("historical scope = %#v, want %q", request.Recording.Scope, inspectionSessionID)
			}
			return recordings.HistoricalRecordingQueryResult{
				Recording: request.Recording,
				Dispatches: []recordings.HistoricalDispatch{{
					ID: "dispatch-inspection-001", Status: recordings.FactoryDispatchStatusCompleted,
					DispatchKind: recordings.FactoryDispatchKindJavaScriptScript,
				}},
			}, nil
		},
	}

	response, err := mcprecording.ListFactorySessionDispatches(
		context.Background(), fake,
		mcprecording.FactorySessionListDispatchesInput{SessionID: inspectionSessionID, Status: "COMPLETED"},
	)
	if err != nil {
		t.Fatalf("ListFactorySessionDispatches() error = %v", err)
	}
	if !statusCalled || !historyCalled {
		t.Fatalf("Recordings calls = status:%t history:%t, want both", statusCalled, historyCalled)
	}
	if response.SessionId != inspectionSessionID || len(response.Dispatches) != 1 {
		t.Fatalf("response = %#v, want one dispatch for session", response)
	}
	if response.Dispatches[0].Id != "dispatch-inspection-001" ||
		string(response.Dispatches[0].DispatchKind) != "JAVASCRIPT_SCRIPT" {
		t.Fatalf("dispatch = %#v, want mapped historical facts", response.Dispatches[0])
	}
}

func TestFactorySessionInspection_ListArtifactsUsesRecordingsArtifactProjection(t *testing.T) {
	t.Parallel()

	var buildCalled, reconstructCalled bool
	fake := inspectionRootFake{
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: "artifact-inspection-002", State: recordings.RecordingFinalized,
			}}, nil
		},
		buildArtifact: func(request recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
			buildCalled = true
			return recordings.BuildPortableArtifactResult{Artifact: recordings.PortableArtifact{
				SchemaVersion: recordings.PortableArtifactSchemaV1,
				Summary: recordings.PortableArtifactSummary{
					RecordingID: request.RecordingID,
					Reference:   "artifact-inspection-002",
					Scope:       recordings.CanonicalEventScope{FactorySessionID: inspectionSessionID},
					State:       recordings.RecordingFinalized,
					Available:   true,
				},
			}}, nil
		},
		reconstruct: func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			reconstructCalled = true
			payload, err := json.Marshal(factorydefinitions.FactoryWorldState{Artifacts: []factorydefinitions.FactorySessionArtifactState{{
				ID: "artifact-row-001", Kind: "CHECKPOINT", Visibility: "SESSION", CaptureMetadata: map[string]string{
					"sourceDispatchId": "dispatch-inspection-002",
				},
			}}})
			if err != nil {
				return recordings.ReconstructWorldStateResult{}, err
			}
			return recordings.ReconstructWorldStateResult{WorldState: recordings.WorldStateView{
				Scope: request.Scope, SchemaVersion: recordings.WorldStateViewSchemaV1, Payload: string(payload),
			}}, nil
		},
	}

	response, err := mcprecording.ListFactorySessionArtifacts(
		context.Background(), fake,
		mcprecording.FactorySessionListArtifactsInput{SessionID: inspectionSessionID},
	)
	if err != nil {
		t.Fatalf("ListFactorySessionArtifacts() error = %v", err)
	}
	if !buildCalled || !reconstructCalled {
		t.Fatalf("Recordings calls = build:%t reconstruct:%t, want both", buildCalled, reconstructCalled)
	}
	if len(response.Artifacts) != 1 || response.Artifacts[0].Id != "artifact-row-001" ||
		response.Artifacts[0].DispatchId == nil || *response.Artifacts[0].DispatchId != "dispatch-inspection-002" {
		t.Fatalf("response = %#v, want mapped artifact projection", response)
	}
}

func TestFactorySessionInspection_ReadEventsPreservesOrderAndEventIDReconnectSuffix(t *testing.T) {
	t.Parallel()

	events := []recordings.CanonicalEvent{
		inspectionCanonicalEvent("event-inspection-001", 1),
		inspectionCanonicalEvent("event-inspection-002", 2),
	}
	fake := inspectionRootFake{
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			last := events[len(events)-1].Cursor
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, Artifact: "artifact-inspection-003", LastEvent: &last,
			}}, nil
		},
		subscribe: func(_ context.Context, request recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			selected := append([]recordings.CanonicalEvent(nil), events...)
			if request.Cursor != nil {
				selected = selected[1:]
			}
			return inspectionSubscription(selected), nil
		},
	}

	all, err := mcprecording.ReadFactorySessionEvents(
		context.Background(), fake,
		mcprecording.FactorySessionReadEventsInput{SessionID: inspectionSessionID},
	)
	if err != nil {
		t.Fatalf("ReadFactorySessionEvents(all) error = %v", err)
	}
	if len(all.Events) != 2 || all.Events[0].Id != "event-inspection-001" || all.Events[1].Id != "event-inspection-002" {
		t.Fatalf("all events = %#v, want ordered canonical events", all.Events)
	}

	suffix, err := mcprecording.ReadFactorySessionEvents(
		context.Background(), fake,
		mcprecording.FactorySessionReadEventsInput{SessionID: inspectionSessionID, AfterEventID: "event-inspection-001"},
	)
	if err != nil {
		t.Fatalf("ReadFactorySessionEvents(suffix) error = %v", err)
	}
	if len(suffix.Events) != 1 || suffix.Events[0].Id != "event-inspection-002" {
		t.Fatalf("suffix events = %#v, want one event after reconnect id", suffix.Events)
	}
}

func TestFactorySessionInspection_ReadEventsPreservesTypedStaleCursorFailure(t *testing.T) {
	t.Parallel()

	fake := inspectionRootFake{
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			last := inspectionCanonicalEvent("event-inspection-004", 4).Cursor
			return recordings.RecordingStatusResult{Status: recordings.RecordingStatusFacts{
				RecordingID: request.RecordingID, LastEvent: &last,
			}}, nil
		},
		subscribe: func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorExpired
		},
	}
	_, err := mcprecording.ReadFactorySessionEvents(
		context.Background(), fake,
		mcprecording.FactorySessionReadEventsInput{SessionID: inspectionSessionID, AfterSequence: intPointer(4)},
	)
	if !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("stale cursor error = %v, want ErrReconnectCursorExpired", err)
	}
}

type inspectionRootFake struct {
	recordings.Service
	queryStatus   func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	queryHistory  func(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	buildArtifact func(recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error)
	reconstruct   func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error)
	subscribe     func(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error)
}

func (fake inspectionRootFake) QueryRecordingStatus(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	if fake.queryStatus == nil {
		return recordings.RecordingStatusResult{}, errors.New("unexpected QueryRecordingStatus")
	}
	return fake.queryStatus(request)
}

func (fake inspectionRootFake) QueryHistoricalRecording(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
	if fake.queryHistory == nil {
		return recordings.HistoricalRecordingQueryResult{}, errors.New("unexpected QueryHistoricalRecording")
	}
	return fake.queryHistory(request)
}

func (fake inspectionRootFake) BuildPortableArtifact(request recordings.BuildPortableArtifactRequest) (recordings.BuildPortableArtifactResult, error) {
	if fake.buildArtifact == nil {
		return recordings.BuildPortableArtifactResult{}, errors.New("unexpected BuildPortableArtifact")
	}
	return fake.buildArtifact(request)
}

func (fake inspectionRootFake) ReconstructWorldState(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
	if fake.reconstruct == nil {
		return recordings.ReconstructWorldStateResult{}, errors.New("unexpected ReconstructWorldState")
	}
	return fake.reconstruct(request)
}

func (fake inspectionRootFake) SubscribeFrom(ctx context.Context, request recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
	if fake.subscribe == nil {
		return recordings.SubscribeResult{}, errors.New("unexpected SubscribeFrom")
	}
	return fake.subscribe(ctx, request)
}

func inspectionSubscription(events []recordings.CanonicalEvent) recordings.SubscribeResult {
	return recordings.SubscribeResult{
		RetainedEventCount: len(events),
		Subscription: func(ctx context.Context) recordings.SubscriptionOutcome {
			if err := ctx.Err(); err != nil {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			if len(events) == 0 {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			event := events[0]
			events = events[1:]
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event}
		},
	}
}

func inspectionCanonicalEvent(id string, sequence int64) recordings.CanonicalEvent {
	position := recordings.CanonicalEventSequence(sequence)
	return recordings.CanonicalEvent{
		ID: recordings.CanonicalEventID(id), Sequence: position, FactoryTick: int(sequence),
		Scope:      recordings.CanonicalEventScope{FactorySessionID: inspectionSessionID},
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "inspection-generation", Sequence: position},
		RecordedAt: time.Date(2026, time.August, 16, 0, 0, 0, int(sequence), time.UTC),
		Kind:       recordings.CanonicalEventKind("SESSION_PROGRESS"), Payload: `{}`,
	}
}

func intPointer(value int) *int { return &value }

func TestQueryHistoryMapsContextAndTypedFailures(t *testing.T) {
	t.Parallel()

	input := queryHistoryErrorInput()
	assertQueryHistoryContextErrors(t, input)
	assertQueryHistoryTypedErrors(t, input)
	assertQueryHistorySafeErrors(t, input)
}

func queryHistoryErrorInput() mcprecording.QueryHistoryInput {
	return mcprecording.QueryHistoryInput{
		RecordingID: " recording-history-errors-001 ", Artifact: " artifact-1 ", FactorySessionID: " session-1 ",
	}
}

func assertQueryHistoryContextErrors(t *testing.T, input mcprecording.QueryHistoryInput) {
	t.Helper()
	if response := mcprecording.QueryHistory(nil, fakeRecordingsRoot{}, input); response.Error == nil || response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("nil-context response = %#v, want bad request envelope", response)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if response := mcprecording.QueryHistory(canceled, fakeRecordingsRoot{}, input); response.Error == nil || response.Error.Code != "recording.request.canceled" {
		t.Fatalf("canceled response = %#v, want canceled envelope", response)
	}
	if response := mcprecording.QueryHistory(context.Background(), nil, input); response.Error == nil || response.Error.Code != "recording.service.unavailable" {
		t.Fatalf("nil-service response = %#v, want unavailable envelope", response)
	}
}

func assertQueryHistoryTypedErrors(t *testing.T, input mcprecording.QueryHistoryInput) {
	t.Helper()
	tests := []struct {
		kind      recordings.HistoricalRecordingQueryErrorKind
		code      string
		retryable bool
	}{
		{kind: recordings.HistoricalRecordingQueryErrorInvalidRequest, code: "recording.history.invalid"},
		{kind: recordings.HistoricalRecordingQueryErrorMissingHistory, code: "recording.history.not_found"},
		{kind: recordings.HistoricalRecordingQueryErrorCorruptHistory, code: "recording.history.corrupt"},
		{kind: recordings.HistoricalRecordingQueryErrorUnavailable, code: "recording.history.unavailable", retryable: true},
	}
	for _, test := range tests {
		test := test
		fake := fakeRecordingsRoot{queryHistory: func(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
				Kind: test.kind, RecordingID: request.Recording.RecordingID,
			}
		}}
		response := mcprecording.QueryHistory(context.Background(), fake, input)
		if response.Error == nil || response.Error.Code != test.code || response.Error.Retryable != test.retryable || response.Error.RecordingID != "recording-history-errors-001" {
			t.Fatalf("kind %s response = %#v, want code=%s retryable=%v", test.kind, response, test.code, test.retryable)
		}
	}
}

func assertQueryHistorySafeErrors(t *testing.T, input mcprecording.QueryHistoryInput) {
	t.Helper()
	clientError := mcprecording.QueryHistory(context.Background(), fakeRecordingsRoot{
		queryHistory: func(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{}, errors.New("invalid scope")
		},
	}, input)
	if clientError.Error == nil || clientError.Error.Code != "BAD_REQUEST" {
		t.Fatalf("safe client error = %#v, want bad request", clientError)
	}
	internalError := mcprecording.QueryHistory(context.Background(), fakeRecordingsRoot{
		queryHistory: func(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
			return recordings.HistoricalRecordingQueryResult{}, errors.New("pkg/services/recordings/internal failure")
		},
	}, input)
	if internalError.Error == nil || internalError.Error.Code != "recording.execution.internal" {
		t.Fatalf("internal error = %#v, want sanitized internal envelope", internalError)
	}
}

func TestLegacyFactorySessionInspectionAdaptsStandaloneReads(t *testing.T) {
	t.Parallel()

	service, sessionID := newLegacyInspectionService(t)
	assertLegacyStatus(t, service, sessionID)
	assertLegacyHistory(t, service, sessionID)
	assertLegacyPortableArtifact(t, service, sessionID)
	assertLegacyWorldState(t, service, sessionID)
	assertLegacySubscription(t, service, sessionID)
}

func newLegacyInspectionService(t *testing.T) (mcprecording.FactorySessionInspectionService, string) {
	t.Helper()
	sessionID := "standalone-session-001"
	event := json.RawMessage(`{"context":{"eventTime":"2026-08-16T03:00:00Z","sequence":2,"tick":4,"sessionId":"` + sessionID + `"},"id":"event-standalone-001","payload":{"status":"COMPLETED"},"schemaVersion":"agent-factory.event.v1","type":"RUN_RESPONSE"}`)
	legacy := &legacyInspectionFake{
		events:     []json.RawMessage{event},
		dispatches: []factorysessions.DispatchSummary{{ID: "dispatch-standalone-001", Status: factorysessions.DispatchStatus("COMPLETED"), DispatchKind: "JAVASCRIPT_SCRIPT"}},
		artifacts:  []factorysessions.ArtifactSummary{{ID: "artifact-standalone-001", Kind: "LOG", Visibility: "PUBLIC", Label: "log", ContentHash: "hash", SizeBytes: 9, DispatchID: "dispatch-standalone-001"}},
	}
	service := mcprecording.NewLegacyFactorySessionInspection(legacy)
	if service == nil || mcprecording.NewLegacyFactorySessionInspection(nil) != nil || mcprecording.NewLegacyFactorySessionInspection(struct{}{}) != nil {
		t.Fatal("legacy inspection adapter should accept only the legacy inspection contract")
	}
	return service, sessionID
}

func assertLegacyStatus(t *testing.T, service mcprecording.FactorySessionInspectionService, sessionID string) {
	t.Helper()
	status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: recordings.RecordingID(sessionID)})
	if err != nil || string(status.Status.Artifact) != "standalone://"+sessionID || status.Status.LastEvent == nil || status.Status.LastEvent.Sequence != 2 {
		t.Fatalf("legacy status = %#v err=%v, want artifact and last cursor", status, err)
	}
}

func assertLegacyHistory(t *testing.T, service mcprecording.FactorySessionInspectionService, sessionID string) {
	t.Helper()
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{
		RecordingID: recordings.RecordingID(sessionID), Artifact: "artifact-standalone-001", Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID},
	}})
	if err != nil || len(history.Events) != 1 || len(history.Dispatches) != 1 || history.Dispatches[0].ID != "dispatch-standalone-001" {
		t.Fatalf("legacy history = %#v err=%v, want event and dispatch", history, err)
	}
}

func assertLegacyPortableArtifact(t *testing.T, service mcprecording.FactorySessionInspectionService, sessionID string) {
	t.Helper()
	portable, err := service.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{RecordingID: recordings.RecordingID(sessionID)})
	if err != nil || portable.Artifact.Summary.EventCount != 1 || portable.Artifact.Summary.FirstCursor == nil || portable.Artifact.Summary.LastCursor == nil {
		t.Fatalf("legacy portable artifact = %#v err=%v, want cursor bounds", portable, err)
	}
}

func assertLegacyWorldState(t *testing.T, service mcprecording.FactorySessionInspectionService, sessionID string) {
	t.Helper()
	world, err := service.ReconstructWorldState(recordings.ReconstructWorldStateRequest{Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID}})
	if err != nil || !strings.Contains(world.WorldState.Payload, "artifact-standalone-001") {
		t.Fatalf("legacy world state = %#v err=%v, want artifact projection", world, err)
	}
}

func assertLegacySubscription(t *testing.T, service mcprecording.FactorySessionInspectionService, sessionID string) {
	t.Helper()

	cursor := recordings.CanonicalEventCursor{Sequence: 1}
	subscribed, err := service.SubscribeFrom(context.Background(), recordings.SubscribeRequest{Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID}, Cursor: &cursor})
	if err != nil || subscribed.RetainedEventCount != 1 || subscribed.Subscription == nil {
		t.Fatalf("legacy subscription = %#v err=%v, want retained event subscription", subscribed, err)
	}
	if outcome := subscribed.Subscription(context.Background()); outcome.Kind != recordings.SubscriptionEvent || outcome.Event.ID != "event-standalone-001" {
		t.Fatalf("legacy subscription outcome = %#v, want standalone event", outcome)
	}
	if outcome := subscribed.Subscription(context.Background()); outcome.Kind != recordings.SubscriptionClosed {
		t.Fatalf("legacy exhausted subscription outcome = %#v, want closed", outcome)
	}
}

type legacyInspectionFake struct {
	events     []json.RawMessage
	dispatches []factorysessions.DispatchSummary
	artifacts  []factorysessions.ArtifactSummary
}

func (fake *legacyInspectionFake) QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{Dispatches: fake.dispatches}, nil
}

func (fake *legacyInspectionFake) ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error) {
	return factorysessions.ListArtifactsResult{Artifacts: fake.artifacts}, nil
}

func (fake *legacyInspectionFake) ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	return factorysessions.EventReadResult{Events: fake.events}, nil
}

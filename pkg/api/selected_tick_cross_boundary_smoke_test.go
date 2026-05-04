package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/dashboard"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestSelectedTickCrossBoundarySmoke_ReconstructsCanonicalStateAcrossSupportedBoundaries(t *testing.T) {
	t0 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	worldState, err := projections.ReconstructFactoryWorldState(crossBoundarySelectedTickEvents(t0), 11)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	assertSelectedTickCanonicalState(t, worldState)
	assertSelectedTickBoundaryAllowlist(t)

	worldView := projections.BuildFactoryWorldView(worldState)
	assertSelectedTickWorldView(t, worldView)

	requestSlice := BuildFactoryWorldWorkstationRequestProjectionSlice(worldState)
	assertSelectedTickWorkstationRequests(t, requestSlice)

	output := dashboard.FormatSimpleDashboardWithRenderData(
		interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			FactoryState:  "RUNNING",
			RuntimeStatus: interfaces.RuntimeStatusActive,
			TickCount:     11,
			Uptime:        11 * time.Second,
		},
		dashboardrender.SimpleDashboardRenderDataFromWorldState(worldState),
		t0.Add(12*time.Second),
	)
	for _, want := range []string{
		"Active Workstations (1)",
		"Pending Runtime Story",
		"Completed Runtime Story",
		"Failed Runtime Story",
		"provider_rate_limit - Provider rate limit exceeded while reviewing the failed runtime story.",
		"Provider sessions:",
		"sess-runtime-completed",
		"sess-runtime-failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dashboard output missing %q:\n%s", want, output)
		}
	}
}

func assertSelectedTickCanonicalState(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()

	if _, ok := worldState.ActiveDispatches["dispatch-runtime-pending"]; !ok {
		t.Fatalf("active dispatches = %#v, want dispatch-runtime-pending", worldState.ActiveDispatches)
	}
	if len(worldState.CompletedDispatches) != 2 {
		t.Fatalf("completed dispatches = %#v, want two completed dispatches", worldState.CompletedDispatches)
	}
	if len(worldState.ProviderSessions) != 2 {
		t.Fatalf("provider sessions = %#v, want two canonical provider sessions", worldState.ProviderSessions)
	}
	completedAttempts := worldState.InferenceAttemptsByDispatchID["dispatch-runtime-completed"]
	if len(completedAttempts) != 1 {
		t.Fatalf("completed inference attempts = %#v, want one attempt", completedAttempts)
	}
	failedAttempts := worldState.InferenceAttemptsByDispatchID["dispatch-runtime-failed"]
	if len(failedAttempts) != 1 {
		t.Fatalf("failed inference attempts = %#v, want one attempt", failedAttempts)
	}
	if got := failedAttempts["dispatch-runtime-failed/inference-request/1"].ErrorClass; got != "rate_limited" {
		t.Fatalf("failed inference error_class = %q, want rate_limited", got)
	}
	if got := worldState.FailureDetailsByWorkID["work-runtime-failed"].FailureReason; got != "provider_rate_limit" {
		t.Fatalf("failed work detail reason = %q, want provider_rate_limit", got)
	}
}

func assertSelectedTickWorldView(t *testing.T, worldView interfaces.FactoryWorldView) {
	t.Helper()

	if !reflect.DeepEqual(worldView.Topology.SubmitWorkTypes, []interfaces.FactoryWorldSubmitWorkType{{WorkTypeName: "task"}}) {
		t.Fatalf("submit work types = %#v, want [task]", worldView.Topology.SubmitWorkTypes)
	}
	if worldView.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("in-flight dispatch count = %d, want 1", worldView.Runtime.InFlightDispatchCount)
	}
	if worldView.Runtime.Session.CompletedCount != 1 || worldView.Runtime.Session.FailedCount != 1 {
		t.Fatalf("session counts = %#v, want completed=1 failed=1", worldView.Runtime.Session)
	}
	if len(worldView.Runtime.Session.DispatchHistory) != 2 {
		t.Fatalf("dispatch history = %#v, want two completed rows", worldView.Runtime.Session.DispatchHistory)
	}
	if len(worldView.Runtime.Session.ProviderSessions) != 2 {
		t.Fatalf("provider sessions = %#v, want two provider-session rows", worldView.Runtime.Session.ProviderSessions)
	}
}

func assertSelectedTickWorkstationRequests(
	t *testing.T,
	slice generated.FactoryWorldWorkstationRequestProjectionSlice,
) {
	t.Helper()

	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request slice missing projection map")
	}
	requests := *slice.WorkstationRequestsByDispatchId
	if len(requests) != 3 {
		t.Fatalf("workstation request count = %d, want 3", len(requests))
	}

	active := requests["dispatch-runtime-pending"]
	if active.Response != nil || active.Request.RequestTime != nil {
		t.Fatalf("active request = %#v, want no response or inference timestamp yet", active)
	}

	completed := requests["dispatch-runtime-completed"]
	if completed.Request.RequestMetadata == nil || (*completed.Request.RequestMetadata)["prompt_source"] != "factory-renderer" {
		t.Fatalf("completed request metadata = %#v, want prompt_source=factory-renderer", completed.Request.RequestMetadata)
	}
	if completed.Response == nil || completed.Response.ResponseText == nil || *completed.Response.ResponseText != "The completed runtime story is ready for review." {
		t.Fatalf("completed response = %#v, want projected response text", completed.Response)
	}
	if completed.Response.ProviderSession == nil || completed.Response.ProviderSession.Id == nil || *completed.Response.ProviderSession.Id != "sess-runtime-completed" {
		t.Fatalf("completed provider session = %#v, want sess-runtime-completed", completed.Response.ProviderSession)
	}

	failed := requests["dispatch-runtime-failed"]
	if failed.Counts.DispatchedCount != 1 || failed.Counts.ErroredCount != 1 || failed.Counts.RespondedCount != 0 {
		t.Fatalf("failed request counts = %#v, want dispatched=1 errored=1 responded=0", failed.Counts)
	}
	if failed.Response == nil || failed.Response.FailureReason == nil || *failed.Response.FailureReason != "provider_rate_limit" {
		t.Fatalf("failed response = %#v, want provider_rate_limit", failed.Response)
	}
	if failed.Response.Diagnostics == nil || failed.Response.Diagnostics.Provider == nil {
		t.Fatalf("failed response diagnostics = %#v, want provider diagnostics", failed.Response.Diagnostics)
	}
}

func assertSelectedTickBoundaryAllowlist(t *testing.T) {
	t.Helper()

	paths, err := filepath.Glob("../interfaces/*.go")
	if err != nil {
		t.Fatalf("glob interfaces package: %v", err)
	}

	fset := token.NewFileSet()
	liveViews := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		liveViews = append(liveViews, selectedTickBoundaryViews(file)...)
	}
	sort.Strings(liveViews)

	want := []string{
		"FactoryWorldRuntimeView",
		"FactoryWorldTopologyView",
		"FactoryWorldView",
	}
	if !reflect.DeepEqual(liveViews, want) {
		t.Fatalf("live FactoryWorld*View allowlist = %#v, want %#v", liveViews, want)
	}
}

func selectedTickBoundaryViews(file *ast.File) []string {
	views := []string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			name := typeSpec.Name.Name
			if strings.HasPrefix(name, "FactoryWorld") && strings.HasSuffix(name, "View") {
				views = append(views, name)
			}
		}
	}
	return views
}

func crossBoundarySelectedTickEvents(t0 time.Time) []generated.FactoryEvent {
	pending := interfaces.FactoryWorkItem{ID: "work-runtime-pending", WorkTypeID: "task", DisplayName: "Pending Runtime Story", TraceID: "trace-runtime-pending", PlaceID: "task:init"}
	completed := interfaces.FactoryWorkItem{ID: "work-runtime-completed", WorkTypeID: "task", DisplayName: "Completed Runtime Story", TraceID: "trace-runtime-completed", PlaceID: "task:init"}
	failed := interfaces.FactoryWorkItem{ID: "work-runtime-failed", WorkTypeID: "task", DisplayName: "Failed Runtime Story", TraceID: "trace-runtime-failed", PlaceID: "task:init"}

	return []generated.FactoryEvent{
		crossBoundaryInitialStructureEvent(t0),
		crossBoundaryWorkRequestEvent(1, t0.Add(time.Second), pending),
		crossBoundaryWorkRequestEvent(2, t0.Add(2*time.Second), completed),
		crossBoundaryWorkRequestEvent(3, t0.Add(3*time.Second), failed),
		crossBoundaryDispatchCreatedEvent(4, t0.Add(4*time.Second), "dispatch-runtime-pending", pending),
		crossBoundaryDispatchCreatedEvent(5, t0.Add(5*time.Second), "dispatch-runtime-completed", completed),
		crossBoundaryInferenceRequestEvent(6, t0.Add(6*time.Second), "dispatch-runtime-completed", "Review the completed runtime story.", "/work/completed-runtime", "/work/completed-runtime/.worktrees/runtime"),
		crossBoundaryInferenceResponseEvent(
			7,
			t0.Add(7*time.Second),
			"dispatch-runtime-completed",
			generated.InferenceOutcomeSucceeded,
			875,
			"The completed runtime story is ready for review.",
			"",
			&generated.ProviderSessionMetadata{Id: stringPtr("sess-runtime-completed"), Kind: stringPtr("session_id"), Provider: stringPtr("codex")},
			&generated.SafeWorkDiagnostics{
				Provider: &generated.ProviderDiagnostic{
					Model:    stringPtr("gpt-5.4"),
					Provider: stringPtr("codex"),
					RequestMetadata: crossBoundaryStringMapPtr(map[string]string{
						"prompt_source": "factory-renderer",
						"source":        "cross-boundary-smoke",
					}),
					ResponseMetadata: crossBoundaryStringMapPtr(map[string]string{
						"provider_session_id": "sess-runtime-completed",
						"retry_count":         "0",
					}),
				},
				RenderedPrompt: &generated.RenderedPromptDiagnostic{
					SystemPromptHash: stringPtr("sha256:runtime-system"),
					UserMessageHash:  stringPtr("sha256:runtime-user"),
				},
			},
		),
		crossBoundaryAcceptedResponseEvent(8, t0.Add(8*time.Second), completed),
		crossBoundaryDispatchCreatedEvent(9, t0.Add(9*time.Second), "dispatch-runtime-failed", failed),
		crossBoundaryInferenceRequestEvent(10, t0.Add(10*time.Second), "dispatch-runtime-failed", "Retry the failed runtime story.", "/work/failed-runtime", "/work/failed-runtime/.worktrees/runtime"),
		crossBoundaryInferenceResponseEvent(
			10,
			t0.Add(10*time.Second),
			"dispatch-runtime-failed",
			generated.InferenceOutcomeFailed,
			600,
			"",
			"rate_limited",
			&generated.ProviderSessionMetadata{Id: stringPtr("sess-runtime-failed"), Kind: stringPtr("session_id"), Provider: stringPtr("anthropic")},
			&generated.SafeWorkDiagnostics{
				Provider: &generated.ProviderDiagnostic{
					Model:    stringPtr("claude-3.7"),
					Provider: stringPtr("anthropic"),
					RequestMetadata: crossBoundaryStringMapPtr(map[string]string{
						"prompt_source": "retry-renderer",
						"source":        "cross-boundary-smoke",
					}),
					ResponseMetadata: crossBoundaryStringMapPtr(map[string]string{
						"provider_session_id": "sess-runtime-failed",
						"retry_count":         "1",
					}),
				},
			},
		),
		crossBoundaryFailedResponseEvent(11, t0.Add(11*time.Second), failed),
	}
}

func crossBoundaryInitialStructureEvent(eventTime time.Time) generated.FactoryEvent {
	workstations := []generated.Workstation{crossBoundaryGeneratedWorkstation()}
	workTypes := []generated.WorkType{{
		Name: "task",
		States: []generated.WorkState{
			{Name: "init", Type: generated.WorkStateTypeINITIAL},
			{Name: "review", Type: generated.WorkStateTypePROCESSING},
			{Name: "complete", Type: generated.WorkStateTypeTERMINAL},
			{Name: "failed", Type: generated.WorkStateTypeFAILED},
		},
	}}
	return crossBoundaryEvent(
		generated.FactoryEventTypeInitialStructureRequest,
		"initial-structure",
		0,
		eventTime,
		generated.FactoryEventContext{},
		generated.InitialStructureRequestEventPayload{
			Factory: generated.Factory{
				WorkTypes:    &workTypes,
				Workstations: &workstations,
			},
		},
	)
}

func crossBoundaryWorkRequestEvent(
	tick int,
	eventTime time.Time,
	workItem interfaces.FactoryWorkItem,
) generated.FactoryEvent {
	requestID := "request/" + workItem.ID
	works := []generated.Work{crossBoundaryGeneratedWork(workItem, requestID)}
	return crossBoundaryEvent(
		generated.FactoryEventTypeWorkRequest,
		"work-request/"+workItem.ID,
		tick,
		eventTime,
		generated.FactoryEventContext{
			RequestId: stringPtr(requestID),
			TraceIds:  crossBoundaryStringSlicePtr([]string{workItem.TraceID}),
			WorkIds:   crossBoundaryStringSlicePtr([]string{workItem.ID}),
		},
		generated.WorkRequestEventPayload{
			Type:  generated.WorkRequestTypeFactoryRequestBatch,
			Works: &works,
		},
	)
}

func crossBoundaryDispatchCreatedEvent(
	tick int,
	eventTime time.Time,
	dispatchID string,
	workItem interfaces.FactoryWorkItem,
) generated.FactoryEvent {
	return crossBoundaryEvent(
		generated.FactoryEventTypeDispatchRequest,
		"dispatch-created/"+dispatchID,
		tick,
		eventTime,
		generated.FactoryEventContext{
			DispatchId: stringPtr(dispatchID),
			TraceIds:   crossBoundaryStringSlicePtr([]string{workItem.TraceID}),
			WorkIds:    crossBoundaryStringSlicePtr([]string{workItem.ID}),
		},
		generated.DispatchRequestEventPayload{
			Inputs:       []generated.DispatchConsumedWorkRef{{WorkId: workItem.ID}},
			TransitionId: "t-review",
		},
	)
}

func crossBoundaryInferenceRequestEvent(
	tick int,
	eventTime time.Time,
	dispatchID string,
	prompt string,
	workingDirectory string,
	worktree string,
) generated.FactoryEvent {
	return crossBoundaryEvent(
		generated.FactoryEventTypeInferenceRequest,
		"inference-request/"+dispatchID,
		tick,
		eventTime,
		generated.FactoryEventContext{DispatchId: stringPtr(dispatchID)},
		generated.InferenceRequestEventPayload{
			Attempt:            1,
			InferenceRequestId: dispatchID + "/inference-request/1",
			Prompt:             prompt,
			WorkingDirectory:   workingDirectory,
			Worktree:           worktree,
		},
	)
}

func crossBoundaryInferenceResponseEvent(
	tick int,
	eventTime time.Time,
	dispatchID string,
	outcome generated.InferenceOutcome,
	durationMillis int64,
	response string,
	errorClass string,
	providerSession *generated.ProviderSessionMetadata,
	diagnostics *generated.SafeWorkDiagnostics,
) generated.FactoryEvent {
	return crossBoundaryEvent(
		generated.FactoryEventTypeInferenceResponse,
		"inference-response/"+dispatchID,
		tick,
		eventTime,
		generated.FactoryEventContext{DispatchId: stringPtr(dispatchID)},
		generated.InferenceResponseEventPayload{
			Attempt:            1,
			Diagnostics:        diagnostics,
			DurationMillis:     durationMillis,
			ErrorClass:         stringPtr(errorClass),
			InferenceRequestId: dispatchID + "/inference-request/1",
			Outcome:            outcome,
			ProviderSession:    providerSession,
			Response:           stringPtr(response),
		},
	)
}

func crossBoundaryAcceptedResponseEvent(
	tick int,
	eventTime time.Time,
	workItem interfaces.FactoryWorkItem,
) generated.FactoryEvent {
	outputWork := []generated.Work{crossBoundaryGeneratedWork(workItem, "")}
	return crossBoundaryEvent(
		generated.FactoryEventTypeDispatchResponse,
		"dispatch-completed/dispatch-runtime-completed",
		tick,
		eventTime,
		generated.FactoryEventContext{
			DispatchId: stringPtr("dispatch-runtime-completed"),
			TraceIds:   crossBoundaryStringSlicePtr([]string{workItem.TraceID}),
			WorkIds:    crossBoundaryStringSlicePtr([]string{workItem.ID}),
		},
		generated.DispatchResponseEventPayload{
			DurationMillis: crossBoundaryInt64Ptr(875),
			Outcome:        generated.WorkOutcomeAccepted,
			OutputWork:     &outputWork,
			TransitionId:   "t-review",
		},
	)
}

func crossBoundaryFailedResponseEvent(
	tick int,
	eventTime time.Time,
	workItem interfaces.FactoryWorkItem,
) generated.FactoryEvent {
	outputWork := []generated.Work{crossBoundaryGeneratedWork(workItem, "")}
	return crossBoundaryEvent(
		generated.FactoryEventTypeDispatchResponse,
		"dispatch-completed/dispatch-runtime-failed",
		tick,
		eventTime,
		generated.FactoryEventContext{
			DispatchId: stringPtr("dispatch-runtime-failed"),
			TraceIds:   crossBoundaryStringSlicePtr([]string{workItem.TraceID}),
			WorkIds:    crossBoundaryStringSlicePtr([]string{workItem.ID}),
		},
		generated.DispatchResponseEventPayload{
			DurationMillis: crossBoundaryInt64Ptr(600),
			FailureMessage: stringPtr("Provider rate limit exceeded while reviewing the failed runtime story."),
			FailureReason:  stringPtr("provider_rate_limit"),
			Outcome:        generated.WorkOutcomeFailed,
			OutputWork:     &outputWork,
			TransitionId:   "t-review",
		},
	)
}

func crossBoundaryGeneratedWork(
	workItem interfaces.FactoryWorkItem,
	requestID string,
) generated.Work {
	return generated.Work{
		Name:         workItem.DisplayName,
		RequestId:    stringPtr(requestID),
		TraceId:      stringPtr(workItem.TraceID),
		WorkId:       stringPtr(workItem.ID),
		WorkTypeName: stringPtr(workItem.WorkTypeID),
	}
}

func crossBoundaryGeneratedWorkstation() generated.Workstation {
	return generated.Workstation{
		Id:        stringPtr("t-review"),
		Name:      "Review",
		Worker:    "reviewer",
		Inputs:    []generated.WorkstationIO{{WorkType: "task", State: "init"}},
		Outputs:   []generated.WorkstationIO{{WorkType: "task", State: "complete"}},
		OnFailure: &[]generated.WorkstationIO{{WorkType: "task", State: "failed"}},
	}
}

func crossBoundaryEvent(
	eventType generated.FactoryEventType,
	id string,
	tick int,
	eventTime time.Time,
	context generated.FactoryEventContext,
	payload any,
) generated.FactoryEvent {
	context.Tick = tick
	context.EventTime = eventTime
	event := generated.FactoryEvent{
		Context:       context,
		Id:            id,
		SchemaVersion: generated.AgentFactoryEventV1,
		Type:          eventType,
	}
	switch typed := payload.(type) {
	case generated.InitialStructureRequestEventPayload:
		if err := event.Payload.FromInitialStructureRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case generated.WorkRequestEventPayload:
		if err := event.Payload.FromWorkRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case generated.DispatchRequestEventPayload:
		if err := event.Payload.FromDispatchRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case generated.InferenceRequestEventPayload:
		if err := event.Payload.FromInferenceRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case generated.InferenceResponseEventPayload:
		if err := event.Payload.FromInferenceResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case generated.DispatchResponseEventPayload:
		if err := event.Payload.FromDispatchResponseEventPayload(typed); err != nil {
			panic(err)
		}
	default:
		panic("unsupported selected-tick smoke payload")
	}
	return event
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func crossBoundaryInt64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func crossBoundaryStringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func crossBoundaryStringMapPtr(values map[string]string) *generated.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := generated.StringMap(values)
	return &converted
}

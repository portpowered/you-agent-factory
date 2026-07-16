package events

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

func TestFactoryEventHistory_RecordInitialStructure_UsesRuntimeConfigProjection(t *testing.T) {
	runtimeConfig := eventHistoryRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "codex-cli",
				ModelProvider:    "openai",
				Model:            "gpt-5.4",
			},
		},
	}
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig,
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("Workers = %#v, want one runtime worker", payload.Factory.Workers)
	}
	worker := (*payload.Factory.Workers)[0]
	if worker.Name != "builder" || stringValueForEventHistoryTest(worker.ExecutorProvider) != "SCRIPT_WRAP" ||
		stringValueForEventHistoryTest(worker.ModelProvider) != "CODEX" ||
		stringValueForEventHistoryTest(worker.Type) != string(factoryapi.WorkerTypeInferenceWorker) ||
		stringValueForEventHistoryTest(worker.Model) != "gpt-5.4" {
		t.Fatalf("worker metadata = %#v, want runtime-config provider/model metadata", worker)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_IncludesEditableFactoryDocumentMetadata(t *testing.T) {
	versionTime := time.Date(2026, 6, 9, 12, 30, 0, 0, time.UTC)
	runtimeConfig := eventHistoryRuntimeConfig{
		Factory: &interfaces.FactoryConfig{
			Name: "editable-factory",
			Version: &interfaces.FactoryVersion{
				Logical:  42,
				Physical: versionTime,
			},
			ResourceManifest: &interfaces.PortableResourceManifestConfig{
				BundledFiles: []interfaces.BundledFileConfig{
					{
						Type:       interfaces.BundledFileTypeScript,
						TargetPath: "factory/scripts/run.sh",
						Content: interfaces.BundledFileContentConfig{
							Encoding: interfaces.BundledFileEncodingUTF8,
							Inline:   "echo run\n",
						},
					},
				},
			},
			Layout: &interfaces.FactoryLayoutConfig{
				SchemaVersion: interfaces.SupportedFactoryLayoutSchemaVersion,
				Edges: []interfaces.FactoryLayoutEdgeConfig{{
					ID: "edge:review->done",
					Waypoints: []interfaces.FactoryLayoutPointConfig{
						{X: 10, Y: 20},
					},
				}},
				Viewport: &interfaces.FactoryLayoutViewportConfig{
					X:    100,
					Y:    -40,
					Zoom: 1.25,
				},
			},
		},
	}
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig,
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	assertInitialStructureFactoryVersion(t, payload, versionTime)
	assertInitialStructureBundledFile(t, payload)
	assertInitialStructureFactoryLayout(t, payload)
}

func assertInitialStructureFactoryVersion(t *testing.T, payload factoryapi.InitialStructureRequestEventPayload, versionTime time.Time) {
	t.Helper()
	if payload.Factory.Version == nil || payload.Factory.Version.Logical.Int64() != 42 || !payload.Factory.Version.Physical.Equal(versionTime) {
		t.Fatalf("factory version = %#v, want logical=42 physical=%s", payload.Factory.Version, versionTime)
	}
}

func assertInitialStructureBundledFile(t *testing.T, payload factoryapi.InitialStructureRequestEventPayload) {
	t.Helper()
	if payload.Factory.SupportingFiles == nil || payload.Factory.SupportingFiles.BundledFiles == nil || len(*payload.Factory.SupportingFiles.BundledFiles) != 1 {
		t.Fatalf("supporting files = %#v, want one bundled file", payload.Factory.SupportingFiles)
	}
	bundledFile := (*payload.Factory.SupportingFiles.BundledFiles)[0]
	if bundledFile.TargetPath != "factory/scripts/run.sh" || bundledFile.Content.Inline != "echo run\n" {
		t.Fatalf("bundled file = %#v, want script content", bundledFile)
	}
}

func assertInitialStructureFactoryLayout(t *testing.T, payload factoryapi.InitialStructureRequestEventPayload) {
	t.Helper()
	if payload.Factory.Layout == nil || payload.Factory.Layout.Viewport == nil {
		t.Fatalf("layout = %#v, want viewport", payload.Factory.Layout)
	}
	if payload.Factory.Layout.Viewport.X != 100 || payload.Factory.Layout.Viewport.Y != -40 || payload.Factory.Layout.Viewport.Zoom != 1.25 {
		t.Fatalf("viewport = %#v, want persisted viewport", payload.Factory.Layout.Viewport)
	}
	if payload.Factory.Layout.Edges == nil || len(*payload.Factory.Layout.Edges) != 1 || (*payload.Factory.Layout.Edges)[0].Waypoints == nil {
		t.Fatalf("layout edges = %#v, want waypoint edge", payload.Factory.Layout.Edges)
	}
	if waypoint := (*(*payload.Factory.Layout.Edges)[0].Waypoints)[0]; waypoint.X != 10 || waypoint.Y != 20 {
		t.Fatalf("waypoint = %#v, want persisted waypoint", waypoint)
	}
}

func TestFactoryEventHistory_SubscribeCancelClosesStreamWithoutPanickingAppenders(t *testing.T) {
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	history.RecordInitialStructure()

	select {
	case <-stream.Events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial live event")
	}

	cancel()

	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-stream.Events:
			if !ok {
				goto closed
			}
		case <-deadline:
			t.Fatal("timed out waiting for stream closure after cancellation")
		}
	}

closed:
	for i := 0; i < 32; i++ {
		history.RecordFactoryStateChange(i, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning, "post-cancel", time.Unix(int64(i+1), 0).UTC())
	}
}

func TestFactoryEventHistory_RecordInitialStructure_EmitsCanonicalPublicWorkstationKinds(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"Build": {Name: "Build", Kind: interfaces.WorkstationKindRepeater},
			},
		},
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}
	workstation := (*payload.Factory.Workstations)[0]
	if workstation.Behavior == nil || *workstation.Behavior != factoryapi.WorkstationKindRepeater {
		t.Fatalf("workstation behavior = %#v, want REPEATER", workstation.Behavior)
	}

	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal initial structure event: %v", err)
	}
	if !strings.Contains(string(data), `"behavior":"REPEATER"`) {
		t.Fatalf("initial structure event JSON = %s, want canonical uppercase workstation behavior", data)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_PreservesNonSuccessRouteArrays(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNetWithOrderedNonSuccessRoutes(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}

	workstation := (*payload.Factory.Workstations)[0]
	if workstation.OnContinue == nil || !reflect.DeepEqual(*workstation.OnContinue, []factoryapi.WorkstationIO{{WorkType: "story", State: "retry"}, {WorkType: "story", State: "init"}}) {
		t.Fatalf("workstation onContinue = %#v, want authored route array", workstation.OnContinue)
	}
	if workstation.OnRejection == nil || !reflect.DeepEqual(*workstation.OnRejection, []factoryapi.WorkstationIO{{WorkType: "story", State: "triage"}, {WorkType: "story", State: "init"}}) {
		t.Fatalf("workstation onRejection = %#v, want authored route array", workstation.OnRejection)
	}
	if workstation.OnFailure == nil || !reflect.DeepEqual(*workstation.OnFailure, []factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}, {WorkType: "story", State: "abandoned"}}) {
		t.Fatalf("workstation onFailure = %#v, want authored route array", workstation.OnFailure)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_ProjectsImplicitCronFailureRoutes(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "cron-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "poll-for-work",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "cron-worker",
			Cron:           &interfaces.CronConfig{Schedule: "* * * * *", TriggerAtStart: true},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}
	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	history := NewFactoryEventHistory(net, func() time.Time { return time.Unix(0, 0).UTC() })
	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workstations == nil {
		t.Fatalf("workstations = %#v, want generated workstations", payload.Factory.Workstations)
	}

	want := []factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}}
	for _, workstation := range *payload.Factory.Workstations {
		if workstation.Name != "poll-for-work" {
			continue
		}
		if workstation.OnFailure == nil || !reflect.DeepEqual(*workstation.OnFailure, want) {
			t.Fatalf("workstation onFailure = %#v, want implicit failed-state route", workstation.OnFailure)
		}
		return
	}
	t.Fatalf("workstations = %#v, want generated cron workstation", payload.Factory.Workstations)
}

func TestFactoryEventHistory_RecordInitialStructure_PreservesGeneratedPublicEnumPointerValues(t *testing.T) {
	runtimeConfig := eventHistoryRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"builder": {
				Type:             "  MODEL_WORKER  ",
				ExecutorProvider: "  local-claude  ",
				ModelProvider:    "  openai  ",
				Model:            "gpt-5.4",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"Build": {
				Name:           "Build",
				Kind:           interfaces.WorkstationKind("  REPEATER  "),
				Type:           "  LOGICAL_MOVE  ",
				WorkerTypeName: "builder",
			},
		},
	}
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig,
	)

	history.RecordInitialStructure()

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("workers = %#v, want one generated worker", payload.Factory.Workers)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one generated workstation", payload.Factory.Workstations)
	}

	worker := (*payload.Factory.Workers)[0]
	if got, want := stringValueForEventHistoryTest(worker.ExecutorProvider), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkerProviderPtr(runtimeConfig.Workers["builder"].ExecutorProvider)); got != want {
		t.Fatalf("worker executor provider = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(worker.ModelProvider), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkerModelProviderPtr(runtimeConfig.Workers["builder"].ModelProvider)); got != want {
		t.Fatalf("worker model provider = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(worker.Type), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkerTypePtr(runtimeConfig.Workers["builder"].Type)); got != want {
		t.Fatalf("worker type = %q, want %q", got, want)
	}

	workstation := (*payload.Factory.Workstations)[0]
	if got, want := stringValueForEventHistoryTest(workstation.Type), stringValueForEventHistoryTest(interfaces.GeneratedPublicFactoryWorkstationTypePtr(runtimeConfig.Workstations["Build"].Type)); got != want {
		t.Fatalf("workstation type = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(workstation.Behavior), stringValueForEventHistoryTest(interfaces.GeneratedPublicWorkstationKindPtr(runtimeConfig.Workstations["Build"].Kind)); got != want {
		t.Fatalf("workstation behavior = %q, want %q", got, want)
	}
}

func TestFactoryEventHistory_RecordDispatchCompletion_PreservesSelectedClassificationLabel(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)

	result := interfaces.WorkResult{
		DispatchID:                  "dispatch-1",
		TransitionID:                "t-review",
		Outcome:                     interfaces.OutcomeAccepted,
		SelectedClassificationLabel: "approved",
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "t-review",
		Outcome:         interfaces.OutcomeAccepted,
		ConsumedTokens:  []interfaces.Token{{ID: "token-1", Color: interfaces.TokenColor{WorkID: "work-1", TraceID: "trace-1"}}},
		OutputMutations: nil,
	}

	history.RecordWorkstationResponse(3, result, completed)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if got := stringValueForEventHistoryTest(payload.SelectedClassificationLabel); got != "approved" {
		t.Fatalf("selected classification label = %q, want approved", got)
	}
}

func TestFactoryEventHistory_RecordDispatchCompletion_PreservesOutputWorkStateFromTokenPlace(t *testing.T) {
	history := NewFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)

	result := interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Outcome:      interfaces.OutcomeAccepted,
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Outcome:      interfaces.OutcomeAccepted,
		ConsumedTokens: []interfaces.Token{{
			ID:    "token-1",
			Color: interfaces.TokenColor{WorkID: "work-1", WorkTypeID: "task", TraceID: "trace-1"},
		}},
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type: interfaces.MutationMove,
			Token: &interfaces.Token{
				ID:      "token-terminal",
				PlaceID: "task:complete",
				Color: interfaces.TokenColor{
					WorkID:     "work-1",
					WorkTypeID: "task",
					Name:       "Write docs",
					TraceID:    "trace-1",
				},
			},
		}},
	}

	history.RecordWorkstationResponse(3, result, completed)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one generated output work item", payload.OutputWork)
	}
	state := (*payload.OutputWork)[0].State
	if state == nil || state.Name != "complete" {
		t.Fatalf("output work state = %#v, want complete derived from task:complete", state)
	}
}

type eventHistoryRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

type eventHistoryDefinitionOnlyRuntimeConfig struct {
	Workers      map[string]*interfaces.WorkerConfig
	Workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (c eventHistoryDefinitionOnlyRuntimeConfig) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := c.Workers[name]
	return worker, ok
}

func (c eventHistoryDefinitionOnlyRuntimeConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := c.Workstations[name]
	return workstation, ok
}

func eventHistoryProjectionNet() *state.Net {
	return &state.Net{
		ID: "event-history-projection-net",
		Places: map[string]*petri.Place{
			"story:init":   {ID: "story:init", TypeID: "story", State: "init"},
			"story:review": {ID: "story:review", TypeID: "story", State: "review"},
			"story:done":   {ID: "story:done", TypeID: "story", State: "done"},
			"story:failed": {ID: "story:failed", TypeID: "story", State: "failed"},
		},
		Transitions: map[string]*petri.Transition{
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs:  []petri.Arc{{Name: "work", PlaceID: "story:init"}},
				OutputArcs: []petri.Arc{{PlaceID: "story:review"}},
				FailureArcs: []petri.Arc{
					{PlaceID: "story:failed"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": {
				ID:   "story",
				Name: "Story",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "done", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
	}
}

func eventHistoryProjectionNetWithOrderedNonSuccessRoutes() *state.Net {
	return &state.Net{
		ID: "event-history-projection-net-non-success-routes",
		Places: map[string]*petri.Place{
			"story:init":      {ID: "story:init", TypeID: "story", State: "init"},
			"story:review":    {ID: "story:review", TypeID: "story", State: "review"},
			"story:retry":     {ID: "story:retry", TypeID: "story", State: "retry"},
			"story:triage":    {ID: "story:triage", TypeID: "story", State: "triage"},
			"story:failed":    {ID: "story:failed", TypeID: "story", State: "failed"},
			"story:abandoned": {ID: "story:abandoned", TypeID: "story", State: "abandoned"},
		},
		Transitions: map[string]*petri.Transition{
			"build": {
				ID:         "build",
				Name:       "Build",
				WorkerType: "builder",
				InputArcs:  []petri.Arc{{Name: "work", PlaceID: "story:init"}},
				OutputArcs: []petri.Arc{{PlaceID: "story:review"}},
				ContinueArcs: []petri.Arc{
					{PlaceID: "story:retry"},
					{PlaceID: "story:init"},
				},
				RejectionArcs: []petri.Arc{
					{PlaceID: "story:triage"},
					{PlaceID: "story:init"},
				},
				FailureArcs: []petri.Arc{
					{PlaceID: "story:failed"},
					{PlaceID: "story:abandoned"},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"story": {
				ID:   "story",
				Name: "Story",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "retry", Category: state.StateCategoryProcessing},
					{Value: "triage", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
					{Value: "abandoned", Category: state.StateCategoryFailed},
				},
			},
		},
	}
}

func TestRecordedEventBridgePreservesCanonicalEvent(t *testing.T) {
	t.Parallel()

	history := NewFactoryEventHistory(nil, nil)
	eventTime := time.Date(2026, time.July, 16, 1, 2, 3, 0, time.FixedZone("test", -7*60*60))
	history.AppendRecordedEvent(factoryapi.FactoryEvent{
		Id:      "recorded-event-1",
		Type:    factoryapi.FactoryEventTypeRunRequest,
		Context: factoryapi.FactoryEventContext{EventTime: eventTime},
	})

	recorded := history.Events()
	if len(recorded) != 1 {
		t.Fatalf("len(Events()) = %d, want 1", len(recorded))
	}
	if recorded[0].Id != "recorded-event-1" || recorded[0].SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("recorded event identity = (%q, %q), want preserved ID and canonical schema", recorded[0].Id, recorded[0].SchemaVersion)
	}
	if got, want := recorded[0].Context.EventTime, eventTime.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("recorded event time = %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}

func stringValueForEventHistoryTest[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSliceValueForEventHistoryTest(value *[]string) []string {
	if value == nil {
		return nil
	}
	out := make([]string, len(*value))
	copy(out, *value)
	return out
}

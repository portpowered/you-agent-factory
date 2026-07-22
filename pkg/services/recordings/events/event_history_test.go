package events

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func generatedHistoryEvents(t testing.TB, history *FactoryEventHistory) []factoryapi.FactoryEvent {
	t.Helper()
	canonical := history.CanonicalEvents()
	generated := make([]factoryapi.FactoryEvent, len(canonical))
	for index, event := range canonical {
		if err := event.Decode(&generated[index]); err != nil {
			t.Fatalf("decode canonical Factory event %q for compatibility assertion: %v", event.Id, err)
		}
	}
	return generated
}

func TestFactoryEventHistory_EventRecorderCannotMutateCanonicalHistory(t *testing.T) {
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() })

	history.AddEventRecorder(func(event interfaces.FactoryEvent) {
		event.Payload[0] = 'X'
		event.Context.EventTime = time.Unix(10, 0).UTC()
	})

	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	if events[0].Context.EventTime != time.Unix(0, 0).UTC() {
		t.Fatalf("canonical event time mutated through recorder: %s", events[0].Context.EventTime)
	}
	if _, err := events[0].Payload.AsInitialStructureRequestEventPayload(); err != nil {
		t.Fatalf("canonical payload mutated through recorder: %v", err)
	}
}

func TestFactoryEventHistory_InitialStructureAndRunRequestUseFactoryOwnedPayloads(t *testing.T) {
	recordedAt := time.Date(2026, 7, 15, 23, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return recordedAt })
	editable, err := interfaces.NewFactorySnapshot(map[string]any{
		"name":         "editable-factory",
		"unknownField": "preserved",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	history.SetInitialStructureFactory(editable)
	(*editable)[0] = 'X'

	var canonical []interfaces.FactoryEvent
	history.AddEventRecorder(func(event interfaces.FactoryEvent) {
		canonical = append(canonical, event)
	})
	history.RecordInitialStructure()
	history.RecordRunRequest()

	if len(canonical) != 2 {
		t.Fatalf("canonical event count = %d, want 2", len(canonical))
	}
	if canonical[0].Type != interfaces.FactoryEventTypeInitialStructureRequest || canonical[1].Type != interfaces.FactoryEventTypeRunRequest {
		t.Fatalf("canonical event types = [%s, %s], want initial structure then run request", canonical[0].Type, canonical[1].Type)
	}

	var initial interfaces.InitialStructureRequestEventPayload
	if err := canonical[0].DecodePayload(&initial); err != nil {
		t.Fatalf("decode canonical initial structure payload: %v", err)
	}
	var initialFactory map[string]any
	if err := initial.Factory.Decode(&initialFactory); err != nil {
		t.Fatalf("decode canonical initial Factory snapshot: %v", err)
	}
	if initialFactory["name"] != "editable-factory" || initialFactory["unknownField"] != "preserved" {
		t.Fatalf("initial Factory snapshot = %#v, want detached editable document", initialFactory)
	}

	var run interfaces.RunRequestEventPayload
	if err := canonical[1].DecodePayload(&run); err != nil {
		t.Fatalf("decode canonical run request payload: %v", err)
	}
	if !run.RecordedAt.Equal(recordedAt) || run.Factory == nil {
		t.Fatalf("run request payload = %#v, want recorded time and Factory snapshot", run)
	}
	var generated factoryapi.FactoryEvent
	if err := canonical[1].Decode(&generated); err != nil {
		t.Fatalf("decode run request at generated boundary: %v", err)
	}
	if _, err := generated.Payload.AsRunRequestEventPayload(); err != nil {
		t.Fatalf("generated run request payload compatibility: %v", err)
	}
}

func TestFactoryEventHistory_FactoryChangeUsesFactoryOwnedPayloadAndRetainsPublicWireShape(t *testing.T) {
	eventTime := time.Date(2026, 7, 15, 23, 15, 0, 0, time.FixedZone("Factory/Local", 2*60*60))
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name":         "replacement-factory",
		"unknownField": "preserved",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	metadata := map[string]string{"source": "activation"}
	sourceDirectory := "/tmp/replacement"

	history.RecordFactoryChange(4, interfaces.FactoryChangeEventPayload{
		Factory:         snapshot,
		Metadata:        &metadata,
		SourceDirectory: &sourceDirectory,
	}, eventTime)

	assertCanonicalFactoryChangeEvent(t, history.CanonicalEvents(), sourceDirectory)
	assertPublicFactoryChangeEvent(t, generatedHistoryEvents(t, history))
}

func assertCanonicalFactoryChangeEvent(t *testing.T, canonical []interfaces.FactoryEvent, sourceDirectory string) {
	t.Helper()
	if len(canonical) != 1 {
		t.Fatalf("canonical event count = %d, want 1", len(canonical))
	}
	event := canonical[0]
	if event.Type != interfaces.FactoryEventTypeFactoryChange || event.Context.Tick != 4 {
		t.Fatalf("canonical event = %#v, want FACTORY_CHANGE at tick 4", event)
	}
	var payload interfaces.FactoryChangeEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode canonical Factory change payload: %v", err)
	}
	var factory map[string]any
	if err := payload.Factory.Decode(&factory); err != nil {
		t.Fatalf("decode replacement Factory snapshot: %v", err)
	}
	if factory["name"] != "replacement-factory" || factory["unknownField"] != "preserved" {
		t.Fatalf("replacement Factory snapshot = %#v, want preserved owner fields", factory)
	}
	if payload.Metadata == nil || (*payload.Metadata)["source"] != "activation" || payload.SourceDirectory == nil || *payload.SourceDirectory != sourceDirectory {
		t.Fatalf("canonical Factory change payload = %#v, want metadata and source directory", payload)
	}
	if event.Context.EventTime.Location() != time.UTC {
		t.Fatalf("canonical event time location = %s, want UTC", event.Context.EventTime.Location())
	}
}

func assertPublicFactoryChangeEvent(t *testing.T, publicEvents []factoryapi.FactoryEvent) {
	t.Helper()
	if len(publicEvents) != 1 {
		t.Fatalf("public event count = %d, want 1", len(publicEvents))
	}
	publicPayload, err := publicEvents[0].Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode generated Factory change payload: %v", err)
	}
	if publicPayload.Factory.Name != "replacement-factory" || publicPayload.Metadata == nil || (*publicPayload.Metadata)["source"] != "activation" {
		t.Fatalf("public Factory change payload = %#v, want compatible generated shape", publicPayload)
	}
}

func TestFactoryEventHistory_RecordInitialStructure_UsesRuntimeConfigProjection(t *testing.T) {
	runtimeConfig := eventHistoryRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"builder": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "codex-cli",
				ModelProvider:    "openai",
				Model:            "gpt-5.4",
			},
		},
	}
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig)

	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
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
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig)

	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
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
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

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
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		eventHistoryRuntimeConfig{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"Build": {Name: "Build", Kind: interfaces.WorkstationKindRepeater},
			},
		})

	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
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
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNetWithOrderedNonSuccessRoutes(),
		func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
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
	topology := eventHistoryProjectionSource{payload: interfaces.InitialStructurePayload{
		Workstations: []interfaces.FactoryWorkstation{{
			ID:              "poll-for-work",
			Name:            "poll-for-work",
			FailurePlaceIDs: []string{"task:failed"},
		}},
		Places: []interfaces.FactoryPlace{{
			ID: "task:failed", TypeID: "task", State: "failed",
		}},
	}}
	history := newTestFactoryEventHistory(topology, func() time.Time { return time.Unix(0, 0).UTC() })
	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
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
		Workers: map[string]*interfaces.FactoryWorkerConfig{
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
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
		runtimeConfig)

	history.RecordInitialStructure()

	events := generatedHistoryEvents(t, history)
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
	if got, want := stringValueForEventHistoryTest(worker.ExecutorProvider), "SCRIPT_WRAP"; got != want {
		t.Fatalf("worker executor provider = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(worker.ModelProvider), "CODEX"; got != want {
		t.Fatalf("worker model provider = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(worker.Type), "INFERENCE_WORKER"; got != want {
		t.Fatalf("worker type = %q, want %q", got, want)
	}

	workstation := (*payload.Factory.Workstations)[0]
	if got, want := stringValueForEventHistoryTest(workstation.Type), "LOGICAL_MOVE"; got != want {
		t.Fatalf("workstation type = %q, want %q", got, want)
	}
	if got, want := stringValueForEventHistoryTest(workstation.Behavior), "REPEATER"; got != want {
		t.Fatalf("workstation behavior = %q, want %q", got, want)
	}
}

func TestFactoryEventHistory_RecordDispatchCompletion_PreservesSelectedClassificationLabel(t *testing.T) {
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() })

	result := workerexecution.WorkResult{
		DispatchID:                  "dispatch-1",
		TransitionID:                "t-review",
		Outcome:                     workerexecution.OutcomeAccepted,
		SelectedClassificationLabel: "approved",
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "t-review",
		Outcome:         workerexecution.OutcomeAccepted,
		ConsumedTokens:  []workerexecution.Token{{ID: "token-1", Color: workerexecution.Color{WorkID: "work-1", TraceID: "trace-1"}}},
		OutputMutations: nil,
	}

	history.RecordWorkstationResponse(3, result, completed)

	events := generatedHistoryEvents(t, history)
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
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() })

	result := workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Outcome:      workerexecution.OutcomeAccepted,
	}
	completed := interfaces.CompletedDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Outcome:      workerexecution.OutcomeAccepted,
		ConsumedTokens: []workerexecution.Token{{
			ID:    "token-1",
			Color: workerexecution.Color{WorkID: "work-1", WorkTypeID: "task", TraceID: "trace-1"},
		}},
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type: interfaces.MutationMove,
			Token: &workerexecution.Token{
				ID:      "token-terminal",
				PlaceID: "task:complete",
				Color: workerexecution.Color{
					WorkID:     "work-1",
					WorkTypeID: "task",
					Name:       "Write docs",
					TraceID:    "trace-1",
				},
			},
		}},
	}

	history.RecordWorkstationResponse(3, result, completed)

	events := generatedHistoryEvents(t, history)
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
	Workers      map[string]*interfaces.FactoryWorkerConfig
	Workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (c eventHistoryDefinitionOnlyRuntimeConfig) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	worker, ok := c.Workers[name]
	return worker, ok
}

func (c eventHistoryDefinitionOnlyRuntimeConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := c.Workstations[name]
	return workstation, ok
}

type eventHistoryProjectionSource struct {
	payload interfaces.InitialStructurePayload
}

func (s eventHistoryProjectionSource) RecordingInitialStructure(
	runtimeConfigs ...interfaces.RuntimeDefinitionLookup,
) interfaces.InitialStructurePayload {
	payload := s.payload
	runtimeConfig := interfaces.FirstRuntimeDefinitionLookup(runtimeConfigs...)
	if runtimeConfig == nil {
		return payload
	}
	if reader, ok := runtimeConfig.(interfaces.RuntimeFactoryConfigLookup); ok {
		if factory := reader.FactoryConfig(); factory != nil {
			cloned, err := interfaces.CloneFactoryConfig(factory)
			if err == nil && cloned != nil {
				payload.Name = cloned.Name
				payload.Version = cloned.Version
				payload.ResourceManifest = cloned.ResourceManifest
				payload.Layout = cloned.Layout
			}
		}
	}
	workerIDs := make(map[string]bool)
	for index := range payload.Workstations {
		workstation := &payload.Workstations[index]
		if workstation.WorkerID != "" {
			workerIDs[workstation.WorkerID] = true
		}
		if definition, ok := runtimeConfig.Workstation(workstation.Name); ok && definition != nil {
			workstation.Kind = interfaces.CanonicalPublicWorkstationKind(definition.Kind)
			workstation.Config = map[string]string{
				"type":   strings.TrimSpace(definition.Type),
				"worker": strings.TrimSpace(definition.WorkerTypeName),
			}
		}
	}
	payload.Workers = nil
	for workerID := range workerIDs {
		definition, ok := runtimeConfig.Worker(workerID)
		if !ok || definition == nil {
			continue
		}
		payload.Workers = append(payload.Workers, interfaces.FactoryWorker{
			ID:            workerID,
			Name:          workerID,
			Provider:      strings.TrimSpace(definition.ExecutorProvider),
			ModelProvider: strings.TrimSpace(definition.ModelProvider),
			Model:         definition.Model,
			Config:        map[string]string{"type": strings.TrimSpace(definition.Type)},
		})
	}
	return payload
}

func eventHistoryProjectionNet() eventHistoryProjectionSource {
	return eventHistoryProjectionSource{payload: interfaces.InitialStructurePayload{
		WorkTypes: []interfaces.FactoryWorkType{{
			ID: "story", Name: "Story",
			States: []interfaces.FactoryStateDefinition{
				{Value: "init", Category: "initial"},
				{Value: "review", Category: "processing"},
				{Value: "done", Category: "terminal"},
				{Value: "failed", Category: "failed"},
			},
		}},
		Places: []interfaces.FactoryPlace{
			{ID: "story:init", TypeID: "story", State: "init"},
			{ID: "story:review", TypeID: "story", State: "review"},
			{ID: "story:done", TypeID: "story", State: "done"},
			{ID: "story:failed", TypeID: "story", State: "failed"},
		},
		Workstations: []interfaces.FactoryWorkstation{{
			ID: "build", Name: "Build", WorkerID: "builder",
			InputPlaceIDs: []string{"story:init"}, OutputPlaceIDs: []string{"story:review"},
			FailurePlaceIDs: []string{"story:failed"},
		}},
	}}
}

func eventHistoryProjectionNetWithOrderedNonSuccessRoutes() eventHistoryProjectionSource {
	source := eventHistoryProjectionNet()
	source.payload.Places = append(source.payload.Places,
		interfaces.FactoryPlace{ID: "story:retry", TypeID: "story", State: "retry"},
		interfaces.FactoryPlace{ID: "story:triage", TypeID: "story", State: "triage"},
		interfaces.FactoryPlace{ID: "story:abandoned", TypeID: "story", State: "abandoned"},
	)
	source.payload.Workstations[0].ContinuePlaceIDs = []string{"story:retry", "story:init"}
	source.payload.Workstations[0].RejectionPlaceIDs = []string{"story:triage", "story:init"}
	source.payload.Workstations[0].FailurePlaceIDs = []string{"story:failed", "story:abandoned"}
	return source
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

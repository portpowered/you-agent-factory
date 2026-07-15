package replay

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestRunStartedPayloadFromEvent_RejectsRetiredFactoryAliases(t *testing.T) {
	rawEvent := map[string]any{
		"id":            "factory-event/run-started",
		"schemaVersion": factoryapi.AgentFactoryEventV1,
		"type":          factoryapi.FactoryEventTypeRunRequest,
		"context": map[string]any{
			"eventTime": time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"tick":      0,
		},
		"payload": map[string]any{
			"recordedAt": time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"factory": map[string]any{
				"name": "retired-factory-alias-event",
				"workTypes": []map[string]any{
					{
						"name": "story",
						"states": []map[string]string{
							{"name": "ready", "type": "PROCESSING"},
							{"name": "complete", "type": "TERMINAL"},
						},
					},
				},
				"workers": []map[string]any{
					{
						"name":           "executor",
						"type":           "MODEL_WORKER",
						"modelProvider":  "CODEX",
						"model_provider": "anthropic",
					},
				},
				"workstations": []map[string]any{
					{
						"name":     "scheduled-story",
						"behavior": "CRON",
						"worker":   "executor",
						"cron": map[string]any{
							"schedule":         "*/5 * * * *",
							"triggerAtStart":   false,
							"trigger_at_start": true,
							"expiryWindow":     "30s",
							"expiry_window":    "45s",
						},
						"outputs": []map[string]string{
							{"workType": "story", "state": "complete"},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(rawEvent)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal factory event: %v", err)
	}
	domainEvent, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		t.Fatalf("convert factory event: %v", err)
	}

	_, err = runStartedPayloadFromEvent(domainEvent)
	if err == nil {
		t.Fatal("expected retired factory aliases to be rejected")
	}
	if want := "workers[0].model_provider is not supported; use modelProvider"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected model_provider retirement guidance, got %v", err)
	}
}

func TestRunStartedPayloadFromEvent_AllowsLegacyOnFailureObjectWhileNormalizingFactoryBoundary(t *testing.T) {
	recordedAt := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	rawEvent := map[string]any{
		"id":            "factory-event/run-started",
		"schemaVersion": factoryapi.AgentFactoryEventV1,
		"type":          factoryapi.FactoryEventTypeRunRequest,
		"context": map[string]any{
			"eventTime": recordedAt.Format(time.RFC3339),
			"tick":      0,
		},
		"payload": map[string]any{
			"recordedAt": recordedAt.Format(time.RFC3339),
			"factory": map[string]any{
				"name": "legacy-onfailure-shape",
				"workTypes": []map[string]any{
					{
						"name": "story",
						"states": []map[string]string{
							{"name": "ready", "type": "INITIAL"},
							{"name": "failed", "type": "FAILED"},
							{"name": "complete", "type": "TERMINAL"},
						},
					},
				},
				"workers": []map[string]any{
					{
						"name":          "executor",
						"type":          "MODEL_WORKER",
						"modelProvider": "CODEX",
					},
				},
				"workstations": []map[string]any{
					{
						"name":      "scheduled-story",
						"worker":    "executor",
						"inputs":    []map[string]string{{"workType": "story", "state": "ready"}},
						"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
						"onFailure": map[string]string{"workType": "story", "state": "failed"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(rawEvent)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal factory event: %v", err)
	}
	domainEvent, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		t.Fatalf("convert factory event: %v", err)
	}

	payload, err := runStartedPayloadFromEvent(domainEvent)
	if err != nil {
		t.Fatalf("runStartedPayloadFromEvent() error = %v", err)
	}
	if !payload.RecordedAt.Equal(recordedAt) {
		t.Fatalf("RecordedAt = %s, want %s", payload.RecordedAt, recordedAt)
	}
	generatedFactory, err := generatedFactoryFromSnapshot(payload.Factory)
	if err != nil {
		t.Fatalf("decode run request Factory snapshot: %v", err)
	}
	if generatedFactory.Name != "legacy-onfailure-shape" {
		t.Fatalf("factory name = %q, want legacy-onfailure-shape", generatedFactory.Name)
	}
	if generatedFactory.Workstations == nil || len(*generatedFactory.Workstations) != 1 {
		t.Fatalf("factory workstations = %#v, want normalized workstation", generatedFactory.Workstations)
	}
	if got := (*generatedFactory.Workstations)[0].OnFailure; got == nil || !reflect.DeepEqual(*got, []factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}}) {
		t.Fatalf("normalized onFailure = %#v, want [{WorkType:story State:failed}]", got)
	}
}

func TestApplyReplayRunRequest_DecodesFactoryOwnedPayloadAndSafeDiagnostics(t *testing.T) {
	recordedAt := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"recordedAt": recordedAt,
		"factory":    testGeneratedFactory(),
		"wallClock":  map[string]any{"startedAt": recordedAt},
		"diagnostics": map[string]any{
			"notes": []string{"domain-owned"},
			"workers": map[string]any{
				"executor": map[string]any{
					"renderedPrompt": map[string]any{"systemPromptHash": "sha256:prompt"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal run request payload: %v", err)
	}
	event := interfaces.FactoryEvent{
		Id:      "factory-event/run-started",
		Type:    interfaces.FactoryEventTypeRunRequest,
		Payload: payload,
	}

	reduced := &replayEventLog{}
	if err := applyReplayRunRequest(reduced, event); err != nil {
		t.Fatalf("applyReplayRunRequest: %v", err)
	}
	if reduced.Factory.Name != testGeneratedFactory().Name {
		t.Fatalf("factory name = %q, want %q", reduced.Factory.Name, testGeneratedFactory().Name)
	}
	if reduced.WallClock == nil || !reduced.WallClock.StartedAt.Equal(recordedAt) {
		t.Fatalf("wall clock = %#v, want startedAt %s", reduced.WallClock, recordedAt)
	}
	if !reflect.DeepEqual(reduced.Diagnostics.Notes, []string{"domain-owned"}) {
		t.Fatalf("diagnostic notes = %#v, want domain-owned", reduced.Diagnostics.Notes)
	}
	worker := reduced.Diagnostics.Workers["executor"]
	if worker.RenderedPrompt == nil || worker.RenderedPrompt.SystemPromptHash != "sha256:prompt" {
		t.Fatalf("worker diagnostics = %#v, want camel-case rendered prompt", worker)
	}

	event.Payload = json.RawMessage(`{"factory":`)
	if err := applyReplayRunRequest(reduced, event); err == nil || !strings.Contains(err.Error(), "decode run started event") {
		t.Fatalf("malformed payload error = %v, want run-started decode context", err)
	}
}

func TestRunFinishedEvent_EmitsDomainPayload(t *testing.T) {
	startedAt := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(3 * time.Second)
	event := runFinishedTestEvent(startedAt, finishedAt)

	var domainPayload interfaces.RunResponseEventPayload
	if err := event.DecodePayload(&domainPayload); err != nil {
		t.Fatalf("decode domain run response payload: %v", err)
	}
	if domainPayload.State == nil || *domainPayload.State != interfaces.FactoryStateCompleted {
		t.Fatalf("domain state = %#v, want COMPLETED", domainPayload.State)
	}
	if domainPayload.WallClock == nil || domainPayload.WallClock.StartedAt == nil || !domainPayload.WallClock.StartedAt.Equal(startedAt) {
		t.Fatalf("domain wall clock = %#v, want startedAt %s", domainPayload.WallClock, startedAt)
	}
	if domainPayload.Diagnostics == nil || !reflect.DeepEqual(domainPayload.Diagnostics.Notes, []string{"completed safely"}) {
		t.Fatalf("domain diagnostics = %#v, want completion note", domainPayload.Diagnostics)
	}
}

func TestRunFinishedEvent_RetainsGeneratedWireCompatibility(t *testing.T) {
	startedAt := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(3 * time.Second)
	event := runFinishedTestEvent(startedAt, finishedAt)

	var generatedEvent factoryapi.FactoryEvent
	if err := event.Decode(&generatedEvent); err != nil {
		t.Fatalf("decode generated run response event: %v", err)
	}
	generatedPayload, err := generatedEvent.Payload.AsRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode generated run response payload: %v", err)
	}
	if generatedPayload.State == nil || *generatedPayload.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("generated state = %#v, want COMPLETED", generatedPayload.State)
	}
	if generatedPayload.WallClock == nil || generatedPayload.WallClock.FinishedAt == nil || !generatedPayload.WallClock.FinishedAt.Equal(finishedAt) {
		t.Fatalf("generated wall clock = %#v, want finishedAt %s", generatedPayload.WallClock, finishedAt)
	}
	if generatedPayload.Diagnostics == nil || !reflect.DeepEqual(stringSliceValue(generatedPayload.Diagnostics.Notes), []string{"completed safely"}) {
		t.Fatalf("generated diagnostics = %#v, want completion note", generatedPayload.Diagnostics)
	}
}

func runFinishedTestEvent(startedAt, finishedAt time.Time) interfaces.FactoryEvent {
	return runFinishedEvent(finishedAt, &interfaces.ReplayWallClockMetadata{
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}, interfaces.ReplayDiagnostics{Notes: []string{"completed safely"}})
}

func TestApplyReplayRunResponse_DecodesDomainPayloadAndRejectsMalformedPayload(t *testing.T) {
	finishedAt := time.Date(2026, 4, 21, 12, 0, 3, 0, time.UTC)
	event := interfaces.FactoryEvent{
		Id:   "factory-event/run-finished",
		Type: interfaces.FactoryEventTypeRunResponse,
		Payload: json.RawMessage(`{
			"diagnostics":{"notes":["domain-owned"]},
			"state":"COMPLETED",
			"wallClock":{"finishedAt":"2026-04-21T12:00:03Z"}
		}`),
	}
	reduced := &replayEventLog{}
	if err := applyReplayRunResponse(reduced, event); err != nil {
		t.Fatalf("applyReplayRunResponse: %v", err)
	}
	if reduced.WallClock == nil || !reduced.WallClock.FinishedAt.Equal(finishedAt) {
		t.Fatalf("reduced wall clock = %#v, want finishedAt %s", reduced.WallClock, finishedAt)
	}
	if !reflect.DeepEqual(reduced.Diagnostics.Notes, []string{"domain-owned"}) {
		t.Fatalf("reduced diagnostics = %#v, want domain-owned note", reduced.Diagnostics)
	}

	event.Payload = json.RawMessage(`{"wallClock":`)
	if err := applyReplayRunResponse(reduced, event); err == nil || !strings.Contains(err.Error(), "decode run finished event") {
		t.Fatalf("malformed payload error = %v, want run-finished decode context", err)
	}
}

func TestMergeGeneratedWorkers_ReplacesExistingEntriesAndAppendsRuntimeOnlyInSortedOrder(t *testing.T) {
	factory := &factoryapi.Factory{
		Workers: &[]factoryapi.Worker{
			{
				Name:    "alpha",
				Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
				Command: stringPtrIfNotEmpty("stale-alpha"),
			},
			{
				Name:    "zeta",
				Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
				Command: stringPtrIfNotEmpty("keep-zeta"),
			},
		},
	}

	runtimeWorkers := map[string]workerconfig.Config{
		"charlie": {
			Type:      string(factoryapi.WorkerTypeScriptWorker),
			Command:   "charlie-command",
			Args:      []string{"charlie-arg"},
			StopToken: "DONE",
		},
		"alpha": {
			Type:      string(factoryapi.WorkerTypeScriptWorker),
			Command:   "fresh-alpha",
			Args:      []string{"alpha-arg"},
			StopToken: "COMPLETE",
		},
		"bravo": {
			Type:    string(factoryapi.WorkerTypeScriptWorker),
			Command: "bravo-command",
		},
	}

	if err := mergeGeneratedWorkers(factory, runtimeWorkers, nil); err != nil {
		t.Fatalf("mergeGeneratedWorkers() error = %v", err)
	}
	if factory.Workers == nil {
		t.Fatal("merged workers = nil, want generated worker list")
	}

	got := *factory.Workers
	if len(got) != 4 {
		t.Fatalf("merged workers count = %d, want 4", len(got))
	}
	if got[0].Name != "alpha" || stringValue(got[0].Command) != "fresh-alpha" || !reflect.DeepEqual(stringSliceValue(got[0].Args), []string{"alpha-arg"}) {
		t.Fatalf("merged alpha worker = %#v, want replaced runtime definition", got[0])
	}
	if stringValue(got[0].StopToken) != "COMPLETE" {
		t.Fatalf("merged alpha stop token = %q, want COMPLETE", stringValue(got[0].StopToken))
	}
	if got[1].Name != "zeta" || stringValue(got[1].Command) != "keep-zeta" {
		t.Fatalf("merged zeta worker = %#v, want untouched existing generated entry", got[1])
	}
	if got[2].Name != "bravo" || stringValue(got[2].Command) != "bravo-command" {
		t.Fatalf("merged bravo worker = %#v, want first sorted runtime-only append", got[2])
	}
	if got[3].Name != "charlie" || stringValue(got[3].Command) != "charlie-command" || !reflect.DeepEqual(stringSliceValue(got[3].Args), []string{"charlie-arg"}) {
		t.Fatalf("merged charlie worker = %#v, want second sorted runtime-only append", got[3])
	}
}

func TestMergeGeneratedWorkstations_ReplacesExistingEntriesAndAppendsRuntimeOnlyInSortedOrder(t *testing.T) {
	factory, runtimeWorkstations := mergeGeneratedWorkstationsFixture()
	if err := mergeGeneratedWorkstations(factory, runtimeWorkstations, nil); err != nil {
		t.Fatalf("mergeGeneratedWorkstations() error = %v", err)
	}
	assertMergedGeneratedWorkstations(t, factory)
}

func workstationKindPtr(value factoryapi.WorkstationKind) *factoryapi.WorkstationKind {
	return &value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func mergeGeneratedWorkstationsFixture() (*factoryapi.Factory, map[string]interfaces.FactoryWorkstationConfig) {
	return &factoryapi.Factory{
			Workstations: &[]factoryapi.Workstation{
				{
					Name:     "alpha",
					Worker:   "stale-worker",
					Behavior: workstationKindPtr(factoryapi.WorkstationKindCron),
					Cron: &factoryapi.WorkstationCron{
						Schedule: "0 * * * *",
					},
					Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "stale"}},
					Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "stale-done"}},
				},
				{
					Name:    "zeta",
					Worker:  "keep-worker",
					Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "ready"}},
					Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "done"}},
				},
			},
		}, map[string]interfaces.FactoryWorkstationConfig{
			"charlie": {
				Name:           "charlie",
				Kind:           interfaces.WorkstationKindStandard,
				Type:           interfaces.WorkstationTypeLogical,
				WorkerTypeName: "charlie-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			},
			"alpha": {
				Kind:             interfaces.WorkstationKindCron,
				Type:             interfaces.WorkstationTypeLogical,
				WorkerTypeName:   "fresh-worker",
				Cron:             &interfaces.CronConfig{Schedule: "*/5 * * * *", TriggerAtStart: true, ExpiryWindow: "30s"},
				Inputs:           []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				Outputs:          []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
				OnFailure:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
				Resources:        []factoryresource.Config{{Name: "agent-slot", Capacity: 2}},
				WorkingDirectory: "/repo/runtime",
			},
			"bravo": {
				Name:           "bravo",
				Kind:           interfaces.WorkstationKindStandard,
				Type:           interfaces.WorkstationTypeLogical,
				WorkerTypeName: "bravo-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "ready"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
				Resources:      []factoryresource.Config{{Name: "gpu", Capacity: 1}},
			},
		}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the merged generated workstation artifact contract in one place.
func assertMergedGeneratedWorkstations(t *testing.T, factory *factoryapi.Factory) {
	t.Helper()
	if factory.Workstations == nil {
		t.Fatal("merged workstations = nil, want generated workstation list")
	}

	got := *factory.Workstations
	if len(got) != 4 {
		t.Fatalf("merged workstations count = %d, want 4", len(got))
	}
	if got[0].Name != "alpha" || got[0].Worker != "fresh-worker" {
		t.Fatalf("merged alpha workstation = %#v, want replaced runtime definition", got[0])
	}
	if got[0].Behavior == nil || *got[0].Behavior != factoryapi.WorkstationKindCron {
		t.Fatalf("merged alpha behavior = %#v, want CRON", got[0].Behavior)
	}
	if got[0].Cron == nil || got[0].Cron.Schedule != "*/5 * * * *" || !boolValue(got[0].Cron.TriggerAtStart) || stringValue(got[0].Cron.ExpiryWindow) != "30s" {
		t.Fatalf("merged alpha cron = %#v, want runtime cron fields", got[0].Cron)
	}
	if !reflect.DeepEqual(got[0].Inputs, []factoryapi.WorkstationIO{{WorkType: "story", State: "review"}}) {
		t.Fatalf("merged alpha inputs = %#v, want runtime inputs", got[0].Inputs)
	}
	if got[0].Outputs == nil || !reflect.DeepEqual(*got[0].Outputs, []factoryapi.WorkstationIO{{WorkType: "story", State: "complete"}}) {
		t.Fatalf("merged alpha outputs = %#v, want runtime outputs", got[0].Outputs)
	}
	if got[0].OnFailure == nil || !reflect.DeepEqual(*got[0].OnFailure, []factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}}) {
		t.Fatalf("merged alpha onFailure = %#v, want runtime onFailure", got[0].OnFailure)
	}
	if stringValue(got[0].WorkingDirectory) != "/repo/runtime" {
		t.Fatalf("merged alpha working directory = %q, want /repo/runtime", stringValue(got[0].WorkingDirectory))
	}
	if got[0].Resources == nil || !reflect.DeepEqual(*got[0].Resources, []factoryapi.ResourceRequirement{{Name: "agent-slot", Capacity: 2}}) {
		t.Fatalf("merged alpha resources = %#v, want runtime resources", got[0].Resources)
	}
	if got[1].Name != "zeta" || got[1].Worker != "keep-worker" {
		t.Fatalf("merged zeta workstation = %#v, want untouched existing generated entry", got[1])
	}
	if got[2].Name != "bravo" || got[2].Worker != "bravo-worker" {
		t.Fatalf("merged bravo workstation = %#v, want first sorted runtime-only append", got[2])
	}
	if got[2].Resources == nil || !reflect.DeepEqual(*got[2].Resources, []factoryapi.ResourceRequirement{{Name: "gpu", Capacity: 1}}) {
		t.Fatalf("merged bravo resources = %#v, want appended runtime resources", got[2].Resources)
	}
	if got[3].Name != "charlie" || got[3].Worker != "charlie-worker" {
		t.Fatalf("merged charlie workstation = %#v, want second sorted runtime-only append", got[3])
	}
	if got[3].Behavior == nil || *got[3].Behavior != factoryapi.WorkstationKindStandard {
		t.Fatalf("merged charlie behavior = %#v, want STANDARD", got[3].Behavior)
	}
}

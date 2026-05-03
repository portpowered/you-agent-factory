package runtime_api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestGeneratedSchemaDeserializationSmoke_FileHTTPAndReplayTransportsStayAligned(t *testing.T) {
	support.SkipLongFunctional(t, "slow generated-schema transport-alignment sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recordDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "generated-schema-deserialization.replay.json")

	fileBoundary, loaded := loadGeneratedSchemaFileBoundaryAndRuntime(t, dir)
	assertGeneratedSmokeTopologyBoundary(t, fileBoundary)
	fileTransportSummary := generatedSchemaTransportSummaryFromRuntimeConfig(t, loaded.Worker, loaded.Workstation)
	fileRuntimeSummary := generatedSchemaRuntimeSummaryFromLoadedConfig(t, loaded)
	httpTransportSummary := generatedSchemaTransportSummaryFromHTTPBoundary(t, dir)
	replayTransportSummary, replayRuntimeSummary := generatedSchemaTransportAndRuntimeSummaryFromRecordedReplay(t, recordDir, artifactPath)

	if !reflect.DeepEqual(httpTransportSummary, fileTransportSummary) {
		t.Fatalf("HTTP initial structure transport summary mismatch\nhttp: %#v\nfile: %#v", httpTransportSummary, fileTransportSummary)
	}
	if !reflect.DeepEqual(replayTransportSummary, fileTransportSummary) {
		t.Fatalf("recorded run-started transport summary mismatch\nreplay: %#v\nfile:   %#v", replayTransportSummary, fileTransportSummary)
	}
	if !reflect.DeepEqual(replayTransportSummary, httpTransportSummary) {
		t.Fatalf("recorded run-started and HTTP transport summaries diverged\nreplay: %#v\nhttp:   %#v", replayTransportSummary, httpTransportSummary)
	}
	if !reflect.DeepEqual(replayRuntimeSummary, fileRuntimeSummary) {
		t.Fatalf("recorded run-started full runtime summary mismatch\nreplay: %#v\nfile:   %#v", replayRuntimeSummary, fileRuntimeSummary)
	}
}

func loadGeneratedSchemaFileBoundaryAndRuntime(t *testing.T, dir string) (factoryapi.Factory, *config.LoadedFactoryConfig) {
	t.Helper()

	fileJSON, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	fileBoundary, err := config.GeneratedFactoryFromOpenAPIJSON(fileJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	return fileBoundary, loaded
}

func generatedSchemaRuntimeSummaryFromLoadedConfig(t *testing.T, loaded *config.LoadedFactoryConfig) generatedSchemaRuntimeSummary {
	t.Helper()

	return generatedSchemaRuntimeSummaryFromRuntimeConfig(t, loaded.Worker, loaded.Workstation)
}

func generatedSchemaTransportSummaryFromRuntimeConfig(
	t *testing.T,
	workerLookup func(string) (*interfaces.WorkerConfig, bool),
	workstationLookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
) generatedSchemaTransportSummary {
	t.Helper()

	return generatedSchemaTransportSummary{
		workers: []generatedSchemaWorkerSummary{
			requireGeneratedSchemaWorkerSummary(t, workerLookup, "worker-a"),
			requireGeneratedSchemaWorkerSummary(t, workerLookup, "worker-b"),
		},
		workstations: []generatedSchemaTransportWorkstationSummary{
			requireGeneratedSchemaTransportWorkstationSummary(t, workstationLookup, "step-one"),
			requireGeneratedSchemaTransportWorkstationSummary(t, workstationLookup, "step-two"),
		},
	}
}

func generatedSchemaRuntimeSummaryFromRuntimeConfig(
	t *testing.T,
	workerLookup func(string) (*interfaces.WorkerConfig, bool),
	workstationLookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
) generatedSchemaRuntimeSummary {
	t.Helper()

	return generatedSchemaRuntimeSummary{
		workers: []generatedSchemaWorkerSummary{
			requireGeneratedSchemaWorkerSummary(t, workerLookup, "worker-a"),
			requireGeneratedSchemaWorkerSummary(t, workerLookup, "worker-b"),
		},
		workstations: []generatedSchemaRuntimeWorkstationSummary{
			requireGeneratedSchemaRuntimeWorkstationSummary(t, workstationLookup, "step-one"),
			requireGeneratedSchemaRuntimeWorkstationSummary(t, workstationLookup, "step-two"),
		},
	}
}

func generatedSchemaTransportSummaryFromHTTPBoundary(t *testing.T, dir string) generatedSchemaTransportSummary {
	t.Helper()

	server := startFunctionalServer(t, dir, false, factory.WithServiceMode())
	stream := openFactoryEventHTTPStream(t, server.URL()+"/events")
	_, first := requireFunctionalEventStreamPrelude(t, stream)
	initialStructurePayload, err := first.Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("decode initial-structure payload: %v", err)
	}
	assertGeneratedSmokeTransportBoundary(t, initialStructurePayload.Factory)
	httpRuntime, err := replay.RuntimeConfigFromGeneratedFactory(initialStructurePayload.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory(initial structure HTTP payload): %v", err)
	}
	stream.close()
	return generatedSchemaTransportSummaryFromRuntimeConfig(t, httpRuntime.Worker, httpRuntime.Workstation)
}

func generatedSchemaTransportAndRuntimeSummaryFromRecordedReplay(
	t *testing.T,
	dir string,
	artifactPath string,
) (generatedSchemaTransportSummary, generatedSchemaRuntimeSummary) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "generated-schema-runtime-work",
		TraceID:    "generated-schema-runtime-trace",
		Payload:    []byte(`{"title":"generated schema deserialization smoke"}`),
	})
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	harness.RunUntilComplete(t, 10*time.Second)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	runStarted := requireGeneratedSchemaRunStartedPayload(t, artifact.Events)
	assertGeneratedSmokeTransportBoundary(t, runStarted.Factory)
	assertGeneratedSmokeRuntimeDefinitions(t, runStarted.Factory)
	replayRuntime, err := replay.RuntimeConfigFromGeneratedFactory(runStarted.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory(run started): %v", err)
	}
	return generatedSchemaTransportSummaryFromRuntimeConfig(t, replayRuntime.Worker, replayRuntime.Workstation),
		generatedSchemaRuntimeSummaryFromRuntimeConfig(t, replayRuntime.Worker, replayRuntime.Workstation)
}

func TestGeneratedSchemaDeserializationSmoke_FileAndRecordedTransportRejectRetiredFieldsAtSameBoundaryStage(t *testing.T) {
	support.SkipLongFunctional(t, "slow generated-schema retired-field sweep")
	dir := t.TempDir()
	factoryJSON := []byte(`{
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"worker-a"}],
		"workstations": [{
			"name":"step-one",
			"worker":"worker-a",
			"inputs":[{"workType":"task","state":"init"}],
			"outputs":[{"workType":"task","state":"complete"}],
			"join":{"waitFor":"task","waitState":"complete","require":"all"}
		}]
	}`)
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), factoryJSON, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	_, fileErr := config.LoadRuntimeConfig(dir, nil)
	assertGeneratedSchemaBoundaryFailure(t, fileErr)

	artifactPath := filepath.Join(t.TempDir(), "retired-generated-schema-boundary.replay.json")
	writeGeneratedSchemaReplayArtifact(t, artifactPath, factoryJSON)
	_, replayErr := replay.Load(artifactPath)
	assertGeneratedSchemaBoundaryFailure(t, replayErr)
}

type generatedSchemaRuntimeSummary struct {
	workers      []generatedSchemaWorkerSummary
	workstations []generatedSchemaRuntimeWorkstationSummary
}

type generatedSchemaTransportSummary struct {
	workers      []generatedSchemaWorkerSummary
	workstations []generatedSchemaTransportWorkstationSummary
}

type generatedSchemaWorkerSummary struct {
	name       string
	workerType string
	model      string
}

type generatedSchemaTransportWorkstationSummary struct {
	name            string
	workerTypeName  string
	workstationType string
}

type generatedSchemaRuntimeWorkstationSummary struct {
	name            string
	workerTypeName  string
	workstationType string
	body            string
}

func assertGeneratedSmokeTopologyBoundary(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	if generated.WorkTypes == nil || len(*generated.WorkTypes) != 1 {
		t.Fatalf("file boundary work types = %#v, want one task work type", generated.WorkTypes)
	}
	if generated.Workers == nil || len(*generated.Workers) != 2 {
		t.Fatalf("file boundary workers = %#v, want two workers", generated.Workers)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 2 {
		t.Fatalf("file boundary workstations = %#v, want two workstations", generated.Workstations)
	}
	assertGeneratedSmokeWorkstationBoundary(t, *generated.Workstations, "step-one", "worker-a")
	assertGeneratedSmokeWorkstationBoundary(t, *generated.Workstations, "step-two", "worker-b")
}

func assertGeneratedSmokeTransportBoundary(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	assertGeneratedSmokeTopologyBoundary(t, generated)
	if generated.Workers == nil {
		t.Fatal("runtime boundary workers = nil")
	}
	for _, worker := range *generated.Workers {
		if worker.Name != "worker-a" && worker.Name != "worker-b" {
			continue
		}
		if stringValueFromFunctionalPtr(worker.Type) != interfaces.WorkerTypeModel {
			t.Fatalf("runtime boundary worker %q type = %q, want %q", worker.Name, stringValueFromFunctionalPtr(worker.Type), interfaces.WorkerTypeModel)
		}
	}
	if generated.Workstations == nil {
		t.Fatal("runtime boundary workstations = nil")
	}
	for _, workstation := range *generated.Workstations {
		if workstation.Name != "step-one" && workstation.Name != "step-two" {
			continue
		}
		if stringValueFromFunctionalPtr(workstation.Type) != interfaces.WorkstationTypeModel {
			t.Fatalf("runtime boundary workstation %q type = %q, want %q", workstation.Name, stringValueFromFunctionalPtr(workstation.Type), interfaces.WorkstationTypeModel)
		}
	}
}

func assertGeneratedSmokeRuntimeDefinitions(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	if generated.Workstations == nil {
		t.Fatal("runtime boundary workstations = nil")
	}
	for _, workstation := range *generated.Workstations {
		if workstation.Name != "step-one" && workstation.Name != "step-two" {
			continue
		}
		if !strings.Contains(stringValueFromFunctionalPtr(workstation.Body), "Do the work.") {
			t.Fatalf("runtime boundary workstation %q body = %q, want split runtime prompt", workstation.Name, stringValueFromFunctionalPtr(workstation.Body))
		}
	}
}

func requireGeneratedSchemaWorkerSummary(
	t *testing.T,
	workerLookup func(string) (*interfaces.WorkerConfig, bool),
	name string,
) generatedSchemaWorkerSummary {
	t.Helper()

	worker, ok := workerLookup(name)
	if !ok {
		t.Fatalf("worker lookup missing %q", name)
	}
	return generatedSchemaWorkerSummary{
		name:       worker.Name,
		workerType: worker.Type,
		model:      worker.Model,
	}
}

func requireGeneratedSchemaTransportWorkstationSummary(
	t *testing.T,
	workstationLookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
	name string,
) generatedSchemaTransportWorkstationSummary {
	t.Helper()

	workstation, ok := workstationLookup(name)
	if !ok {
		t.Fatalf("workstation lookup missing %q", name)
	}
	return generatedSchemaTransportWorkstationSummary{
		name:            workstation.Name,
		workerTypeName:  workstation.WorkerTypeName,
		workstationType: workstation.Type,
	}
}

func requireGeneratedSchemaRuntimeWorkstationSummary(
	t *testing.T,
	workstationLookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
	name string,
) generatedSchemaRuntimeWorkstationSummary {
	t.Helper()

	workstation, ok := workstationLookup(name)
	if !ok {
		t.Fatalf("workstation lookup missing %q", name)
	}
	return generatedSchemaRuntimeWorkstationSummary{
		name:            workstation.Name,
		workerTypeName:  workstation.WorkerTypeName,
		workstationType: workstation.Type,
		body:            workstation.Body,
	}
}

func assertGeneratedSmokeWorkstationBoundary(t *testing.T, workstations []factoryapi.Workstation, name, worker string) {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Name == name {
			if workstation.Worker != worker {
				t.Fatalf("workstation %q worker = %q, want %q", name, workstation.Worker, worker)
			}
			return
		}
	}
	t.Fatalf("workstations = %#v, want %q", workstations, name)
}

func assertGeneratedSchemaBoundaryFailure(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected generated schema boundary failure, got nil")
	}
	text := err.Error()
	for _, snippet := range []string{
		"is not supported",
		"use ",
	} {
		if !strings.Contains(text, snippet) {
			t.Fatalf("generated schema boundary error = %q, want substring %q", text, snippet)
		}
	}
}

func requireGeneratedSchemaRunStartedPayload(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.RunRequestEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeRunRequest {
			continue
		}
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			t.Fatalf("decode run-request payload %q: %v", event.Id, err)
		}
		if payload.Factory.WorkTypes == nil || len(*payload.Factory.WorkTypes) == 0 {
			t.Fatalf("run-request payload factory missing work types: %#v", payload.Factory)
		}
		return payload
	}
	t.Fatalf("recorded events missing RUN_REQUEST: %#v", functionalEventTypes(events))
	return factoryapi.RunRequestEventPayload{}
}

func writeGeneratedSchemaReplayArtifact(t *testing.T, path string, factoryJSON []byte) {
	t.Helper()

	recordedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	artifact := map[string]any{
		"schemaVersion": replay.CurrentSchemaVersion,
		"recordedAt":    recordedAt.UTC().Format(time.RFC3339),
		"events": []any{
			map[string]any{
				"id":            "factory-event/run-started",
				"schemaVersion": string(factoryapi.AgentFactoryEventV1),
				"type":          string(factoryapi.FactoryEventTypeRunRequest),
				"context": map[string]any{
					"eventTime": recordedAt.UTC().Format(time.RFC3339),
					"sequence":  0,
					"tick":      0,
				},
				"payload": map[string]any{
					"recordedAt": recordedAt.UTC().Format(time.RFC3339),
					"factory":    json.RawMessage(factoryJSON),
				},
			},
		},
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("marshal replay artifact: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write replay artifact: %v", err)
	}
}

func stringValueFromFunctionalPtr[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

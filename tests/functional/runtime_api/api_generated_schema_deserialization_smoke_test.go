package runtime_api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestGeneratedSchemaDeserializationSmoke_FileHTTPAndReplayTransportsStayAligned(t *testing.T) {
	support.SkipLongFunctional(t, "slow generated-schema transport-alignment sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recordDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "generated-schema-deserialization.replay.json")

	fileBoundary := flattenGeneratedSchemaFileBoundary(t, dir)
	assertGeneratedSmokeTopologyBoundary(t, fileBoundary)
	fileTransportSummary := generatedSchemaTransportSummaryFromFactory(t, fileBoundary)
	httpBoundary := generatedSchemaFactoryFromHTTPBoundary(t, dir)
	recordedBoundary, replayBoundary := generatedSchemaFactoriesFromRecordedReplay(t, recordDir, artifactPath)
	httpTransportSummary := generatedSchemaTransportSummaryFromFactory(t, httpBoundary)
	recordedTransportSummary := generatedSchemaTransportSummaryFromFactory(t, recordedBoundary)
	replayTransportSummary := generatedSchemaTransportSummaryFromFactory(t, replayBoundary)

	if !reflect.DeepEqual(httpTransportSummary, fileTransportSummary) {
		t.Fatalf("HTTP initial structure transport summary mismatch\nhttp: %#v\nfile: %#v", httpTransportSummary, fileTransportSummary)
	}
	if !reflect.DeepEqual(recordedTransportSummary, fileTransportSummary) {
		t.Fatalf("recorded run-request transport summary mismatch\nrecorded: %#v\nfile:     %#v", recordedTransportSummary, fileTransportSummary)
	}
	if !reflect.DeepEqual(replayTransportSummary, httpTransportSummary) {
		t.Fatalf("replayed and live HTTP transport summaries diverged\nreplay: %#v\nhttp:   %#v", replayTransportSummary, httpTransportSummary)
	}
	if !reflect.DeepEqual(replayTransportSummary, recordedTransportSummary) {
		t.Fatalf("replayed and recorded transport summaries diverged\nreplay:   %#v\nrecorded: %#v", replayTransportSummary, recordedTransportSummary)
	}
}

func flattenGeneratedSchemaFileBoundary(t *testing.T, dir string) factoryapi.Factory {
	t.Helper()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "flatten", dir,
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(factory config flatten) error = %v; stderr=%q", err, inputs.Stderr())
	}
	fileBoundary, err := factorymapping.GeneratedFactoryFromOpenAPIJSON([]byte(inputs.Stdout()))
	if err != nil {
		t.Fatalf("decode flattened Factory output: %v", err)
	}
	return fileBoundary
}

func generatedSchemaTransportSummaryFromFactory(
	t *testing.T,
	factory factoryapi.Factory,
) generatedSchemaTransportSummary {
	t.Helper()

	return generatedSchemaTransportSummary{
		workers: []generatedSchemaWorkerSummary{
			requireGeneratedSchemaWorkerSummary(t, generatedSchemaWorkers(factory), "worker-a"),
			requireGeneratedSchemaWorkerSummary(t, generatedSchemaWorkers(factory), "worker-b"),
		},
		workstations: []generatedSchemaTransportWorkstationSummary{
			requireGeneratedSchemaTransportWorkstationSummary(t, generatedSchemaWorkstations(factory), "step-one"),
			requireGeneratedSchemaTransportWorkstationSummary(t, generatedSchemaWorkstations(factory), "step-two"),
		},
	}
}

func generatedSchemaWorkers(factory factoryapi.Factory) []factoryapi.Worker {
	if factory.Workers == nil {
		return nil
	}
	return *factory.Workers
}

func generatedSchemaWorkstations(factory factoryapi.Factory) []factoryapi.Workstation {
	if factory.Workstations == nil {
		return nil
	}
	return *factory.Workstations
}

func generatedSchemaFactoryFromHTTPBoundary(t *testing.T, dir string) factoryapi.Factory {
	t.Helper()

	server := startFunctionalServer(t, dir, false)
	defer server.Stop(t)
	stream := openDefaultSessionFactoryEventHTTPStream(t, server.URL())
	_, first := requireFunctionalEventStreamPrelude(t, stream)
	initialStructurePayload, err := first.Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("decode initial-structure payload: %v", err)
	}
	assertGeneratedSmokeTransportBoundary(t, initialStructurePayload.Factory)
	stream.close()
	return initialStructurePayload.Factory
}

func generatedSchemaFactoriesFromRecordedReplay(
	t *testing.T,
	dir string,
	artifactPath string,
) (factoryapi.Factory, factoryapi.Factory) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "generated-schema-runtime-work",
		TraceID:    "generated-schema-runtime-trace",
		Payload:    []byte(`{"title":"generated schema deserialization smoke"}`),
	})
	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	runStarted := requireGeneratedSchemaRunStartedPayload(t, testutil.GeneratedFactoryEvents(t, artifact.Events))
	assertGeneratedSmokeTransportBoundary(t, runStarted.Factory)
	assertGeneratedSmokeRuntimeDefinitions(t, runStarted.Factory)

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	stream := openDefaultSessionFactoryEventHTTPStream(t, replayServer.URL())
	_, first := requireFunctionalEventStreamPrelude(t, stream)
	initial, err := first.Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("decode replay initial-structure payload: %v", err)
	}
	stream.close()
	replayServer.Stop(t)
	return runStarted.Factory, initial.Factory
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

	fileInputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--no-record",
	})
	fileErr := support.BuildProcess(t, serviceedges.Edges{}).Execute(fileInputs.Input)
	assertGeneratedSchemaBoundaryFailure(t, fileErr)

	artifactPath := filepath.Join(t.TempDir(), "retired-generated-schema-boundary.replay.json")
	writeGeneratedSchemaReplayArtifact(t, artifactPath, factoryJSON)
	replayInputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", t.TempDir(),
		"--replay", artifactPath,
		"--no-record",
	})
	replayErr := support.BuildProcess(t, serviceedges.Edges{}).Execute(replayInputs.Input)
	assertGeneratedSchemaBoundaryFailure(t, replayErr)
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
	assertGeneratedSmokeSerializedWorkstationBoundary(t, generated, false)
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
		if stringValueFromFunctionalPtr(worker.Type) != interfaces.WorkerTypeAgent {
			t.Fatalf("runtime boundary worker %q type = %q, want %q", worker.Name, stringValueFromFunctionalPtr(worker.Type), interfaces.WorkerTypeAgent)
		}
	}
	if generated.Workstations == nil {
		t.Fatal("runtime boundary workstations = nil")
	}
	for _, workstation := range *generated.Workstations {
		if workstation.Name != "step-one" && workstation.Name != "step-two" {
			continue
		}
		if stringValueFromFunctionalPtr(workstation.Type) != interfaces.WorkstationTypeAgent {
			t.Fatalf("runtime boundary workstation %q type = %q, want %q", workstation.Name, stringValueFromFunctionalPtr(workstation.Type), interfaces.WorkstationTypeAgent)
		}
	}
	assertGeneratedSmokeSerializedWorkstationBoundary(t, generated, false)
}

func assertGeneratedSmokeRuntimeDefinitions(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	if generated.Workstations == nil {
		t.Fatal("runtime boundary workstations = nil")
	}
	assertGeneratedSmokeSerializedWorkstationBoundary(t, generated, true)
	for _, workstation := range *generated.Workstations {
		if workstation.Name != "step-one" && workstation.Name != "step-two" {
			continue
		}
		if !strings.Contains(stringValueFromFunctionalPtr(workstation.Body), "Do the work.") {
			t.Fatalf("runtime boundary workstation %q body = %q, want split runtime prompt", workstation.Name, stringValueFromFunctionalPtr(workstation.Body))
		}
	}
}

func assertGeneratedSmokeSerializedWorkstationBoundary(t *testing.T, generated factoryapi.Factory, requireBody bool) {
	t.Helper()

	data, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory boundary: %v", err)
	}
	var serialized struct {
		Workstations []map[string]any `json:"workstations"`
	}
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatalf("unmarshal generated factory boundary JSON: %v", err)
	}
	if len(serialized.Workstations) == 0 {
		t.Fatalf("serialized generated factory boundary missing workstations: %s", string(data))
	}
	for _, workstation := range serialized.Workstations {
		name, _ := workstation["name"].(string)
		if name != "step-one" && name != "step-two" {
			continue
		}
		if _, ok := workstation["promptTemplate"]; ok {
			t.Fatalf("expected serialized generated workstation %q to omit promptTemplate, got %#v", name, workstation)
		}
		if !requireBody {
			continue
		}
		body, ok := workstation["body"].(string)
		if !ok || !strings.Contains(body, "Do the work.") {
			t.Fatalf("expected serialized generated workstation %q body to preserve canonical prompt content, got %#v", name, workstation)
		}
	}
}

func requireGeneratedSchemaWorkerSummary(
	t *testing.T,
	workers []factoryapi.Worker,
	name string,
) generatedSchemaWorkerSummary {
	t.Helper()

	for _, worker := range workers {
		if worker.Name != name {
			continue
		}
		return generatedSchemaWorkerSummary{
			name:       worker.Name,
			workerType: stringValueFromFunctionalPtr(worker.Type),
			model:      stringValueFromFunctionalPtr(worker.Model),
		}
	}
	t.Fatalf("public Factory workers = %#v, missing %q", workers, name)
	return generatedSchemaWorkerSummary{}
}

func requireGeneratedSchemaTransportWorkstationSummary(
	t *testing.T,
	workstations []factoryapi.Workstation,
	name string,
) generatedSchemaTransportWorkstationSummary {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Name != name {
			continue
		}
		return generatedSchemaTransportWorkstationSummary{
			name:            workstation.Name,
			workerTypeName:  workstation.Worker,
			workstationType: stringValueFromFunctionalPtr(workstation.Type),
		}
	}
	t.Fatalf("public Factory workstations = %#v, missing %q", workstations, name)
	return generatedSchemaTransportWorkstationSummary{}
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
		"schemaVersion": interfaces.ReplayV1SourceFormat,
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

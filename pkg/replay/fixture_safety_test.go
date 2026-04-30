package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/internal/testpath"
)

func TestCheckedInReplayEventFixturesUseSafeFactoryEvents(t *testing.T) {
	for _, fixture := range replayFixturePaths(t) {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			assertFixtureUsesGeneratedFactoryConfig(t, fixture)
			assertFixtureUsesThinEventContract(t, fixture)
			assertFixtureOmitsUnsafeEventDiagnostics(t, fixture)
		})
	}
}

func TestCheckedInReplayInferenceFixtureDocumentsPromptExposure(t *testing.T) {
	fixture := filepath.FromSlash("testdata/inference-events.replay.json")
	artifact := loadRawReplayFixture(t, fixture)
	requests := make(map[string]map[string]any)
	responses := make(map[string]map[string]any)
	for _, event := range artifact.Events {
		payload, _ := event["payload"].(map[string]any)
		requestID, _ := payload["inferenceRequestId"].(string)
		switch event["type"] {
		case "INFERENCE_REQUEST":
			prompt, _ := payload["prompt"].(string)
			if prompt == "" {
				t.Fatalf("%s inference request %q must include the intentionally exposed prompt", fixture, requestID)
			}
			requests[requestID] = payload
		case "INFERENCE_RESPONSE":
			responses[requestID] = payload
		}
	}
	if len(requests) == 0 {
		t.Fatalf("%s has no INFERENCE_REQUEST events", fixture)
	}
	for requestID := range requests {
		if _, ok := responses[requestID]; !ok {
			t.Fatalf("%s inference request %q has no matching INFERENCE_RESPONSE", fixture, requestID)
		}
	}
}

func assertFixtureUsesGeneratedFactoryConfig(t *testing.T, fixture string) {
	t.Helper()

	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read replay fixture %s: %v", fixture, err)
	}
	for _, legacyKey := range forbiddenReplayConfigKeys() {
		if strings.Contains(string(data), legacyKey) {
			t.Fatalf("%s must not contain legacy config key %q", fixture, legacyKey)
		}
	}
	var artifact struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("parse replay fixture %s: %v", fixture, err)
	}
	for index, event := range artifact.Events {
		if event["type"] != "RUN_REQUEST" {
			continue
		}
		payload, _ := event["payload"].(map[string]any)
		factory, _ := payload["factory"].(map[string]any)
		if len(factory) == 0 {
			t.Fatalf("%s events[%d].payload.factory must contain generated Factory config", fixture, index)
		}
		return
	}
	t.Fatalf("%s must contain a RUN_REQUEST event", fixture)
}

func forbiddenReplayConfigKeys() []string {
	return []string{
		strings.Join([]string{"effective", "Config"}, ""),
		strings.Join([]string{"__replay", "Effective", "Config"}, ""),
		strings.Join([]string{"runtime", "Worker", "Config"}, ""),
	}
}

func replayFixturePaths(t *testing.T) []string {
	t.Helper()

	return existingFiles([]string{
		filepath.FromSlash("testdata/inference-events.replay.json"),
		testpath.MustRepoPathFromCaller(t, 0, "tests", "adhoc", "factory-recording-04-11-02.json"),
		testpath.MustRepoPathFromCaller(t, 0, "tests", "functional_test", "testdata", "adhoc-recording-batch-event-log.json"),
	})
}

func assertFixtureUsesThinEventContract(t *testing.T, fixture string) {
	t.Helper()

	artifact := loadRawReplayFixture(t, fixture)
	if len(artifact.Events) == 0 {
		t.Fatalf("replay fixture %s has no events", fixture)
	}
	for index, event := range artifact.Events {
		payload, _ := event["payload"].(map[string]any)
		if len(payload) == 0 {
			continue
		}
		switch event["type"] {
		case "DISPATCH_REQUEST":
			assertFixtureKeysAbsent(t, fixture, index, payload, "payload", "dispatchId", "worker", "workstation")
			if metadata, ok := payload["metadata"].(map[string]any); ok {
				assertFixtureKeysAbsent(t, fixture, index, metadata, "payload.metadata", "requestId")
			}
			assertDispatchRequestInputsUseCanonicalWorkIDKey(t, fixture, index, payload)
		case "INFERENCE_REQUEST", "INFERENCE_RESPONSE":
			assertFixtureKeysAbsent(t, fixture, index, payload, "payload", "dispatchId", "transitionId")
		case "DISPATCH_RESPONSE":
			assertFixtureKeysAbsent(t, fixture, index, payload, "payload", "dispatchId", "worker", "workstation", "providerSession", "diagnostics", "inputs")
		}
	}
}

func assertDispatchRequestInputsUseCanonicalWorkIDKey(t *testing.T, fixture string, eventIndex int, payload map[string]any) {
	t.Helper()

	inputs, ok := payload["inputs"].([]any)
	if !ok {
		return
	}
	for inputIndex, raw := range inputs {
		ref, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := ref["work_id"]; exists {
			t.Fatalf("%s events[%d].payload.inputs[%d].work_id must not be present", fixture, eventIndex, inputIndex)
		}
	}
}

func assertFixtureOmitsUnsafeEventDiagnostics(t *testing.T, fixture string) {
	t.Helper()

	artifact := loadRawReplayFixture(t, fixture)
	if len(artifact.Events) == 0 {
		t.Fatalf("replay fixture %s has no events", fixture)
	}
	for index, event := range artifact.Events {
		if event["type"] != "DISPATCH_RESPONSE" {
			continue
		}
		payload, _ := event["payload"].(map[string]any)
		diagnostics, _ := payload["diagnostics"].(map[string]any)
		assertNoUnsafeDiagnosticKeys(t, fixture, index, diagnostics, "payload.diagnostics")
	}
}

type rawReplayFixture struct {
	Events []map[string]any `json:"events"`
}

func loadRawReplayFixture(t *testing.T, fixture string) rawReplayFixture {
	t.Helper()

	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read replay fixture %s: %v", fixture, err)
	}
	var artifact rawReplayFixture
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("parse replay fixture %s: %v", fixture, err)
	}
	return artifact
}

func assertNoUnsafeDiagnosticKeys(t *testing.T, fixture string, eventIndex int, value any, path string) {
	t.Helper()

	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, child := range object {
		switch key {
		case "command", "stdin", "env":
			t.Fatalf("%s events[%d].%s.%s must not appear in FactoryEvent diagnostics", fixture, eventIndex, path, key)
		}
		assertNoUnsafeDiagnosticKeys(t, fixture, eventIndex, child, path+"."+key)
	}
}

func assertFixtureKeysAbsent(t *testing.T, fixture string, eventIndex int, object map[string]any, path string, keys ...string) {
	t.Helper()

	for _, key := range keys {
		if _, ok := object[key]; ok {
			t.Fatalf("%s events[%d].%s.%s must not be present", fixture, eventIndex, path, key)
		}
	}
}

func existingFiles(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}

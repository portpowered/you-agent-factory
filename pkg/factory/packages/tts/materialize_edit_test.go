package tts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestEditedMaterializedPackagedTTSFactoryChangesInvocationBackendMetadata(t *testing.T) {
	const editedCommand = "customer-tts-command"
	factoryDir := materializePackagedTTSFactory(t, t.TempDir())
	initialWorker := loadPackagedTTSWorker(t, factoryDir)
	initialBackend := metadataBackendForWorker(t, sampleAudioWorkerOutput, initialWorker)
	if initialBackend != "OMNIVOICE_Q4_K_M/LLAMACPP" {
		t.Fatalf("initial backend = %q, want default packaged label", initialBackend)
	}

	editMaterializedWorkerCommand(t, factoryDir, "tts-executor", editedCommand)
	editedWorker := loadPackagedTTSWorker(t, factoryDir)
	if editedWorker.Command != editedCommand {
		t.Fatalf("edited worker command = %q, want %q", editedWorker.Command, editedCommand)
	}

	editedBackend := metadataBackendForWorker(t, sampleAudioWorkerOutput, editedWorker)
	if editedBackend == initialBackend {
		t.Fatalf("edited backend = %q, want change from %q after on-disk factory edit", editedBackend, initialBackend)
	}
	if editedBackend != "OMNIVOICE_Q4_K_M/"+editedCommand {
		t.Fatalf("edited backend = %q, want OMNIVOICE_Q4_K_M/%s", editedBackend, editedCommand)
	}
}

const sampleAudioWorkerOutput = `[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`

func materializePackagedTTSFactory(t *testing.T, globalRoot string) string {
	t.Helper()
	factoryDir, err := factoryconfig.PersistNamedFactory(globalRoot, "@you/tts", BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	return factoryDir
}

func loadPackagedTTSWorker(t *testing.T, factoryDir string) *workerconfig.Config {
	t.Helper()
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", factoryDir, err)
	}
	worker, ok := loaded.Worker("tts-executor")
	if !ok {
		t.Fatal("expected materialized tts-executor worker")
	}
	return worker
}

func editMaterializedWorkerCommand(t *testing.T, factoryDir, workerName, command string) {
	t.Helper()
	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var factoryDoc map[string]any
	if err := json.Unmarshal(factoryJSON, &factoryDoc); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	if !setWorkerCommand(factoryDoc, workerName, command) {
		t.Fatalf("expected factory.json %q worker command field to be editable", workerName)
	}
	editedJSON, err := json.MarshalIndent(factoryDoc, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(edited factory.json): %v", err)
	}
	if string(editedJSON) == string(factoryJSON) {
		t.Fatal("expected edited factory.json to differ from initial materialized content")
	}
	if err := os.WriteFile(factoryJSONPath, editedJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(edited factory.json): %v", err)
	}
}

func setWorkerCommand(factoryDoc map[string]any, workerName, command string) bool {
	workers, ok := factoryDoc["workers"].([]any)
	if !ok {
		return false
	}
	for _, worker := range workers {
		workerDoc, ok := worker.(map[string]any)
		if !ok || workerDoc["name"] != workerName {
			continue
		}
		workerDoc["command"] = command
		return true
	}
	return false
}

func metadataBackendForWorker(t *testing.T, output string, worker *workerconfig.Config) string {
	t.Helper()
	content, err := MetadataContentFromWorkerOutput(output, "trace-edit", "session-edit", BackendLabelFromWorker(worker))
	if err != nil {
		t.Fatalf("MetadataContentFromWorkerOutput: %v", err)
	}
	return metadataBackend(t, content)
}

func metadataBackend(t *testing.T, content []work.WorkContentPart) string {
	t.Helper()
	if len(content) != 1 {
		t.Fatalf("metadata content = %#v, want one text part", content)
	}
	var metadata InvocationMetadata
	if err := json.Unmarshal([]byte(content[0].Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	return metadata.Backend
}

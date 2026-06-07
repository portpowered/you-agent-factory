package tts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestEditedMaterializedPackagedTTSFactoryChangesInvocationBackendMetadata(t *testing.T) {
	globalRoot := t.TempDir()

	factoryDir, err := factoryconfig.PersistNamedFactory(globalRoot, "@you/tts", factoryconfig.BuiltInTTSFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(initial): %v", err)
	}
	initialWorker, ok := loaded.Worker("tts-executor")
	if !ok {
		t.Fatal("expected materialized tts-executor worker")
	}

	output := `[{"type":"AUDIO","file":"/tmp/speech.wav","contentType":"audio/wav","slot":"audio"}]`
	initialContent, err := MetadataContentFromWorkerOutput(output, "trace-edit", "session-edit", BackendLabelFromWorker(initialWorker))
	if err != nil {
		t.Fatalf("MetadataContentFromWorkerOutput(initial): %v", err)
	}
	initialBackend := metadataBackend(t, initialContent)
	if initialBackend != "OMNIVOICE_Q4_K_M/LLAMACPP" {
		t.Fatalf("initial backend = %q, want default packaged label", initialBackend)
	}

	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	editedJSON := strings.ReplaceAll(string(factoryJSON), `"model": "OMNIVOICE_Q4_K_M"`, `"model": "CUSTOMER_EDITED_TTS_MODEL"`)
	if editedJSON == string(factoryJSON) {
		t.Fatal("expected factory.json worker model field to be editable")
	}
	if err := os.WriteFile(factoryJSONPath, []byte(editedJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(edited factory.json): %v", err)
	}

	reloaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(edited): %v", err)
	}
	editedWorker, ok := reloaded.Worker("tts-executor")
	if !ok {
		t.Fatal("expected edited tts-executor worker")
	}
	if editedWorker.Model != "CUSTOMER_EDITED_TTS_MODEL" {
		t.Fatalf("edited worker model = %q, want CUSTOMER_EDITED_TTS_MODEL", editedWorker.Model)
	}

	editedContent, err := MetadataContentFromWorkerOutput(output, "trace-edit", "session-edit", BackendLabelFromWorker(editedWorker))
	if err != nil {
		t.Fatalf("MetadataContentFromWorkerOutput(edited): %v", err)
	}
	editedBackend := metadataBackend(t, editedContent)
	if editedBackend == initialBackend {
		t.Fatalf("edited backend = %q, want change from %q after on-disk factory edit", editedBackend, initialBackend)
	}
	if editedBackend != "CUSTOMER_EDITED_TTS_MODEL/LLAMACPP" {
		t.Fatalf("edited backend = %q, want CUSTOMER_EDITED_TTS_MODEL/LLAMACPP", editedBackend)
	}
}

func metadataBackend(t *testing.T, content []interfaces.WorkContentPart) string {
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

package root_composition_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertTTSRoleBundleReads(t *testing.T, story ttsStory) {
	t.Helper()
	paths := story.assetTrace.snapshot()
	observed := make([]string, 0, 3)
	for _, role := range []string{
		"vibevoice-realtime-0.5B-q8_0.gguf",
		"tokenizer.gguf",
		"voice-en-Carter_man.gguf",
	} {
		want := "open:" + filepath.Join(story.home, "tts-role-bundle", role)
		found := false
		for _, path := range paths {
			if path == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("TTS role %q was not observed in controlled asset reads; reads=%v", role, paths)
		}
		observed = append(observed, role)
	}
	t.Logf("controlled TTS role bundle source files opened: %v", observed)
}

func runExactChainTTS(t *testing.T, story ttsStory, phrase string) ([]byte, string) {
	t.Helper()
	ttsPath := filepath.Join(story.dir, "exact-chain.wav")
	var stdout, stderr bytes.Buffer
	inputs := functionalExactChainInputs(t, story, []string{
		"you", "models", "invoke", models.BuiltInModelNameTTS, "--operation", "TTS", "--text", phrase, "--output", ttsPath,
	})
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := story.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(exact-chain TTS) error = %v", err)
	}
	audio, err := os.ReadFile(ttsPath)
	if err != nil {
		t.Fatalf("read exact-chain TTS output: %v", err)
	}
	if want := story.protocol.audioFor(phrase); !bytes.Equal(audio, want) {
		t.Fatalf("exact-chain TTS bytes = %d/%s, want fixture bytes %d/%s", len(audio), ttsDigest(audio), len(want), ttsDigest(want))
	}
	assertSemanticTTSAudio(t, audio, "exact-chain TTS output")
	if stdout.String() != "Wrote audio: "+ttsPath+"\n" || stderr.Len() != 0 {
		t.Fatalf("exact-chain TTS streams = stdout %q stderr %q, want cache-hit status only", stdout.String(), stderr.String())
	}
	return audio, ttsPath
}

func functionalExactChainInputs(t *testing.T, story ttsStory, args []string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = story.environment
	inputs.Input.WorkingDirectory = story.dir
	return inputs
}

func writeGenericBuiltinTTSCache(t *testing.T, home string) {
	t.Helper()
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameTTS)
	if !ok {
		t.Fatal("built-in catalog did not publish the TTS model definition")
	}
	writeGenericBuiltinModelCache(t, home, definition.Source)
}

func writeGenericBuiltinTTSManagedRuntimeCache(t *testing.T, home string) {
	t.Helper()
	const revision = "a67807e65e3002e187179a856e96043f75060bc9"
	files := []struct {
		path string
		body []byte
	}{
		{path: "vibevoice-realtime-0.5B-q8_0.gguf", body: []byte("joined built-in tts model fixture")},
		{path: "tokenizer.gguf", body: []byte("joined built-in tts tokenizer fixture")},
		{path: "voice-en-Carter_man.gguf", body: []byte("joined built-in tts voice fixture")},
	}
	canonicalModelName := strings.ToUpper(strings.TrimSpace(models.BuiltInModelNameTTS))
	revisionPath := filepath.Join(home, ".agent-factory", "models", canonicalModelName, revision)
	if err := os.MkdirAll(revisionPath, 0o755); err != nil {
		t.Fatalf("create managed TTS runtime fixture: %v", err)
	}
	metadataFiles := make([]map[string]any, 0, len(files))
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(revisionPath, file.path), file.body, 0o644); err != nil {
			t.Fatalf("write managed TTS runtime fixture %q: %v", file.path, err)
		}
		digest := sha256.Sum256(file.body)
		metadataFiles = append(metadataFiles, map[string]any{
			"path": file.path, "bytes": len(file.body), "sha256": hex.EncodeToString(digest[:]),
		})
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": "tts",
		"revision":  revision,
		"files":     metadataFiles,
	})
	if err != nil {
		t.Fatalf("marshal managed TTS runtime metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(revisionPath), ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write managed TTS runtime metadata: %v", err)
	}
}

func writeControlledBuiltinTTSSource(t *testing.T, home string) {
	t.Helper()
	bundle := filepath.Join(home, "tts-role-bundle")
	artifacts := map[string][]byte{
		"vibevoice-realtime-0.5B-q8_0.gguf": []byte("controlled-vibevoice-model"),
		"tokenizer.gguf":                    []byte("controlled-vibevoice-tokenizer"),
		"voice-en-Carter_man.gguf":          []byte("controlled-vibevoice-voice"),
	}
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatalf("create controlled TTS role bundle: %v", err)
	}
	for name, body := range artifacts {
		if err := os.WriteFile(filepath.Join(bundle, name), body, 0o644); err != nil {
			t.Fatalf("write controlled TTS role %q: %v", name, err)
		}
	}
	source := (&url.URL{Scheme: "file", Path: filepath.ToSlash(bundle)}).String()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create controlled TTS operator config directory: %v", err)
	}
	config, err := json.Marshal(map[string]any{
		"models": map[string]any{
			"tts": map[string]any{"source": source},
		},
	})
	if err != nil {
		t.Fatalf("marshal controlled TTS operator config: %v", err)
	}
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("write controlled TTS operator config: %v", err)
	}
}

func pinnedTTSBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		Bytes:    22,
		SHA256:   "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
}

func writeGenericBuiltinTTSBackendCache(t *testing.T, home string) {
	t.Helper()
	selection := pinnedTTSBackendSelection()
	writeGenericBackendCache(t, home, "localai-vibevoice", selection, []byte("pinned-backend-fixture"))
}

func writeGenericBackendCache(
	t *testing.T,
	home, backend string,
	selection serviceedges.ModelBackendArtifactSelection,
	body []byte,
) {
	t.Helper()
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://" + backend + "/release://" + urlHash
	digest := selection.SHA256
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create generic backend snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), body, 0o644); err != nil {
		t.Fatalf("write generic backend snapshot: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{
			"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256,
		}},
	})
	if err != nil {
		t.Fatalf("marshal generic backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o644); err != nil {
		t.Fatalf("write generic backend metadata: %v", err)
	}
}

func wavDurationMilliseconds(audio []byte) float64 {
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	return float64(dataSize/uint32(blockAlign)) * 1000 / float64(sampleRate)
}

package wire

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

func TestAppendManagedBackendEnvironmentPreservesExplicitValuesAndReplacesKeys(t *testing.T) {
	t.Parallel()

	base := []string{
		"PATH=C:\\runtime",
		"VIBEVOICECPP_LIBRARY=C:\\stale\\library.dll",
		"MODEL=tts",
	}
	got := appendManagedBackendEnvironment(base, []string{
		"vibevoicecpp_library=C:\\managed\\library.dll",
		"MODEL_ROOT=C:\\models",
	})
	if len(got) != len(base)+1 {
		t.Fatalf("merged environment length = %d, want %d: %#v", len(got), len(base)+1, got)
	}
	if got[0] != base[0] || got[2] != base[2] {
		t.Fatalf("merged environment changed unrelated values: %#v", got)
	}
	if got[1] != "vibevoicecpp_library=C:\\managed\\library.dll" {
		t.Fatalf("merged environment did not replace case-insensitive library key: %#v", got)
	}
	if got[3] != "MODEL_ROOT=C:\\models" {
		t.Fatalf("merged environment omitted new value: %#v", got)
	}
}

func TestProvideModelRuntimeEvidenceRecorderIsOptionalAndOwnerOnlyJSONL(t *testing.T) {
	t.Setenv(modelRuntimeEvidenceEnvironment, "")
	if recorder, err := provideModelRuntimeEvidenceRecorder(); err != nil || recorder != nil {
		t.Fatalf("absent runtime evidence recorder = (%v, %v), want (nil, nil)", recorder, err)
	}

	path := filepath.Join(t.TempDir(), "runtime.jsonl")
	t.Setenv(modelRuntimeEvidenceEnvironment, path)
	recorder, err := provideModelRuntimeEvidenceRecorder()
	if err != nil {
		t.Fatalf("provide runtime evidence recorder: %v", err)
	}
	if recorder == nil {
		t.Fatal("configured runtime evidence recorder is nil")
	}
	recorder = modelswire.NewOrderedRuntimeEvidenceRecorder(recorder)
	recorder.RecordRuntimeEvidence(modelswire.RuntimeEvidenceRecord{
		Kind:           "STAGE",
		Stage:          "PROTOCOL_LOAD",
		Outcome:        "FAILED",
		Class:          "PROTOCOL_INCOMPATIBLE",
		CauseSHA256:    strings.Repeat("a", 64),
		DurationMillis: 7,
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat runtime evidence file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		got := info.Mode().Perm()
		t.Fatalf("runtime evidence permissions = %o, want owner-only 600", got)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime evidence file: %v", err)
	}
	if bytes.Count(body, []byte{'\n'}) != 1 {
		t.Fatalf("runtime evidence lines = %d, want one JSONL record", bytes.Count(body, []byte{'\n'}))
	}
	var got modelswire.RuntimeEvidenceRecord
	if err := json.Unmarshal(bytes.TrimSpace(body), &got); err != nil {
		t.Fatalf("decode runtime evidence record: %v", err)
	}
	if got.Sequence != 1 || got.Kind != "STAGE" || got.Stage != "PROTOCOL_LOAD" ||
		got.Outcome != "FAILED" || got.Class != "PROTOCOL_INCOMPATIBLE" {
		t.Fatalf("runtime evidence record = %#v, want ordered bounded record", got)
	}
}

func TestProvideModelRuntimeEvidenceRecorderRejectsRelativePath(t *testing.T) {
	t.Setenv(modelRuntimeEvidenceEnvironment, "runtime.jsonl")
	if recorder, err := provideModelRuntimeEvidenceRecorder(); recorder != nil || err == nil {
		t.Fatalf("relative runtime evidence path = (%v, %v), want error and nil", recorder, err)
	}
}

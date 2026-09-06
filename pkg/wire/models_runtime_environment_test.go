package wire

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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

func TestAppendManagedBackendEnvironmentCollapsesCaseInsensitiveDuplicates(t *testing.T) {
	t.Parallel()

	got := appendManagedBackendEnvironment(
		[]string{
			"PATH=C:\\runtime",
			"VIBEVOICECPP_LIBRARY=C:\\stale\\first.dll",
			"vibevoicecpp_library=C:\\stale\\second.dll",
			"TEMP=C:\\temp",
		},
		[]string{"VibeVoiceCpp_Library=C:\\managed\\library.dll"},
	)
	var libraries []string
	for _, entry := range got {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "VIBEVOICECPP_LIBRARY") {
			libraries = append(libraries, value)
		}
	}
	if len(libraries) != 1 || libraries[0] != `C:\managed\library.dll` {
		t.Fatalf("merged VibeVoice values = %#v, want one managed value", libraries)
	}
	if !containsEnvironmentValue(got, "PATH", `C:\runtime`) ||
		!containsEnvironmentValue(got, "TEMP", `C:\temp`) {
		t.Fatalf("merged environment dropped unrelated values: %#v", got)
	}
}

func TestManagedEnvironmentFactsUseOnlyAllowlistedValueDigests(t *testing.T) {
	t.Parallel()

	secretPath := `C:\isolated\private-model.gguf`
	secretToken := "token=private-value"
	managedPath := `C:\managed\libgovibevoicecpp.dll`
	facts := managedEnvironmentFacts([]string{
		"PATH=C:\\runtime",
		"TEMP=C:\\temp",
		"TMP=C:\\temp",
		"VIBEVOICECPP_LIBRARY=" + managedPath,
		"MODEL_SECRET=" + secretToken,
		"MODEL_PATH=" + secretPath,
	})
	body, err := json.Marshal(facts)
	if err != nil {
		t.Fatalf("marshal managed environment facts: %v", err)
	}
	serialized := string(body)
	for _, marker := range []string{secretPath, secretToken, "MODEL_SECRET", "MODEL_PATH"} {
		if strings.Contains(serialized, marker) {
			t.Fatalf("managed environment facts leaked %q: %s", marker, serialized)
		}
	}
	want := map[string]string{
		"PATH":                 environmentValueSHA256(`C:\runtime`),
		"TEMP":                 environmentValueSHA256(`C:\temp`),
		"TMP":                  environmentValueSHA256(`C:\temp`),
		"VIBEVOICECPP_LIBRARY": environmentValueSHA256(managedPath),
	}
	if len(facts) != len(want) {
		t.Fatalf("managed environment facts = %#v, want four allowlisted facts", facts)
	}
	for _, fact := range facts {
		if !fact.Present || fact.ValueSHA256 != want[fact.Name] {
			t.Fatalf("managed environment fact = %#v, want digest for %q", fact, fact.Name)
		}
	}
}

func TestManagedChildEvidenceUsesBoundedIdentityAndSharedSequence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: path}
	recorder.RecordRuntimeEvidence(modelswire.RuntimeEvidenceRecord{
		Kind:           "STAGE",
		Stage:          "PROTOCOL_LOAD",
		Outcome:        "COMPLETED",
		DurationMillis: 1,
	})
	recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
		Kind:      managedChildEvidenceKind,
		Backend:   boundedManagedBackendID(`C:\private\backend.exe`),
		ProcessID: 42,
		Phase:     managedChildPhaseStarted,
	})
	recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
		Kind:      managedChildEvidenceKind,
		Backend:   "localai-vibevoice",
		ProcessID: 42,
		Phase:     managedChildPhaseExited,
		ExitClass: managedChildExitClassNonzero,
	})

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared runtime evidence: %v", err)
	}
	var records []struct {
		Sequence uint64 `json:"sequence"`
		Kind     string `json:"kind"`
		Backend  string `json:"backend"`
		Phase    string `json:"phase"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		var record struct {
			Sequence uint64 `json:"sequence"`
			Kind     string `json:"kind"`
			Backend  string `json:"backend"`
			Phase    string `json:"phase"`
		}
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode shared runtime evidence: %v", err)
		}
		records = append(records, record)
	}
	if len(records) != 3 || records[0].Sequence != 1 || records[1].Sequence != 2 || records[2].Sequence != 3 {
		t.Fatalf("shared runtime evidence records = %#v, want three ordered lines", records)
	}
	if records[1].Backend != "UNKNOWN" {
		t.Fatalf("unbounded backend identity = %q, want UNKNOWN", records[1].Backend)
	}
	if strings.Contains(string(body), `C:\private\backend.exe`) {
		t.Fatalf("shared runtime evidence leaked raw backend identity: %s", body)
	}
}

func TestModelsProcessLauncherStartFailureDoesNotEmitChildEvidence(t *testing.T) {
	t.Parallel()

	evidencePath := filepath.Join(t.TempDir(), "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: evidencePath}
	missingCommand := filepath.Join(t.TempDir(), "missing-model-backend.exe")
	_, err := (modelsProcessLauncher{recorder: recorder}).Start(
		context.Background(),
		serviceedges.HostProcessStartSpec{
			Command:        missingCommand,
			Backend:        "localai-vibevoice",
			HealthEndpoint: "grpc://127.0.0.1:1",
		},
	)
	if err == nil {
		t.Fatal("missing managed backend start error = nil, want typed start failure")
	}
	var classifier interface {
		ModelRuntimeStage() string
		ModelRuntimeFailureClass() string
	}
	if !errors.As(err, &classifier) || classifier == nil ||
		classifier.ModelRuntimeStage() != "BACKEND_START" ||
		classifier.ModelRuntimeFailureClass() != "PROCESS_START_FAILED" {
		t.Fatalf("start failure classification = %v, want BACKEND_START/PROCESS_START_FAILED", err)
	}
	if strings.Contains(err.Error(), missingCommand) {
		t.Fatalf("start failure leaked command path: %q", err.Error())
	}
	body, readErr := os.ReadFile(evidencePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read start failure evidence: %v", readErr)
	}
	if len(bytes.TrimSpace(body)) != 0 {
		t.Fatalf("start failure emitted false child evidence: %s", body)
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

func containsEnvironmentValue(environment []string, name, want string) bool {
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, name) && value == want {
			return true
		}
	}
	return false
}

func environmentValueSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

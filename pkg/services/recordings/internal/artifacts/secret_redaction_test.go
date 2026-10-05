package artifacts

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestBuildPortableRecordingRedactsDeclaredResultBeforeReturning(t *testing.T) {
	t.Parallel()

	facts := minimalCanonicalFacts()
	facts.Result = &CanonicalResult{
		Status:        "FINAL",
		Mode:          "final",
		PrimaryResult: json.RawMessage(`{"credential":"portable-build-secret-002","control":"portable-control"}`),
	}
	facts.SecretProvenance = []recordings.RecordingSecret{{
		JSONPointer: "/result/primaryResult/credential",
		Provenance:  recordings.RecordingSecretProvenanceDeclared,
	}}

	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if value.Redaction.SecretsRedacted != 1 {
		t.Fatalf("secretsRedacted = %d, want 1", value.Redaction.SecretsRedacted)
	}
	assertPortableResultRedacted(t, value, "portable-control")
}

func TestAtomicWriterRedactsBeforeTemporaryWrite(t *testing.T) {
	t.Parallel()

	facts := minimalCanonicalFacts()
	facts.Result = &CanonicalResult{
		Status:        "FINAL",
		Mode:          "final",
		PrimaryResult: json.RawMessage(`{"credential":"portable-write-secret-002","control":"portable-write-control"}`),
	}
	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	value.SecretProvenance = []recordings.RecordingSecret{{
		JSONPointer: "/result/primaryResult/credential",
		Provenance:  recordings.RecordingSecretProvenanceDeclared,
	}}

	writer, err := NewAtomicWriter(
		os.MkdirAll,
		func(dir, pattern string) (TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		os.Remove,
		os.Rename,
	)
	if err != nil {
		t.Fatalf("NewAtomicWriter: %v", err)
	}
	path := filepath.Join(t.TempDir(), "nested", "recording.json")
	if err := writer.Write(path, value); err != nil {
		t.Fatalf("Write: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted Recording
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("decode persisted recording: %v", err)
	}
	if persisted.Redaction.SecretsRedacted != 1 {
		t.Fatalf("persisted secretsRedacted = %d, want 1", persisted.Redaction.SecretsRedacted)
	}
	assertPortableResultRedacted(t, persisted, "portable-write-control")
}

func TestAtomicWriterRejectsClassifiedPathBeforeCreatingDestination(t *testing.T) {
	t.Parallel()

	const declaredSecret = "portable-write-failure-secret-002"
	facts := minimalCanonicalFacts()
	facts.Result = &CanonicalResult{
		Status:        "FINAL",
		Mode:          "final",
		PrimaryResult: json.RawMessage(`{"credential":"portable-write-failure-secret-002"}`),
	}
	value, err := Build(facts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	value.SecretProvenance = []recordings.RecordingSecret{{
		JSONPointer: "/result/primaryResult/missing",
		Provenance:  recordings.RecordingSecretProvenanceDeclared,
	}}
	writer, err := NewAtomicWriter(
		os.MkdirAll,
		func(dir, pattern string) (TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		os.Remove,
		os.Rename,
	)
	if err != nil {
		t.Fatalf("NewAtomicWriter: %v", err)
	}
	path := filepath.Join(t.TempDir(), "not-created", "recording.json")
	err = writer.Write(path, value)
	if !errors.Is(err, recordings.ErrRecordingSecretPathNotFound) {
		t.Fatalf("Write error = %v, want missing classified path", err)
	}
	if strings.Contains(err.Error(), declaredSecret) {
		t.Fatalf("Write error exposed declared secret: %v", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination stat = %v, want not exist", statErr)
	}
}

func assertPortableResultRedacted(t *testing.T, value Recording, wantControl string) {
	t.Helper()
	if value.Result == nil {
		t.Fatal("portable result is nil")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value.Result.PrimaryResult, &fields); err != nil {
		t.Fatalf("decode portable result: %v", err)
	}
	var marker recordings.RecordingRedactedValue
	if err := json.Unmarshal(fields["credential"], &marker); err != nil {
		t.Fatalf("decode portable redaction marker: %v", err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("portable redaction marker: %v", err)
	}
	var control string
	if err := json.Unmarshal(fields["control"], &control); err != nil || control != wantControl {
		t.Fatalf("control = %q, want %q (err=%v)", control, wantControl, err)
	}
	if err := Validate(value); err != nil {
		t.Fatalf("redacted portable result failed validation: %v", err)
	}
}

// The real temporary file preserves filesystem cleanup and destination behavior;
// only the failure at the publication boundary is controlled.
type incompleteRecordingFile struct {
	*os.File
	stage   string
	failure error
}

func (file *incompleteRecordingFile) Write(data []byte) (int, error) {
	if file.stage == "short write" {
		return file.File.Write(data[:len(data)-1])
	}
	return file.File.Write(data)
}
func (file *incompleteRecordingFile) Sync() error {
	if file.stage == "sync" {
		return file.failure
	}
	return file.File.Sync()
}
func (file *incompleteRecordingFile) Close() error {
	err := file.File.Close()
	if file.stage == "close" {
		return file.failure
	}
	return err
}
func TestAtomicWriterPreservesDestinationOnPublicationFailure(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"short write", "sync", "close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			destination := filepath.Join(dir, "recording.json")
			original := []byte("previous complete snapshot")
			if err := os.WriteFile(destination, original, 0600); err != nil {
				t.Fatal(err)
			}
			failure := errors.New(stage)
			writer, err := NewAtomicWriter(os.MkdirAll, func(dir, pattern string) (TemporaryFile, error) {
				file, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return &incompleteRecordingFile{File: file, stage: stage, failure: failure}, nil
			}, os.Remove, func(source, target string) error {
				if stage == "rename" {
					return failure
				}
				return os.Rename(source, target)
			})
			if err != nil {
				t.Fatal(err)
			}
			value, err := Build(minimalCanonicalFacts())
			if err != nil {
				t.Fatal(err)
			}
			err = writer.Write(destination, value)
			if stage == "short write" {
				failure = io.ErrShortWrite
			}
			if !errors.Is(err, failure) {
				t.Fatalf("Write error = %v, want %v", err, failure)
			}
			actual, err := os.ReadFile(destination)
			if err != nil || string(actual) != string(original) {
				t.Fatalf("destination = %q, %v", actual, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary file cleanup = %v, %v", entries, err)
			}
		})
	}
}

package customer_journeys_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Replacement deliberately serializes only this customer-owned destination.
// All invocations and public export/read operations reuse one canonical process.
func TestRecordingExportDestinationRecovery(t *testing.T) {
	t.Parallel()
	destination := filepath.Join(t.TempDir(), "selected.json")
	var denied atomic.Bool
	var owner recordings.Service
	identities := make(chan string, 10)
	runner := support.NewRecordingCommandRunner("selected export result COMPLETE")
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:                    runner,
		FactorySessionRuntimeInstanceIDGenerator: func() string { id := uuid.NewString(); identities <- id; return id },
		RecordingsRootObserver:                   func(service recordings.Service) { owner = service },
		RecordingRenamePath: func(source, target string) error {
			if target == destination && denied.Load() {
				return errors.New("selected export denied")
			}
			return os.Rename(source, target)
		},
	})
	dir := scaffoldExportCustomerFactory(t)
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	home := t.TempDir()
	firstSession := uuid.NewString()
	runExportRecordingInvocation(t, process, dir, home, firstSession, destination, "older customer value")
	firstID := <-identities
	prior, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	denied.Store(true)
	_, err = owner.ExportPortableArtifact(t.Context(), recordings.ExportPortableArtifactRequest{RecordingID: recordings.RecordingID(firstID)})
	if !errors.Is(err, recordings.ErrPortableArtifactExportFailed) {
		t.Fatalf("B07-F export = %v, want typed publication failure", err)
	}
	after, readErr := os.ReadFile(destination)
	if readErr != nil || !bytes.Equal(after, prior) {
		t.Fatalf("B07-F changed prior artifact: %v", readErr)
	}
	denied.Store(false)
	assertSelectedExportRead(t, owner, firstID, firstSession, destination, "")
	// Finish a newer customer invocation at the same selected destination. Its
	// recording scope is different even though the public path is reused.
	newSession := uuid.NewString()
	runExportRecordingInvocation(t, process, dir, home, newSession, destination, "newer customer value")
	newID := <-identities
	assertSelectedExportRead(t, owner, newID, newSession, destination, firstSession)
	if runner.CallCount() != 2 {
		t.Fatalf("export/read redispatched customer Work: calls=%d", runner.CallCount())
	}
}

func runExportRecordingInvocation(t *testing.T, process support.Process, dir, home, session, destination, text string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "run", "--factory", filepath.Join(dir, "factory.json"), "--session", session, "--record", destination, "--output", "primary", text})
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("recorded invocation: %v / %s", err, inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "selected export result COMPLETE") {
		t.Fatalf("recorded output = %s", inputs.Stdout())
	}
}

func assertSelectedExportRead(t *testing.T, owner recordings.Service, id, session, path, excludedSession string) {
	t.Helper()
	exported, err := owner.ExportPortableArtifact(t.Context(), recordings.ExportPortableArtifactRequest{RecordingID: recordings.RecordingID(id)})
	if err != nil || string(exported.Reference) != path {
		t.Fatalf("B07-S export = %s, %v", exported.Reference, err)
	}
	read, err := owner.ReadPortableArtifact(t.Context(), recordings.ReadPortableArtifactRequest{RecordingID: recordings.RecordingID(id), Reference: exported.Reference})
	if err != nil {
		t.Fatalf("public exported read: %v", err)
	}
	if read.Artifact.Summary.Scope.FactorySessionID != session || len(read.Artifact.Events) == 0 {
		t.Fatalf("selected export scope = %#v", read.Artifact.Summary)
	}
	data, err := json.Marshal(read.Artifact.Events)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("selected export result COMPLETE")) {
		t.Fatal("export lost accepted Work result")
	}
	if excludedSession != "" && bytes.Contains(data, []byte(excludedSession)) {
		t.Fatal("B07-R replacement retained old session records")
	}
}

func scaffoldExportCustomerFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "export-customer")
	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["workTypes"].([]any)[0].(map[string]any)["handlingBehavior"] = []string{"DEFAULT"}
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

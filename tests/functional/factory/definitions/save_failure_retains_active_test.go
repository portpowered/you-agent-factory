package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type saveDeniedDefinitionFiles struct {
	platformfilesystem.Local
	activated atomic.Bool
}

func (files *saveDeniedDefinitionFiles) MkdirTemp(parent, pattern string) (string, error) {
	if !files.activated.Load() {
		panic("Definitions created a staging directory during construction")
	}
	if strings.HasPrefix(pattern, ".blocked.staging-") {
		return "", os.ErrPermission
	}
	return files.Local.MkdirTemp(parent, pattern)
}

func (files *saveDeniedDefinitionFiles) ReadFile(path string) ([]byte, error) {
	if !files.activated.Load() {
		panic("Definitions read an authored file during construction")
	}
	return files.Local.ReadFile(path)
}

func (files *saveDeniedDefinitionFiles) WriteFile(path string, data []byte, mode os.FileMode) error {
	if !files.activated.Load() {
		panic("Definitions wrote an authored file during construction")
	}
	if strings.Contains(filepath.Base(filepath.Dir(path)), ".write-blocked.staging-") {
		return os.ErrPermission
	}
	if filepath.Base(path) == "AGENTS.md" && strings.Contains(string(data), "definition-after") && (strings.Contains(path, ".corrupt-write.staging-") || strings.Contains(path, ".save-corrupt-replace.staging-")) {
		data = []byte("---\ntype: [\n")
	}
	return files.Local.WriteFile(path, data, mode)
}

type definitionResultCommand struct{ activated *atomic.Bool }

func (runner definitionResultCommand) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if !runner.activated.Load() {
		panic("provider executed during construction")
	}
	prompt := strings.Join(request.Args, " ") + string(request.Stdin)
	marker := "definition-before"
	if strings.Contains(prompt, "definition-after") {
		marker = "definition-after"
	} else if !strings.Contains(prompt, marker) {
		return platformprocess.CommandResult{}, errors.New("selected definition instructions missing from provider request")
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(marker + " COMPLETE")}, nil
}

// Save/readback and activation are session-owned API contracts; subsequent
// invocation proves the accepted definition is actually used by the runtime.
// The parent owns one exact persistence-edge graph through all parallel cells.
func TestFactorySaveFailureRetainsActiveDefinition(t *testing.T) {
	t.Parallel()
	api := support.NewProcessAPIServer()
	files := &saveDeniedDefinitionFiles{}
	var recordingReader recordings.Service
	durableWitness := make(chan *definitionSaveCursorWitness, 1)
	recordingPath := filepath.Join(t.TempDir(), "__factory_session_id__.json")
	process := support.BuildProcess(t, serviceedges.Edges{
		RecordingsRootObserver:                    func(service recordings.Service) { recordingReader = service },
		APIServerStarter:                          api.Start,
		ProviderCommandRunner:                     definitionResultCommand{activated: &files.activated},
		FactoryDefinitionPersistenceFileSystem:    files,
		FactoryDefinitionLoadingFileSystem:        files,
		FactoryDefinitionAuthoredReaderFileSystem: files,
		FactoryDefinitionAuthoredWriterFileSystem: files,
	})
	support.CleanupProcess(t, process)
	files.activated.Store(true)
	hostDir := support.ScaffoldSingleStepFactory(t, "definition-save-host")
	support.ClearSeedInputs(t, hostDir)
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	selected := []selectedDefinition{}
	for _, name := range []string{"save-success", "save-denied", "save-write-denied", "save-corrupt-create", "save-corrupt-replace", "save-missing"} {
		selected = append(selected, prepareSaveDefinition(t, process, env, name))
	}
	inputs := support.FakeInputs(context.Background(), []string{"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--no-record", "--quiet"})
	inputs.Input.Env, inputs.Input.WorkingDirectory = env, hostDir
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() {
		command.Stop(t)
		select {
		case witness := <-durableWitness:
			witness.assertDurable(t, process, env, recordingReader)
		default:
		}
	})
	baseURL := api.WaitForURL(t)
	for _, cell := range selected {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			assertSavedDefinitionSessionIsolation(t, process, env, baseURL, cell, durableWitness, recordingPath)
		})
	}
}

func assertDefinitionSaveOutcome(t *testing.T, endpoint string, before factoryapi.Factory, cell string) {
	t.Helper()
	// Detach slices from readback so rejected input cannot change the expected
	// retained definition in memory. A new named target avoids stale version.
	raw, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var updated factoryapi.Factory
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	selectSaveTarget(&updated, before, cell)
	body := "definition-after {{ (index .Inputs 0).Payload }}"
	(*updated.Workstations)[0].Body = &body
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: updated, Mode: &mode})
	if err != nil {
		t.Fatal(err)
	}
	response, _, status := definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
	if cell == "save-denied" {
		assertDefinitionsHTTPError(t, response, status, http.StatusInternalServerError, "INTERNAL_ERROR")
		return
	}
	if cell == "save-write-denied" || strings.HasPrefix(cell, "save-corrupt-") {
		assertDefinitionsHTTPError(t, response, status, http.StatusBadRequest, "INVALID_FACTORY")
		return
	}
	if status != http.StatusOK {
		t.Fatalf("save = %d %s", status, response)
	}
	var saved factoryapi.Factory
	if err := json.Unmarshal(response, &saved); err != nil {
		t.Fatal(err)
	}
	readback := support.GetJSON[factoryapi.Factory](t, endpoint)
	// Save returns disk-backed asset references; readback hydrates their bytes
	// and recomputes content hashes. Other selected fields remain identical.
	assertImportExportPortableBundledFileInline(t, saved, factoryapi.BundledFileTypeDOC, importExportPortableDocPath, "", "save response")
	assertImportExportPortableBundledFileInline(t, saved, factoryapi.BundledFileTypeSCRIPT, importExportPortableScriptPath, "", "save response")
	saved.SupportingFiles, saved.Metadata = readback.SupportingFiles, readback.Metadata
	if saved.Name != updated.Name || !reflect.DeepEqual(saved, readback) || (*readback.Workstations)[0].Body == nil || *(*readback.Workstations)[0].Body != body {
		t.Fatal("accepted save/readback lost updated instructions or selected fields")
	}
}

func assertSavedDefinitionPortableRoundTrip(t *testing.T, process support.Process, env []string, baseURL, root string, saved factoryapi.Factory) {
	t.Helper()
	assertImportExportPortableBundledFileInline(t, saved, factoryapi.BundledFileTypeDOC, importExportPortableDocPath, importExportPortableDocBody, "saved snapshot")
	assertImportExportPortableBundledFileInline(t, saved, factoryapi.BundledFileTypeSCRIPT, importExportPortableScriptPath, importExportPortableScriptBody, "saved snapshot")
	path := filepath.Join(root, "accepted-update", "factory.json")
	durable, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	station := (*saved.Workstations)[0]
	if strings.Contains(string(durable), stringValue(station.Body)) {
		t.Fatal("saved factory.json retained inline workstation content")
	}
	authored, err := os.ReadFile(filepath.Join(filepath.Dir(path), "workstations", station.Name, "AGENTS.md"))
	if err != nil || !strings.Contains(string(authored), stringValue(station.Body)) {
		t.Fatalf("saved split workstation content = %s, %v", authored, err)
	}
	portable, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, env, path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := support.DecodeFactoryDefinition(portable)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != exported.Name || !reflect.DeepEqual(saved.WorkTypes, exported.WorkTypes) || !reflect.DeepEqual(saved.Workstations, exported.Workstations) || !reflect.DeepEqual(saved.Version, exported.Version) {
		t.Fatal("portable export changed the selected definition fields or version")
	}
	assertImportExportPortableBundledFileInline(t, exported, factoryapi.BundledFileTypeDOC, importExportPortableDocPath, importExportPortableDocBody, "exported snapshot")
	assertImportExportPortableBundledFileInline(t, exported, factoryapi.BundledFileTypeSCRIPT, importExportPortableScriptPath, importExportPortableScriptBody, "exported snapshot")
	source := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(source, portable, 0o600); err != nil {
		t.Fatal(err)
	}
	importRoot := t.TempDir()
	dir := support.CreateAndActivateNamedFactoryAtRootWithProcess(t, process, env, filepath.Dir(source), importRoot, "snapshot-import", source)
	assertImportExportPortableFileOnDisk(t, filepath.Join(dir, "docs", "standards", "review.md"), importExportPortableDocBody)
	assertImportExportPortableFileOnDisk(t, filepath.Join(dir, "scripts", "execute-task.sh"), importExportPortableScriptBody)
	id := openDefinitionsNamedSession(t, baseURL, importRoot, "snapshot-import")
	t.Cleanup(func() { closeDefinitionsFactorySession(t, baseURL, id) })
	assertDefinitionInvocation(t, baseURL, id, "definition-after")
}

func assertMissingDefinitionKeepsOwner(t *testing.T, baseURL, root string) {
	t.Helper()
	name := "missing-definition"
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: root, Target: &factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindNamed, Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions", payload)
	assertDefinitionsHTTPError(t, body, status, http.StatusBadRequest, "BAD_REQUEST")
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &failure); err != nil || !strings.Contains(failure.Message, "Factory source could not be loaded or validated") {
		t.Fatal("missing definition lost its source diagnostic")
	}
}

func assertDefinitionInvocation(t *testing.T, baseURL, id, marker string) {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{Type: factoryapi.WorkContentPartTypeText, Text: "definition request"}); err != nil {
		t.Fatal(err)
	}
	content := factoryapi.WorkContent{part}
	kind := factoryapi.InvocationInputSourceKindText
	payload, err := json.Marshal(factoryapi.InvocationRequest{SourceKind: &kind, Content: &content})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions/"+id+"/invocations", payload)
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || response.Status != factoryapi.InvocationTerminalStatusCompleted || response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("definition invocation = %d %s", status, body)
	}
	text, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || text.Text != marker+" COMPLETE" {
		t.Fatalf("definition result = %q, %v; want %s", text.Text, err, marker)
	}
}

type selectedDefinition struct{ name, root string }

func prepareSaveDefinition(t *testing.T, process support.Process, env []string, name string) selectedDefinition {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, name)
	support.ClearSeedInputs(t, dir)
	support.UpdateFactoryConfig(t, dir, func(config map[string]any) {
		config["workTypes"].([]any)[0].(map[string]any)["handlingBehavior"] = []string{"DEFAULT"}
		config["supportingFiles"] = importExportPortableFactoryConfig()["supportingFiles"]
	})
	seedImportExportPortableFilesOnDisk(t, dir)
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "definition-model"))
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\ndefinition-before {{ (index .Inputs 0).Payload }}\n")
	portable, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, env, filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "portable.json")
	if err := os.WriteFile(source, portable, 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if name == "save-corrupt-replace" {
		root = filepath.Join(root, "factory")
	}
	support.CreateAndActivateNamedFactoryAtRootWithProcess(t, process, env, dir, root, name, source)
	return selectedDefinition{name, root}
}

func assertSavedDefinitionSessionIsolation(t *testing.T, process support.Process, env []string, baseURL string, cell selectedDefinition, durableWitness chan<- *definitionSaveCursorWitness, recordingPath string) {
	t.Helper()
	id, witness := definitionSaveSession(t, process, env, baseURL, cell, recordingPath)
	closed := false
	t.Cleanup(func() {
		if !closed {
			closeDefinitionsFactorySession(t, baseURL, id)
		}
	})
	peer := openDefinitionsNamedSession(t, baseURL, cell.root, cell.name)
	t.Cleanup(func() { closeDefinitionsFactorySession(t, baseURL, peer) })
	peerEndpoint := baseURL + "/factory-sessions/" + peer + "/factory"
	peerBefore := support.GetJSON[factoryapi.Factory](t, peerEndpoint)
	endpoint := baseURL + "/factory-sessions/" + id + "/factory"
	before := support.GetJSON[factoryapi.Factory](t, endpoint)
	durablePath := filepath.Join(cell.root, cell.name, "factory.json")
	durableBefore, err := os.ReadFile(durablePath)
	if err != nil {
		t.Fatal(err)
	}
	assertDefinitionInvocation(t, baseURL, id, "definition-before")
	witness.capture(t, baseURL)
	if cell.name == "save-missing" {
		assertMissingDefinitionKeepsOwner(t, baseURL, cell.root)
	} else {
		assertDefinitionSaveOutcome(t, endpoint, before, cell.name)
	}
	assertNoDefinitionStaging(t, cell.root)
	if cell.name != "save-success" {
		durableAfter, err := os.ReadFile(durablePath)
		if err != nil || string(durableAfter) != string(durableBefore) {
			t.Fatalf("rejected definition request changed durable content: %s, %v", durableAfter, err)
		}
		if cell.name == "save-corrupt-create" {
			if _, err := os.Stat(filepath.Join(cell.root, "corrupt-write")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected create left a partial named Factory: %v", err)
			}
		}
	}
	after := support.GetJSON[factoryapi.Factory](t, endpoint)
	want := "definition-before"
	if cell.name == "save-success" {
		want = "definition-after"
	} else if !reflect.DeepEqual(after, before) {
		t.Fatal("failed definition request changed active readback")
	}
	assertDefinitionInvocation(t, baseURL, id, want)
	witness.assertAfter(t, baseURL)
	if cell.name == "save-success" {
		fresh := openDefinitionsNamedSession(t, baseURL, cell.root, "accepted-update")
		t.Cleanup(func() { closeDefinitionsFactorySession(t, baseURL, fresh) })
		assertDefinitionInvocation(t, baseURL, fresh, "definition-after")
		assertSavedDefinitionPortableRoundTrip(t, process, env, baseURL, cell.root, after)
	}
	if got := support.GetJSON[factoryapi.Factory](t, peerEndpoint); !reflect.DeepEqual(got, peerBefore) {
		t.Fatal("saving one session changed its peer snapshot")
	}
	if !closeDefinitionsFactorySession(t, baseURL, id) {
		return
	}
	closed = true
	if witness != nil {
		witness.command.Stop(t)
		durableWitness <- witness
	}
	body, _, status := definitionsHTTPRequest(t, http.MethodGet, endpoint, nil)
	assertDefinitionsHTTPError(t, body, status, http.StatusNotFound, "NOT_FOUND")
	payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: before})
	if err != nil {
		t.Fatal(err)
	}
	body, _, status = definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
	assertDefinitionsHTTPError(t, body, status, http.StatusNotFound, "NOT_FOUND")
	assertDefinitionInvocation(t, baseURL, peer, "definition-before")
}

func assertNoDefinitionStaging(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".staging-") || entry.Name() == "blocked" || entry.Name() == "write-blocked" {
			t.Fatalf("failed save left owned artifact %q", entry.Name())
		}
	}
}

func selectSaveTarget(updated *factoryapi.Factory, before factoryapi.Factory, cell string) {
	updated.Name = "accepted-update"
	if cell == "save-denied" {
		updated.Name = "blocked"
	} else if cell == "save-write-denied" {
		updated.Name = "write-blocked"
	} else if cell == "save-corrupt-create" {
		updated.Name = "corrupt-write"
	} else if cell == "save-corrupt-replace" {
		updated.Name = cell
		updated.Version.Logical++
		updated.Version.Physical = updated.Version.Physical.Add(1)
	}
}

func assertDefinitionCursorRetained(t *testing.T, baseURL, id string, before []factoryapi.FactoryEvent, cursor support.FactoryEventReadCursor) {
	t.Helper()
	after := support.GetFactoryEventsForSessionAt(t, baseURL, id)
	if len(after) <= len(before) {
		t.Fatal("useful Work did not append history after save")
	}
	for i, event := range before {
		if after[i].Id != event.Id || support.ReconnectSequenceForFactoryEvent(after[i]) != support.ReconnectSequenceForFactoryEvent(event) {
			t.Fatal("save changed retained cursor history")
		}
	}
	tail := support.GetFactoryEventsAfterForSessionAt(t, baseURL, id, cursor)
	if len(tail) != len(after)-len(before) {
		t.Fatalf("reconnect tail=%d, want %d", len(tail), len(after)-len(before))
	}
	for i, event := range tail {
		if event.Id != after[len(before)+i].Id || support.ReconnectSequenceForFactoryEvent(event) <= *cursor.AfterSequence {
			t.Fatal("reconnect lost event identity/order")
		}
	}
}
func assertDefinitionRecordedCursor(t *testing.T, process support.Process, env []string, reader recordings.Service, path, id string, cursor support.FactoryEventReadCursor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--replay", path, "--no-record"})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("public recording replay: %v; %s", err, inputs.Stderr())
	}
	loaded, err := reader.(recordings.ReplayInputLoader).LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if err != nil || loaded.Legacy == nil {
		t.Fatalf("public recording inspection: %v", err)
	}
	for _, event := range loaded.Legacy.Events {
		if event.Id == cursor.AfterEventID {
			sequence := event.Context.Sequence
			if event.Context.SessionSequence != nil {
				sequence = *event.Context.SessionSequence
			}
			if sequence != *cursor.AfterSequence {
				t.Fatalf("public durable cursor sequence = %d, want %d", sequence, *cursor.AfterSequence)
			}
			if event.Context.SessionID == nil || *event.Context.SessionID != id {
				t.Fatalf("public durable cursor session = %#v, want %s", event.Context.SessionID, id)
			}
			return
		}
	}

	t.Fatal("public durable inspection lost the pre-save cursor")
}

func startRecordedDefinitionSession(t *testing.T, process support.Process, env []string, baseURL string, cell selectedDefinition, artifact string) (string, *support.ProcessCommand, string) {
	t.Helper()
	id := uuid.NewString()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--named", cell.name, "--session", id, "--record", artifact, "--continuously", "--quiet"})
	inputs.Input.Env, inputs.Input.WorkingDirectory = env, filepath.Dir(cell.root)
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	_, err := support.WaitForObservation(10*time.Second, func() (int, error) {
		select {
		case <-command.Done():
			return 0, command.Err()
		default:
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/factory-sessions/"+id+"/factory", nil)
		if err != nil {
			return 0, err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return 0, err
		}
		if err := response.Body.Close(); err != nil {
			return 0, err
		}
		return response.StatusCode, nil
	}, func(status int) bool { return status == http.StatusOK })
	if err != nil {
		t.Fatalf("recorded session not ready: %v", err)
	}
	return id, command, strings.ReplaceAll(artifact, "__factory_session_id__", id)
}

type definitionSaveCursorWitness struct {
	id       string
	command  *support.ProcessCommand
	artifact string
	history  []factoryapi.FactoryEvent
	cursor   support.FactoryEventReadCursor
}

func definitionSaveSession(t *testing.T, process support.Process, env []string, baseURL string, cell selectedDefinition, artifactPath string) (string, *definitionSaveCursorWitness) {
	t.Helper()
	if cell.name != "save-corrupt-replace" {
		return openDefinitionsNamedSession(t, baseURL, cell.root, cell.name), nil
	}
	id, command, artifact := startRecordedDefinitionSession(t, process, env, baseURL, cell, artifactPath)
	return id, &definitionSaveCursorWitness{id: id, command: command, artifact: artifact}
}
func (w *definitionSaveCursorWitness) capture(t *testing.T, baseURL string) {
	t.Helper()
	if w == nil {
		return
	}
	w.history = support.GetFactoryEventsForSessionAt(t, baseURL, w.id)
	if len(w.history) == 0 {
		t.Fatal("initial Work produced no cursor")
	}
	pivot := w.history[len(w.history)-1]
	sequence := support.ReconnectSequenceForFactoryEvent(pivot)
	w.cursor = support.FactoryEventReadCursor{AfterEventID: pivot.Id, AfterSequence: &sequence}
}
func (w *definitionSaveCursorWitness) assertAfter(t *testing.T, baseURL string) {
	t.Helper()
	if w != nil {
		assertDefinitionCursorRetained(t, baseURL, w.id, w.history, w.cursor)
	}
}
func (w *definitionSaveCursorWitness) assertDurable(t *testing.T, process support.Process, env []string, reader recordings.Service) {
	t.Helper()
	if w == nil {
		return
	}
	w.command.Stop(t)
	assertDefinitionRecordedCursor(t, process, env, reader, w.artifact, w.id, w.cursor)
}

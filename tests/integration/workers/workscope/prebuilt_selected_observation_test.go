package workscope_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const selectedProviderSessionEnv = "FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_SESSION_ID"

type selectedArtifactScenario struct {
	binary, factoryDir, serverURL, sessionID, workID, providerID string
	environment                                                  []string
	daemon                                                       *prebuiltWorkscopeDaemon
}

// This single delivered-artifact witness substitutes only the Codex command.
// The build lane supplies the binary; the test never builds it.
func TestPrebuiltWorkerSessionSelectedObservationConsistency(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	f, control := startSelectedArtifactScenario(t, ctx)
	defer control.Close()
	list := f.list(t, ctx, "--session", f.sessionID, "--work-id", f.workID)
	if len(list.Sessions) != 1 {
		t.Fatalf("owned Work observations = %#v", list)
	}
	workerID := list.Sessions[0].WorkerSessionId
	stream := openPrebuiltWorkscopeLiveEventStream(t, ctx, f.serverURL, f.sessionID, workerID)
	defer stream.Close()
	// The OS pipe is asynchronous. Wait for the public provider association
	// instead of assuming the socket handshake means projection is ready.
	f.waitProvider(t, ctx, workerID)
	f.assertParity(t, ctx, workerID, true)
	if _, err := io.WriteString(control, "release\n"); err != nil {
		t.Fatal(err)
	}
	waitForPrebuiltWorkscopeTerminalEvent(t, stream, workerID)
	f.assertParity(t, ctx, workerID, false)
	stopPrebuiltWorkscopeDaemon(t, ctx, f.daemon, f.binary, f.factoryDir, f.environment, f.serverURL)
	t.Logf("SELECTED-OBSERVATION artifact=%s sha256=%s session=%s work=%s worker=%s provider=controlled-Codex/%s commands_exit=0 active=RUNNING terminal=COMPLETED cleanup=owned-daemon-joined",
		f.binary, sha256FileDigest(t, f.binary), f.sessionID, f.workID, workerID, f.providerID)
}

func startSelectedArtifactScenario(t *testing.T, ctx context.Context) (selectedArtifactScenario, net.Conn) {
	t.Helper()
	root := t.TempDir()
	hostDir, factoryDir, home, providerDir := filepath.Join(root, "host"), filepath.Join(root, "factory"), filepath.Join(root, "home"), filepath.Join(root, "provider")
	for _, dir := range []string{hostDir, factoryDir, home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writePrebuiltWorkscopeLiveFactory(t, hostDir)
	writeSelectedArtifactFactory(t, factoryDir)
	writePrebuiltWorkscopeGatedCodex(t, providerDir)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	f := selectedArtifactScenario{binary: resolvePrebuiltWorkscopeBinary(t), factoryDir: factoryDir, providerID: uuid.NewString()}
	f.environment = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	f.environment = replacePrebuiltWorkscopeEnv(f.environment, "PATH", providerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.environment = replacePrebuiltWorkscopeEnv(f.environment, selectedProviderSessionEnv, f.providerID)
	f.environment = replacePrebuiltWorkscopeEnv(f.environment, prebuiltWorkscopeLiveProviderControlAddress, listener.Addr().String())
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil || port == 7437 {
		t.Fatalf("isolated port = %d, %v", port, err)
	}
	f.serverURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	f.daemon = startPrebuiltWorkscopeDaemon(t, ctx, f.binary, hostDir, f.environment,
		"run", "--dir", hostDir, "--continuously", "--with-server", "--listen", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), "--record", filepath.Join(root, "owned-recording.json"))
	waitForPrebuiltWorkscopeLiveServer(t, ctx, f.daemon, f.serverURL)
	var opened factoryapi.OpenFactorySessionResponse
	selectedArtifactPOST(t, ctx, f.serverURL+"/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: factoryDir}, http.StatusOK, &opened)
	if opened.Session == nil || opened.Session.IsDefault || opened.Session.Id == "~default" {
		t.Fatalf("non-default open = %#v", opened)
	}
	f.sessionID = opened.Session.Id
	var submitted factoryapi.SubmitWorkResponse
	selectedArtifactPOST(t, ctx, f.serverURL+"/factory-sessions/"+f.sessionID+"/work",
		factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"title": "selected observation"}}, http.StatusCreated, &submitted)
	if submitted.WorkId == nil {
		t.Fatal("submission omitted Work identity")
	}
	f.workID = *submitted.WorkId
	control := waitForPrebuiltWorkscopeProviderStarted(t, ctx, f.daemon, listener)
	return f, control
}

func selectedArtifactPOST(t *testing.T, ctx context.Context, endpoint string, payload any, wantStatus int, output any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, status := readPrebuiltWorkscopeResponse(t, response)
	if status != wantStatus {
		t.Fatalf("POST %s = %d: %s", endpoint, status, raw)
	}
	if err := json.Unmarshal(raw, output); err != nil {
		t.Fatal(err)
	}
}

func writeSelectedArtifactFactory(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"factory.json":                   `{"name":"selected-observation","workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"processor"}],"workstations":[{"name":"process","worker":"processor","inputs":[{"workType":"task","state":"ready"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}]}]}`,
		"workers/processor/AGENTS.md":    "---\ntype: MODEL_WORKER\nmodel: gpt-5-codex\nmodelProvider: CODEX\nexecutorProvider: CODEX\nstopToken: COMPLETE\n---\nObserve the selected Work.\n",
		"workstations/process/AGENTS.md": "---\ntype: MODEL_WORKSTATION\n---\nProcess the selected Work.\n",
	}
	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func (f selectedArtifactScenario) waitProvider(t *testing.T, ctx context.Context, workerID string) {
	t.Helper()
	// The provider stdout pipe and association publication are asynchronous.
	// Read the public ID observation until the owned tuple becomes observable.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		response := doPrebuiltWorkscopeGET(t, ctx, &http.Client{Timeout: 5 * time.Second}, f.serverURL+"/factory-sessions/"+f.sessionID+"/worker-sessions/"+workerID)
		raw, status := readPrebuiltWorkscopeResponse(t, response)
		var observation factoryapi.WorkerSessionObservation
		if status != http.StatusOK || json.Unmarshal(raw, &observation) != nil {
			t.Fatalf("provider readiness = %d %s", status, raw)
		}
		if observation.ProviderSession != nil && observation.ProviderSession.Id == f.providerID {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("provider tuple never became observable: %v %s", ctx.Err(), raw)
		case <-ticker.C:
		}
	}
}

func (f selectedArtifactScenario) list(t *testing.T, ctx context.Context, args ...string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	command := exec.CommandContext(ctx, f.binary,
		append([]string{"--server", f.serverURL, "--json", "--debug", "worker-sessions", "list"}, args...)...)
	command.Dir, command.Env = f.factoryDir, append([]string(nil), f.environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	started := time.Now()
	if err := command.Run(); err != nil {
		t.Fatalf("compiled list: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	// Includes executable launch, application construction and consumed output.
	// Timing is diagnostic here; the hard 200-row budget belongs to load tests.
	elapsed := time.Since(started)
	for _, phase := range []string{"headerWaitMicros=", "bodyDecodeMicros=", "list render durationMicros=", "list write durationMicros=", "list command durationMicros="} {
		if !strings.Contains(stderr.String(), phase) {
			t.Fatalf("compiled list omitted phase %q: %s", phase, stderr.String())
		}
	}
	if count := strings.Count(stderr.String(), "worker sessions list request endpointPath="); count != 1 {
		t.Fatalf("compiled list request diagnostics=%d, want 1: %s", count, stderr.String())
	}
	output := stdout.Bytes()
	t.Logf("CLI list args=%v exit=0 launchThroughConsumedOutputMicros=%d bytes=%d diagnostics=%s", args, elapsed.Microseconds(), len(output), stderr.String())
	var list factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(output, &list); err != nil {
		t.Fatalf("decode list: %v %s", err, output)
	}
	return list
}

func (f selectedArtifactScenario) assertParity(t *testing.T, ctx context.Context, workerID string, active bool) {
	t.Helper()
	// No Work or Factory Session selector: exercise the actual global fleet CLI.
	args := []string{"--max-results", "1000"}
	wantState := factoryapi.WorkerSessionObservationStateCompleted
	if active {
		args = append(args, "--state", "RUNNING", "--state", "STARTING")
		wantState = factoryapi.WorkerSessionObservationStateRunning
	}
	fleet := f.list(t, ctx, args...)
	var matching []factoryapi.WorkerSessionObservation
	for _, row := range fleet.Sessions {
		if row.WorkerSessionId == workerID {
			matching = append(matching, row)
		}
	}
	work := f.list(t, ctx, "--session", f.sessionID, "--work-id", f.workID)
	if !active {
		f.assertScopedHTTPParity(t, ctx, work)
	}
	if len(matching) != 1 || len(work.Sessions) != 1 {
		t.Fatalf("fleet=%#v Work=%#v", matching, work)
	}
	output := runPrebuiltWorkscopeCLI(t, ctx, f.binary, f.factoryDir, f.environment,
		"--server", f.serverURL, "--json", "worker-sessions", "show", "--session", f.sessionID, "--worker-session-id", workerID)
	t.Logf("CLI --server %s --json worker-sessions show --session %s --worker-session-id %s exit=0 output=%s", f.serverURL, f.sessionID, workerID, output)
	var shown factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(output, &shown); err != nil {
		t.Fatal(err)
	}
	for i, row := range []factoryapi.WorkerSessionObservation{matching[0], work.Sessions[0], shown} {
		wantTranscript := factoryapi.WorkerSessionObservationTranscriptAVAILABLE
		// Scoped reads expose a transcript only after capture completes. Fleet
		// reads retain identity facts without projecting captured transcripts.
		if i == 0 || active {
			wantTranscript = factoryapi.WorkerSessionObservationTranscriptUNAVAILABLE
		}
		f.assertExpectedFacts(t, row, workerID, wantState, wantTranscript)
		if row.TurnUsage != nil || row.Parse.EventCount != 0 || len(row.Parse.Errors) != 0 {
			t.Fatalf("observation invented native enrichment: %#v", row)
		}
		if row.ConfirmationState != matching[0].ConfirmationState || !reflect.DeepEqual(row.RecordingHealth, matching[0].RecordingHealth) || !reflect.DeepEqual(row.RecordingHealthReason, matching[0].RecordingHealthReason) {
			t.Fatalf("compiled observation health/confirmation disagree: fleet=%#v scoped=%#v", matching[0], row)
		}
	}
}

func (f selectedArtifactScenario) assertScopedHTTPParity(t *testing.T, ctx context.Context, cliList factoryapi.ListWorkerSessionsResponse) {
	t.Helper()
	endpoint := f.serverURL + "/factory-sessions/" + f.sessionID + "/worker-sessions?workId=" + f.workID
	started := time.Now()
	response := doPrebuiltWorkscopeGET(t, ctx, &http.Client{Timeout: 5 * time.Second}, endpoint)
	headerWait := time.Since(started)
	raw, status := readPrebuiltWorkscopeResponse(t, response)
	var httpList factoryapi.ListWorkerSessionsResponse
	if status != http.StatusOK || json.Unmarshal(raw, &httpList) != nil || !reflect.DeepEqual(cliList, httpList) {
		t.Fatalf("compiled scoped CLI/HTTP parity: status=%d CLI=%#v HTTP=%s", status, cliList, raw)
	}
	elapsed := time.Since(started)
	t.Logf("equivalent HTTP endpoint=%s status=%d rows=%d headerWaitMicros=%d bodyThroughDecodeMicros=%d totalMicros=%d bytes=%d",
		endpoint, status, len(httpList.Sessions), headerWait.Microseconds(), (elapsed - headerWait).Microseconds(), elapsed.Microseconds(), len(raw))
}

func (f selectedArtifactScenario) assertExpectedFacts(t *testing.T, row factoryapi.WorkerSessionObservation, workerID string, wantState factoryapi.WorkerSessionObservationState, wantTranscript factoryapi.WorkerSessionObservationTranscript) {
	t.Helper()
	if row.State != wantState || row.WorkerSessionId != workerID || row.FactorySessionId == nil || *row.FactorySessionId != f.sessionID || row.Failure != nil {
		t.Fatalf("compiled live identity = %#v, want %s in %s", row, wantState, f.sessionID)
	}
	wantHealth := factoryapi.WorkerSessionObservationRecordingHealthComplete
	if row.State == factoryapi.WorkerSessionObservationStateRunning {
		wantHealth = factoryapi.WorkerSessionObservationRecordingHealthIncomplete
	}
	if row.RecordingHealth == nil || *row.RecordingHealth != wantHealth || row.RecordingHealthReason != nil {
		t.Fatalf("compiled recording health = %#v/%v, want %s without interruption", row.RecordingHealth, row.RecordingHealthReason, wantHealth)
	}
	wantProvider := &factoryapi.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: f.providerID}
	if !row.ProviderSessionAvailable || !reflect.DeepEqual(row.ProviderSession, wantProvider) || row.Transcript != wantTranscript {
		t.Fatalf("compiled provider facts = %#v, want controlled transcript %s", row, wantTranscript)
	}
}

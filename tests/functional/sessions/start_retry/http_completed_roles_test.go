package start_retry_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// HTTP owns the contract here: isolated handler tests cannot prove that both
// profiles registered by canonical Wire reach the selected Session owner.
// One immutable process serves the live host and the durable workflow host.
// Children own their directories and sessions; cleanup outlives parallel leaves.
func TestHTTPCompletedRoles(t *testing.T) {
	t.Parallel()
	live, durable := support.NewProcessAPIServer(), support.NewProcessAPIServer()
	durableJoined := make(chan struct{})
	durableBound := make(chan struct{})
	durableRelease := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(durableRelease) }) }
	durable.HoldShutdownUntilSignaled(durableRelease)
	t.Cleanup(release)
	var starts atomic.Int32
	effects := &initialOpeningEffects{calls: make(map[string]int)}
	failed := newInitialOpeningProviderScenario(t)
	logCore, logs := observer.New(zap.InfoLevel)
	overlap := make([]initialOpeningScenario, 4)
	providerGate := &selectedProviderGate{paths: make(map[string]string), entered: make(chan platformprocess.CommandRequest, 4), release: make(chan struct{})}
	for i := range overlap {
		overlap[i] = newInitialOpeningProviderScenario(t)
		providerGate.paths[overlap[i].candidateDir] = overlap[i].candidateID
	}
	t.Cleanup(providerGate.unblock)
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		RecordingReadFile:     os.ReadFile,
		ProcessLogger:         zap.New(logCore).With(zap.String("selected_backend", "completed-http")),
		ScriptCommandRunner:   initialOpeningScriptRunner{effects: effects},
		ProviderCommandRunner: completedHTTPProviderRunner{initialOpeningProviderRunner: initialOpeningProviderRunner{effects: effects, selected: providerGate}, failedDir: failed.candidateDir},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			starts.Add(1)
			request.Handler = completedHTTPContextHandler(request.Handler)
			if request.Port == 18013 {
				onBound := request.OnBound
				request.OnBound = func(binding platformhttpserver.Binding) {
					if onBound != nil {
						onBound(binding)
					}
					close(durableBound)
				}
				stop := context.AfterFunc(ctx, func() { close(durableJoined) })
				defer stop()
				return durable.Start(ctx, request)
			}
			return live.Start(ctx, request)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	if starts.Load() != 0 || len(effects.calls) != 0 {
		t.Fatal("H01: construction activated a host or worker before Execute")
	}
	dir := support.ScaffoldFactory(t, initialOpeningFactoryConfig())
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--no-record"})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	support.InitializeCustomerHomeWithProcess(t, process, inputs.Input.Env, dir)
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL := live.WaitForURL(t)
	t.Run("H01 H03 live HTTP admission authoring control and selected output", func(t *testing.T) {
		t.Parallel()
		testCompletedLiveHTTP(t, baseURL)
	})
	t.Run("H05 invalid admission controls and compatibility", func(t *testing.T) {
		t.Parallel()
		sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
		testCompletedHTTPValidation(t, sessions, baseURL)
	})
	t.Run("H10 H11 overlapping HTTP Work and retained then live responses", func(t *testing.T) {
		t.Parallel()
		sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
		testCompletedHTTPOverlap(t, sessions, baseURL, overlap, providerGate, failed, effects, logs)
	})
	t.Run("H02 H13 durable readiness result and unavailable roles", func(t *testing.T) {
		t.Parallel()
		// Release before command cleanup on any assertion failure.
		t.Cleanup(release)
		testCompletedDurableHTTP(t, process, durable, durableBound, durableJoined, release, baseURL)
	})
}

func testCompletedLiveHTTP(t *testing.T, baseURL string) {
	t.Helper()
	scenario := newInitialOpeningScenario(t)
	body, _ := json.Marshal(map[string]any{"folderPath": scenario.candidateDir})
	opened := completedHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions", string(body), http.StatusOK)
	var response factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(opened, &response); err != nil || response.Session == nil {
		t.Fatalf("H01: open response = %s, %v", opened, err)
	}
	id := response.Session.Id
	t.Cleanup(func() {
		completedHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions/"+id+"/cancel", `{}`, http.StatusOK)
		completedHTTPRequest(t, http.MethodDelete, baseURL+"/factory-sessions/"+id, "", http.StatusNoContent)
	})
	selected := baseURL + "/factory-sessions/" + id
	read := support.GetJSON[factoryapi.FactorySession](t, selected)
	if read.Id != id || read.FolderPath != scenario.candidateDir {
		t.Fatalf("H01: admitted Session = %#v", read)
	}
	testCompletedHTTPAuthoringAndControl(t, baseURL, selected, scenario.candidateDir)
	// Petri Sessions expose completed Work, while the session result route
	// retains its documented unavailable outcome even after successful Work.
	completedHTTPError(t, selected+"/result", http.StatusNotFound, "NOT_FOUND")
	invoked := completedHTTPRequest(t, http.MethodPost, selected+"/invocations", `{"sourceKind":"text","content":[{"type":"text","text":"selected HTTP Work"}]}`, http.StatusOK)
	var invocation factoryapi.InvocationResponse
	if err := json.Unmarshal(invoked, &invocation); err != nil || invocation.Status != "COMPLETED" || !strings.Contains(string(invoked), "initial opening COMPLETE") {
		t.Fatalf("H01: invocation = %s, %v", invoked, err)
	}
	completedHTTPError(t, selected+"/result", http.StatusNotFound, "NOT_FOUND")
	work := support.GetJSON[factoryapi.ListWorkResponse](t, selected+"/work")
	if len(work.Results) != 1 || fmt.Sprint(work.Results[0].Payload) != "selected HTTP Work" {
		t.Fatalf("H01: selected Work = %#v", work)
	}
	completedHTTPError(t, baseURL+"/factory-sessions/unknown-completed-role", http.StatusNotFound, "NOT_FOUND")
	completedHTTPError(t, baseURL+"/factory-sessions/unknown-completed-role/status", http.StatusNotFound, "NOT_FOUND")
	completedHTTPError(t, support.SessionResponseEventsURL(baseURL, "unknown-completed-role"), http.StatusNotFound, "RESPONSE_EVENT_SESSION_NOT_FOUND")
}

func testCompletedHTTPAuthoringAndControl(t *testing.T, baseURL, selected, projectRoot string) {
	t.Helper()
	definition := completedHTTPRequest(t, http.MethodGet, selected+"/factory", "", http.StatusOK)
	var factory factoryapi.Factory
	if err := json.Unmarshal(definition, &factory); err != nil || factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 || (*factory.WorkTypes)[0].Name != "task" {
		t.Fatalf("H03: Current Factory = %s, %v", definition, err)
	}
	validation := completedHTTPRequest(t, http.MethodPost, baseURL+"/factory-validations", string(definition), http.StatusOK)
	var validated factoryapi.FactoryValidationResult
	if err := json.Unmarshal(validation, &validated); err != nil || len(validated.Targets) != 0 {
		t.Fatalf("H03: valid definition diagnostics = %s, %v", validation, err)
	}
	// A stale editable version is rejected with a targeted diagnostic and
	// cannot replace this Session's admitted definition.
	var stale map[string]any
	if err := json.Unmarshal(definition, &stale); err != nil {
		t.Fatal(err)
	}
	stale["version"] = map[string]string{"logical": "0", "physical": "1970-01-01T00:00:00Z"}
	body, _ := json.Marshal(map[string]any{"factory": stale})
	saved := completedHTTPRequest(t, http.MethodPut, selected+"/factory", string(body), http.StatusConflict)
	if !strings.Contains(string(saved), "STALE_FACTORY_VERSION") || !strings.Contains(string(saved), "factory.version.stale") {
		t.Fatalf("H03: stale definition diagnostic = %s", saved)
	}
	after := completedHTTPRequest(t, http.MethodGet, selected+"/factory", "", http.StatusOK)
	if string(after) != string(definition) {
		t.Fatalf("H03: rejected save changed Current Factory: %s", after)
	}
	if factory.Version == nil {
		t.Fatal("H03: Current Factory has no editable version")
	}
	stale["version"] = map[string]string{
		"logical":  strconv.FormatInt(factory.Version.Logical.Int64()+1, 10),
		"physical": factory.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano),
	}
	body, _ = json.Marshal(map[string]any{"factory": stale})
	completedHTTPRequest(t, http.MethodPut, selected+"/factory", string(body), http.StatusOK)
	reloaded := support.GetJSON[factoryapi.Factory](t, selected+"/factory")
	if reloaded.Version == nil || reloaded.Version.Logical <= factory.Version.Logical || reloaded.WorkTypes == nil || (*reloaded.WorkTypes)[0].Name != "task" {
		t.Fatalf("H03: accepted save did not preserve definition and advance version: %#v", reloaded)
	}
	previewBody, _ := json.Marshal(map[string]string{"sourceKind": "INLINE_WORKFLOW", "inlineSource": "return 7;", "projectRoot": projectRoot})
	preview := completedHTTPRequest(t, http.MethodPost, baseURL+"/factories/preview", string(previewBody), http.StatusOK)
	if !strings.Contains(string(preview), "JAVASCRIPT") {
		t.Fatalf("H03: preview = %s", preview)
	}
	for _, operation := range []string{"pause", "resume"} {
		controlled := completedHTTPRequest(t, http.MethodPost, selected+"/"+operation, `{}`, http.StatusOK)
		var control factoryapi.FactorySessionLifecycleControlResponse
		if err := json.Unmarshal(controlled, &control); err != nil || control.SessionId != strings.TrimPrefix(selected, baseURL+"/factory-sessions/") || string(control.Operation) != strings.ToUpper(operation) || control.Outcome != "ACCEPTED" {
			t.Fatalf("H03: %s control = %s, %v", operation, controlled, err)
		}
	}
	completedHTTPRequest(t, http.MethodGet, selected+"/status", "", http.StatusOK)
}

func testCompletedHTTPValidation(t *testing.T, sessions factorysessions.Service, baseURL string) {
	t.Helper()
	for _, body := range []string{`{`, `{}`, `{"folderPath":""}`} {
		data := completedHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions", body, http.StatusBadRequest)
		assertCompletedHTTPErrorBody(t, data, "BAD_REQUEST")
		if body != `{` {
			assertCompletedHTTPValidationTarget(t, data, "folderPath", "factory.session.field.required")
		}
	}
	scenario := newInitialOpeningScenario(t)
	body, _ := json.Marshal(map[string]any{"folderPath": scenario.candidateDir, "ignoredProbe": "compatibility-secret-sentinel"})
	for _, mediaType := range []string{"text/plain", "not-a-media-type"} {
		data, headers := completedHTTPResponse(t, http.MethodPost, baseURL+"/factory-sessions", string(body), mediaType, http.StatusUnsupportedMediaType)
		assertCompletedHTTPErrorBody(t, data, "UNSUPPORTED_MEDIA_TYPE")
		if headers.Get("Warning") != "" {
			t.Fatalf("H05: rejected media type emitted compatibility warning: %v", headers)
		}
	}
	// A supported JSON suffix preserves forward compatibility. The header
	// names ignored paths only, never their possibly sensitive values.
	data, headers := completedHTTPResponse(t, http.MethodPost, baseURL+"/factory-sessions", string(body), "application/vnd.factory+json; charset=utf-8", http.StatusOK)
	if warning := headers.Get("Warning"); warning != `299 - "ignored unknown request fields at $.ignoredProbe"` || strings.Contains(string(data), "compatibility-secret-sentinel") {
		t.Fatalf("H05: accepted compatibility response = %s, Warning=%q", data, warning)
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(data, &opened); err != nil || opened.Session == nil {
		t.Fatalf("H05: compatible admission = %s, %v", data, err)
	}
	t.Cleanup(func() { closeInitialOpeningSession(t, sessions, opened.Session.Id) })
	selected := baseURL + "/factory-sessions/" + opened.Session.Id
	for _, control := range []string{"pause", "resume", "cancel"} {
		data := completedHTTPRequest(t, http.MethodPost, selected+"/"+control, `{`, http.StatusBadRequest)
		assertCompletedHTTPErrorBody(t, data, "BAD_REQUEST")
	}
	testCompletedHTTPInvalidDefinition(t, selected, baseURL)
	work := support.GetJSON[factoryapi.ListWorkResponse](t, selected+"/work")
	if len(work.Results) != 0 {
		t.Fatalf("H05: invalid controls or definition admitted Work: %#v", work)
	}
	completedHTTPRequest(t, http.MethodGet, selected+"/status", "", http.StatusOK)
}

func testCompletedHTTPInvalidDefinition(t *testing.T, selected, baseURL string) {
	t.Helper()
	before := completedHTTPRequest(t, http.MethodGet, selected+"/factory", "", http.StatusOK)
	var definition factoryapi.Factory
	if err := json.Unmarshal(before, &definition); err != nil || definition.WorkTypes == nil || len(*definition.WorkTypes) == 0 || definition.Version == nil {
		t.Fatalf("H05: selected definition = %s, %v", before, err)
	}
	duplicate := append(*definition.WorkTypes, (*definition.WorkTypes)[0])
	definition.WorkTypes = &duplicate
	// Use a fresh version so validation, rather than the stale-save guard,
	// rejects this definition and leaves the admitted Session unchanged.
	definition.Version.Logical++
	definition.Version.Physical = definition.Version.Physical.Add(time.Nanosecond)
	body, _ := json.Marshal(map[string]any{"factory": definition})
	rejected := completedHTTPRequest(t, http.MethodPut, selected+"/factory", string(body), http.StatusBadRequest)
	assertCompletedHTTPValidationTarget(t, rejected, "", "factory.duplicateIdentifier")
	body, _ = json.Marshal(definition)
	validated := completedHTTPRequest(t, http.MethodPost, baseURL+"/factory-validations", string(body), http.StatusOK)
	assertCompletedHTTPValidationTarget(t, validated, "", "factory.duplicateIdentifier")
	after := completedHTTPRequest(t, http.MethodGet, selected+"/factory", "", http.StatusOK)
	if string(after) != string(before) {
		t.Fatalf("H05: rejected definition changed Current Factory: before=%s after=%s", before, after)
	}
}

func assertCompletedHTTPValidationTarget(t *testing.T, body []byte, subjectID, code string) {
	t.Helper()
	var response struct {
		Targets []factoryapi.FactoryValidationTarget `json:"targets"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	for _, target := range response.Targets {
		if target.Code == code && (subjectID == "" || target.Subject.Id == subjectID) {
			return
		}
	}
	t.Fatalf("H05: response lacks validation target code=%s subject=%s: %s", code, subjectID, body)
}

func assertCompletedHTTPErrorBody(t *testing.T, body []byte, code string) {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil || string(response.Code) != code {
		t.Fatalf("HTTP error = %s, %v, want %s", body, err, code)
	}
}

func testCompletedDurableHTTP(t *testing.T, process support.Process, server *support.ProcessAPIServer, bound, joined <-chan struct{}, release func(), liveURL string) {
	t.Helper()
	dir := t.TempDir()
	workflow := filepath.Join(dir, "completed.js")
	if err := os.WriteFile(workflow, []byte(`return "completed durable HTTP";`), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--factory", workflow, "--with-server", "--listen", "127.0.0.1:18013", "--json"})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	support.InitializeCustomerHomeWithProcess(t, process, inputs.Input.Env, dir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(release)
	baseURL := completedHTTPHostURL(t, server, bound, command, inputs)
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	select {
	case <-joined: // Completion canceled its host; listener stays readable.
	case <-ctx.Done():
		t.Fatalf("H02: durable completion did not stop its host: %v", ctx.Err())
	}
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions?scope=persisted")
	if listed.DurableSessions == nil || len(*listed.DurableSessions) != 1 {
		t.Fatalf("H02: durable listing = %#v", listed)
	}
	id := (*listed.DurableSessions)[0].SessionId
	selected := baseURL + "/factory-sessions/" + id
	read := completedHTTPRequest(t, http.MethodGet, selected, "", http.StatusOK)
	if !strings.Contains(string(read), id) || !strings.Contains(string(read), "SUCCEEDED") {
		t.Fatalf("H02: durable read = %s", read)
	}
	testCompletedDurableHTTPControl(t, selected, id, read)
	completedHTTPError(t, selected+"/status", http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
	completedHTTPError(t, selected+"/factory", http.StatusInternalServerError, "INTERNAL_ERROR")
	invoked := completedHTTPRequest(t, http.MethodPost, selected+"/invocations", `{}`, http.StatusInternalServerError)
	if !strings.Contains(string(invoked), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("H13: unavailable invocation = %s", invoked)
	}
	release()
	select {
	case <-command.Done():
	case <-ctx.Done():
		t.Fatalf("H02: host cleanup did not join: %v", ctx.Err())
	}
	if command.Err() != nil || !strings.Contains(inputs.Stdout(), id) || !strings.Contains(inputs.Stdout(), "completed durable HTTP") {
		t.Fatalf("H02: Execute = %v, output=%s stderr=%s", command.Err(), inputs.Stdout(), inputs.Stderr())
	}
	var outcome factoryapi.FactorySessionSyncExecutionResponse
	lines := strings.Split(strings.TrimSpace(inputs.Stdout()), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &outcome); err != nil || outcome.SessionId != id || outcome.Status != "SUCCEEDED" || outcome.Result == nil || outcome.Result.SessionId != id || outcome.Result.ResultStatus != "FINAL" {
		t.Fatalf("H02: canonical outcome = %s, %v", inputs.Stdout(), err)
	}
	// Durable host teardown leaves the concurrently hosted live profile usable.
	completedHTTPRequest(t, http.MethodGet, liveURL+"/factory-sessions", "", http.StatusOK)
}

func testCompletedDurableHTTPControl(t *testing.T, selected, id string, before []byte) {
	t.Helper()
	for _, operation := range []string{"pause", "resume", "cancel", "terminate"} {
		data := completedHTTPRequest(t, http.MethodPost, selected+"/"+operation, `{}`, http.StatusConflict)
		var response factoryapi.FactorySessionLifecycleControlResponse
		if err := json.Unmarshal(data, &response); err != nil || response.SessionId != id || string(response.Operation) != strings.ToUpper(operation) || response.Outcome != "TERMINAL_SESSION" || response.Status != "SUCCEEDED" {
			t.Fatalf("H02: terminal durable %s = %s, %v", operation, data, err)
		}
	}
	after := completedHTTPRequest(t, http.MethodGet, selected, "", http.StatusOK)
	if string(after) != string(before) {
		t.Fatalf("H02: rejected durable control changed completed result: before=%s after=%s", before, after)
	}
}

func completedHTTPHostURL(t *testing.T, server *support.ProcessAPIServer, bound <-chan struct{}, command *support.ProcessCommand, inputs *support.CapturedInputs) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	select {
	case <-command.Done():
		t.Fatalf("H02: command exited before host readiness: %v, stdout=%s stderr=%s", command.Err(), inputs.Stdout(), inputs.Stderr())
	case <-bound:
		return server.WaitForURL(t)
	case <-ctx.Done():
		t.Fatalf("H02: host did not become ready: %v", ctx.Err())
	}
	return ""
}

// The provider gate makes coexistence deterministic. Only the selected Session
// is closed; peer invocations and cursor ownership remain live until release.
func testCompletedHTTPOverlap(t *testing.T, sessions factorysessions.Service, baseURL string, scenarios []initialOpeningScenario, gate *selectedProviderGate, failed initialOpeningScenario, effects *initialOpeningEffects, logs *observer.ObservedLogs) {
	t.Helper()
	defer gate.unblock()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	ids := make([]string, len(scenarios))
	closes := make([]func(), len(scenarios))
	done := make([]<-chan completedHTTPInvocation, len(scenarios))
	t.Cleanup(func() {
		cancel()
		gate.unblock()
		for _, joined := range done {
			if joined == nil {
				continue
			}
			select {
			case <-joined:
			case <-time.After(time.Minute):
				t.Error("HTTP invocation did not join during cleanup")
			}
		}
	})
	streams := make([]*support.FactoryResponseEventStream, len(scenarios))
	last := make([]int64, len(scenarios))
	for i, scenario := range scenarios {
		ids[i], closes[i] = completedHTTPOpen(t, sessions, baseURL, scenario.candidateDir)
		selected := baseURL + "/factory-sessions/" + ids[i]
		empty := support.GetJSON[factoryapi.ListWorkResponse](t, selected+"/work")
		if len(empty.Results) != 0 {
			t.Fatalf("H04: empty Session %s has Work: %#v", ids[i], empty)
		}
		done[i] = startCompletedHTTPInvocation(ctx, selected+"/invocations", scenario.candidateID)
	}
	awaitSelectedProviders(t, ctx, gate)
	testCompletedHTTPRequestContexts(t, sessions, baseURL, ids[1])
	testCompletedHTTPProviderFailure(t, sessions, baseURL, failed, effects, logs)
	testCompletedHTTPFailedCronRecovery(t, sessions, baseURL, logs)
	for i, id := range ids {
		assertSelectedRunningWorker(t, baseURL, id)
		completedHTTPRequest(t, http.MethodGet, baseURL+"/factory-sessions/"+id+"/status", "", http.StatusOK)
		streams[i] = support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, id))
		if !streams[i].HasRetainedFrameCount || streams[i].RetainedFrameCount == 0 {
			t.Fatal("H11: admitted Work did not retain a response prefix")
		}
		for range streams[i].RetainedFrameCount {
			frame := streams[i].NextFrame(time.Minute)
			last[i] = assertCompletedHTTPFrame(t, frame, id, last[i])
		}
	}
	// Detach one cursor before closing that Session; peer cursors continue to
	// observe the same generation and ordered native provider output.
	streams[0].Close()
	closes[0]()
	canceled := awaitCompletedHTTPInvocation(t, ctx, done[0])
	var closedError factoryapi.ErrorResponse
	if err := json.Unmarshal(canceled.body, &closedError); err != nil || canceled.status != http.StatusNotFound || closedError.Code != "NOT_FOUND" {
		t.Fatalf("H10: closed invocation = %d %s, %v, want typed 404", canceled.status, canceled.body, err)
	}
	completedHTTPError(t, baseURL+"/factory-sessions/"+ids[0], http.StatusNotFound, "NOT_FOUND")
	for i := 1; i < len(ids); i++ {
		assertSelectedRunningWorker(t, baseURL, ids[i])
		select {
		case result := <-done[i]:
			t.Fatalf("H10: closing one Session stopped peer %s: %s, %v", ids[i], result.body, result.err)
		default:
		}
	}
	gate.unblock()
	finishCompletedHTTPPeers(t, ctx, baseURL, ids, scenarios, done, streams, last)
}

func testCompletedHTTPFailedCronRecovery(t *testing.T, sessions factorysessions.Service, baseURL string, logs *observer.ObservedLogs) {
	t.Helper()
	fixture, path := completedHTTPFailedCronRecording(t)
	scenario := newInitialOpeningScenario(t)
	request := scenario.request()
	request.RuntimeSelection.Recording.ResumePath = path
	startInitialOpeningSession(t, sessions, request)
	selected := baseURL + "/factory-sessions/" + scenario.candidateID
	read := support.GetJSON[factoryapi.FactorySession](t, selected)
	if read.Id != scenario.candidateID {
		t.Fatalf("H09: recovery replaced selected Session identity: %#v", read)
	}
	completedHTTPRequest(t, http.MethodGet, selected+"/status", "", http.StatusOK)
	work := support.GetJSON[factoryapi.ListWorkResponse](t, selected+"/work")
	if len(work.Results) != 2 || !support.HasWorkAtCustomerState(work, "work-recovered-after-automation", "task:complete") || !support.HasWorkAtCustomerState(work, "failed-cron-output", "task:failed") {
		t.Fatalf("H09: failed cron recovery Work = %#v", work)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	history, err := sessions.SubscribeFactoryEventsForSession(ctx, scenario.candidateID, nil)
	if err != nil || history == nil {
		t.Fatalf("H09: recovered history = %#v, %v", history, err)
	}
	if len(history.History) < len(fixture.Events) {
		t.Fatal("H09: recovery discarded recorded events")
	}
	for i, recorded := range fixture.Events {
		restored := history.History[i]
		if restored.Id != recorded.Id || restored.Type != recorded.Type || !restored.Context.EventTime.Equal(recorded.Context.EventTime) {
			t.Fatalf("H09: recovery replaced recorded identity/order/time at %d: %#v", i, restored)
		}
	}
	dispatches := 0
	for _, event := range history.History {
		if event.Type != factorydefinitions.FactoryEventTypeDispatchRequest {
			continue
		}
		var payload factorydefinitions.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.TransitionID == "daily-refresh" {
			dispatches++
		}
	}
	if dispatches != 1 {
		t.Fatalf("H09: consumed cron dispatches=%d, want recorded one", dispatches)
	}
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if fields["event"] == "run.restore.disposition" && fields["session_id"] == scenario.candidateID && fields["dispatch_id"] == "dispatch-daily-refresh" && fields["outcome"] == "FAILED" && fields["selected_backend"] == "completed-http" {
			return
		}
	}
	t.Fatal("H09: failed cron recovery lacks selected scoped disposition")
}

// A small controlled failed completion extends the existing recording; its
// failed output supplies the conclusive ownership required by recovery.
// Generated scenario bytes stay in temporary storage, not the repository.
func completedHTTPFailedCronRecording(t *testing.T) (*factorydefinitions.ReplayArtifact, string) {
	t.Helper()
	fixture := testutil.LoadReplayArtifact(t, testutil.MustRepoPath(t, "tests/functional/sessions/isolation_and_recovery/testdata/automation-work-missing-occupancy.replay.json"))
	for i := range fixture.Events {
		event := &fixture.Events[i]
		if event.Type == factorydefinitions.FactoryEventTypeDispatchResponse && event.Context.DispatchID != nil && *event.Context.DispatchID == "dispatch-daily-refresh" {
			var payload map[string]any
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload["outcome"], payload["error"] = "FAILED", "controlled cron failure"
			payload["outputWork"] = []map[string]any{{"name": "failed-cron-output", "workId": "failed-cron-output", "workTypeName": "task", "traceId": "trace-automation-recovery", "state": map[string]string{"name": "failed", "type": "FAILED"}}}
			event.Payload, _ = json.Marshal(payload)
		}
	}
	path := filepath.Join(t.TempDir(), "failed-cron.json")
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return fixture, path
}

// The failure is injected only at the provider command effect. Four other
// Session invocations remain held while the failed attempt returns its public
// outcome; their completion/output checks follow in the owning overlap cell.
func testCompletedHTTPProviderFailure(t *testing.T, sessions factorysessions.Service, baseURL string, scenario initialOpeningScenario, effects *initialOpeningEffects, logs *observer.ObservedLogs) {
	t.Helper()
	id, _ := completedHTTPOpen(t, sessions, baseURL, scenario.candidateDir)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	result := awaitCompletedHTTPInvocation(t, ctx, startCompletedHTTPInvocation(ctx, baseURL+"/factory-sessions/"+id+"/invocations", completedHTTPSecret))
	outcome := result.response
	if result.status != http.StatusOK || outcome.Status != "FAILED" || outcome.ErrorCode == nil || *outcome.ErrorCode != "INVOCATION_RUNTIME_FAILURE" ||
		outcome.WorkState == nil || *outcome.WorkState != "task:failed" || outcome.WorkId == nil || outcome.RequestId == "" || outcome.TraceId == "" || outcome.PrimaryResult != nil {
		t.Fatalf("H12: provider failure lost its typed attributed outcome: %d %s", result.status, result.body)
	}
	work := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+id+"/work")
	if len(work.Results) != 1 || !support.HasWorkAtCustomerState(work, *outcome.WorkId, "task:failed") {
		t.Fatalf("H12: failed invocation disagrees with selected Work: %#v", work)
	}
	if strings.Contains(string(result.body), completedHTTPSecret) {
		t.Fatal("H12: public failure exposed secret")
	}
	assertInitialOpeningProviderSelection(t, effects, scenario.candidateDir)
	correlated := false
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if strings.Contains(fmt.Sprint(entry.Message, fields), completedHTTPSecret) {
			t.Fatal("H12: selected diagnostic exposed secret")
		}
		if fields["session_id"] == id && fields["selected_backend"] == "completed-http" && entry.Level >= zap.WarnLevel {
			correlated = true
		}
	}
	if !correlated {
		t.Fatal("H12: provider failure has no correlated selected diagnostic")
	}
}

// The server effect supplies an already terminal request context to the real
// router. This models cancellation/expiry before admission without a timer race
// or replacing a service. Ordinary requests retain the listener's context.
func completedHTTPContextHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ctx context.Context
		var cancel context.CancelFunc
		switch r.Header.Get("X-Completed-Request-Context") {
		case "canceled":
			ctx, cancel = context.WithCancel(r.Context())
			cancel()
		case "expired":
			ctx, cancel = context.WithDeadline(r.Context(), time.Unix(0, 0))
		default:
			next.ServeHTTP(w, r)
			return
		}
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func testCompletedHTTPRequestContexts(t *testing.T, sessions factorysessions.Service, baseURL, peerID string) {
	t.Helper()
	scenario := newInitialOpeningScenario(t)
	body, err := json.Marshal(map[string]string{"folderPath": scenario.candidateDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"canceled", "expired"} {
		for _, operation := range []struct{ method, endpoint, body string }{
			{http.MethodPost, baseURL + "/factory-sessions", string(body)},
			{http.MethodDelete, baseURL + "/factory-sessions/" + peerID, ""},
		} {
			request, err := http.NewRequestWithContext(t.Context(), operation.method, operation.endpoint, strings.NewReader(operation.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Completed-Request-Context", state)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if state == "canceled" {
				if response.StatusCode != http.StatusOK || len(data) != 0 {
					t.Fatalf("H06: canceled %s = %d %s, want empty response", operation.method, response.StatusCode, data)
				}
			} else {
				if response.StatusCode != http.StatusGatewayTimeout {
					t.Fatalf("H07: expired %s = %d %s, want 504", operation.method, response.StatusCode, data)
				}
				assertCompletedHTTPErrorBody(t, data, "INTERNAL_ERROR")
			}
		}
		reads, err := sessions.ListFactorySessions(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, read := range reads {
			if read.Context.Session != nil && read.Context.Session.FolderPath == scenario.candidateDir {
				t.Fatalf("H06/H07: %s request admitted its rejected Factory", state)
			}
		}
		assertSelectedRunningWorker(t, baseURL, peerID)
	}
}

func finishCompletedHTTPPeers(t *testing.T, ctx context.Context, baseURL string, ids []string, scenarios []initialOpeningScenario, done []<-chan completedHTTPInvocation, streams []*support.FactoryResponseEventStream, last []int64) {
	t.Helper()
	for i := 1; i < len(ids); i++ {
		result := awaitCompletedHTTPInvocation(t, ctx, done[i])
		if result.status != http.StatusOK || result.response.Status != "COMPLETED" || !strings.Contains(string(result.body), scenarios[i].candidateID+" COMPLETE") {
			t.Fatalf("H10: peer %s lost selected output: %s", ids[i], result.body)
		}
		for {
			frame := streams[i].NextFrame(time.Minute)
			last[i] = assertCompletedHTTPFrame(t, frame, ids[i], last[i])
			if frame.Event.Kind == "MESSAGE" && frame.Event.Phase == "COMPLETED" {
				message, err := json.Marshal(frame.Event.Payload)
				if err != nil || !strings.Contains(string(message), scenarios[i].candidateID+" COMPLETE") {
					t.Fatalf("H11: peer response output = %s, %v", message, err)
				}
				break
			}
		}
		streams[i].Close()
		testCompletedHTTPReconnect(t, baseURL, ids[i], last[i])
	}
}

func completedHTTPOpen(t *testing.T, sessions factorysessions.Service, baseURL, dir string) (string, func()) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"folderPath": dir})
	opened := completedHTTPRequest(t, http.MethodPost, baseURL+"/factory-sessions", string(body), http.StatusOK)
	var result factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(opened, &result); err != nil || result.Session == nil {
		t.Fatalf("HTTP open = %s, %v", opened, err)
	}
	id := result.Session.Id
	var once sync.Once
	closeSession := func() {
		once.Do(func() {
			closeInitialOpeningSession(t, sessions, id)
		})
	}
	t.Cleanup(closeSession)
	return id, closeSession
}

type completedHTTPInvocation struct {
	response factoryapi.InvocationResponse
	status   int
	body     []byte
	err      error
}

type completedHTTPProviderRunner struct {
	initialOpeningProviderRunner
	failedDir string
}

const completedHTTPSecret = "private-completed-http-provider-sentinel"

func (runner completedHTTPProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if request.WorkDir == runner.failedDir {
		runner.effects.record("worker.codex", request.WorkDir)
		return platformprocess.CommandResult{}, fmt.Errorf("controlled provider failure: %s", completedHTTPSecret)
	}
	return runner.initialOpeningProviderRunner.Run(ctx, request)
}

func (runner completedHTTPProviderRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	progress := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"completed-http-progress\",\"type\":\"command_execution\",\"command\":\"controlled inspection\",\"aggregated_output\":\"HTTP progress before release\",\"exit_code\":0}}\n")
	if observe != nil {
		observe(platformprocess.OutputStreamStdout, progress)
	}
	result, err := runner.Run(ctx, request)
	if err == nil && observe != nil {
		observe(platformprocess.OutputStreamStdout, result.Stdout)
	}
	result.Stdout = append(progress, result.Stdout...)
	return result, err
}

func startCompletedHTTPInvocation(ctx context.Context, endpoint, payload string) <-chan completedHTTPInvocation {
	done := make(chan completedHTTPInvocation, 1)
	go func() {
		defer close(done)
		result := completedHTTPInvocation{}
		body, _ := json.Marshal(map[string]any{"sourceKind": "text", "content": []map[string]string{{"type": "text", "text": payload}}})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
		if err == nil {
			request.Header.Set("Content-Type", "application/json")
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				result.status = response.StatusCode
				result.body, err = io.ReadAll(response.Body)
				if err == nil && response.StatusCode == http.StatusOK {
					err = json.Unmarshal(result.body, &result.response)
				}
			}
		}
		result.err = err
		done <- result
	}()
	return done
}

func awaitCompletedHTTPInvocation(t *testing.T, ctx context.Context, done <-chan completedHTTPInvocation) completedHTTPInvocation {
	t.Helper()
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("HTTP invocation = %s, %v", result.body, result.err)
		}
		return result
	case <-ctx.Done():
		t.Fatalf("HTTP invocation did not join: %v", ctx.Err())
		return completedHTTPInvocation{}
	}
}

func assertCompletedHTTPFrame(t *testing.T, frame support.FactoryResponseEventFrame, id string, previous int64) int64 {
	t.Helper()
	if frame.Event.FactorySessionId != id || frame.Event.Sequence <= previous || frame.SSEID != strconv.FormatInt(frame.Event.Sequence, 10) {
		t.Fatalf("H11: attributed ordered SSE frame = %#v, previous=%d, Session=%s", frame, previous, id)
	}
	return frame.Event.Sequence
}

func testCompletedHTTPReconnect(t *testing.T, baseURL, id string, sequence int64) {
	t.Helper()
	retained := support.GetFactoryResponseEventsAt(t, baseURL, id)
	reconnected := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURLWithAfterSequence(baseURL, id, sequence))
	defer reconnected.Close()
	count := 0
	for _, event := range retained {
		if event.Sequence <= sequence {
			continue
		}
		frame := reconnected.NextFrame(time.Minute)
		if frame.Event.EventId != event.EventId || frame.Event.Sequence != event.Sequence {
			t.Fatalf("H11: reconnect replayed or replaced an acknowledged event: %#v, want %#v", frame.Event, event)
		}
		count++
	}
	if !reconnected.HasRetainedFrameCount || reconnected.RetainedFrameCount != count {
		t.Fatalf("H11: reconnect retained count = %d, want %d", reconnected.RetainedFrameCount, count)
	}
}

func completedHTTPError(t *testing.T, endpoint string, status int, code string) {
	t.Helper()
	data := completedHTTPRequest(t, http.MethodGet, endpoint, "", status)
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.Code != code {
		t.Fatalf("error %s = %s, %v, want %s", endpoint, data, err, code)
	}
}

func completedHTTPRequest(t *testing.T, method, endpoint, body string, status int) []byte {
	t.Helper()
	data, _ := completedHTTPResponse(t, method, endpoint, body, "application/json", status)
	return data
}

func completedHTTPResponse(t *testing.T, method, endpoint, body, contentType string, status int) ([]byte, http.Header) {
	t.Helper()
	parent := t.Context()
	if method == http.MethodDelete || strings.HasSuffix(endpoint, "/cancel") {
		parent = context.WithoutCancel(parent)
	}
	ctx, cancel := context.WithTimeout(parent, initialOpeningReadCeiling)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", contentType)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status {
		t.Fatalf("%s %s = %d %s, %v, want %d", method, endpoint, response.StatusCode, data, err, status)
	}
	return data, response.Header.Clone()
}

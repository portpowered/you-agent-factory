package workers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	sharedJavaScriptSuccessPrompt = "shared process spine permission success"
	sharedJavaScriptSuccessOutput = "shared process spine success output"
)

// TestJavaScriptSharedProcessSpine proves that compatible JavaScript worker
// cells can use one root-built process and one hosted API listener while the
// ProviderCommandRunner edge selects an owning request by immutable command
// content rather than by invocation order.
func TestJavaScriptSharedProcessSpine(t *testing.T) {
	fixture := newJavaScriptSharedProcessFixture(t)

	t.Run("permission-matrix-cli-success", func(t *testing.T) {
		runJavaScriptSharedSuccess(t, fixture)
	})

	t.Run("invalid-permission-pre-dispatch-failure", func(t *testing.T) {
		runJavaScriptSharedFailure(t, fixture)
	})

	t.Run("reverse-order", func(t *testing.T) {
		reverseFixture := newJavaScriptSharedProcessFixture(t)
		runJavaScriptSharedFailure(t, reverseFixture)
		runJavaScriptSharedSuccess(t, reverseFixture)
	})
}

func runJavaScriptSharedSuccess(t testing.TB, fixture *javascriptSharedProcessFixture) {
	t.Helper()
	runner := support.NewRecordingCommandRunner(sharedJavaScriptSuccessOutput)
	if err := fixture.router.register(sharedJavaScriptSuccessPrompt, runner); err != nil {
		t.Fatalf("register success route: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(sharedJavaScriptSuccessPrompt); err != nil {
			t.Errorf("unregister success route: %v", err)
		}
	})

	before := fixture.persistentSessionIDs(t)
	beforeRecords := fixture.router.callCount()
	inputs, err := fixture.executeRemote(t, fixture.successFactoryName, sharedJavaScriptSuccessPrompt)
	if err != nil {
		t.Fatalf("shared remote success Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	assertSharedRemoteCommandPlacement(t, inputs.Input.Args, fixture.baseURL)
	if !strings.Contains(inputs.Stdout(), sharedJavaScriptSuccessOutput) {
		t.Fatalf("shared remote success output = %q, want %q", inputs.Stdout(), sharedJavaScriptSuccessOutput)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("owning Codex runner calls = %d, want one", runner.CallCount())
	}
	requests := fixture.router.requestRecords()
	if len(requests) != beforeRecords+1 {
		t.Fatalf("shared provider requests = %d, want %d; requests=%#v", len(requests), beforeRecords+1, requests)
	}
	request := requests[len(requests)-1]
	if request.Command != "codex" || !reflect.DeepEqual(request.Args, []string{"exec", "--json", "-"}) {
		t.Fatalf("shared provider command = %q %#v, want codex %#v", request.Command, request.Args, []string{"exec", "--json", "-"})
	}
	if !bytes.Contains(request.Stdin, []byte(sharedJavaScriptSuccessPrompt)) {
		t.Fatalf("shared provider stdin = %q, want owning request selector %q", request.Stdin, sharedJavaScriptSuccessPrompt)
	}
	if strings.TrimSpace(request.WorkDir) == "" {
		t.Fatal("shared provider request has empty working directory")
	}

	after := fixture.persistentSessionIDs(t)
	newSessions := differenceJavaScriptSessionIDs(before, after)
	if len(newSessions) != 1 || strings.TrimSpace(newSessions[0]) == "" {
		t.Fatalf("persisted session IDs before=%v after=%v new=%v, want one unique host-owned session", before, after, newSessions)
	}
	t.Logf("ROUTE-001 success: root_process_builds=1 api_server_starts=%d request_records=%d session_id=%s command=%s args=%v", fixture.apiStarter.starts.Load(), len(requests), newSessions[0], request.Command, request.Args)
}

func runJavaScriptSharedFailure(t testing.TB, fixture *javascriptSharedProcessFixture) {
	t.Helper()
	beforeCalls := fixture.router.callCount()
	beforeSessions := fixture.persistentSessionIDs(t)
	inputs, err := fixture.executeRemote(t, fixture.failureFactoryName, "shared invalid permissions")
	if err == nil {
		t.Fatalf("shared remote invalid-permission Process.Execute() error = nil\nstdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}
	assertSharedRemoteCommandPlacement(t, inputs.Input.Args, fixture.baseURL)
	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
		t.Fatalf("decode shared invalid-permission invocation response: %v; stdout=%q", decodeErr, inputs.Stdout())
	}
	if response.SessionId == nil || strings.TrimSpace(*response.SessionId) == "" {
		t.Fatalf("shared invalid-permission response = %#v, want failed session identity", response)
	}
	session := readJavaScriptSharedDurableSession(t, fixture.baseURL, *response.SessionId)
	if session.FailureDetail == nil {
		t.Fatalf("shared invalid-permission durable session = %#v, want failure detail", session)
	}
	diagnostic := strings.ToLower(strings.Join([]string{err.Error(), inputs.Stdout(), inputs.Stderr(), session.FailureDetail.Message}, "\n"))
	if !strings.Contains(diagnostic, "permissions") {
		t.Fatalf("shared invalid-permission diagnostic = %q, want permissions detail", diagnostic)
	}
	if strings.Contains(diagnostic, strings.ToLower(sharedJavaScriptSuccessOutput)) {
		t.Fatalf("shared invalid-permission diagnostic cross-observed success output: %q", diagnostic)
	}
	if got := fixture.router.callCount(); got != beforeCalls {
		t.Fatalf("provider command calls after invalid permission = %d, want unchanged %d", got, beforeCalls)
	}
	after := fixture.persistentSessionIDs(t)
	if duplicate := duplicateJavaScriptSessionID(after); duplicate != "" {
		t.Fatalf("persisted session ID %q was reused across shared cells: %v", duplicate, after)
	}
	if !containsJavaScriptSessionID(after, *response.SessionId) {
		t.Fatalf("persisted session IDs after=%v omit failed session %q", after, *response.SessionId)
	}
	newSessions := differenceJavaScriptSessionIDs(beforeSessions, after)
	if len(newSessions) != 1 || newSessions[0] != *response.SessionId {
		t.Fatalf("persisted session IDs before=%v after=%v new=%v, want exactly the failed cell's unique session", beforeSessions, after, newSessions)
	}
	t.Logf("ROUTE-001 failure: request_records=%d provider_calls=%d persisted_session_ids=%v diagnostic_contains_permissions=true", fixture.router.callCount(), fixture.router.callCount(), after)
}

type javascriptSharedProcessFixture struct {
	process            support.ApplicationProcess
	router             *javascriptSharedCommandRouter
	apiStarter         *javascriptSharedAPIServerStarter
	environment        []string
	baseURL            string
	hostDir            string
	successFactoryName string
	failureFactoryName string
}

type javascriptSharedAPIServerStarter struct {
	api    *support.ProcessAPIServer
	starts atomic.Int32
}

func (starter *javascriptSharedAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	return starter.api.Start(ctx, request)
}

// javascriptSharedCommandRouter is the only mutable part of the fixture. Its
// route registry is keyed by a request-content selector and rejects zero or
// ambiguous matches, so a sibling invocation cannot receive another cell's
// provider result accidentally.
type javascriptSharedCommandRouter struct {
	mu         sync.Mutex
	routes     map[string]platformprocess.CommandRunner
	requestLog []platformprocess.CommandRequest
}

func newJavaScriptSharedCommandRouter() *javascriptSharedCommandRouter {
	return &javascriptSharedCommandRouter{routes: make(map[string]platformprocess.CommandRunner)}
}

func (router *javascriptSharedCommandRouter) register(
	selector string,
	runner platformprocess.CommandRunner,
) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return fmt.Errorf("JavaScript route selector is required")
	}
	if runner == nil {
		return fmt.Errorf("JavaScript route %q has no command runner", selector)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[selector]; exists {
		return fmt.Errorf("JavaScript route %q is already registered", selector)
	}
	router.routes[selector] = runner
	return nil
}

func (router *javascriptSharedCommandRouter) unregister(selector string) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[selector]; !exists {
		return fmt.Errorf("JavaScript route %q is not registered", selector)
	}
	delete(router.routes, selector)
	return nil
}

func (router *javascriptSharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	router.requestLog = append(router.requestLog, cloneJavaScriptCommandRequest(request))
	matched := make([]platformprocess.CommandRunner, 0, 1)
	for selector, runner := range router.routes {
		if bytes.Contains(request.Stdin, []byte(selector)) {
			matched = append(matched, runner)
		}
	}
	router.mu.Unlock()

	switch len(matched) {
	case 0:
		return platformprocess.CommandResult{}, fmt.Errorf("JavaScript request did not match an immutable provider route")
	case 1:
		return matched[0].Run(ctx, request)
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("JavaScript request matched %d provider routes; want exactly one", len(matched))
	}
}

func (router *javascriptSharedCommandRouter) requestRecords() []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(router.requestLog))
	for index := range router.requestLog {
		requests[index] = cloneJavaScriptCommandRequest(router.requestLog[index])
	}
	return requests
}

func (router *javascriptSharedCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.requestLog)
}

func (router *javascriptSharedCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func newJavaScriptSharedProcessFixture(t *testing.T) *javascriptSharedProcessFixture {
	t.Helper()
	homeDir := t.TempDir()
	hostDir := support.ScaffoldFactory(t, permissionMatrixFactoryConfig(
		permissionMatrixWorkflowWithPrompt("DEFAULT", "shared process spine host"),
	))
	successFactoryName := "shared-javascript-success"
	failureFactoryName := "shared-javascript-invalid"
	successConfig := permissionMatrixFactoryConfig(
		permissionMatrixWorkflowWithPrompt("DEFAULT", sharedJavaScriptSuccessPrompt),
	)
	successConfig["name"] = successFactoryName
	failureConfig := permissionsOverrideFactoryConfig(invalidPermissionsOverrideWorkflow())
	failureConfig["name"] = failureFactoryName
	successSourceDir := support.ScaffoldFactory(t, successConfig)
	failureSourceDir := support.ScaffoldFactory(t, failureConfig)
	api := support.NewProcessAPIServer()
	apiStarter := &javascriptSharedAPIServerStarter{api: api}
	router := newJavaScriptSharedCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:                   apiStarter.Start,
		FactoryRuntimeWorkflowHome:         func() (string, error) { return homeDir, nil },
		FactorySessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
		ProviderCommandRunner:              router,
	})
	if err != nil {
		t.Fatalf("BuildProcess(shared JavaScript spine): %v", err)
	}
	fixture := &javascriptSharedProcessFixture{
		process:            process,
		router:             router,
		apiStarter:         apiStarter,
		environment:        javascriptSharedCustomerEnvironment(homeDir),
		hostDir:            hostDir,
		successFactoryName: successFactoryName,
		failureFactoryName: failureFactoryName,
	}
	// Cleanup is registered before the process and host cleanups so the probe
	// runs after the hosted command stops and the one reusable process closes.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	support.CleanupProcess(t, process)
	support.CopyFactoryAsNamed(t, successSourceDir, homeDir, successFactoryName)
	support.CopyFactoryAsNamed(t, failureSourceDir, homeDir, failureFactoryName)

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = hostDir
	support.StartProcessCommand(t, process, inputs.Input)
	fixture.baseURL = api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("shared API server starts = %d, want one host-owned listener", got)
	}
	return fixture
}

func (fixture *javascriptSharedProcessFixture) executeRemote(
	t testing.TB,
	namedFactory string,
	prompt string,
) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--remote", "--server", fixture.baseURL, "--json", "run",
		"--named", namedFactory,
		"--output", "primary", "--no-record", prompt,
	})
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = fixture.hostDir
	return inputs, fixture.process.Execute(inputs.Input)
}

func (fixture *javascriptSharedProcessFixture) persistentSessionIDs(t testing.TB) []string {
	t.Helper()
	response := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions?scope=persisted",
	)
	if response.DurableSessions == nil {
		return nil
	}
	ids := make([]string, 0, len(*response.DurableSessions))
	for _, session := range *response.DurableSessions {
		ids = append(ids, session.SessionId)
	}
	return ids
}

func (fixture *javascriptSharedProcessFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.router.routeCount(); got != 0 {
		t.Errorf("ROUTE-001 routes remaining after subtest cleanup = %d, want 0", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("ROUTE-001 API server starts = %d, want one", got)
	}
	if fixture.baseURL == "" {
		return
	}
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
	if err == nil {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Errorf("ROUTE-001 shared API listener remained available after process close: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func javascriptSharedCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		switch {
		case strings.EqualFold(name, "HOME"), strings.EqualFold(name, "USERPROFILE"),
			strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL_PROVIDER"), strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL"):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func readJavaScriptSharedDurableSession(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode shared durable session: %v", err)
	}
	return session
}

func assertSharedRemoteCommandPlacement(t testing.TB, args []string, serverURL string) {
	t.Helper()
	wantPrefix := []string{"you", "--remote", "--server", serverURL}
	if len(args) < len(wantPrefix) || !reflect.DeepEqual(args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("public remote command prefix = %#v, want %#v", args, wantPrefix)
	}
	if len(args) < len(wantPrefix)+2 || !reflect.DeepEqual(args[len(wantPrefix):len(wantPrefix)+2], []string{"--json", "run"}) {
		t.Fatalf("public remote command mode = %#v, want --json run after --remote --server", args)
	}
}

func cloneJavaScriptCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func differenceJavaScriptSessionIDs(before, after []string) []string {
	seen := make(map[string]struct{}, len(before))
	for _, id := range before {
		seen[id] = struct{}{}
	}
	var difference []string
	for _, id := range after {
		if _, exists := seen[id]; !exists {
			difference = append(difference, id)
		}
	}
	return difference
}

func duplicateJavaScriptSessionID(ids []string) string {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return id
		}
		seen[id] = struct{}{}
	}
	return ""
}

func containsJavaScriptSessionID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

var _ platformprocess.CommandRunner = (*javascriptSharedCommandRouter)(nil)

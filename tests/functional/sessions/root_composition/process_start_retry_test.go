package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The attempts are ordered because recovery must reuse the failed process.
// This process owns the entire live-list observation window; independent tests
// retain separate processes, homes, listeners, and Factory Sessions.
func TestRootProcessStartFailureThenRetrySucceedsWithoutLiveSession(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	router := &reusableRootAPIServerStarter{}
	listenerContexts := make(chan context.Context, 2)
	listenerReturns := make(chan struct{}, 2)
	listenerHandlers := make(chan http.Handler, 2)
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			listenerContexts <- ctx
			listenerHandlers <- request.Handler
			defer func() { listenerReturns <- struct{}{} }()
			return router.start(ctx, request)
		},
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	factoryDir := support.ScaffoldFactory(t, processExecuteRuntimeOpeningFactoryConfig())
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	support.InitializeCustomerHomeWithProcess(t, process, env, factoryDir)
	newInputs := func() *support.CapturedInputs {
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "run", "--factory", filepath.Join(factoryDir, "factory.json"),
			"--continuously", "--with-server", "--quiet", "--no-record",
		})
		inputs.Input.Env, inputs.Input.WorkingDirectory = env, factoryDir
		inputs.Input.Stdin = strings.NewReader("")
		return inputs
	}
	failure := errors.New("injected ordinary hosted listener startup failure")
	router.setFailure(failure)
	failed := newInputs()
	if err := process.Execute(failed.Input); !errors.Is(err, failure) {
		t.Fatalf("M1 Execute error = %v, want injected failure; stdout=%q stderr=%q", err, failed.Stdout(), failed.Stderr())
	}
	assertStartRetryListenerJoined(t, listenerContexts, listenerReturns)
	assertStartRetryNoLiveSession(t, <-listenerHandlers)
	if failed.Stdout() != "" {
		t.Fatalf("M1 failed opening stdout = %q, want no success result", failed.Stdout())
	}

	api := support.NewProcessAPIServer()
	router.setCurrent(api)
	command := support.StartProcessCommand(t, process, newInputs().Input)
	baseURL := api.WaitForURL(t)
	assertStartRetryPublicSession(t, baseURL, factoryDir)
	select {
	case <-command.Done():
		t.Fatalf("M2 retry returned before owned shutdown: %v", command.Err())
	default:
	}
	command.Stop(t)
	select {
	case <-command.Done():
	default:
		t.Fatal("M4 Execute did not complete after owned shutdown")
	}
	if err := command.Err(); err != nil {
		t.Fatalf("M4 Execute after shutdown error = %v, want nil", err)
	}
	assertStartRetryListenerJoined(t, listenerContexts, listenerReturns)
	closeCtx, cancel := context.WithTimeout(context.Background(), processLifecycleCloseTimeout)
	defer cancel()
	if err := process.Close(closeCtx); err != nil {
		t.Fatalf("M4 Process.Close() error = %v, want nil", err)
	}
}

func assertStartRetryNoLiveSession(t *testing.T, handler http.Handler) {
	t.Helper()
	// Observe the same public handler supplied to the failed listener, without
	// acquiring another listener or inspecting the session registry.
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/factory-sessions?scope=live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("M1 live-list status = %d, body = %s", response.Code, response.Body)
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatalf("M1 decode live-list: %v", err)
	}
	if len(listed.Sessions) != 0 {
		t.Fatalf("M1 live Sessions after rollback = %#v, want none", listed.Sessions)
	}
}

func assertStartRetryListenerJoined(t *testing.T, contexts <-chan context.Context, returns <-chan struct{}) {
	t.Helper()
	select {
	case ctx := <-contexts:
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("listener context error = %v, want canceled before Execute returns", ctx.Err())
		}
	default:
		t.Fatal("listener did not acquire its context")
	}
	select {
	case <-returns:
	default:
		t.Fatal("listener starter did not return before Execute completed")
	}
}

func assertStartRetryPublicSession(t *testing.T, baseURL, factoryDir string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions?scope="+string(factoryapi.FactorySessionListScopeLive))
	if len(listed.Sessions) != 1 {
		t.Fatalf("M2 live Factory Sessions = %#v, want exactly one complete retry Session", listed.Sessions)
	}
	summary := listed.Sessions[0]
	if id, err := uuid.Parse(summary.Id); err != nil || id == uuid.Nil {
		t.Fatalf("M2 returned Session ID = %q, want valid nonzero UUID", summary.Id)
	}
	if !summary.IsDefault || filepath.Clean(summary.FactoryDir) != filepath.Clean(factoryDir) ||
		filepath.Clean(summary.FolderPath) != filepath.Clean(factoryDir) || summary.Runtime == nil ||
		summary.Runtime.Status != factoryapi.FactorySessionStatusIDLE {
		t.Fatalf("M2 live Session = %#v, want intended idle hosted Session", summary)
	}
	sessionURL := baseURL + "/factory-sessions/" + summary.Id
	session := support.GetJSON[factoryapi.FactorySession](t, sessionURL)
	if session.Id != summary.Id || session.FactoryDir != summary.FactoryDir ||
		session.FolderPath != summary.FolderPath || session.IsDefault != summary.IsDefault ||
		session.Runtime.Status != summary.Runtime.Status || !reflect.DeepEqual(session.Target, summary.Target) {
		t.Fatalf("M3 Session detail = %#v, disagrees with live summary %#v", session, summary)
	}
	identity := session.Runtime.StreamIdentity
	if identity == nil || identity.FactorySessionID != summary.Id || identity.BackendScopeID == "" ||
		identity.LogicalSessionKeyID == "" || identity.StreamGenerationID == "" ||
		!reflect.DeepEqual(identity, summary.Runtime.StreamIdentity) {
		t.Fatalf("M3 stream identity = %#v, want complete linkage matching live listing", identity)
	}
	current := support.GetJSON[factoryapi.Factory](t, sessionURL+"/factory")
	assertStartRetryFactory(t, current, factoryDir)
}

func assertStartRetryFactory(t *testing.T, current factoryapi.Factory, factoryDir string) {
	t.Helper()
	if current.FactoryDirectory == nil || filepath.Clean(*current.FactoryDirectory) != filepath.Clean(factoryDir) {
		t.Fatalf("M3 Current Factory directory = %#v, want %q", current.FactoryDirectory, factoryDir)
	}
	if current.WorkTypes == nil || len(*current.WorkTypes) != 1 || (*current.WorkTypes)[0].Name != "task" {
		t.Fatalf("M3 Current Factory work types = %#v, want task", current.WorkTypes)
	}
	wantStates := map[string]factoryapi.WorkStateType{
		"init": factoryapi.WorkStateTypeINITIAL, "complete": factoryapi.WorkStateTypeTERMINAL, "failed": factoryapi.WorkStateTypeFAILED,
	}
	states := (*current.WorkTypes)[0].States
	if len(states) != len(wantStates) {
		t.Fatalf("M3 task states = %#v, want init/complete/failed", states)
	}
	for _, state := range states {
		if want, ok := wantStates[state.Name]; !ok || state.Type != want {
			t.Fatalf("M3 task state = %#v, want authored state/type", state)
		}
		delete(wantStates, state.Name)
	}
	if current.Workers == nil || len(*current.Workers) != 1 || (*current.Workers)[0].Name != "worker-a" {
		t.Fatalf("M3 workers = %#v, want worker-a", current.Workers)
	}
	if current.Workstations == nil || len(*current.Workstations) != 1 {
		t.Fatalf("M3 workstations = %#v, want process", current.Workstations)
	}
	station := (*current.Workstations)[0]
	if station.Name != "process" || station.Worker == nil || *station.Worker != "worker-a" ||
		len(station.Inputs) != 1 || station.Inputs[0].WorkType != "task" || station.Inputs[0].State != "init" ||
		station.Outputs == nil || len(*station.Outputs) != 1 || (*station.Outputs)[0].WorkType != "task" || (*station.Outputs)[0].State != "complete" ||
		station.OnFailure == nil || len(*station.OnFailure) != 1 || (*station.OnFailure)[0].WorkType != "task" || (*station.OnFailure)[0].State != "failed" {
		t.Fatalf("M3 workstation = %#v, want authored worker and task routes", station)
	}
}

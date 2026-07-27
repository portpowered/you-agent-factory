package climanifestcobra_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

type factoryConfigInitHandlerStub struct{}

func (factoryConfigInitHandlerStub) FactoryQuery(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryList(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryCreate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryUpdate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryDelete(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryReplaceCurrent(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryConfigValidate(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryConfigFlatten(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) FactoryConfigExpand(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}
func (factoryConfigInitHandlerStub) Init(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

func TestNewFactoryConfigInitFamilyComponentsProjectsContractedTree(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	for _, test := range []struct {
		root *cobra.Command
		path []string
	}{
		{root: components.Factory, path: []string{"query"}},
		{root: components.Factory, path: []string{"list"}},
		{root: components.Factory, path: []string{"create"}},
		{root: components.Factory, path: []string{"config", "validate"}},
		{root: components.Init, path: nil},
	} {
		command, _, findErr := test.root.Find(test.path)
		if findErr != nil {
			t.Fatalf("%s Find(%v) error = %v", test.root.Name(), test.path, findErr)
		}
		if command == nil {
			t.Fatalf("%s Find(%v) returned nil", test.root.Name(), test.path)
		}
	}
}

func TestFactoryConfigInitFamilyUsesManifestDefaultsAndRequiredness(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	create, _, err := components.Factory.Find([]string{"create"})
	if err != nil {
		t.Fatalf("find factory create: %v", err)
	}
	if got := create.Flags().Lookup("dir").DefValue; got != "factory" {
		t.Fatalf("factory create --dir default = %q, want factory", got)
	}
	components.Factory.SetArgs([]string{"create", "staging"})
	components.Factory.SetOut(&strings.Builder{})
	components.Factory.SetErr(&strings.Builder{})
	if err := components.Factory.Execute(); err == nil || !strings.Contains(err.Error(), `required flag(s) "--from" not set`) {
		t.Fatalf("factory create missing --from error = %v", err)
	}
}

func TestFactoryConfigInitFamilyOmitsRetiredScaffoldFlags(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	if len(components.Config.Commands()) != 0 {
		t.Fatalf("you config subcommands = %v, want retired config init absent", components.Config.Commands())
	}
	for _, name := range []string{"type", "executor"} {
		if flag := components.Init.Flags().Lookup(name); flag != nil {
			t.Fatalf("you init retained retired --%s flag", name)
		}
	}
	for _, name := range []string{"package", "dir", "format", "replace"} {
		if flag := components.Init.Flags().Lookup(name); flag == nil {
			t.Fatalf("you init --%s missing", name)
		}
	}
}

func TestFactoryConfigInitFamilyProjectsProviderModelSetupInputs(t *testing.T) {
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(factoryConfigInitHandlerStub{})
	if err != nil {
		t.Fatalf("NewFactoryConfigInitFamilyComponents() error = %v", err)
	}
	for _, name := range []string{"provider", "model"} {
		if flag := components.Init.Flags().Lookup(name); flag == nil {
			t.Fatalf("you init --%s missing", name)
		}
	}
	if got := components.Init.Short; got != "Configure provider and model defaults" {
		t.Fatalf("you init short help = %q", got)
	}
}

func TestNewFactoryConfigInitFamilyComponentsRejectsNilHandler(t *testing.T) {
	if _, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(nil); err == nil {
		t.Fatal("NewFactoryConfigInitFamilyComponents(nil) error = nil")
	}
}

func TestSessionResolvedHandlersMapDefaultsChangedValuesAndStableArguments(t *testing.T) {
	var (
		creates []sessioncli.CreateConfig
		lists   []sessioncli.ListConfig
		deletes []sessioncli.DeleteConfig
		diag    bytes.Buffer
	)
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Create: func(cfg sessioncli.CreateConfig) error {
			creates = append(creates, cfg)
			return nil
		},
		List: func(cfg sessioncli.ListConfig) error {
			lists = append(lists, cfg)
			return nil
		},
		Delete: func(cfg sessioncli.DeleteConfig) error {
			deletes = append(deletes, cfg)
			return nil
		},
	}, nil, func(*cobra.Command) io.Writer { return &diag })

	if err := executeResolvedSession(t, services, "session", "create", "--dir", "fleet"); err != nil {
		t.Fatalf("default create Execute() error = %v", err)
	}
	if len(creates) != 1 {
		t.Fatalf("create calls = %d, want 1", len(creates))
	}
	assertDefaultResolvedCreate(t, creates[0], &diag)

	if err := executeResolvedSession(
		t,
		services,
		"--server", "https://factory.example", "--json", "--verbose", "--debug",
		"session", "create", "--dir", "fleet", "--port", "9444",
		"--init-new-factory", "--target-kind", "named", "--target-name", "alpha",
	); err != nil {
		t.Fatalf("changed create Execute() error = %v", err)
	}
	assertChangedResolvedCreate(t, creates[1])

	if err := executeResolvedSession(t, services, "session", "list"); err != nil {
		t.Fatalf("default list Execute() error = %v", err)
	}
	assertDefaultResolvedList(t, lists[0])
	if err := executeResolvedSession(
		t, services, "--server", "https://factory.example",
		"session", "list", "--scope", "all",
	); err != nil {
		t.Fatalf("changed list Execute() error = %v", err)
	}
	assertChangedResolvedList(t, lists[1])

	if err := executeResolvedSession(t, services, "--json", "session", "delete", "session-beta"); err != nil {
		t.Fatalf("delete Execute() error = %v", err)
	}
	assertResolvedDelete(t, deletes)
}

func TestSessionResolvedHandlersRejectInvalidInputsBeforeOperation(t *testing.T) {
	calls := 0
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Create: func(sessioncli.CreateConfig) error {
			calls++
			return nil
		},
		Delete: func(sessioncli.DeleteConfig) error {
			calls++
			return nil
		},
	}, nil, nil)
	if err := executeResolvedSession(
		t, services, "session", "create", "--dir", "fleet", "--port", "not-an-int",
	); err == nil {
		t.Fatal("invalid typed port error = nil")
	}
	if err := executeResolvedSession(t, services, "session", "delete"); err == nil {
		t.Fatal("missing session ID error = nil")
	}
	if calls != 0 {
		t.Fatalf("operation calls = %d, want 0", calls)
	}
}

func TestSessionResolvedCreatePreservesHumanOutputAndDebugDiagnostics(t *testing.T) {
	var requests []factoryapi.OpenFactorySessionRequest
	protocol := newSessionCreateTestProtocol(t, &requests)
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Create: sessioncli.NewCreate(protocol),
	}, nil, func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() })

	stdout, stderr, err := executeResolvedSessionWithOutput(
		t, services,
		"--debug", "session", "create", "--dir", "/workspace/fleet",
		"--init-new-factory",
		"--target-kind", "named", "--target-name", "alpha",
	)
	if err != nil {
		t.Fatalf("human create Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "Opened factory session session-alpha") {
		t.Fatalf("human create stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "session create request") ||
		!strings.Contains(stderr, "session create response") {
		t.Fatalf("debug create stderr = %q, want request and response diagnostics", stderr)
	}
	assertNamedCreateRequest(t, requests)
}

func TestSessionResolvedCreatePreservesJSONAndValidationOnly(t *testing.T) {
	var requests []factoryapi.OpenFactorySessionRequest
	protocol := newSessionCreateTestProtocol(t, &requests)
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Create: sessioncli.NewCreate(protocol),
	}, nil, func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() })

	stdout, stderr, err := executeResolvedSessionWithOutput(
		t, services,
		"--json", "session", "create", "--dir", "/workspace/fleet",
		"--validate-only",
	)
	if err != nil {
		t.Fatalf("JSON create Execute() error = %v", err)
	}
	var response factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode JSON create output: %v\n%s", err, stdout)
	}
	if response.Session == nil || response.Session.Id != "session-alpha" {
		t.Fatalf("JSON create response = %#v", response)
	}
	if stderr != "" {
		t.Fatalf("non-diagnostic create stderr = %q", stderr)
	}
	if len(requests) != 1 || requests[0].ValidateOnly == nil || !*requests[0].ValidateOnly {
		t.Fatalf("validate-only create request = %#v", requests)
	}
}

func TestSessionResolvedCreateRejectsConflictBeforeOperation(t *testing.T) {
	calls := 0
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Create: func(sessioncli.CreateConfig) error {
			calls++
			return nil
		},
	}, nil, nil)
	stdout, _, err := executeResolvedSessionWithOutput(
		t, services,
		"session", "create", "--dir", "/workspace/fleet",
		"--init-new-factory", "--validate-only",
	)
	if err == nil {
		t.Fatal("mutually exclusive create flags error = nil")
	}
	if calls != 0 {
		t.Fatalf("create operation calls = %d, want 0", calls)
	}
	if stdout != "" {
		t.Fatalf("create validation stdout = %q, want empty", stdout)
	}
}

func TestSessionResolvedDeletePreservesExecutableBehavior(t *testing.T) {
	var deletedPaths []string
	protocol := newSessionTestHTTPProtocol(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodDelete {
			t.Fatalf("request method = %s, want DELETE", request.Method)
		}
		deletedPaths = append(deletedPaths, request.URL.Path)
		return sessionTestResponse(http.StatusNoContent, ""), nil
	})
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Delete: sessioncli.NewDelete(protocol),
	}, nil, func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() })

	stdout, stderr, err := executeResolvedSessionWithOutput(
		t, services, "--verbose", "session", "delete", "session/beta",
	)
	if err != nil {
		t.Fatalf("human delete Execute() error = %v", err)
	}
	if stdout != "Closed factory session session/beta\n" {
		t.Fatalf("human delete stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "session delete request") ||
		!strings.Contains(stderr, "session delete response") {
		t.Fatalf("verbose delete stderr = %q, want request and response diagnostics", stderr)
	}

	stdout, stderr, err = executeResolvedSessionWithOutput(
		t, services, "--json", "session", "delete", "session-beta",
	)
	if err != nil {
		t.Fatalf("JSON delete Execute() error = %v", err)
	}
	var result sessioncli.DeleteResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode JSON delete output: %v\n%s", err, stdout)
	}
	if result.SessionID != "session-beta" {
		t.Fatalf("JSON delete result = %#v", result)
	}
	if stderr != "" {
		t.Fatalf("non-diagnostic delete stderr = %q", stderr)
	}
	if len(deletedPaths) != 2 ||
		deletedPaths[0] != "/factory-sessions/session%2Fbeta" ||
		deletedPaths[1] != "/factory-sessions/session-beta" {
		t.Fatalf("delete paths = %#v", deletedPaths)
	}
}

func TestSessionResolvedCreateDeletePreserveOperationFailures(t *testing.T) {
	operationFailure := errors.New("operation failed")
	tests := []struct {
		name     string
		args     []string
		services commandregistry.SessionResolvedServices
		want     error
	}{
		{
			name: "create failure",
			args: []string{"session", "create", "--dir", "/workspace/fleet"},
			services: commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
				Create: func(sessioncli.CreateConfig) error { return operationFailure },
			}, nil, nil),
			want: operationFailure,
		},
		{
			name: "delete cancellation",
			args: []string{"session", "delete", "session-beta"},
			services: commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
				Delete: func(sessioncli.DeleteConfig) error { return context.Canceled },
			}, nil, nil),
			want: context.Canceled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, _, err := executeResolvedSessionWithOutput(t, test.services, test.args...)
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if stdout != "" {
				t.Fatalf("failure stdout = %q, want empty", stdout)
			}
		})
	}

	calls := 0
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Delete: func(sessioncli.DeleteConfig) error {
			calls++
			return nil
		},
	}, nil, nil)
	for _, args := range [][]string{
		{"session", "delete"},
		{"session", "delete", "one", "two"},
	} {
		stdout, _, err := executeResolvedSessionWithOutput(t, services, args...)
		if err == nil {
			t.Fatalf("delete args %v error = nil", args)
		}
		if stdout != "" {
			t.Fatalf("delete args %v stdout = %q, want empty", args, stdout)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid delete operation calls = %d, want 0", calls)
	}
}

func TestSessionResolvedInspectionPreservesLiveAndPersistedListing(t *testing.T) {
	var requestCount int
	protocol := newSessionTestHTTPProtocol(t, func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodGet || request.URL.Path != "/factory-sessions" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if requestCount == 2 {
			return sessionTestResponse(http.StatusOK, `{"sessions":[]}`), nil
		}
		return sessionTestResponse(http.StatusOK, `{
			"sessions": [{
				"id": "session-alpha",
				"project": "alpha",
				"factoryDir": "/workspace/fleet/alpha",
				"folderPath": "/workspace/fleet",
				"isDefault": false,
				"target": {"kind": "named", "name": "alpha"}
			}]
		}`), nil
	})
	list := sessioncli.NewList(protocol, sessionListPreparation(prepareSessionListRequest))
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		List: func(cfg sessioncli.ListConfig) error {
			cfg.DurableLister = func(
				_ context.Context,
				request fse.ListSessionsRequest,
			) (fse.ListSessionsResult, error) {
				return fse.ListSessionsResult{
					Scope: request.Scope,
					DurableSessions: []fse.DurableSessionListSummary{{
						SessionID:        "dur-sess-review-001",
						Status:           fse.LifecycleStatusSucceeded,
						OrchestratorKind: "JAVASCRIPT",
						ResolvedSource:   fse.ResolvedSource{Kind: "WORKFLOW_FILE", SourceRef: "workflows/review.js"},
					}},
				}, nil
			}
			return list(cfg)
		},
	}, nil, nil)

	stdout, _, err := executeResolvedSessionWithOutput(t, services, "--json", "session", "list")
	if err != nil {
		t.Fatalf("live list Execute() error = %v", err)
	}
	var live factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal([]byte(stdout), &live); err != nil {
		t.Fatalf("decode live list JSON: %v\n%s", err, stdout)
	}
	if len(live.Sessions) != 1 || live.Sessions[0].Id != "session-alpha" {
		t.Fatalf("live sessions = %#v", live.Sessions)
	}

	stdout, _, err = executeResolvedSessionWithOutput(t, services, "session", "list", "--scope", "persisted")
	if err != nil {
		t.Fatalf("persisted list Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "dur-sess-review-001") || !strings.Contains(stdout, "Factory Sessions (durable):") {
		t.Fatalf("persisted list output = %q", stdout)
	}
	stdout, _, err = executeResolvedSessionWithOutput(t, services, "session", "list")
	if err != nil {
		t.Fatalf("empty live list Execute() error = %v", err)
	}
	if stdout != "No live factory sessions were found.\n" {
		t.Fatalf("empty live list output = %q", stdout)
	}
	if requestCount != 2 {
		t.Fatalf("live HTTP request count = %d, want 2", requestCount)
	}
}

func TestSessionResolvedInspectionPreservesShowAndDispatches(t *testing.T) {
	var dispatchQuery string
	protocol := newSessionTestHTTPProtocol(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/factory-sessions/session-alpha":
			return sessionTestResponse(http.StatusOK, `{
				"id": "session-alpha",
				"project": "alpha",
				"factoryDir": "/workspace/fleet/alpha",
				"folderPath": "/workspace/fleet",
				"isDefault": false,
				"target": {"kind": "named", "name": "alpha"},
				"runtime": {"orchestratorKind": "PETRI", "status": "IDLE", "lifecycle": {
					"startedAt": "2026-07-27T00:00:00Z",
					"updatedAt": "2026-07-27T00:00:00Z"
				}}
			}`), nil
		case "/factory-sessions/dur-sess-review-001/dispatches":
			dispatchQuery = request.URL.RawQuery
			return sessionTestResponse(http.StatusOK, `{
				"sessionId": "dur-sess-review-001",
				"dispatches": [{
					"id": "dispatch-review",
					"status": "COMPLETED",
					"dispatchKind": "JAVASCRIPT_AGENT",
					"phase": "review"
				}]
			}`), nil
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
			return nil, nil
		}
	})
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Show:           sessioncli.NewShow(protocol),
		ListDispatches: sessioncli.NewDispatches(protocol),
	}, nil, func(cmd *cobra.Command) io.Writer {
		return cmd.ErrOrStderr()
	})

	stdout, stderr, err := executeResolvedSessionWithOutput(
		t, services, "--verbose", "--json", "session", "show", "session-alpha",
	)
	if err != nil {
		t.Fatalf("show Execute() error = %v", err)
	}
	var shown factoryapi.FactorySession
	if err := json.Unmarshal([]byte(stdout), &shown); err != nil {
		t.Fatalf("decode show JSON: %v\n%s", err, stdout)
	}
	if shown.Id != "session-alpha" || !strings.Contains(stderr, "session show request") {
		t.Fatalf("show result = %#v, stderr = %q", shown, stderr)
	}

	stdout, _, err = executeResolvedSessionWithOutput(
		t, services, "session", "dispatches", "dur-sess-review-001",
		"--phase", "review", "--status", "COMPLETED",
	)
	if err != nil {
		t.Fatalf("dispatches Execute() error = %v", err)
	}
	if !strings.Contains(stdout, "dispatch-review") ||
		dispatchQuery != "phase=review&status=COMPLETED" {
		t.Fatalf("dispatches output = %q, query = %q", stdout, dispatchQuery)
	}

	stdout, _, err = executeResolvedSessionWithOutput(
		t, services, "session", "dispatches", "session-alpha",
	)
	if err == nil || !strings.Contains(err.Error(), "not a durable Factory Session id") {
		t.Fatalf("live dispatches error = %v", err)
	}
	if stdout != "" {
		t.Fatalf("live dispatches stdout = %q, want empty", stdout)
	}
}

func TestSessionResolvedShowPreservesDurableProjection(t *testing.T) {
	protocol := newSessionTestHTTPProtocol(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/factory-sessions/dur-sess-review-001" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		return sessionTestResponse(http.StatusOK, `{
			"sessionId": "dur-sess-review-001",
			"status": "SUCCEEDED",
			"orchestratorKind": "JAVASCRIPT",
			"resolvedSource": {"kind": "WORKFLOW_FILE", "sourceRef": "workflows/review.js"}
		}`), nil
	})
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Show: sessioncli.NewShow(protocol),
	}, nil, nil)

	stdout, _, err := executeResolvedSessionWithOutput(
		t, services, "--json", "session", "show", "dur-sess-review-001",
	)
	if err != nil {
		t.Fatalf("durable show Execute() error = %v", err)
	}
	var shown factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal([]byte(stdout), &shown); err != nil {
		t.Fatalf("decode durable show JSON: %v\n%s", err, stdout)
	}
	if shown.SessionId != "dur-sess-review-001" ||
		shown.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("durable show result = %#v", shown)
	}
}

func TestSessionResolvedInspectionRejectsInvalidInputsBeforeSideEffects(t *testing.T) {
	var listCalls, showCalls, dispatchCalls int
	services := commandregistry.SessionResolvedServices{
		PrepareList: func(context.Context, *sessioncli.ListConfig) error {
			return errors.New("scope must be live, persisted, or all")
		},
		Sessions: sessioncli.Bind(sessioncli.Operations{
			List: func(sessioncli.ListConfig) error {
				listCalls++
				return nil
			},
			Show: func(cfg sessioncli.ShowConfig) error {
				showCalls++
				if cfg.SessionID != "" {
					t.Fatalf("default show SessionID = %q, want empty compatibility target", cfg.SessionID)
				}
				return errors.New("default session unavailable")
			},
			ListDispatches: func(sessioncli.DispatchesConfig) error {
				dispatchCalls++
				return nil
			},
		}),
	}
	stdout, _, err := executeResolvedSessionWithOutput(t, services, "session", "show")
	if err == nil || stdout != "" {
		t.Fatalf("default show error = %v, stdout = %q", err, stdout)
	}
	showCalls = 0
	for _, args := range [][]string{
		{"session", "list", "--scope", "workspace"},
		{"session", "show", "one", "two"},
		{"session", "dispatches"},
		{"session", "dispatches", "one", "two"},
	} {
		stdout, _, err := executeResolvedSessionWithOutput(t, services, args...)
		if err == nil {
			t.Fatalf("Execute(%v) error = nil", args)
		}
		if stdout != "" {
			t.Fatalf("Execute(%v) stdout = %q, want empty", args, stdout)
		}
	}
	if listCalls != 0 || showCalls != 0 || dispatchCalls != 0 {
		t.Fatalf("operation calls = list:%d show:%d dispatches:%d", listCalls, showCalls, dispatchCalls)
	}
}

func TestSessionResolvedHandlersRejectDeprecatedPortBeforeSideEffects(t *testing.T) {
	var operationCalls int
	services := commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
		Show:           func(sessioncli.ShowConfig) error { operationCalls++; return nil },
		ListDispatches: func(sessioncli.DispatchesConfig) error { operationCalls++; return nil },
		Pause:          func(sessioncli.LifecycleControlConfig) error { operationCalls++; return nil },
		Resume:         func(sessioncli.LifecycleControlConfig) error { operationCalls++; return nil },
	}, nil, nil)
	for _, args := range [][]string{
		{"session", "show", "session-alpha", "--port", "7444"},
		{"session", "dispatches", "dur-sess-review-001", "--port", "7444"},
		{"session", "pause", "--port", "7444"},
		{"session", "resume", "--port", "7444"},
	} {
		stdout, _, err := executeResolvedSessionWithOutput(t, services, args...)
		if err == nil || !strings.Contains(err.Error(), "--port is no longer supported") {
			t.Fatalf("Execute(%v) error = %v, want deprecated-port rejection", args, err)
		}
		if stdout != "" {
			t.Fatalf("Execute(%v) stdout = %q, want empty", args, stdout)
		}
	}
	if operationCalls != 0 {
		t.Fatalf("operation calls = %d, want zero", operationCalls)
	}
}

func TestSessionResolvedInspectionPreservesFailuresAndCancellation(t *testing.T) {
	operationFailure := errors.New("inspection unavailable")
	tests := []struct {
		name     string
		args     []string
		services commandregistry.SessionResolvedServices
		want     error
	}{
		{
			name: "list failure", args: []string{"session", "list"},
			services: commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
				List: func(sessioncli.ListConfig) error { return operationFailure },
			}, nil, nil),
			want: operationFailure,
		},
		{
			name: "show cancellation", args: []string{"session", "show", "session-alpha"},
			services: commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
				Show: func(sessioncli.ShowConfig) error { return context.Canceled },
			}, nil, nil),
			want: context.Canceled,
		},
		{
			name: "dispatch failure", args: []string{"session", "dispatches", "dur-sess-review-001"},
			services: commandregistry.SessionResolvedServicesFromOps(sessioncli.Operations{
				ListDispatches: func(sessioncli.DispatchesConfig) error { return operationFailure },
			}, nil, nil),
			want: operationFailure,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, _, err := executeResolvedSessionWithOutput(t, test.services, test.args...)
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
		})
	}
}

type sessionListPreparation func(fse.ListSessionsRequest) (fse.ListSessionsRequest, error)

func prepareSessionListRequest(
	request fse.ListSessionsRequest,
) (fse.ListSessionsRequest, error) {
	if request.Scope == "" {
		request.Scope = fse.SessionListScopeLive
	}
	switch request.Scope {
	case fse.SessionListScopeLive, fse.SessionListScopePersisted, fse.SessionListScopeAll:
		return request, nil
	default:
		return fse.ListSessionsRequest{}, errors.New("scope must be live, persisted, or all")
	}
}

func (prepare sessionListPreparation) PrepareListSessions(
	request fse.ListSessionsRequest,
) (fse.ListSessionsRequest, error) {
	return prepare(request)
}

func TestSessionCompatibilityInputsRetainResolvedProvenance(t *testing.T) {
	defaultLocal, defaultInherited := observeSessionCreateInputs(
		t, "session", "create", "--dir", "fleet",
	)
	assertSessionResolvedState(
		t, defaultLocal, "you.session.create.flag.port",
		resolvedinput.SourceManifestDefault, false, true,
	)
	assertSessionResolvedState(
		t, defaultInherited, "you.flag.server",
		resolvedinput.SourceManifestDefault, false, true,
	)

	changedLocal, changedInherited := observeSessionCreateInputs(
		t,
		"--server", "https://factory.example",
		"session", "create", "--dir", "fleet", "--port", "9444",
	)
	assertSessionResolvedState(
		t, changedLocal, "you.session.create.flag.dir",
		resolvedinput.SourceCLIFlag, true, false,
	)
	assertSessionResolvedState(
		t, changedLocal, "you.session.create.flag.port",
		resolvedinput.SourceCLIFlag, true, false,
	)
	assertSessionResolvedState(
		t, changedInherited, "you.flag.server",
		resolvedinput.SourceCLIFlag, true, false,
	)
}

func observeSessionCreateInputs(
	t *testing.T,
	args ...string,
) (resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	manifest := mustSessionManifest(t)
	registry := commandregistry.NewRegistry()
	var local, inherited resolvedinput.Inputs
	for _, record := range manifest.Commands {
		if !record.Runnable {
			continue
		}
		commandID := record.ID
		err := registry.RegisterResolved(
			record.Handler.ID,
			func(_ *cobra.Command, inputs, globals resolvedinput.Inputs) error {
				if commandID == "you.session.create" {
					local, inherited = inputs, globals
				}
				return nil
			},
		)
		if err != nil {
			t.Fatalf("RegisterResolved(%q) error = %v", record.Handler.ID, err)
		}
	}
	root, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
	if err != nil {
		t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v", err)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("session create Execute() error = %v", err)
	}
	return local, inherited
}

func assertSessionResolvedState(
	t *testing.T,
	inputs resolvedinput.Inputs,
	inputID string,
	provenance resolvedinput.Source,
	changed bool,
	defaulted bool,
) {
	t.Helper()
	state, ok := inputs.State(inputID)
	if !ok {
		t.Fatalf("resolved input %q state is missing", inputID)
	}
	if state.Provenance != provenance || state.Changed != changed || state.Default != defaulted {
		t.Fatalf("resolved input %q state = %#v", inputID, state)
	}
}

func executeResolvedSession(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
	args ...string,
) error {
	t.Helper()
	_, _, err := executeResolvedSessionWithOutput(t, services, args...)
	return err
}

func executeResolvedSessionWithOutput(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
	args ...string,
) (string, string, error) {
	t.Helper()
	manifest := mustSessionManifest(t)
	registry, err := commandregistry.NewSessionResolvedRegistry(manifest, services)
	if err != nil {
		t.Fatalf("NewSessionResolvedRegistry() error = %v", err)
	}
	root, err := climanifestcobra.NewSessionFamilyCommandFromManifest(manifest, registry)
	if err != nil {
		t.Fatalf("NewSessionFamilyCommandFromManifest() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err = root.Execute()
	return stdout.String(), stderr.String(), err
}

type sessionTestDoer func(*http.Request) (*http.Response, error)

func (do sessionTestDoer) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

type sessionTestClock struct{}

func (sessionTestClock) Now() time.Time { return time.Unix(1, 0) }

func newSessionTestHTTPProtocol(
	t *testing.T,
	do sessionTestDoer,
) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(do, sessionTestClock{})
	if err != nil {
		t.Fatalf("NewProtocol() error = %v", err)
	}
	return protocol
}

func newSessionCreateTestProtocol(
	t *testing.T,
	requests *[]factoryapi.OpenFactorySessionRequest,
) clihttp.Protocol {
	t.Helper()
	return newSessionTestHTTPProtocol(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/factory-sessions" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var payload factoryapi.OpenFactorySessionRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode create request: %v", err)
		}
		*requests = append(*requests, payload)
		return sessionTestResponse(http.StatusOK, `{
			"session": {
				"id": "session-alpha",
				"project": "alpha",
				"factoryDir": "/workspace/fleet/alpha",
				"folderPath": "/workspace/fleet",
				"isDefault": false,
				"target": {"kind": "named", "name": "alpha"}
			}
		}`), nil
	})
}

func assertNamedCreateRequest(
	t *testing.T,
	requests []factoryapi.OpenFactorySessionRequest,
) {
	t.Helper()
	if len(requests) != 1 {
		t.Fatalf("create request count = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.FolderPath != "/workspace/fleet" {
		t.Fatalf("create folder path = %q", request.FolderPath)
	}
	if request.InitNewFactory == nil || !*request.InitNewFactory {
		t.Fatalf("create initNewFactory = %#v, want true", request.InitNewFactory)
	}
	if request.Target == nil ||
		request.Target.Kind != factoryapi.FactorySessionTargetRefKindNamed ||
		request.Target.Name == nil || *request.Target.Name != "alpha" {
		t.Fatalf("create target = %#v, want named alpha", request.Target)
	}
}

func sessionTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func assertDefaultResolvedCreate(
	t *testing.T,
	cfg sessioncli.CreateConfig,
	diagnostics io.Writer,
) {
	t.Helper()
	if cfg.Dir != "fleet" || cfg.Port != 7437 ||
		cfg.PortExplicit || cfg.Server != "http://localhost:7437" ||
		cfg.InitNewFactory || cfg.ValidateOnly ||
		cfg.JSON || cfg.Verbose || cfg.Debug {
		t.Fatalf("default create config = %#v", cfg)
	}
	if cfg.Diagnostics != diagnostics || cfg.Output == nil {
		t.Fatalf("default create writers = output:%T diagnostics:%T", cfg.Output, cfg.Diagnostics)
	}
}

func assertChangedResolvedCreate(t *testing.T, cfg sessioncli.CreateConfig) {
	t.Helper()
	if cfg.Server != "https://factory.example" ||
		cfg.Port != 9444 || !cfg.PortExplicit ||
		!cfg.JSON || !cfg.Verbose || !cfg.Debug ||
		!cfg.InitNewFactory ||
		cfg.TargetKind != "named" || cfg.TargetName != "alpha" {
		t.Fatalf("changed create config = %#v", cfg)
	}
}

func assertDefaultResolvedList(t *testing.T, cfg sessioncli.ListConfig) {
	t.Helper()
	if cfg.Scope != "live" || cfg.Port != 7437 || cfg.Server != "" {
		t.Fatalf("default list config = %#v", cfg)
	}
}

func assertChangedResolvedList(t *testing.T, cfg sessioncli.ListConfig) {
	t.Helper()
	if cfg.Scope != "all" || cfg.Server != "https://factory.example" {
		t.Fatalf("changed list config = %#v", cfg)
	}
}

func assertResolvedDelete(t *testing.T, configs []sessioncli.DeleteConfig) {
	t.Helper()
	if len(configs) != 1 || configs[0].SessionID != "session-beta" ||
		configs[0].Port != 7437 || !configs[0].JSON {
		t.Fatalf("delete configs = %#v", configs)
	}
}

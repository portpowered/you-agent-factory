package climanifestparity_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/internal/testutil"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_SessionFamily(t *testing.T) {
	familyCases := sessionFamilyParityCases(t)

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustSessionConstructorRoots(t)
		for _, tc := range familyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyCmd := mustFindSessionCommand(t, legacyRoot, tc.path, "legacy")
				generatedCmd := mustFindSessionCommand(t, generatedRoot, tc.path, "generated")
				assertNoConstructorMismatches(t, climanifestparity.CompareConstructorIdentityParity(tc.commandID, legacyCmd, generatedCmd))
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		for _, tc := range familyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustSessionConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range familyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustSessionConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range sessionGeneratedVsLegacyParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustSessionConstructorRoots(t)
				mismatches := climanifestparity.CompareConstructorParseParity(
					tc.commandID, legacyRoot, generatedRoot, tc.argv, tc.wantParseErr, tc.errContains,
				)
				assertNoConstructorMismatches(t, mismatches)
				if tc.wantParseErr {
					return
				}

				legacyLeaf, _, legacyErr := climanifestparity.ParseArgvOnRoot(legacyRoot, tc.argv)
				if legacyErr != nil {
					t.Fatalf("legacy ParseArgvOnRoot(%v) error = %v", tc.argv, legacyErr)
				}
				generatedLeaf, _, generatedErr := climanifestparity.ParseArgvOnRoot(generatedRoot, tc.argv)
				if generatedErr != nil {
					t.Fatalf("generated ParseArgvOnRoot(%v) error = %v", tc.argv, generatedErr)
				}
				for _, flagLong := range tc.flagChecks {
					assertNoConstructorMismatches(t, climanifestparity.CompareConstructorFlagParity(tc.commandID, flagLong, legacyLeaf, generatedLeaf))
				}
				if tc.checkPreRun {
					assertNoConstructorMismatches(t, climanifestparity.CompareConstructorPreRunParity(tc.commandID, legacyLeaf, generatedLeaf, tc.errContains))
				}
			})
		}
	})
}

func TestGeneratedVsLegacySessionInvalidInputExecutionParity(t *testing.T) {
	for _, tc := range sessionInvalidExecutionCases() {
		t.Run(tc.name, func(t *testing.T) {
			legacyRoot, generatedRoot := mustSessionConstructorRoots(t)
			legacy := executeInvalidSessionInput(t, legacyRoot, tc.argv)
			generated := executeInvalidSessionInput(t, generatedRoot, tc.argv)

			if legacy.err == nil || generated.err == nil {
				t.Fatalf("errors: legacy=%v generated=%v, want both rejected before handler execution", legacy.err, generated.err)
			}
			if !strings.Contains(legacy.err.Error(), tc.errContains) || !strings.Contains(generated.err.Error(), tc.errContains) {
				t.Fatalf("errors: legacy=%q generated=%q, want class %q", legacy.err, generated.err, tc.errContains)
			}
			if legacy.err.Error() != generated.err.Error() {
				t.Fatalf("error text mismatch: legacy=%q generated=%q", legacy.err, generated.err)
			}
			if legacy.stdout != generated.stdout || legacy.stderr != generated.stderr {
				t.Fatalf("output mismatch:\nlegacy stdout=%q stderr=%q\ngenerated stdout=%q stderr=%q", legacy.stdout, legacy.stderr, generated.stdout, generated.stderr)
			}
		})
	}
}

func TestGeneratedVsLegacySessionShellCompletionParity(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		contains []string
	}{
		{
			name: "session command names",
			argv: []string{"session", ""},
			contains: []string{
				"create", "delete", "dispatches", "list", "pause", "resume", "show",
			},
		},
		{name: "create required flag", argv: []string{"session", "create", "--"}, contains: []string{"--dir"}},
		{name: "create remaining flags", argv: []string{"session", "create", "--dir", ".", "--"}, contains: []string{"--init-new-factory", "--validate-only", "--target-kind", "--target-name"}},
		{name: "list scope values or no-option directive", argv: []string{"session", "list", "--scope", ""}},
		{name: "show argument completion", argv: []string{"session", "show", ""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacyRoot, generatedRoot := mustSessionConstructorRoots(t)
			legacy := executeSessionCompletion(t, legacyRoot, tc.argv)
			generated := executeSessionCompletion(t, generatedRoot, tc.argv)
			if legacy.err != nil || generated.err != nil {
				t.Fatalf("completion errors: legacy=%v generated=%v", legacy.err, generated.err)
			}
			if legacy.stdout != generated.stdout || legacy.stderr != generated.stderr {
				t.Fatalf("completion mismatch:\nlegacy stdout=%q stderr=%q\ngenerated stdout=%q stderr=%q", legacy.stdout, legacy.stderr, generated.stdout, generated.stderr)
			}
			for _, want := range tc.contains {
				if !strings.Contains(legacy.stdout, want) {
					t.Fatalf("completion output missing %q:\n%s", want, legacy.stdout)
				}
			}
		})
	}
}

type sessionFamilyParityCase struct {
	commandID string
	path      string
}

func sessionFamilyParityCases(t *testing.T) []sessionFamilyParityCase {
	t.Helper()
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
	}
	cases := make([]sessionFamilyParityCase, 0, len(climanifestgen.SessionFamilyCommandIDs))
	for _, commandID := range climanifestgen.SessionFamilyCommandIDs {
		record, lookupErr := manifest.CommandByID(commandID)
		if lookupErr != nil {
			t.Fatalf("CommandByID(%q) error = %v", commandID, lookupErr)
		}
		cases = append(cases, sessionFamilyParityCase{commandID: commandID, path: record.Path})
	}
	return cases
}

type sessionParsingParityCase struct {
	name         string
	commandID    string
	argv         []string
	wantParseErr bool
	errContains  string
	flagChecks   []string
	checkPreRun  bool
}

func sessionGeneratedVsLegacyParsingCases() []sessionParsingParityCase {
	cases := make([]sessionParsingParityCase, 0)
	cases = append(cases, sessionCreateListParsingCases()...)
	cases = append(cases, sessionShowControlParsingCases()...)
	cases = append(cases, sessionDeleteDispatchesParsingCases()...)
	return cases
}

func sessionCreateListParsingCases() []sessionParsingParityCase {
	return []sessionParsingParityCase{
		{name: "create rejects missing required dir", commandID: "you.session.create", argv: []string{"session", "create"}, wantParseErr: true, errContains: "required flag(s)"},
		{name: "create accepts unbounded legacy positionals", commandID: "you.session.create", argv: []string{"session", "create", "one", "two", "--dir", "."}, flagChecks: []string{"dir"}},
		{name: "create accepts target selection", commandID: "you.session.create", argv: []string{"session", "create", "--dir", ".", "--target-kind", "named", "--target-name", "beta"}, flagChecks: []string{"target-kind", "target-name"}},
		{name: "create rejects mutually exclusive modes", commandID: "you.session.create", argv: []string{"session", "create", "--dir", ".", "--init-new-factory", "--validate-only"}, wantParseErr: true, errContains: "none of the others can be"},
		{name: "create preserves port and json values", commandID: "you.session.create", argv: []string{"session", "create", "--dir", ".", "--port", "9090", "--json"}, flagChecks: []string{"port", "json"}},
		{name: "list preserves live scope default", commandID: "you.session.list", argv: []string{"session", "list"}, flagChecks: []string{"scope", "port", "json"}},
		{name: "list accepts persisted scope", commandID: "you.session.list", argv: []string{"session", "list", "--scope", "persisted"}, flagChecks: []string{"scope"}},
		{name: "list accepts all scope with local output flags", commandID: "you.session.list", argv: []string{"session", "list", "--scope", "all", "--port", "9090", "--json"}, flagChecks: []string{"scope", "port", "json"}},
		{name: "list accepts unbounded legacy positionals", commandID: "you.session.list", argv: []string{"session", "list", "one", "two"}},
	}
}

func sessionShowControlParsingCases() []sessionParsingParityCase {
	return []sessionParsingParityCase{
		{name: "show accepts omitted default session", commandID: "you.session.show", argv: []string{"session", "show"}, flagChecks: []string{"server", "json", "port"}},
		{name: "show accepts named session and inherited flags", commandID: "you.session.show", argv: []string{"--server", "http://127.0.0.1:9090", "--json", "session", "show", "session-beta"}, flagChecks: []string{"server", "json"}},
		{name: "show rejects excess positionals", commandID: "you.session.show", argv: []string{"session", "show", "one", "two"}, wantParseErr: true, errContains: "accepts at most 1 arg"},
		{name: "show rejects deprecated port", commandID: "you.session.show", argv: []string{"session", "show", "--port", "9090"}, errContains: "--port is no longer supported", flagChecks: []string{"port"}, checkPreRun: true},
		{name: "pause accepts omitted default session", commandID: "you.session.pause", argv: []string{"session", "pause"}, flagChecks: []string{"server", "json", "port"}},
		{name: "pause accepts durable session", commandID: "you.session.pause", argv: []string{"session", "pause", "dur-sess-js-run-n-001"}},
		{name: "pause rejects excess positionals", commandID: "you.session.pause", argv: []string{"session", "pause", "one", "two"}, wantParseErr: true, errContains: "accepts at most 1 arg"},
		{name: "resume accepts omitted default session", commandID: "you.session.resume", argv: []string{"session", "resume"}, flagChecks: []string{"server", "json", "port"}},
		{name: "resume accepts named session and server", commandID: "you.session.resume", argv: []string{"--server", "http://127.0.0.1:9090", "session", "resume", "session-beta"}, flagChecks: []string{"server"}},
		{name: "resume rejects deprecated port", commandID: "you.session.resume", argv: []string{"session", "resume", "--port", "9090"}, errContains: "--port is no longer supported", flagChecks: []string{"port"}, checkPreRun: true},
	}
}

func sessionDeleteDispatchesParsingCases() []sessionParsingParityCase {
	return []sessionParsingParityCase{
		{name: "delete requires exact session id", commandID: "you.session.delete", argv: []string{"session", "delete"}, wantParseErr: true, errContains: "accepts 1 arg(s)"},
		{name: "delete accepts local port and json", commandID: "you.session.delete", argv: []string{"session", "delete", "session-beta", "--port", "9090", "--json"}, flagChecks: []string{"port", "json"}},
		{name: "delete rejects excess positionals", commandID: "you.session.delete", argv: []string{"session", "delete", "one", "two"}, wantParseErr: true, errContains: "accepts 1 arg(s)"},
		{name: "dispatches requires durable session positional", commandID: "you.session.dispatches", argv: []string{"session", "dispatches"}, wantParseErr: true, errContains: "accepts 1 arg(s)"},
		{name: "dispatches accepts filters and inherited server", commandID: "you.session.dispatches", argv: []string{"--server", "http://127.0.0.1:9090", "session", "dispatches", "dur-sess-js-run-n-001", "--phase", "review", "--status", "RUNNING"}, flagChecks: []string{"server", "phase", "status"}},
		{name: "dispatches rejects deprecated port", commandID: "you.session.dispatches", argv: []string{"session", "dispatches", "dur-sess-js-run-n-001", "--port", "9090"}, errContains: "--port is no longer supported", flagChecks: []string{"port"}, checkPreRun: true},
		{name: "session leaf rejects unknown flag", commandID: "you.session.show", argv: []string{"session", "show", "--unknown"}, wantParseErr: true, errContains: "unknown flag"},
	}
}

type invalidSessionExecutionCase struct {
	name        string
	argv        []string
	errContains string
}

func sessionInvalidExecutionCases() []invalidSessionExecutionCase {
	return []invalidSessionExecutionCase{
		{name: "missing required create directory", argv: []string{"session", "create"}, errContains: "required flag(s)"},
		{name: "mutually exclusive create modes", argv: []string{"session", "create", "--dir", ".", "--init-new-factory", "--validate-only"}, errContains: "none of the others can be"},
		{name: "missing dispatch session", argv: []string{"session", "dispatches"}, errContains: "accepts 1 arg(s)"},
		{name: "excess show positionals", argv: []string{"session", "show", "one", "two"}, errContains: "accepts at most 1 arg"},
		{name: "unknown show flag", argv: []string{"session", "show", "--unknown"}, errContains: "unknown flag"},
		{name: "deprecated show port", argv: []string{"session", "show", "--port", "9090"}, errContains: "--port is no longer supported"},
	}
}

type invalidSessionExecution struct {
	stdout string
	stderr string
	err    error
}

func executeInvalidSessionInput(t *testing.T, root *cobra.Command, argv []string) invalidSessionExecution {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return invalidSessionExecution{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func executeSessionCompletion(t *testing.T, root *cobra.Command, argv []string) invalidSessionExecution {
	t.Helper()
	completionArgv := append([]string{"__complete"}, argv...)
	return executeInvalidSessionInput(t, root, completionArgv)
}

func mustSessionConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	legacyRoot := cli.NewLegacySessionFamilyCommand(cli.RootCommandOptions{})
	generatedRoot, err := cli.NewGeneratedSessionFamilyCommand(cli.RootCommandOptions{})
	if err != nil {
		t.Fatalf("NewGeneratedSessionFamilyCommand() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func mustFindSessionCommand(t *testing.T, root *cobra.Command, path, label string) *cobra.Command {
	t.Helper()
	cmd, err := climanifestparity.FindCommandByPath(root, path)
	if err != nil {
		t.Fatalf("%s FindCommandByPath(%q) error = %v", label, path, err)
	}
	return cmd
}

func TestProductionManifestHandlerBinding_SessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	record, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}

	mismatches := climanifestparity.CompareDeclaredHandler(record, climanifestparity.SessionShowHandlerID, climanifestparity.SessionShowOperationID)
	mismatches = append(mismatches, climanifestparity.CompareHandlerOpenAPIBinding(
		record,
		loadBundledOpenAPIContract(t),
		climanifestparity.SessionShowHTTPMethod,
		climanifestparity.SessionShowHTTPPath,
	)...)
	if len(mismatches) > 0 {
		t.Fatalf("contract handler/OpenAPI binding drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}
}

func TestProductionManifestHandlerBinding_SessionShowLiveServiceCall(t *testing.T) {
	var gotPaths []string
	var gotMethods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethods = append(gotMethods, r.Method)
		gotPaths = append(gotPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"session-beta","runtime":{"orchestratorKind":"JAVASCRIPT"}}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := sessioncli.Show(sessioncli.ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if len(gotPaths) == 0 || gotPaths[0] != "/factory-sessions/session-beta" {
		t.Fatalf("live HTTP paths = %#v, want primary GET /factory-sessions/session-beta for contracted %s", gotPaths, climanifestparity.SessionShowHTTPPath)
	}
	if len(gotMethods) == 0 || gotMethods[0] != climanifestparity.SessionShowHTTPMethod {
		t.Fatalf("live HTTP methods = %#v, want primary %s for contracted %s binding", gotMethods, climanifestparity.SessionShowHTTPMethod, climanifestparity.SessionShowOperationID)
	}
}

func loadBundledOpenAPIContract(t *testing.T) *openapi3.T {
	t.Helper()
	openAPIPath := testutil.MustRepoPath(t, "api/openapi.yaml")
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(openAPIPath)
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	return doc
}

func TestGeneratedVsLegacySessionCreateListDeleteExecutionParity(t *testing.T) {
	for _, tc := range sessionExecutionParityCases() {
		t.Run(tc.name, func(t *testing.T) {
			var requests []sessionHTTPExecutionRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				}
				requests = append(requests, sessionHTTPExecutionRequest{method: r.Method, path: r.URL.EscapedPath(), body: string(body)})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()

			var buildRequests []sessionexecutioncli.ServiceRequest
			options := sessionExecutionOptions(t, &buildRequests)
			argv := tc.argv(sessionServerPort(t, server))
			legacy, generated := executeSessionFamilyPair(t, options, argv)

			assertSessionCommandResultParity(t, legacy, generated, tc.wantError)
			assertSessionHTTPRequests(t, requests, tc)
			assertSessionBuilderRequests(t, buildRequests, tc.wantBuilderCalls)
		})
	}
}

func TestGeneratedVsLegacySessionInspectionControlExecutionParity(t *testing.T) {
	for _, tc := range sessionInspectionControlParityCases() {
		t.Run(tc.name, func(t *testing.T) {
			var requests []sessionHTTPExecutionRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				}
				requests = append(requests, sessionHTTPExecutionRequest{
					method: r.Method, path: r.URL.EscapedPath(), query: r.URL.RawQuery, body: string(body),
				})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()

			legacy, generated := executeSessionFamilyPair(t, cli.RootCommandOptions{}, tc.argv(server.URL))

			assertSessionCommandResultParity(t, legacy, generated, tc.wantError)
			assertSessionInspectionControlRequests(t, requests, tc)
		})
	}
}

func TestGeneratedVsLegacySessionInspectionControlUnreachableParity(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	serverURL := server.URL
	server.Close()

	for _, tc := range []struct {
		name string
		argv []string
	}{
		{name: "show", argv: []string{"--server", serverURL, "session", "show", "session-beta"}},
		{name: "dispatches", argv: []string{"--server", serverURL, "session", "dispatches", "dur-sess-js-run-n-001"}},
		{name: "pause", argv: []string{"--server", serverURL, "session", "pause", "session-beta"}},
		{name: "resume", argv: []string{"--server", serverURL, "session", "resume", "dur-sess-js-run-n-001"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy, generated := executeSessionFamilyPair(t, cli.RootCommandOptions{}, tc.argv)
			assertSessionCommandResultParity(t, legacy, generated, true)
		})
	}
}

type sessionInspectionControlParityCase struct {
	name       string
	argv       func(string) []string
	status     int
	response   string
	wantMethod string
	wantPaths  []string
	wantQuery  string
	wantError  bool
}

func sessionInspectionControlParityCases() []sessionInspectionControlParityCase {
	liveSession := `{"id":"session-beta","project":"beta","factoryDir":"/workspace/fleet/beta","folderPath":"/workspace/fleet","isDefault":false,"target":{"kind":"named","name":"beta"},"runtime":{"orchestratorKind":"PETRI_NET","status":"IDLE","lifecycle":{"startedAt":"2026-07-15T08:00:00Z","updatedAt":"2026-07-15T08:01:00Z"},"progress":{"factoryState":"RUNNING","categories":{},"inFlightCount":0,"totalTokens":1},"usage":{"resources":[]}}}`
	durableSession := `{"sessionId":"dur-sess-js-run-n-001","status":"RUNNING","orchestratorKind":"JAVASCRIPT","resolvedSource":{"kind":"WORKFLOW_NAME","sourceRef":"workflow/release-train"},"phase":"verify","progress":{"totalDispatches":3,"completedDispatches":1,"inFlightDispatches":1},"usage":{"resources":[]}}`
	dispatches := `{"sessionId":"dur-sess-js-run-n-001","dispatches":[{"id":"disp-js-002","status":"RUNNING","dispatchKind":"JAVASCRIPT_VERIFY","phase":"verify","label":"verify-release","runnerId":"runner-1","model":"model-1","providerSessionRefs":[{"id":"provider-session-1","kind":"SESSION_ID","provider":"CLAUDE"}],"attempt":1,"usage":{"durationMillis":1250},"outputArtifactIds":["artifact-1"]}]}`
	return []sessionInspectionControlParityCase{
		{
			name: "show named live human preserves projection requests",
			argv: func(server string) []string {
				return []string{"--verbose", "--server", server, "session", "show", "session-beta"}
			},
			status: http.StatusOK, response: liveSession, wantMethod: http.MethodGet,
			wantPaths: []string{"/factory-sessions/session-beta", "/factory-sessions/session-beta/partial-result", "/factory-sessions/session-beta/result"},
		},
		{
			name: "show durable json preserves durable projection",
			argv: func(server string) []string {
				return []string{"--json", "--server", server, "session", "show", "dur-sess-js-run-n-001"}
			},
			status: http.StatusOK, response: durableSession, wantMethod: http.MethodGet,
			wantPaths: []string{"/factory-sessions/dur-sess-js-run-n-001"},
		},
		{
			name: "show omitted session preserves default route failure",
			argv: func(server string) []string {
				return []string{"--verbose", "--server", server, "session", "show"}
			},
			status: http.StatusNotFound, response: `{"code":"NOT_FOUND","message":"factory session not found"}`,
			wantMethod: http.MethodGet, wantPaths: []string{"/factory-sessions/~default"}, wantError: true,
		},
		{
			name: "dispatches human preserves filters provider sessions and artifacts",
			argv: func(server string) []string {
				return []string{"--verbose", "--server", server, "session", "dispatches", "dur-sess-js-run-n-001", "--phase", "verify", "--status", "RUNNING"}
			},
			status: http.StatusOK, response: dispatches, wantMethod: http.MethodGet,
			wantPaths: []string{"/factory-sessions/dur-sess-js-run-n-001/dispatches"}, wantQuery: "phase=verify&status=RUNNING",
		},
		{
			name: "dispatches json preserves API response",
			argv: func(server string) []string {
				return []string{"--json", "--server", server, "session", "dispatches", "dur-sess-js-run-n-001"}
			},
			status: http.StatusOK, response: dispatches, wantMethod: http.MethodGet,
			wantPaths: []string{"/factory-sessions/dur-sess-js-run-n-001/dispatches"},
		},
		{
			name: "dispatches not found preserves failure",
			argv: func(server string) []string {
				return []string{"--server", server, "session", "dispatches", "dur-sess-missing"}
			},
			status: http.StatusNotFound, response: `{"code":"NOT_FOUND","message":"factory session not found"}`,
			wantMethod: http.MethodGet, wantPaths: []string{"/factory-sessions/dur-sess-missing/dispatches"}, wantError: true,
		},
		{
			name: "pause omitted session preserves default accepted human outcome",
			argv: func(server string) []string {
				return []string{"--verbose", "--server", server, "session", "pause"}
			},
			status:     http.StatusAccepted,
			response:   `{"sessionId":"~default","operation":"PAUSE","outcome":"ACCEPTED","status":"PAUSED"}`,
			wantMethod: http.MethodPost, wantPaths: []string{"/factory-sessions/~default/pause"},
		},
		{
			name: "pause durable json preserves no-op outcome",
			argv: func(server string) []string {
				return []string{"--json", "--server", server, "session", "pause", "dur-sess-js-run-n-001"}
			},
			status:     http.StatusOK,
			response:   `{"sessionId":"dur-sess-js-run-n-001","operation":"PAUSE","outcome":"NO_OP","status":"PAUSED"}`,
			wantMethod: http.MethodPost, wantPaths: []string{"/factory-sessions/dur-sess-js-run-n-001/pause"},
		},
		{
			name: "resume named live json preserves accepted outcome",
			argv: func(server string) []string {
				return []string{"--json", "--server", server, "session", "resume", "session-beta"}
			},
			status:     http.StatusAccepted,
			response:   `{"sessionId":"session-beta","operation":"RESUME","outcome":"ACCEPTED","status":"RUNNING"}`,
			wantMethod: http.MethodPost, wantPaths: []string{"/factory-sessions/session-beta/resume"},
		},
		{
			name: "resume durable rejection preserves typed response and failure",
			argv: func(server string) []string {
				return []string{"--verbose", "--server", server, "session", "resume", "dur-sess-js-run-n-001"}
			},
			status:     http.StatusConflict,
			response:   `{"sessionId":"dur-sess-js-run-n-001","operation":"RESUME","outcome":"INVALID_STATE","status":"RUNNING","detail":"session is not paused"}`,
			wantMethod: http.MethodPost, wantPaths: []string{"/factory-sessions/dur-sess-js-run-n-001/resume"}, wantError: true,
		},
	}
}

type sessionExecutionParityCase struct {
	name             string
	argv             func(int) []string
	status           int
	response         string
	wantMethod       string
	wantPath         string
	wantBodyContains []string
	wantRequestCount int
	wantBuilderCalls int
	wantError        bool
}

func sessionExecutionParityCases() []sessionExecutionParityCase {
	return []sessionExecutionParityCase{
		{
			name: "create human success preserves target request",
			argv: func(port int) []string {
				return []string{"--verbose", "session", "create", "--dir", "/workspace/fleet", "--port", strconv.Itoa(port), "--init-new-factory", "--target-kind", "named", "--target-name", "beta"}
			},
			status:     http.StatusOK,
			response:   `{"session":{"id":"session-beta","project":"beta","factoryDir":"/workspace/fleet/beta","folderPath":"/workspace/fleet","isDefault":false,"target":{"kind":"named","name":"beta"}}}`,
			wantMethod: http.MethodPost, wantPath: "/factory-sessions", wantRequestCount: 2,
			wantBodyContains: []string{`"folderPath":"/workspace/fleet"`, `"initNewFactory":true`, `"kind":"named"`, `"name":"beta"`},
		},
		{
			name: "create json validation preserves exclusion request",
			argv: func(port int) []string {
				return []string{"session", "create", "--dir", "/workspace/fleet", "--port", strconv.Itoa(port), "--validate-only", "--json"}
			},
			status: http.StatusOK, response: `{}`, wantMethod: http.MethodPost, wantPath: "/factory-sessions", wantRequestCount: 2,
			wantBodyContains: []string{`"validateOnly":true`},
		},
		{
			name: "create target selection failure preserves json and error",
			argv: func(port int) []string {
				return []string{"session", "create", "--dir", "/workspace/fleet", "--port", strconv.Itoa(port), "--json"}
			},
			status:     http.StatusOK,
			response:   `{"targets":[{"label":"beta","ref":{"kind":"named","name":"beta"}}]}`,
			wantMethod: http.MethodPost, wantPath: "/factory-sessions", wantRequestCount: 2, wantError: true,
		},
		{
			name: "list default live human preserves GET",
			argv: func(port int) []string {
				return []string{"--verbose", "session", "list", "--port", strconv.Itoa(port)}
			},
			status:     http.StatusOK,
			response:   `{"sessions":[{"id":"session-beta","project":"beta","factoryDir":"/workspace/fleet/beta","folderPath":"/workspace/fleet","isDefault":false}]}`,
			wantMethod: http.MethodGet, wantPath: "/factory-sessions", wantRequestCount: 2,
		},
		{
			name: "list live json preserves API output",
			argv: func(port int) []string {
				return []string{"session", "list", "--scope", "live", "--port", strconv.Itoa(port), "--json"}
			},
			status: http.StatusOK, response: `{"sessions":[]}`, wantMethod: http.MethodGet, wantPath: "/factory-sessions", wantRequestCount: 2,
		},
		{
			name: "list server failure preserves diagnostics and error",
			argv: func(port int) []string {
				return []string{"--verbose", "session", "list", "--port", strconv.Itoa(port)}
			},
			status: http.StatusInternalServerError, response: `{"code":"INTERNAL","message":"list unavailable"}`,
			wantMethod: http.MethodGet, wantPath: "/factory-sessions", wantRequestCount: 2, wantError: true,
		},
		{
			name: "list persisted uses durable collaborator only",
			argv: func(_ int) []string {
				return []string{"session", "list", "--scope", "persisted", "--json"}
			},
			status: http.StatusOK, wantBuilderCalls: 2,
		},
		{
			name: "delete human success preserves exact session path",
			argv: func(port int) []string {
				return []string{"--verbose", "session", "delete", "session-beta", "--port", strconv.Itoa(port)}
			},
			status: http.StatusNoContent, wantMethod: http.MethodDelete, wantPath: "/factory-sessions/session-beta", wantRequestCount: 2,
		},
		{
			name: "delete json success preserves confirmation",
			argv: func(port int) []string {
				return []string{"session", "delete", "session-beta", "--port", strconv.Itoa(port), "--json"}
			},
			status: http.StatusNoContent, wantMethod: http.MethodDelete, wantPath: "/factory-sessions/session-beta", wantRequestCount: 2,
		},
		{
			name: "delete not found preserves diagnostic error",
			argv: func(port int) []string {
				return []string{"--verbose", "session", "delete", "missing-session", "--port", strconv.Itoa(port)}
			},
			status: http.StatusNotFound, response: `{"code":"NOT_FOUND","message":"factory session not found"}`,
			wantMethod: http.MethodDelete, wantPath: "/factory-sessions/missing-session", wantRequestCount: 2, wantError: true,
		},
	}
}

type sessionHTTPExecutionRequest struct {
	method string
	path   string
	query  string
	body   string
}

type sessionCommandResult struct {
	stdout string
	stderr string
	err    error
}

func sessionExecutionOptions(t *testing.T, requests *[]sessionexecutioncli.ServiceRequest) cli.RootCommandOptions {
	t.Helper()
	service, err := fse.NewFakeServiceFromContractFixtures(testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json"))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures() error = %v", err)
	}
	return cli.RootCommandOptions{BuildSessionExecution: func(_ context.Context, request sessionexecutioncli.ServiceRequest) (sessionexecutioncli.ServiceOwner, error) {
		*requests = append(*requests, request)
		return parityExecutionOwner{Service: service}, nil
	}}
}

type parityExecutionOwner struct {
	fse.Service
}

func (parityExecutionOwner) Close() error { return nil }

func executeSessionFamilyPair(t *testing.T, options cli.RootCommandOptions, argv []string) (sessionCommandResult, sessionCommandResult) {
	t.Helper()
	legacy := cli.NewLegacySessionFamilyCommand(options)
	generated, err := cli.NewGeneratedSessionFamilyCommand(options)
	if err != nil {
		t.Fatalf("NewGeneratedSessionFamilyCommand() error = %v", err)
	}
	return executeSessionCommand(t, legacy, argv), executeSessionCommand(t, generated, argv)
}

func executeSessionCommand(t *testing.T, root *cobra.Command, argv []string) sessionCommandResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return sessionCommandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

var durationMillisPattern = regexp.MustCompile(`durationMillis=\d+`)

func assertSessionCommandResultParity(t *testing.T, legacy, generated sessionCommandResult, wantError bool) {
	t.Helper()
	if legacy.stdout != generated.stdout {
		t.Fatalf("stdout mismatch:\nlegacy=%q\ngenerated=%q", legacy.stdout, generated.stdout)
	}
	legacyStderr := durationMillisPattern.ReplaceAllString(legacy.stderr, "durationMillis=<elapsed>")
	generatedStderr := durationMillisPattern.ReplaceAllString(generated.stderr, "durationMillis=<elapsed>")
	if legacyStderr != generatedStderr {
		t.Fatalf("stderr mismatch:\nlegacy=%q\ngenerated=%q", legacyStderr, generatedStderr)
	}
	if (legacy.err != nil) != wantError || (generated.err != nil) != wantError {
		t.Fatalf("errors: legacy=%v generated=%v, wantError=%t", legacy.err, generated.err, wantError)
	}
	if wantError && legacy.err.Error() != generated.err.Error() {
		t.Fatalf("error mismatch: legacy=%q generated=%q", legacy.err, generated.err)
	}
}

func assertSessionHTTPRequests(t *testing.T, requests []sessionHTTPExecutionRequest, tc sessionExecutionParityCase) {
	t.Helper()
	if len(requests) != tc.wantRequestCount {
		t.Fatalf("HTTP requests = %#v, want %d", requests, tc.wantRequestCount)
	}
	if len(requests) == 0 {
		return
	}
	if requests[0] != requests[1] {
		t.Fatalf("HTTP request mismatch: legacy=%#v generated=%#v", requests[0], requests[1])
	}
	if requests[0].method != tc.wantMethod || requests[0].path != tc.wantPath {
		t.Fatalf("HTTP request = %s %s, want %s %s", requests[0].method, requests[0].path, tc.wantMethod, tc.wantPath)
	}
	for _, fragment := range tc.wantBodyContains {
		if !strings.Contains(requests[0].body, fragment) {
			t.Fatalf("HTTP body missing %q: %s", fragment, requests[0].body)
		}
	}
}

func assertSessionInspectionControlRequests(
	t *testing.T,
	requests []sessionHTTPExecutionRequest,
	tc sessionInspectionControlParityCase,
) {
	t.Helper()
	wantCount := len(tc.wantPaths) * 2
	if len(requests) != wantCount {
		t.Fatalf("HTTP requests = %#v, want %d", requests, wantCount)
	}
	legacy, generated := requests[:len(tc.wantPaths)], requests[len(tc.wantPaths):]
	for i := range legacy {
		if legacy[i] != generated[i] {
			t.Fatalf("HTTP request %d mismatch: legacy=%#v generated=%#v", i, legacy[i], generated[i])
		}
		if legacy[i].method != tc.wantMethod || legacy[i].path != tc.wantPaths[i] {
			t.Fatalf("HTTP request %d = %s %s, want %s %s", i, legacy[i].method, legacy[i].path, tc.wantMethod, tc.wantPaths[i])
		}
	}
	if legacy[0].query != tc.wantQuery {
		t.Fatalf("HTTP query = %q, want %q", legacy[0].query, tc.wantQuery)
	}
}

func assertSessionBuilderRequests(t *testing.T, requests []sessionexecutioncli.ServiceRequest, want int) {
	t.Helper()
	if len(requests) != want {
		t.Fatalf("durable builder requests = %#v, want %d calls", requests, want)
	}
	if len(requests) == 2 && requests[0] != requests[1] {
		t.Fatalf("durable builder request mismatch: legacy=%#v generated=%#v", requests[0], requests[1])
	}
}

func sessionServerPort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	_, rawPort, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("split server address %q: %v", server.URL, err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatalf("parse server port %q: %v", rawPort, err)
	}
	return port
}

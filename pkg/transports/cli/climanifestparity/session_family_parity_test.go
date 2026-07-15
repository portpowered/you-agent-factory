package climanifestparity_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
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

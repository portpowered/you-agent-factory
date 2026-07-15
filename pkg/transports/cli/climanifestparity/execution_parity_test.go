package climanifestparity_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_WorkflowMCPFamily(t *testing.T) {
	cases := []struct {
		commandID string
		path      string
	}{
		{commandID: "you.mcp", path: "you mcp"},
		{commandID: "you.mcp.serve", path: "you mcp serve"},
		{commandID: "you.workflow.preview", path: "you workflow preview"},
		{commandID: "you.workflow.validate", path: "you workflow validate"},
	}

	for _, comparison := range []struct {
		name string
		run  func(*testing.T, string, string, *cobra.Command, *cobra.Command)
	}{
		{name: "identity", run: func(t *testing.T, commandID, path string, legacy, generated *cobra.Command) {
			legacyCmd, err := climanifestparity.FindCommandByPath(legacy, path)
			if err != nil {
				t.Fatalf("legacy FindCommandByPath(%q) error = %v", path, err)
			}
			generatedCmd, err := climanifestparity.FindCommandByPath(generated, path)
			if err != nil {
				t.Fatalf("generated FindCommandByPath(%q) error = %v", path, err)
			}
			assertNoConstructorMismatches(t, climanifestparity.CompareConstructorIdentityParity(commandID, legacyCmd, generatedCmd))
		}},
		{name: "help", run: func(t *testing.T, commandID, path string, legacy, generated *cobra.Command) {
			mismatches, err := climanifestparity.CompareConstructorHelpParity(commandID, legacy, generated, path)
			if err != nil {
				t.Fatalf("CompareConstructorHelpParity(%q) error = %v", path, err)
			}
			assertNoConstructorMismatches(t, mismatches)
		}},
		{name: "completion", run: func(t *testing.T, commandID, path string, legacy, generated *cobra.Command) {
			mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(commandID, path, legacy, generated)
			if err != nil {
				t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", path, err)
			}
			assertNoConstructorMismatches(t, mismatches)
		}},
	} {
		t.Run(comparison.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.commandID, func(t *testing.T) {
					legacy, generated := mustWorkflowMCPConstructorRoots(t)
					comparison.run(t, tc.commandID, tc.path, legacy, generated)
				})
			}
		})
	}

	parsingCases := []struct {
		name        string
		commandID   string
		argv        []string
		flagChecks  []string
		wantErr     bool
		errContains string
	}{
		{name: "mcp defaults", commandID: "you.mcp.serve", argv: []string{"mcp", "serve"}, flagChecks: []string{"runtime", "fixture-catalog", "project-root"}},
		{name: "mcp explicit runtime", commandID: "you.mcp.serve", argv: []string{"mcp", "serve", "--runtime", "--project-root", "project"}, flagChecks: []string{"runtime", "project-root"}},
		{name: "workflow validate source", commandID: "you.workflow.validate", argv: []string{"workflow", "validate", "--kind", "WORKFLOW_NAME", "--value", "review"}, flagChecks: []string{"kind", "value", "dir"}},
		{name: "workflow preview inline", commandID: "you.workflow.preview", argv: []string{"--json", "workflow", "preview", "--kind", "INLINE_WORKFLOW", "--inline", "phase('setup');"}, flagChecks: []string{"json", "kind", "inline"}},
		{name: "unknown workflow flag", commandID: "you.workflow.validate", argv: []string{"workflow", "validate", "--unknown"}, wantErr: true, errContains: "unknown flag"},
	}
	for _, tc := range parsingCases {
		t.Run("parsing/"+tc.name, func(t *testing.T) {
			legacy, generated := mustWorkflowMCPConstructorRoots(t)
			assertNoConstructorMismatches(t, climanifestparity.CompareConstructorParseParity(tc.commandID, legacy, generated, tc.argv, tc.wantErr, tc.errContains))
			if tc.wantErr {
				return
			}
			legacyLeaf, _, _ := climanifestparity.ParseArgvOnRoot(legacy, tc.argv)
			generatedLeaf, _, _ := climanifestparity.ParseArgvOnRoot(generated, tc.argv)
			for _, flag := range tc.flagChecks {
				assertNoConstructorMismatches(t, climanifestparity.CompareConstructorFlagParity(tc.commandID, flag, legacyLeaf, generatedLeaf))
			}
		})
	}
}

func mustWorkflowMCPConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	legacy, generated, err := cli.NewWorkflowMCPFamilyParityRoots()
	if err != nil {
		t.Fatalf("NewWorkflowMCPFamilyParityRoots() error = %v", err)
	}
	return legacy, generated
}

func TestGeneratedVsLegacyParityMatrix_WorkflowMCPExecution(t *testing.T) {
	projectRoot := workflowParityProject(t)
	for _, tc := range workflowExecutionParityCases(projectRoot) {
		t.Run(tc.name, func(t *testing.T) {
			legacyRoot, generatedRoot := mustWorkflowMCPConstructorRoots(t)
			legacyOut, legacyErrOut, legacyErr := executeConstructorRoot(legacyRoot, tc.argv)
			generatedOut, generatedErrOut, generatedErr := executeConstructorRoot(generatedRoot, tc.argv)
			assertWorkflowExecutionParity(t, tc, legacyOut, generatedOut, legacyErrOut, generatedErrOut, legacyErr, generatedErr)
		})
	}
}

type workflowExecutionParityCase struct {
	name            string
	argv            []string
	wantErr         bool
	wantErrContains string
	wantStdout      []string
}

func workflowExecutionParityCases(projectRoot string) []workflowExecutionParityCase {
	validInline := `meta({ name: "review", version: 1 }); phase("setup");`
	return []workflowExecutionParityCase{
		{
			name:       "workflow validate named human success",
			argv:       []string{"workflow", "validate", "--dir", projectRoot, "--kind", "WORKFLOW_NAME", "--value", " review "},
			wantStdout: []string{"Workflow validation passed.", ".claude/workflows/review.js", "Source hash:"},
		},
		{
			name:       "workflow validate file JSON success",
			argv:       []string{"--json", "workflow", "validate", "--dir", projectRoot, "--kind", "WORKFLOW_FILE", "--value", " factory/workflows/review.js "},
			wantStdout: []string{`"valid":true`, `"sourceRef":"factory/workflows/review.js"`, `"blockingDiagnostics":[]`},
		},
		{
			name:       "workflow preview inline human success",
			argv:       []string{"workflow", "preview", "--dir", projectRoot, "--kind", "INLINE_WORKFLOW", "--inline", "  " + validInline + "  "},
			wantStdout: []string{"Factory preview passed.", "Source ref: inline", "Policy hash:", "Result constraints:"},
		},
		{
			name: "workflow preview named JSON success with optional inputs",
			argv: []string{"--json", "workflow", "preview", "--dir", projectRoot, "--kind", "WORKFLOW_NAME", "--value", "review",
				"--args-schema", `{"type":"object"}`, "--requested-policy", `{}`},
			wantStdout: []string{`"valid":true`, `"sourceResolution"`, `"policyPreview"`, `"resultConstraints"`},
		},
		{
			name:            "workflow malformed source failure",
			argv:            []string{"workflow", "validate", "--dir", projectRoot, "--kind", "INLINE_WORKFLOW", "--inline", "phase("},
			wantErr:         true,
			wantErrContains: "workflow validation found blocking issues",
			wantStdout:      []string{"Workflow validation failed.", "Blocking diagnostics:", "workflow.source.syntaxError"},
		},
		{
			name:            "workflow unresolved source JSON failure",
			argv:            []string{"--json", "workflow", "validate", "--dir", projectRoot, "--kind", "WORKFLOW_NAME", "--value", "missing"},
			wantErr:         true,
			wantErrContains: "workflow validation found blocking issues",
			wantStdout:      []string{`"valid":false`, `"blockingDiagnostics"`, "workflow.source.notFound"},
		},
		{
			name:            "workflow incomplete source failure",
			argv:            []string{"workflow", "validate", "--dir", projectRoot, "--kind", "WORKFLOW_NAME"},
			wantErr:         true,
			wantErrContains: "value is required when kind is WORKFLOW_NAME",
		},
		{
			name:            "workflow conflicting source failure",
			argv:            []string{"workflow", "preview", "--dir", projectRoot, "--value", "review", "--inline", validInline},
			wantErr:         true,
			wantErrContains: "--inline cannot be used when kind is WORKFLOW_NAME",
		},
		{
			name:            "MCP mutually exclusive source failure",
			argv:            []string{"mcp", "serve", "--runtime", "--fixture-catalog", "fixtures.json"},
			wantErr:         true,
			wantErrContains: "cannot combine --runtime with --fixture-catalog",
		},
	}
}

func workflowParityProject(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	for _, relativeDir := range []string{filepath.Join(".claude", "workflows"), filepath.Join("factory", "workflows")} {
		dir := filepath.Join(projectRoot, relativeDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create workflow fixture directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "review.js"), []byte(`meta({ name: "review", version: 1 }); phase("setup");`), 0o600); err != nil {
			t.Fatalf("write workflow fixture: %v", err)
		}
	}
	return projectRoot
}

func assertWorkflowExecutionParity(
	t *testing.T,
	tc workflowExecutionParityCase,
	legacyOut, generatedOut, legacyErrOut, generatedErrOut string,
	legacyErr, generatedErr error,
) {
	t.Helper()
	if (legacyErr != nil) != tc.wantErr || (generatedErr != nil) != tc.wantErr {
		t.Fatalf("execution errors: legacy=%v generated=%v wantErr=%t", legacyErr, generatedErr, tc.wantErr)
	}
	if fmt.Sprint(legacyErr) != fmt.Sprint(generatedErr) {
		t.Fatalf("exit/error drift: legacy=%v generated=%v", legacyErr, generatedErr)
	}
	if tc.wantErrContains != "" && !strings.Contains(fmt.Sprint(legacyErr), tc.wantErrContains) {
		t.Fatalf("error = %v, want substring %q", legacyErr, tc.wantErrContains)
	}
	if legacyOut != generatedOut {
		t.Fatalf("stdout drift:\nlegacy:\n%s\ngenerated:\n%s", legacyOut, generatedOut)
	}
	if legacyErrOut != generatedErrOut {
		t.Fatalf("stderr drift:\nlegacy:\n%s\ngenerated:\n%s", legacyErrOut, generatedErrOut)
	}
	for _, want := range tc.wantStdout {
		if !strings.Contains(legacyOut, want) {
			t.Fatalf("stdout missing %q:\n%s", want, legacyOut)
		}
	}
}

func executeConstructorRoot(root *cobra.Command, argv []string) (stdout, stderr string, err error) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(argv)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestProductionManifestCompletionParity_RootAndSessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	sessionShowRecord, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}

	liveRoot := cli.NewRootCommand()
	inventory, err := cliinputs.Walk(liveRoot)
	if err != nil {
		t.Fatalf("cliinputs.Walk() error = %v", err)
	}

	for _, record := range []climanifest.Command{rootRecord, sessionShowRecord} {
		t.Run(record.ID, func(t *testing.T) {
			liveArgs, liveFlags := climanifestparity.InputsForCommandPath(inventory, record.Path)
			mismatches := climanifestparity.CompareCompletionParity(record, liveArgs, liveFlags)
			if len(mismatches) == 0 {
				return
			}
			t.Fatalf("contract vs live completion drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
		})
	}
}

func TestProductionManifestBaselineExecutionParity_RootAndSessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	baselinePath := testutil.MustRepoPath(t, climanifestparity.ExecutionBaselinePath)
	baseline, err := climanifestparity.LoadExecutionBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadExecutionBaseline() error = %v", err)
	}

	for _, commandID := range []string{"you", "you.session.show"} {
		t.Run(commandID, func(t *testing.T) {
			record, err := manifest.CommandByID(commandID)
			if err != nil {
				t.Fatalf("CommandByID(%s) error = %v", commandID, err)
			}
			evidence, ok := baseline.Commands[commandID]
			if !ok {
				t.Fatalf("execution baseline missing command %q", commandID)
			}

			mismatches := climanifestparity.CompareBaselineSideEffects(record, evidence.SideEffectKinds)
			mismatches = append(mismatches, climanifestparity.CompareBaselineConstraints(record, evidence.Constraints)...)
			if len(mismatches) == 0 {
				return
			}
			t.Fatalf("contract vs baseline execution metadata drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
		})
	}
}

func TestProductionManifestLiveExitCodeParity_RootAndSessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	sessionShowRecord, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}

	assertLiveExitCodeContractParity(t, rootRecord, sessionShowRecord)

	env := parityTestEnvironment(t)
	assertObservedRootExitCodes(t, rootRecord, env)
	assertObservedSessionShowExitCodes(t, sessionShowRecord, env)
}

func assertLiveExitCodeContractParity(t *testing.T, records ...climanifest.Command) {
	t.Helper()
	for _, record := range records {
		t.Run(record.ID+"/contract-vs-live-boundary", func(t *testing.T) {
			if mismatches := climanifestparity.CompareLiveExitCodes(record); len(mismatches) > 0 {
				t.Fatalf("contract exit codes disagree with live root.Run boundary:\n%s", climanifestparity.FormatMismatchReport(mismatches))
			}
		})
	}
}

func assertObservedRootExitCodes(t *testing.T, record climanifest.Command, env []string) {
	t.Helper()
	t.Run("you/observed-success", func(t *testing.T) {
		want, ok := climanifestparity.ExitCodeForKind(record, "success")
		if !ok {
			t.Fatal("contract missing success exit")
		}
		var help bytes.Buffer
		if code := root.Run(root.Input{
			Args: []string{"you", "--help"}, Env: env, Stdout: &help,
		}, root.Dependencies{}); code != want {
			t.Fatalf("success exit code = %d, want contracted success code %d", code, want)
		}
	})
	t.Run("you/observed-usage", func(t *testing.T) {
		want, ok := climanifestparity.ExitCodeForKind(record, "usage")
		if !ok {
			t.Fatal("contract missing usage exit")
		}
		var stderr bytes.Buffer
		if code := root.Run(root.Input{
			Args: []string{"you", "unknown-command"}, Env: env, Stderr: &stderr,
		}, root.Dependencies{}); code != want {
			t.Fatalf("usage exit code = %d, want contracted usage code %d", code, want)
		}
	})
}

func assertObservedSessionShowExitCodes(t *testing.T, record climanifest.Command, env []string) {
	t.Helper()
	originalShowSession := cli.ShowSessionAccessor()
	defer cli.SetShowSessionAccessor(originalShowSession)

	t.Run("you.session.show/observed-success", func(t *testing.T) {
		want, ok := climanifestparity.ExitCodeForKind(record, "success")
		if !ok {
			t.Fatal("contract missing success exit")
		}
		cli.SetShowSessionAccessor(func(_ sessioncli.ShowConfig) error { return nil })
		if code := root.Run(root.Input{
			Args: []string{"you", "session", "show"}, Env: env, Stdout: io.Discard, Stderr: io.Discard,
		}, root.Dependencies{}); code != want {
			t.Fatalf("success exit code = %d, want contracted success code %d", code, want)
		}
	})
	t.Run("you.session.show/observed-failure", func(t *testing.T) {
		want, ok := climanifestparity.ExitCodeForKind(record, "failure")
		if !ok {
			t.Fatal("contract missing failure exit")
		}
		cli.SetShowSessionAccessor(func(_ sessioncli.ShowConfig) error { return errors.New("show failed") })
		if code := root.Run(root.Input{
			Args: []string{"you", "session", "show"}, Env: env, Stdout: io.Discard, Stderr: io.Discard,
		}, root.Dependencies{}); code != want {
			t.Fatalf("failure exit code = %d, want contracted failure code %d", code, want)
		}
	})
	t.Run("you.session.show/observed-usage", func(t *testing.T) {
		want, ok := climanifestparity.ExitCodeForKind(record, "usage")
		if !ok {
			t.Fatal("contract missing usage exit")
		}
		var stderr bytes.Buffer
		if code := root.Run(root.Input{
			Args: []string{"you", "session", "show", "one", "two"}, Env: env, Stdout: io.Discard, Stderr: &stderr,
		}, root.Dependencies{}); code != want {
			t.Fatalf("usage exit code = %d, want contracted usage code %d", code, want)
		}
		if !strings.Contains(stderr.String(), "accepts at most 1 arg") {
			t.Fatalf("usage parse stderr = %q, want excess positional rejection", stderr.String())
		}
	})
}

func TestProductionManifestOutputModeParity_SessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	record, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}
	if mismatches := climanifestparity.CompareDeclaredOutputs(record); len(mismatches) > 0 {
		t.Fatalf("contract output declarations drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}

	originalShowSession := cli.ShowSessionAccessor()
	defer cli.SetShowSessionAccessor(originalShowSession)

	var got sessioncli.ShowConfig
	cli.SetShowSessionAccessor(func(cfg sessioncli.ShowConfig) error {
		got = cfg
		return nil
	})

	jsonRoot := cli.NewRootCommand()
	jsonRoot.SetOut(io.Discard)
	jsonRoot.SetErr(io.Discard)
	jsonRoot.SetArgs([]string{"--json", "session", "show", "session-beta"})
	if err := jsonRoot.Execute(); err != nil {
		t.Fatalf("execute session show with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to select contracted JSON stdout output mode")
	}

	cli.SetShowSessionAccessor(func(cfg sessioncli.ShowConfig) error {
		got = cfg
		return nil
	})
	humanRoot := cli.NewRootCommand()
	humanRoot.SetOut(io.Discard)
	humanRoot.SetErr(io.Discard)
	humanRoot.SetArgs([]string{"session", "show"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatalf("execute session show without --json: %v", err)
	}
	if got.JSON {
		t.Fatal("expected default invocation to select contracted human stdout output mode")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySession{
			Id: "session-beta",
			Runtime: factoryapi.FactorySessionRuntime{
				OrchestratorKind: factoryapi.JAVASCRIPT,
			},
		})
	}))
	defer srv.Close()

	var humanOut bytes.Buffer
	if err := sessioncli.Show(sessioncli.ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &humanOut,
	}); err != nil {
		t.Fatalf("Show human mode: %v", err)
	}
	if !strings.Contains(humanOut.String(), "Factory session:") {
		t.Fatalf("human output missing Factory session label:\n%s", humanOut.String())
	}
	if err := json.Unmarshal(humanOut.Bytes(), &factoryapi.FactorySession{}); err == nil {
		t.Fatalf("human output should not be API-shaped JSON:\n%s", humanOut.String())
	}

	var jsonOut bytes.Buffer
	if err := sessioncli.Show(sessioncli.ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &jsonOut,
	}); err != nil {
		t.Fatalf("Show JSON mode: %v", err)
	}
	var payload factoryapi.FactorySession
	if err := json.Unmarshal([]byte(climanifestparity.NormalizeJSONOutput(jsonOut.String())), &payload); err != nil {
		t.Fatalf("JSON output is not API-shaped FactorySession JSON: %v\n%s", err, jsonOut.String())
	}
	if payload.Id != "session-beta" {
		t.Fatalf("JSON session id = %q, want session-beta", payload.Id)
	}
}

func TestProductionManifestNetworkSideEffectParity_SessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	record, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}

	baselinePath := testutil.MustRepoPath(t, climanifestparity.ExecutionBaselinePath)
	baseline, err := climanifestparity.LoadExecutionBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadExecutionBaseline() error = %v", err)
	}
	evidence := baseline.Commands["you.session.show"]
	if mismatches := climanifestparity.CompareBaselineSideEffects(record, evidence.SideEffectKinds); len(mismatches) > 0 {
		t.Fatalf("contract side-effect declarations drift from baseline:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
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
	if requestCount == 0 {
		t.Fatal("expected contracted network side effect to perform at least one HTTP request")
	}
}

func TestProductionManifestOutputModeParity_ModelsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	for _, commandID := range []string{
		"you.models.list",
		"you.models.inspect",
		"you.models.invoke",
		"you.models.pull",
	} {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			t.Fatalf("CommandByID(%s) error = %v", commandID, err)
		}
		if mismatches := climanifestparity.CompareDeclaredOutputs(record); len(mismatches) > 0 {
			t.Fatalf("%s contract output declarations drift detected:\n%s", commandID, climanifestparity.FormatMismatchReport(mismatches))
		}
	}

	originalListModels := cli.ListModelsAccessor()
	originalInspectModel := cli.InspectModelAccessor()
	originalInvokeModel := cli.InvokeModelAccessor()
	originalPullModel := cli.PullModelAccessor()
	defer func() {
		cli.SetListModelsAccessor(originalListModels)
		cli.SetInspectModelAccessor(originalInspectModel)
		cli.SetInvokeModelAccessor(originalInvokeModel)
		cli.SetPullModelAccessor(originalPullModel)
	}()

	assertModelsListOutputModes(t)
	assertModelsInspectOutputModes(t)
	assertModelsInvokeOutputModes(t)
	assertModelsPullOutputModes(t)
}

func assertModelsListOutputModes(t *testing.T) {
	t.Helper()
	var got modelscli.ListConfig
	cli.SetListModelsAccessor(func(cfg modelscli.ListConfig) error {
		got = cfg
		if cfg.Diagnostics != nil {
			if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: models list"); err != nil {
				return err
			}
		}
		if cfg.JSON {
			_, err := fmt.Fprintln(cfg.Output, `{"results":[]}`)
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, "NAME\tREADINESS")
		return err
	})

	jsonRoot := cli.NewRootCommand()
	jsonRoot.SetOut(io.Discard)
	jsonRoot.SetErr(io.Discard)
	jsonRoot.SetArgs([]string{"--json", "models", "list", "--verbose"})
	if err := jsonRoot.Execute(); err != nil {
		t.Fatalf("execute models list with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to select contracted JSON stdout output mode for models list")
	}
	if !got.Verbose {
		t.Fatal("expected --verbose to reach models list config")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cli.SetListModelsAccessor(func(cfg modelscli.ListConfig) error {
		got = cfg
		if cfg.Diagnostics != nil {
			if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: models list"); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintln(cfg.Output, "NAME\tREADINESS")
		return err
	})
	humanRoot := cli.NewRootCommand()
	humanRoot.SetOut(&stdout)
	humanRoot.SetErr(&stderr)
	humanRoot.SetArgs([]string{"models", "list", "--verbose"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatalf("execute models list without --json: %v", err)
	}
	if got.JSON {
		t.Fatal("expected default models list invocation to select contracted human stdout output mode")
	}
	if !strings.Contains(stdout.String(), "NAME") {
		t.Fatalf("human stdout = %q, want table output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "diagnostic: models list") {
		t.Fatalf("stderr = %q, want diagnostics on stderr", stderr.String())
	}
}

func assertModelsInspectOutputModes(t *testing.T) {
	t.Helper()
	var got modelscli.InspectConfig
	cli.SetInspectModelAccessor(func(cfg modelscli.InspectConfig) error {
		got = cfg
		if cfg.JSON {
			_, err := fmt.Fprintln(cfg.Output, `{"name":"OMNIVOICE_Q4_K_M"}`)
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, "Readiness:\tREADY")
		return err
	})

	jsonRoot := cli.NewRootCommand()
	jsonRoot.SetOut(io.Discard)
	jsonRoot.SetErr(io.Discard)
	jsonRoot.SetArgs([]string{"--json", "models", "inspect", "OMNIVOICE_Q4_K_M"})
	if err := jsonRoot.Execute(); err != nil {
		t.Fatalf("execute models inspect with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to select contracted JSON stdout output mode for models inspect")
	}

	var stdout bytes.Buffer
	cli.SetInspectModelAccessor(func(cfg modelscli.InspectConfig) error {
		got = cfg
		_, err := fmt.Fprintln(cfg.Output, "Readiness:\tREADY")
		return err
	})
	humanRoot := cli.NewRootCommand()
	humanRoot.SetOut(&stdout)
	humanRoot.SetErr(io.Discard)
	humanRoot.SetArgs([]string{"models", "inspect", "OMNIVOICE_Q4_K_M"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatalf("execute models inspect without --json: %v", err)
	}
	if got.JSON {
		t.Fatal("expected default models inspect invocation to select contracted human stdout output mode")
	}
	if !strings.Contains(stdout.String(), "Readiness:") {
		t.Fatalf("human stdout = %q, want inspect table output", stdout.String())
	}
}

func assertModelsInvokeOutputModes(t *testing.T) {
	t.Helper()
	var got modelscli.InvokeConfig
	cli.SetInvokeModelAccessor(func(cfg modelscli.InvokeConfig) error {
		got = cfg
		if cfg.JSON {
			_, err := fmt.Fprintln(cfg.Output, `{"modelName":"OMNIVOICE_Q4_K_M","operation":"TTS"}`)
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, "Invoked OMNIVOICE_Q4_K_M")
		return err
	})

	jsonRoot := cli.NewRootCommand()
	jsonRoot.SetOut(io.Discard)
	jsonRoot.SetErr(io.Discard)
	jsonRoot.SetArgs([]string{"--json", "models", "invoke", "OMNIVOICE_Q4_K_M", "--text", "hello"})
	if err := jsonRoot.Execute(); err != nil {
		t.Fatalf("execute models invoke with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to select contracted JSON stdout output mode for models invoke")
	}

	var stdout bytes.Buffer
	cli.SetInvokeModelAccessor(func(cfg modelscli.InvokeConfig) error {
		got = cfg
		_, err := fmt.Fprintln(cfg.Output, "Invoked OMNIVOICE_Q4_K_M")
		return err
	})
	humanRoot := cli.NewRootCommand()
	humanRoot.SetOut(&stdout)
	humanRoot.SetErr(io.Discard)
	humanRoot.SetArgs([]string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--text", "hello"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatalf("execute models invoke without --json: %v", err)
	}
	if got.JSON {
		t.Fatal("expected default models invoke invocation to select contracted human stdout output mode")
	}
	if !strings.Contains(stdout.String(), "Invoked") {
		t.Fatalf("human stdout = %q, want human invoke output", stdout.String())
	}
}

func assertModelsPullOutputModes(t *testing.T) {
	t.Helper()
	var got modelscli.PullConfig
	cli.SetPullModelAccessor(func(cfg modelscli.PullConfig) error {
		got = cfg
		if cfg.JSON {
			_, err := fmt.Fprintln(cfg.Output, `{"name":"OMNIVOICE_Q4_K_M","status":"PULLED"}`)
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, "Pulled OMNIVOICE_Q4_K_M")
		return err
	})

	jsonRoot := cli.NewRootCommand()
	jsonRoot.SetOut(io.Discard)
	jsonRoot.SetErr(io.Discard)
	jsonRoot.SetArgs([]string{"--json", "models", "pull", "OMNIVOICE_Q4_K_M"})
	if err := jsonRoot.Execute(); err != nil {
		t.Fatalf("execute models pull with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to select contracted JSON stdout output mode for models pull")
	}

	var stdout bytes.Buffer
	cli.SetPullModelAccessor(func(cfg modelscli.PullConfig) error {
		got = cfg
		_, err := fmt.Fprintln(cfg.Output, "Pulled OMNIVOICE_Q4_K_M")
		return err
	})
	humanRoot := cli.NewRootCommand()
	humanRoot.SetOut(&stdout)
	humanRoot.SetErr(io.Discard)
	humanRoot.SetArgs([]string{"models", "pull", "OMNIVOICE_Q4_K_M"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatalf("execute models pull without --json: %v", err)
	}
	if got.JSON {
		t.Fatal("expected default models pull invocation to select contracted human stdout output mode")
	}
	if !strings.Contains(stdout.String(), "Pulled") {
		t.Fatalf("human stdout = %q, want human pull output", stdout.String())
	}
}

func parityTestEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		return []string{"USERPROFILE=" + home}
	case "plan9":
		return []string{"home=" + home}
	default:
		return []string{"HOME=" + home}
	}
}

package climanifestparity_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
)

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

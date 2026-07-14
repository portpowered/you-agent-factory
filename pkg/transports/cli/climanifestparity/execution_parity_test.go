package climanifestparity_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
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

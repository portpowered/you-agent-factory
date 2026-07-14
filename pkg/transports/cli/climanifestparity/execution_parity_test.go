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

func TestProductionManifestExecutionParity_RootAndSessionShow(t *testing.T) {
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
			mismatches = append(mismatches, climanifestparity.CompareDeclaredOutputs(record)...)
			mismatches = append(mismatches, climanifestparity.CompareDeclaredChannels(record)...)
			mismatches = append(mismatches, climanifestparity.CompareDeclaredExits(record)...)
			mismatches = append(mismatches, climanifestparity.CompareDeclaredConstraints(record)...)

			switch record.ID {
			case "you":
				mismatches = append(mismatches, climanifestparity.CompareDeclaredSideEffects(record, []string{"filesystem", "network", "process"})...)
			case "you.session.show":
				mismatches = append(mismatches, climanifestparity.CompareDeclaredSideEffects(record, []string{"network"})...)
			}

			if len(mismatches) == 0 {
				return
			}
			t.Fatalf("contract vs live execution metadata drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
		})
	}
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

func TestProductionManifestExitParity_SessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	record, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}
	if mismatches := climanifestparity.CompareDeclaredExits(record); len(mismatches) > 0 {
		t.Fatalf("contract exit declarations drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}

	originalShowSession := cli.ShowSessionAccessor()
	defer cli.SetShowSessionAccessor(originalShowSession)

	env := parityTestEnvironment(t)
	cli.SetShowSessionAccessor(func(_ sessioncli.ShowConfig) error { return nil })
	if code := root.Run(root.Input{
		Args: []string{"you", "session", "show"}, Env: env, Stdout: io.Discard, Stderr: io.Discard,
	}, root.Dependencies{}); code != root.ExitSuccess {
		t.Fatalf("success exit code = %d, want contracted success code %d", code, root.ExitSuccess)
	}

	cli.SetShowSessionAccessor(func(_ sessioncli.ShowConfig) error { return errors.New("show failed") })
	if code := root.Run(root.Input{
		Args: []string{"you", "session", "show"}, Env: env, Stdout: io.Discard, Stderr: io.Discard,
	}, root.Dependencies{}); code != root.ExitFailure {
		t.Fatalf("failure exit code = %d, want contracted failure code %d", code, root.ExitFailure)
	}

	var stderr bytes.Buffer
	if code := root.Run(root.Input{
		Args: []string{"you", "session", "show", "one", "two"}, Env: env, Stdout: io.Discard, Stderr: &stderr,
	}, root.Dependencies{}); code != root.ExitFailure {
		t.Fatalf("usage parse exit code = %d, want non-zero failure exit", code)
	}
	if !strings.Contains(stderr.String(), "accepts at most 1 arg") {
		t.Fatalf("usage parse stderr = %q, want excess positional rejection", stderr.String())
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

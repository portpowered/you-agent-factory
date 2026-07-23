package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	"github.com/spf13/cobra"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestMain(m *testing.M) {
	// Process tests execute Cobra commands in-process. Explorer launch behavior
	// is outside this package's contract, and its Windows process scan dominates
	// the cost of repeated Execute calls.
	cobra.MousetrapHelpText = ""
	m.Run()
}

func TestBuildProcessPreservesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	process, err := BuildProcess(ctx, serviceedges.Edges{})
	if process != nil {
		t.Fatal("BuildProcess() returned a process for a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildProcess() error = %v, want context.Canceled", err)
	}
}

func TestBuildProcessConstructionFailureDoesNotStartExternalLifecycle(t *testing.T) {
	t.Parallel()

	apiStarts := 0
	process, err := BuildProcess(nil, serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if process != nil {
		t.Fatal("BuildProcess() returned a process for a missing context")
	}
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("BuildProcess() error = %v, want required-context diagnostic", err)
	}
	if apiStarts != 0 {
		t.Fatalf("construction failure started API lifecycle %d times, want zero", apiStarts)
	}
}

func TestBuildProcessComposesDetachedExternalProviderWithBuiltInsInertly(t *testing.T) {
	t.Parallel()

	manifest := rootExternalManifest(t, "customer.provider", "customer")
	integration := &rootRecordingIntegration{identity: "customer.provider"}
	registrations := []inference.Registration{
		{Manifest: manifest, Integration: integration},
	}
	apiStarts := 0
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		ProviderRegistrations: registrations,
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	registrations[0] = inference.Registration{
		Manifest:    rootExternalManifest(t, "mutated.provider", "mutated"),
		Integration: &rootRecordingIntegration{identity: "mutated.provider"},
	}

	assertProviderLookup(t, process.ProviderRegistry(), "customer.provider", "customer.provider")
	assertProviderLookup(t, process.ProviderRegistry(), "customer", "customer.provider")
	assertProviderLookup(t, process.ProviderRegistry(), "claude", "claude")
	assertProviderLookup(t, process.ProviderRegistry(), "agent", "cursor")
	if apiStarts != 0 || integration.discoverCalls != 0 ||
		integration.capabilityCalls != 0 || integration.invokeCalls != 0 {
		t.Fatalf(
			"construction side effects = api:%d discover:%d capabilities:%d invoke:%d, want zero",
			apiStarts,
			integration.discoverCalls,
			integration.capabilityCalls,
			integration.invokeCalls,
		)
	}

	independent, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("independent BuildProcess() error = %v", err)
	}
	if _, err := independent.ProviderRegistry().CanonicalIdentity("customer.provider"); err == nil {
		t.Fatal("independent process retained another build's external registration")
	}
}

func TestBuildProcessReportsCanonicalRegistryValidationFailure(t *testing.T) {
	t.Parallel()

	manifest := rootExternalManifest(t, "claude", "customer-claude")
	registration := inference.Registration{
		Manifest:    manifest,
		Integration: &rootRecordingIntegration{identity: "claude"},
	}

	process, buildErr := BuildProcess(context.Background(), serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{registration},
	})
	if process != nil {
		t.Fatal("BuildProcess() returned process for invalid provider registration")
	}
	if buildErr == nil ||
		!strings.Contains(buildErr.Error(), "provider registry validation failed") ||
		!strings.Contains(buildErr.Error(), `"claude": identity collision`) {
		t.Fatalf("BuildProcess() error = %v, want canonical identity-collision diagnostic", buildErr)
	}
}

func TestProcessRoutesHelpAndExplicitCommandsToSuppliedStreams(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var help bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"renamed-binary", "--help"},
		Env:              homeEnvironment(t.TempDir()),
		Stdout:           &help,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Process.Execute(help) error = %v", err)
	}
	if !strings.HasPrefix(help.String(), "Run and manage CPN-based workflow factories") {
		t.Fatalf("help output = %q", help.String())
	}

	var docs bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"you", "docs", "agents"},
		Env:              homeEnvironment(t.TempDir()),
		Stdout:           &docs,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Process.Execute(docs agents) error = %v", err)
	}
	if !strings.Contains(docs.String(), "# Agents") {
		t.Fatalf("docs output does not contain agents topic: %q", docs.String())
	}
	if strings.Contains(help.String(), "# Agents") {
		t.Fatal("sequential execution leaked the second command's output into the first stream")
	}
}

func TestProcessSubmitBatchUsesInjectedFileAndStdinEdges(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	const batch = `{"requestId":"root-process-batch","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task","payload":{"title":"A"}}]}`
	home := t.TempDir()
	batchPath := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batchPath, []byte(batch), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}

	for _, tc := range []struct {
		name  string
		args  []string
		stdin io.Reader
	}{
		{name: "file", args: []string{"you", "submit", "batch", "--dry-run", batchPath}},
		{name: "stdin", args: []string{"you", "submit", "batch", "--dry-run"}, stdin: strings.NewReader(batch)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := process.Execute(Input{
				Args: tc.args, Env: homeEnvironment(home), Stdin: tc.stdin,
				Stdout: &stdout, Context: context.Background(), WorkingDirectory: home,
			})
			if err != nil {
				t.Fatalf("Process.Execute(submit batch %s) error = %v", tc.name, err)
			}
			if !strings.Contains(stdout.String(), "requestId: root-process-batch") ||
				!strings.Contains(stdout.String(), "dry-run: no request sent") {
				t.Fatalf("submit batch %s output = %q", tc.name, stdout.String())
			}
		})
	}
}

func TestProcessInvalidArgumentsReturnsDiagnostic(t *testing.T) {
	t.Parallel()

	process, buildErr := BuildProcess(context.Background(), serviceedges.Edges{})
	if buildErr != nil {
		t.Fatalf("BuildProcess() error = %v", buildErr)
	}
	var stderr bytes.Buffer
	err := process.Execute(Input{
		Args:             []string{"you", "definitely-not-a-command"},
		Env:              homeEnvironment(t.TempDir()),
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Process.Execute(invalid command) error = nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Process.Execute(invalid command) error = %q", err)
	}
}

func TestProcessSequentialHomesControlConfigPaths(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("USERPROFILE", ambientHome)

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	homes := []string{t.TempDir(), t.TempDir()}
	for _, home := range homes {
		var output bytes.Buffer
		if err := process.Execute(Input{
			Args: []string{"you", "config", "init", "--json"}, Env: homeEnvironment(home), Stdout: &output, Context: context.Background(), WorkingDirectory: home,
		}); err != nil {
			t.Fatalf("Process.Execute(config init, home %q) error = %v", home, err)
		}
		var outcome struct {
			HomeDir    string `json:"homeDir"`
			ConfigPath string `json:"configPath"`
		}
		if err := json.Unmarshal(output.Bytes(), &outcome); err != nil {
			t.Fatalf("decode config init output for supplied home %q: %v\noutput:\n%s", home, err, output.String())
		}
		if outcome.HomeDir != home {
			t.Fatalf("config init homeDir = %q, want supplied home %q", outcome.HomeDir, home)
		}
		if _, err := os.Stat(outcome.ConfigPath); err != nil {
			t.Fatalf("Stat(config for supplied home %q) error = %v", home, err)
		}
	}
	ambientEntries, err := os.ReadDir(ambientHome)
	if err != nil {
		t.Fatalf("ReadDir(ambient home) error = %v", err)
	}
	if len(ambientEntries) != 0 {
		t.Fatalf("ambient home contains %d entries after supplied-home invocations, want none", len(ambientEntries))
	}
}

func TestProcessConcurrentCommandsKeepInvocationStateIndependent(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	var help bytes.Buffer
	var docs bytes.Buffer
	start := make(chan struct{})
	errs := make(chan error, 2)
	var commands sync.WaitGroup
	for _, input := range []Input{
		{
			Args:             []string{"you", "--help"},
			Env:              homeEnvironment(t.TempDir()),
			Stdout:           &help,
			Context:          context.Background(),
			WorkingDirectory: t.TempDir(),
		},
		{
			Args:             []string{"you", "docs", "agents"},
			Env:              homeEnvironment(t.TempDir()),
			Stdout:           &docs,
			Context:          context.Background(),
			WorkingDirectory: t.TempDir(),
		},
	} {
		input := input
		commands.Add(1)
		go func() {
			defer commands.Done()
			<-start
			errs <- process.Execute(input)
		}()
	}
	close(start)
	commands.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Process.Execute(concurrent command) error = %v", err)
		}
	}

	if !strings.HasPrefix(help.String(), "Run and manage CPN-based workflow factories") {
		t.Fatalf("help output = %q", help.String())
	}
	if strings.Contains(help.String(), "# Agents") {
		t.Fatalf("help output contains docs output: %q", help.String())
	}
	if !strings.Contains(docs.String(), "# Agents") {
		t.Fatalf("docs output = %q", docs.String())
	}
	if strings.Contains(docs.String(), "Usage:\n  you [command]") {
		t.Fatalf("docs output contains help output: %q", docs.String())
	}
}

func TestProcessSetupAndFactoryAuthoringCommandsThroughProductionComposition(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	factoryDir := filepath.Join(t.TempDir(), "authored-factory")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var initOutput bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "init", "--dir", factoryDir},
		Env:              homeEnvironment(home),
		Stdout:           &initOutput,
		Context:          context.Background(),
		WorkingDirectory: filepath.Dir(factoryDir),
	}); err != nil {
		t.Fatalf("Process.Execute(init) error = %v", err)
	}
	if !strings.Contains(initOutput.String(), "Initialized default factory directory structure") {
		t.Fatalf("init output = %q", initOutput.String())
	}

	var validateOutput bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "factory", "config", "validate", factoryDir},
		Env:              homeEnvironment(home),
		Stdout:           &validateOutput,
		Context:          context.Background(),
		WorkingDirectory: filepath.Dir(factoryDir),
	}); err != nil {
		t.Fatalf("Process.Execute(factory config validate) error = %v", err)
	}
	if !strings.Contains(validateOutput.String(), "Factory validation passed") {
		t.Fatalf("factory validation output = %q", validateOutput.String())
	}

	missingPath := filepath.Join(t.TempDir(), "missing-factory")
	if err := process.Execute(Input{
		Args:             []string{"you", "factory", "config", "validate", missingPath},
		Env:              homeEnvironment(home),
		Context:          context.Background(),
		WorkingDirectory: filepath.Dir(factoryDir),
	}); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("Process.Execute(factory config validate missing path) error = %v", err)
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("missing factory path Stat error = %v, want not-exist", err)
	}
}

func homeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

type rootRecordingIntegration struct {
	identity        inference.Identity
	discoverCalls   int
	capabilityCalls int
	invokeCalls     int
}

func (i *rootRecordingIntegration) Identity() inference.Identity { return i.identity }

func (*rootRecordingIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
}

func (i *rootRecordingIntegration) Discover(context.Context) (inference.Discovery, error) {
	i.discoverCalls++
	panic("process construction must not discover external providers")
}

func (i *rootRecordingIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	i.capabilityCalls++
	panic("process construction must not negotiate external provider capabilities")
}

func (i *rootRecordingIntegration) Invoke(
	_ context.Context,
	_ inference.InvocationRequest,
	_ inference.ResponseWriter,
) error {
	i.invokeCalls++
	panic("process construction must not invoke external providers")
}

func rootExternalManifest(t *testing.T, identity, alias string) inference.Manifest {
	t.Helper()
	var catalog struct {
		Providers []inference.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = inference.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = inference.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = inference.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = inference.ResponseFidelityCapabilities{}
	return manifest
}

func assertProviderLookup(
	t *testing.T,
	registry interface {
		CanonicalIdentity(string) (string, error)
	},
	identity string,
	want inference.Identity,
) {
	t.Helper()
	canonical, err := registry.CanonicalIdentity(identity)
	if err != nil {
		t.Fatalf("CanonicalIdentity(%q) error = %v", identity, err)
	}
	if canonical != string(want) {
		t.Fatalf("CanonicalIdentity(%q) = %q, want %q", identity, canonical, want)
	}
}

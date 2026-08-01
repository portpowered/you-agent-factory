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

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/inference"
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

func TestBuildProcessComposesInertModelsRuntimeHost(t *testing.T) {
	t.Parallel()

	launcher := &rootRecordingModelHostLauncher{}
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		ModelHostProcessLauncher: launcher,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if process == nil {
		t.Fatal("BuildProcess() returned nil process")
	}
	if launcher.starts != 0 {
		t.Fatalf("model host process starts during construction = %d, want 0", launcher.starts)
	}
}

type rootRecordingModelHostLauncher struct {
	starts int
}

func (launcher *rootRecordingModelHostLauncher) Start(
	context.Context,
	modelswire.HostProcessStartSpec,
) (modelswire.HostManagedProcess, error) {
	launcher.starts++
	panic("model host process launcher called during inert construction")
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
	assertProviderLookup(t, process.ProviderRegistry(), "codex", "codex")
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

func TestBuildProcessOpensFactoryWithRegisteredExternalProviderWithoutProviderIO(t *testing.T) {
	t.Parallel()

	manifest := rootExternalManifest(t, "customer.provider", "customer")
	integration := &rootRecordingIntegration{identity: "customer.provider"}
	factoryDir := rootFactoryWithProvider(t, "customer")

	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{{
			Manifest:    manifest,
			Integration: integration,
		}},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(Input{
		Args: []string{
			"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record",
		},
		Env:              homeEnvironment(t.TempDir()),
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	})
	if err != nil {
		t.Fatalf("Process.Execute(run) error = %v", err)
	}
	if integration.discoverCalls != 0 ||
		integration.capabilityCalls != 0 ||
		integration.invokeCalls != 0 {
		t.Fatalf(
			"Factory opening provider I/O = discover:%d capabilities:%d invoke:%d, want zero",
			integration.discoverCalls,
			integration.capabilityCalls,
			integration.invokeCalls,
		)
	}
}

func TestBuildProcessRejectsUnknownAndNonSelectableFactoryProvidersWithoutFallback(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		provider string
		want     string
	}{
		{name: "unknown", provider: "unknown.provider", want: `provider is unknown: "unknown.provider"`},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			process, err := BuildProcess(context.Background(), serviceedges.Edges{})
			if err != nil {
				t.Fatalf("BuildProcess() error = %v", err)
			}
			factoryDir := rootFactoryWithProvider(t, test.provider)
			err = process.Execute(Input{
				Args: []string{
					"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record",
				},
				Env:              homeEnvironment(t.TempDir()),
				Context:          context.Background(),
				WorkingDirectory: factoryDir,
			})
			if err == nil ||
				!strings.Contains(err.Error(), "workers[0].modelProvider") ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("Process.Execute(run) error = %v, want field-local %s", err, test.want)
			}
		})
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

func TestProcessSequentialHomesKeepEffectiveListingReadOnly(t *testing.T) {
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
			Args: []string{
				"you", "--json", "factory", "list", "--dir",
				filepath.Join(home, ".you-agent-factory", "factories"),
			},
			Env: homeEnvironment(home), Stdout: &output, Context: context.Background(), WorkingDirectory: home,
		}); err != nil {
			t.Fatalf("Process.Execute(factory list, home %q) error = %v", home, err)
		}
		if output.Len() == 0 {
			t.Fatalf("factory list output for supplied home %q is empty", home)
		}
		configPath := filepath.Join(home, ".you-agent-factory", "config.json")
		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("Stat(config for supplied home %q) error = %v, want not-exist", home, err)
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

func TestProcessFactoryListDiscoversPackagedFactoriesWithoutMaterialization(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var listOutput bytes.Buffer
	if err := process.Execute(Input{
		Args: []string{
			"you", "--json", "factory", "list", "--dir",
			filepath.Join(home, ".you-agent-factory", "factories"),
		},
		Env:              homeEnvironment(home),
		Stdout:           &listOutput,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v", err)
	}

	if !strings.Contains(listOutput.String(), `"factoryDirectory":"-"`) {
		t.Fatalf("factory list output = %q, want unmaterialized packaged location", listOutput.String())
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("home entries after factory list = (%v, %v), want empty", entries, err)
	}
}

func TestProcessNormalInitializationAndFactoryValidationThroughProductionComposition(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	factoryDir := filepath.Join(home, ".you-agent-factory", "factories", "@you", "goal")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	runNormalInitialization(t, process, home)

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

func rootFactoryWithProvider(t *testing.T, provider string) string {
	t.Helper()
	factoryDir := testutil.CopyFixtureDir(
		t,
		testutil.MustRepoPath(t, filepath.Join("tests", "functional_test", "testdata", "executor_success")),
	)
	workerPath := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	worker := strings.Join([]string{
		"---",
		"model: test-model",
		"modelProvider: " + provider,
		"stopToken: COMPLETE",
		"type: MODEL_WORKER",
		"---",
		"",
		"Test worker.",
		"",
	}, "\n")
	if err := os.WriteFile(workerPath, []byte(worker), 0o600); err != nil {
		t.Fatalf("write provider worker: %v", err)
	}
	return factoryDir
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

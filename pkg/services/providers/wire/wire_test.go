package wire

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/portpowered/infinite-you/internal/providerpackages"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
)

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to providers.Service")
	}

	result, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = %#v, want non-empty migrated catalog", result)
	}
}

func TestNewServiceComposesCatalogAndExecutionWithSharedCatalogAuthority(t *testing.T) {
	t.Parallel()

	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{
		ID: providers.IDCodex,
	})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q", got.Provider.ID)
	}

	_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "shared-catalog-authority",
	})
	if errors.Is(executeErr, providers.ErrUnknownProvider) {
		t.Fatalf(
			"Execute(codex) = %v, want execution bound through shared catalog authority",
			executeErr,
		)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf(
			"Execute(codex) = %#v, want dependency failure from bound adapter without effects",
			executeErr,
		)
	}
}

func TestPackagedACPIdentitiesAndLegacyAliasesResolveToTheirCanonicalIDs(t *testing.T) {
	t.Parallel()

	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	tests := []struct {
		canonical string
		aliases   []string
	}{
		{canonical: "pi-acp"},
		{canonical: "openclaw-acp"},
		{canonical: "gemini-acp"},
		{canonical: "cursor-acp"},
		{canonical: "copilot-acp"},
		{canonical: "droid-acp", aliases: []string{"factory-droid", "factorydroid"}},
		{canonical: "fast-agent-acp"},
		{canonical: "grok-build-acp"},
		{canonical: "iflow-acp"},
		{canonical: "kilocode-acp"},
		{canonical: "kimi-acp"},
		{canonical: "kiro-acp"},
		{canonical: "mux-acp"},
		{canonical: "opencode-acp"},
		{canonical: "pool-acp"},
		{canonical: "qoder-acp"},
		{canonical: "qwen-acp"},
		{canonical: "reasonix-acp"},
		{canonical: "trae-acp"},
		{canonical: "zeroclaw-acp"},
	}
	for _, test := range tests {
		t.Run(test.canonical, func(t *testing.T) {
			canonical, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.ID(test.canonical)})
			if err != nil {
				t.Fatalf("GetProvider(%q) error = %v", test.canonical, err)
			}
			if canonical.Provider.ID.String() != test.canonical {
				t.Fatalf("GetProvider(%q) ID = %q", test.canonical, canonical.Provider.ID)
			}
			if canonical.Provider.Readiness != providers.ReadinessUnverified {
				t.Fatalf("GetProvider(%q) readiness = %q, want unverified", test.canonical, canonical.Provider.Readiness)
			}
			wantCapabilities := []providers.Capability{providers.CapabilityPromptSubmission}
			if test.canonical == "cursor-acp" {
				wantCapabilities = append(
					wantCapabilities,
					providers.CapabilityImageInput,
					providers.CapabilityPermissionBypass,
				)
			}
			if !reflect.DeepEqual(canonical.Provider.Capabilities, wantCapabilities) {
				t.Fatalf("GetProvider(%q) capabilities = %v, want %v", test.canonical, canonical.Provider.Capabilities, wantCapabilities)
			}
			for _, alias := range test.aliases {
				resolved, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.ID(alias)})
				if err != nil {
					t.Fatalf("GetProvider(%q) error = %v", alias, err)
				}
				if resolved.Provider.ID.String() != test.canonical {
					t.Fatalf("GetProvider(%q) ID = %q, want %q", alias, resolved.Provider.ID, test.canonical)
				}
			}
		})
	}
}

func TestNewServiceBuildsUsableRoot(t *testing.T) {
	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := root.ListProviders(
		context.Background(),
		providers.ListProvidersRequest{},
	)
	if err != nil || len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = (%#v, %v), want catalog entries", result, err)
	}
}

const generatedExecutableHelperEnvironment = "YOU_TEST_GENERATED_EXECUTABLE_HELPER"

func TestGeneratedRuntimeExecutableReachesCommandFactoryLosslessly(t *testing.T) {
	if os.Getenv(generatedExecutableHelperEnvironment) != "" {
		os.Exit(0)
	}

	wantExecutable := `agent'\tool`
	wantArguments := []string{"hello world", "semi;colon"}
	source := fstest.MapFS{
		"packages/model-providers/providers/generated-acp/provider.yaml": &fstest.MapFile{Data: []byte(`id: generated-acp
aliases: []
implementationAvailability: externally-supplied
harness: {kind: acp, acpSupport: {support: supported, evidenceRefs: [fixture]}}
modelCatalogPosture: unknown
harnessRoutes: [{direction: input, modality: text, support: supported, transport: inline, evidenceRefs: [fixture]}]
evidence: [{id: fixture, kind: conformance_fixture, verifiedOn: "2026-08-11", factRefs: [harness/acp, harness/input/text]}]
models: []
tools: []
knownLimits: []
discovery: {prerequisites: [{kind: executable, name: generated-acp, description: Install the generated ACP executable.}]}
`)},
		"packages/model-providers/providers/generated-acp/harness.yaml": &fstest.MapFile{Data: []byte(`implementation: {kind: acp_agent, profile: cursor-acp}
launch: {posture: installed_executable, transport: stdio, command: 'agent''\tool', arguments: ["hello world", "semi;colon"]}
`)},
	}
	packages, err := providerpackages.Validate(source, []providerpackages.RuntimeProfile{{ID: "cursor-acp"}})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	document, err := json.Marshal(providerpackages.RuntimeProjection(packages))
	if err != nil {
		t.Fatalf("marshal runtime projection: %v", err)
	}
	integrations, err := ACPIntegrationsFromRuntimeCatalog(document)
	if err != nil {
		t.Fatalf("load generated runtime projection: %v", err)
	}

	var gotExecutable string
	var gotArguments []string
	commandFactory := func(name string, arguments ...string) *exec.Cmd {
		gotExecutable = name
		gotArguments = append([]string(nil), arguments...)
		return exec.Command(os.Args[0], "-test.run=^TestGeneratedRuntimeExecutableReachesCommandFactoryLosslessly$")
	}
	root, err := NewService(
		WithACPIntegrations(integrations...),
		WithCommandFactory(commandFactory),
		WithExecutableLocator(fakeExecutableLocator{wantExecutable: wantExecutable}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:           "generated-acp",
		AttemptID:          "generated-executable-round-trip",
		UserMessage:        "exercise generated executable",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: append(os.Environ(), generatedExecutableHelperEnvironment+"=1"),
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want helper process to terminate before ACP initialize")
	}
	if gotExecutable != wantExecutable {
		t.Fatalf("command executable = %q, want %q", gotExecutable, wantExecutable)
	}
	if !reflect.DeepEqual(gotArguments, wantArguments) {
		t.Fatalf("command arguments = %#v, want %#v", gotArguments, wantArguments)
	}
}

func TestACPWireOptionsComposeConfiguredCatalogAndValidateCommands(t *testing.T) {
	t.Parallel()

	integration := providers.ACPIntegration{ID: "custom-acp", Name: "custom-acp", Transport: "stdio", Command: "custom-agent --acp"}
	root, err := NewService(
		WithACPIntegrations(integration),
		WithCommandFactory(nil),
		WithExecutableLocator(nil),
	)
	if err != nil {
		t.Fatalf("NewService(ACP) = %v", err)
	}
	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: integration.Name})
	if err != nil || got.Provider.ID != integration.Name {
		t.Fatalf("GetProvider(custom-acp) = (%#v, %v)", got, err)
	}

	replaced := effectiveACPIntegrations(
		[]providers.ACPIntegration{{ID: "cursor-acp", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}},
		[]providers.ACPIntegration{{ID: "replacement", Name: "cursor-acp", Transport: "stdio", Command: "replacement acp"}},
	)
	if len(replaced) != 1 || replaced[0].ID != "replacement" {
		t.Fatalf("effectiveACPIntegrations(replacement) = %#v", replaced)
	}
	legacySaved := effectiveACPIntegrations(
		[]providers.ACPIntegration{{
			ID: "entry-1", Name: "cursor-acp", Aliases: []string{"cursor"},
			Transport: "stdio", Command: "cursor-agent acp", Arguments: []string{"acp"},
			RuntimePosture: "installed_executable", ImplementationProfile: "cursor-acp",
		}},
		[]providers.ACPIntegration{{
			ID: "saved-entry", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
		}},
	)
	if len(legacySaved) != 1 || legacySaved[0].ImplementationProfile != "cursor-acp" || legacySaved[0].RuntimePosture != "installed_executable" || !reflect.DeepEqual(legacySaved[0].Arguments, []string{"acp"}) || !reflect.DeepEqual(legacySaved[0].Aliases, []string{"cursor"}) {
		t.Fatalf("effectiveACPIntegrations(legacy package command) = %#v, want package runtime metadata preserved", legacySaved)
	}

	factory := NewFactory(nil)
	if _, err := factory([]providers.ACPIntegration{{ID: "bad", Name: "bad-acp", Transport: "stdio", Command: "'"}}); err == nil {
		t.Fatal("factory(invalid command) error = nil")
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	platformRunner := &inertPlatformCommandRunner{}
	workersRunner := &inertWorkersCommandRunner{}
	agyAllocator := &inertPTYAllocator{}
	agyLocator := &inertExecutableLocator{}
	agyInspector := &inertPathInspector{}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := NewService(
		CatalogOption(catalogwire.WithProbeQuery(func(
			_ context.Context,
			descriptor providers.Descriptor,
		) (catalog.ProbeFacts, error) {
			probeCalls++
			return catalog.ProbeFacts{
				Readiness:     descriptor.Readiness,
				Prerequisites: descriptor.Prerequisites,
			}, nil
		})),
		WithCommandRunner(platformRunner),
		WithWorkersCommandRunner(workersRunner),
		WithAgyPTY(AgyPTYPlatformDependencies{
			Allocator: agyAllocator,
			Locator:   agyLocator,
			Inspector: agyInspector,
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to providers.Service")
	}

	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want 0", probeCalls)
	}
	if platformRunner.calls != 0 {
		t.Fatalf("platform command runner calls = %d, want inert construction", platformRunner.calls)
	}
	if workersRunner.calls != 0 {
		t.Fatalf("workers command runner calls = %d, want inert construction", workersRunner.calls)
	}
	if agyAllocator.calls != 0 || agyLocator.calls != 0 || agyInspector.calls != 0 {
		t.Fatalf(
			"construction invoked Agy PTY platform effects (allocate=%d lookpath=%d stat=%d), want inert construction",
			agyAllocator.calls,
			agyLocator.calls,
			agyInspector.calls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d",
			baseline,
			runtime.NumGoroutine(),
			leaked,
		)
	}

	result, listErr := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if listErr != nil {
		t.Fatalf("ListProviders() = %v", listErr)
	}
	if len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = %#v, want non-empty migrated catalog after inert construction", result)
	}
}

func TestNewServiceAgyExecuteFailsClosedWithoutInjectedPTY(t *testing.T) {
	t.Parallel()

	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDAntigravity,
		AttemptID: "agy-without-pty-effects",
	})
	if !reflectDeepZeroExecuteResult(result) {
		t.Fatalf("Execute(agy) result = %#v, want zero result on dependency failure", result)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency ||
		!strings.Contains(failure.Message, "Antigravity") {
		t.Fatalf(
			"Execute(antigravity) error = %#v, want Antigravity dependency-normalized failure without injected PTY effects",
			executeErr,
		)
	}
}

func TestNewServiceInjectsPlatformDependenciesThroughWireOptions(t *testing.T) {
	t.Parallel()

	workersRunner := &recordingWorkersCommandRunner{}
	agyAllocator := &recordingPTYAllocator{
		result: PTYSessionResult{ExitCode: 0, CleanedText: "agy via wire"},
	}
	agyPath := filepath.Join(t.TempDir(), "agy")
	agyLocator := fakeExecutableLocator{string(providers.IDAntigravity): agyPath}
	agyInspector := fakeExecutableInspector{agyPath: fakeExecutableInfo{directory: false}}

	root, err := NewService(
		WithWorkersCommandRunner(workersRunner),
		WithAgyPTY(AgyPTYPlatformDependencies{
			Allocator: agyAllocator,
			Locator:   agyLocator,
			Inspector: agyInspector,
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if workersRunner.calls != 0 || agyAllocator.calls != 0 {
		t.Fatalf(
			"construction invoked platform effects (runner=%d agy allocate=%d), want inert construction",
			workersRunner.calls,
			agyAllocator.calls,
		)
	}

	agyResult, agyErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAntigravity,
		AttemptID:        "agy-platform-injection",
		WorkingDirectory: t.TempDir(),
		UserMessage:      "hello through wire",
	})
	if agyErr != nil {
		t.Fatalf("Execute(agy) error = %v, want success with injected PTY platform", agyErr)
	}
	if agyAllocator.calls != 1 {
		t.Fatalf("agy allocator calls = %d, want injected Agy PTY platform used on execute", agyAllocator.calls)
	}
	if agyResult.Content != "agy via wire" {
		t.Fatalf("Execute(agy) content = %q, want injected allocator output", agyResult.Content)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestNewServiceServesPublishedCatalogAndExecuteCompositionForMigratedIdentities(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	root, err := NewService(CatalogOption(catalogwire.WithProbeQuery(func(
		_ context.Context,
		descriptor providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{
			Readiness:     descriptor.Readiness,
			Prerequisites: descriptor.Prerequisites,
		}, nil
	})))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want inert construction", probeCalls)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	wantMigratedIDs := []providers.ID{
		providers.IDAntigravity,
		providers.IDClaude,
		providers.IDCodex,
	}
	byID := indexProvidersByID(list.Providers)
	for _, id := range wantMigratedIDs {
		descriptor, ok := byID[id]
		if !ok {
			t.Fatalf("ListProviders() missing migrated identity %q", id)
		}
		if descriptor.ID != id {
			t.Fatalf("ListProviders()[%q].ID = %q", id, descriptor.ID)
		}
	}

	for _, id := range wantMigratedIDs {
		got, getErr := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: id})
		if getErr != nil {
			t.Fatalf("GetProvider(%q) = %v", id, getErr)
		}
		if got.Provider.ID != id {
			t.Fatalf("GetProvider(%q).Provider.ID = %q", id, got.Provider.ID)
		}
	}

	probeCallsBeforeExecute := probeCalls
	executeTests := []struct {
		id   providers.ID
		name string
	}{
		{id: providers.IDCodex, name: "Codex"},
		{id: providers.IDClaude, name: "Claude"},
		{id: providers.IDAntigravity, name: "Antigravity"},
	}
	for _, test := range executeTests {
		_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  test.id,
			AttemptID: "migrated-composition-" + string(test.id),
		})
		if errors.Is(executeErr, providers.ErrUnknownProvider) {
			t.Fatalf(
				"Execute(%q) = %v, want execution bound through published registry",
				test.id,
				executeErr,
			)
		}
		var failure providers.ExecuteFailure
		if !errors.As(executeErr, &failure) ||
			failure.Kind != providers.ExecuteFailureKindDependency ||
			!strings.Contains(failure.Message, test.name) {
			t.Fatalf(
				"Execute(%q) error = %#v, want dependency failure from bound %s adapter without effects",
				test.id,
				executeErr,
				test.name,
			)
		}
	}
	if probeCalls <= probeCallsBeforeExecute {
		t.Fatalf(
			"execution probe calls = %d before %d after explicit Execute, want catalog probing only after Execute",
			probeCallsBeforeExecute,
			probeCalls,
		)
	}
}

func TestNewServiceBindsCodexAndClaudeFromCatalogWithoutEffects(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	root, err := NewService(CatalogOption(catalogwire.WithProbeQuery(func(
		_ context.Context,
		descriptor providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{
			Readiness:     descriptor.Readiness,
			Prerequisites: descriptor.Prerequisites,
		}, nil
	})))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want 0", probeCalls)
	}

	for _, test := range []struct {
		id   providers.ID
		name string
	}{
		{id: providers.IDCodex, name: "Codex"},
		{id: providers.IDClaude, name: "Claude"},
	} {
		_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  test.id,
			AttemptID: "composition-attempt",
		})
		var failure providers.ExecuteFailure
		if !errors.As(executeErr, &failure) ||
			failure.Kind != providers.ExecuteFailureKindDependency ||
			!strings.Contains(failure.Message, test.name) {
			t.Fatalf(
				"Execute(%q) error = %#v, want matching private adapter",
				test.id,
				executeErr,
			)
		}
	}
	if probeCalls != 2 {
		t.Fatalf("execution probe calls = %d, want one per explicit selection", probeCalls)
	}
}

func TestNewServiceRejectsMissingRequiredConstructionPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func() (providers.Service, error)
		want string
	}{
		{
			name: "catalog",
			call: func() (providers.Service, error) {
				return newRoot(
					nil,
					nil,
					nil,
					nil,
					nil,
					AgyPTYPlatformDependencies{},
					nil,
					nil,
					nil,
					nil,
				)
			},
			want: "construct Providers: catalog is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service, err := test.call()
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s construction port", test.name)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService() error = %q, want %q", err.Error(), test.want)
			}
		})
	}

	service, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v, want successful construction with required ports", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service, want non-nil providers.Service")
	}
	var root providers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to providers.Service")
	}
}

type inertPlatformCommandRunner struct {
	calls int
}

func (r *inertPlatformCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	panic("platform command runner invoked during inert construction")
}

type inertWorkersCommandRunner struct {
	calls int
}

func (r *inertWorkersCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	panic("workers command runner invoked during inert construction")
}

type inertPTYAllocator struct {
	calls int
}

func (a *inertPTYAllocator) Allocate(
	_ context.Context,
	_ PTYProcessLaunch,
	_ PTYSessionConfig,
) (PTYSession, error) {
	a.calls++
	panic("agy PTY allocation during inert construction")
}

type inertExecutableLocator struct {
	calls int
}

func (l *inertExecutableLocator) LookPath(string) (string, error) {
	l.calls++
	panic("agy executable lookup during inert construction")
}

type inertPathInspector struct {
	calls int
}

func (i *inertPathInspector) Stat(string) (fs.FileInfo, error) {
	i.calls++
	panic("agy path inspect during inert construction")
}

func reflectDeepZeroExecuteResult(result providers.ExecuteResult) bool {
	return result == providers.ExecuteResult{}
}

func indexProvidersByID(descriptors []providers.Descriptor) map[providers.ID]providers.Descriptor {
	byID := make(map[providers.ID]providers.Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
	}
	return byID
}

type recordingWorkersCommandRunner struct {
	calls int
}

func (r *recordingWorkersCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{}, nil
}

type recordingPTYAllocator struct {
	calls  int
	result PTYSessionResult
}

func (a *recordingPTYAllocator) Allocate(
	_ context.Context,
	_ PTYProcessLaunch,
	_ PTYSessionConfig,
) (PTYSession, error) {
	a.calls++
	return &recordingPTYSession{result: a.result}, nil
}

type recordingPTYSession struct {
	result PTYSessionResult
}

func (s *recordingPTYSession) Run(context.Context) (PTYSessionResult, error) {
	return s.result, nil
}

func (s *recordingPTYSession) Close() error { return nil }

type fakeExecutableLocator map[string]string

func (l fakeExecutableLocator) LookPath(name string) (string, error) {
	if path, ok := l[name]; ok {
		return path, nil
	}
	return "", fs.ErrNotExist
}

type fakeExecutableInspector map[string]fakeExecutableInfo

func (i fakeExecutableInspector) Stat(path string) (fs.FileInfo, error) {
	if info, ok := i[path]; ok {
		return info, nil
	}
	return nil, fs.ErrNotExist
}

type fakeExecutableInfo struct {
	directory bool
}

func (i fakeExecutableInfo) Name() string       { return "agy" }
func (i fakeExecutableInfo) Size() int64        { return 0 }
func (i fakeExecutableInfo) Mode() fs.FileMode  { return 0o755 }
func (i fakeExecutableInfo) ModTime() time.Time { return time.Time{} }
func (i fakeExecutableInfo) IsDir() bool        { return i.directory }
func (i fakeExecutableInfo) Sys() any           { return nil }

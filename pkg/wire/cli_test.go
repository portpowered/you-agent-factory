package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

type processCommandRunner struct{}

func TestProvideSessionsCLIServiceReturnsConstructedAdapter(t *testing.T) {
	t.Parallel()

	standard, err := provideStandardCLIHTTPProtocol()
	if err != nil {
		t.Fatal(err)
	}
	service := provideSessionsCLIService(standard, factorysessionwire.NewRequestPreparation())
	if service == nil {
		t.Fatal("provideSessionsCLIService() = nil, want Sessions CLI service")
	}
	if err := service.Show(sessioncli.ShowConfig{SessionID: "session-alpha"}); err == nil {
		t.Fatal("Show without output = nil, want required output error")
	}
}

func TestProvideLocalWorkerSessionsBoundaryUsesProviderInvocationRoute(t *testing.T) {
	t.Parallel()

	eventsService, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("construct events service: %v", err)
	}
	routes := make(chan string, 1)
	publishers := make(chan workers.ProgressPublisher, 1)
	workerService := localBoundaryWorkersService{routes: routes, publishers: publishers}
	boundary, err := provideLocalWorkerSessionsBoundary(
		eventsService,
		localBoundaryProviderSessions{},
		logging.NoopLogger{},
		workerService,
		nil,
	)
	if err != nil {
		t.Fatalf("provideLocalWorkerSessionsBoundary() error = %v", err)
	}
	t.Cleanup(func() {
		if err := boundary.Close(context.Background()); err != nil {
			t.Errorf("local boundary Close() error = %v", err)
		}
	})

	started, err := boundary.Start(context.Background(), workersessions.StartRequest{
		RequestID: "request-local",
		ID:        "session-local",
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "authored-name",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID:      "dispatch-local",
					WorkstationName: "authored-name",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("local boundary Start() error = %v", err)
	}
	if started.Session.ID != "session-local" {
		t.Fatalf("started session ID = %q, want session-local", started.Session.ID)
	}
	select {
	case route := <-routes:
		if route != workers.ProviderInvocationRoute {
			t.Fatalf("provider invocation route = %q, want %q", route, workers.ProviderInvocationRoute)
		}
	case <-time.After(time.Second):
		t.Fatal("provider invocation did not receive the admitted local dispatch")
	}
	select {
	case publisher := <-publishers:
		if publisher == nil {
			t.Fatal("local Worker Sessions execution omitted the request-scoped progress publisher")
		}
	case <-time.After(time.Second):
		t.Fatal("local Worker Sessions execution did not receive the progress publisher")
	}
}

func TestProvideLocalWorkerSessionsBoundaryRequiresWorkersService(t *testing.T) {
	t.Parallel()

	_, err := provideLocalWorkerSessionsBoundary(
		nil,
		nil,
		logging.NoopLogger{},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "Workers service is required") {
		t.Fatalf("provideLocalWorkerSessionsBoundary(nil Workers service) error = %v, want required-service diagnostic", err)
	}
}

func TestLocalWorkerSessionsBoundaryRejectsControlsWhenServiceUnavailable(t *testing.T) {
	t.Parallel()

	boundary := &localWorkerSessionsBoundary{}
	if _, err := boundary.Start(context.Background(), workersessions.StartRequest{}); err == nil {
		t.Fatal("Start() error = nil, want unavailable local Worker Sessions service")
	}
	if _, err := boundary.Continue(context.Background(), workersessions.ContinueRequest{}); err == nil {
		t.Fatal("Continue() error = nil, want unavailable local Worker Sessions service")
	}
	if _, err := boundary.Interrupt(context.Background(), workersessions.InterruptRequest{}); err == nil {
		t.Fatal("Interrupt() error = nil, want unavailable local Worker Sessions service")
	}
	for _, control := range []struct {
		name string
		call func() (workersessions.ControlResult, error)
	}{
		{name: "pause", call: func() (workersessions.ControlResult, error) {
			return boundary.Pause(context.Background(), workersessions.ControlRequest{})
		}},
		{name: "resume", call: func() (workersessions.ControlResult, error) {
			return boundary.Resume(context.Background(), workersessions.ControlRequest{})
		}},
		{name: "cancel", call: func() (workersessions.ControlResult, error) {
			return boundary.Cancel(context.Background(), workersessions.ControlRequest{})
		}},
		{name: "terminate", call: func() (workersessions.ControlResult, error) {
			return boundary.Terminate(context.Background(), workersessions.ControlRequest{})
		}},
	} {
		t.Run(control.name, func(t *testing.T) {
			t.Parallel()
			if _, err := control.call(); err == nil {
				t.Fatalf("%s() error = nil, want unavailable local Worker Sessions service", control.name)
			}
		})
	}
	if _, err := boundary.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{}); err == nil {
		t.Fatal("StreamObservationsByWorkerSessionID() error = nil, want unavailable local Worker Sessions service")
	}
}

type localBoundaryWorkersService struct {
	workers.ModelInvoker
	routes     chan<- string
	publishers chan<- workers.ProgressPublisher
}

func (e localBoundaryWorkersService) Execute(
	_ context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	e.routes <- request.Target.WorkstationName
	if e.publishers != nil {
		e.publishers <- request.Input.ProgressPublisher
	}
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
	}, nil
}

type localBoundaryProviderSessions struct {
	providersessions.Service
}

func (localBoundaryProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

func TestCLIRunDefaultsRetainWireSelectedRecordingTargetPlanner(t *testing.T) {
	t.Parallel()

	planner := recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		return recordings.LiveRecordingTarget{}, nil
	})
	recordingsCLI := provideRecordingsCLIAdapter()
	defaults := provideCLIRunDefaults(planner, recordingsCLI)
	if defaults.RecordingTargetPlanner == nil {
		t.Fatal("CLI run defaults dropped the Wire-selected recording target planner")
	}
	if defaults.RecordingsCLI == nil {
		t.Fatal("CLI run defaults dropped the Wire-selected Recordings CLI adapter")
	}
}

func TestProductionLiveRecordingTargetPlannerIsUsable(t *testing.T) {
	t.Parallel()

	target, err := provideLiveRecordingTargetPlanner().PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir:           t.TempDir(),
		ReportedSessionID: "~default",
	})
	if err != nil {
		t.Fatalf("PlanLiveRecordingTarget: %v", err)
	}
	if target.ServicePath == "" || target.ReportedPath == "" || target.ServicePath == target.ReportedPath {
		t.Fatalf("target = %#v, want distinct runtime template and reported paths", target)
	}
}

func (*processCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

func TestGeneratedBundleHasNoSecondaryRuntimeOrInvocationBuilder(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("wire_gen.go")
	if err != nil {
		t.Fatalf("read wire_gen.go: %v", err)
	}
	for _, forbidden := range []string{
		"ProvideRuntimeBuilder(",
		"NewInvocationBootstrapBuilder(",
		"NewRuntimeFactory(runtimeBuilder",
		"NewRuntimeFactoryFromOpening(",
		"application.NewFactory(",
		"application2.NewFactory(",
		"stdio.NewFactory(",
		"provideScopeAttacher(",
		"OpenedRuntime",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("wire_gen.go contains secondary composition %q", forbidden)
		}
	}
}

func TestRootBundleProducesFreshDetachedCLIObservation(t *testing.T) {
	t.Parallel()

	observations := make([]cliobservation.Result, 0, 2)
	rootBundle, err := InjectBundle(t.Context(), serviceedges.Edges{CLIObserver: cliobservation.CaptureAppend(&observations)})
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}
	executeCLIObservation(t, rootBundle)
	executeCLIObservation(t, rootBundle)
	if len(observations) != 2 {
		t.Fatalf("CLI observations = %d, want 2", len(observations))
	}
	first, second := observations[0], observations[1]
	if !reflect.DeepEqual(first.Snapshot, second.Snapshot) {
		t.Fatal("reusable process produced different detached CLI snapshots")
	}
	if first.Snapshot.Commands.RootPath != "you" {
		t.Fatalf("root bundle command root = %q, want you", first.Snapshot.Commands.RootPath)
	}
}

func TestInjectBundleReturnsLazyServiceComposition(t *testing.T) {
	t.Parallel()

	var observation cliobservation.Result
	rootBundle, err := InjectBundle(t.Context(), serviceedges.Edges{CLIObserver: cliobservation.Capture(&observation)})
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}
	if rootBundle == nil {
		t.Fatal("InjectBundle() returned incomplete composition")
	}
	executeCLIObservation(t, rootBundle)
	if len(observation.Snapshot.Commands.Commands) == 0 {
		t.Fatal("InjectBundle() omitted the canonical CLI observation")
	}
}

func TestInjectBundlePreservesOverridesInCanonicalLazyComposition(t *testing.T) {
	t.Parallel()

	runner := &processCommandRunner{}
	var observation cliobservation.Result
	overrideEdges := serviceedges.Edges{ProviderCommandRunner: runner, CLIObserver: cliobservation.Capture(&observation)}
	custom, err := InjectBundle(t.Context(), overrideEdges)
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	if custom == nil {
		t.Fatal("InjectBundle() returned nil process")
	}
	executeCLIObservation(t, custom)
	if len(observation.Snapshot.Commands.Commands) == 0 {
		t.Fatal("InjectBundle() omitted the canonical lazy composition")
	}
}

func executeCLIObservation(t *testing.T, process *initializerapplication.Process) {
	t.Helper()
	home := t.TempDir()
	err := process.Execute(initializerapplication.Input{
		Args: []string{"you"}, Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		WorkingDirectory: home, Stdout: io.Discard, Stderr: io.Discard,
	})
	if err != nil {
		t.Fatalf("Process.Execute(observe CLI) error = %v", err)
	}
}

func TestEffectiveFactoryCatalogServiceFeedsListAndNameProjectionsOnce(t *testing.T) {
	t.Parallel()

	calls := 0
	expected := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			{
				Name: "alpha",
				Definition: &factorydefinitions.FactoryConfig{
					Description: &factorydefinitions.NameValueConfig{Value: "Alpha Factory"},
				},
			},
			{
				Name: "beta",
				Definition: &factorydefinitions.FactoryConfig{
					Description: &factorydefinitions.NameValueConfig{Value: "Beta Factory"},
				},
			},
		},
	}
	operation := factorydefinitions.EffectiveFactoryCatalogOperation(func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		calls++
		return expected, nil
	})
	definitions, err := provideEffectiveFactoryDefinitionsService(operation)
	if err != nil {
		t.Fatalf("provide effective Factory Definitions service: %v", err)
	}

	catalog, err := definitions.ListEffectiveFactories(
		context.Background(),
		factorydefinitions.ListEffectiveFactoriesRequest{
			ProjectRoot: "project",
			GlobalRoot:  "global",
		},
	)
	if err != nil {
		t.Fatalf("ListEffectiveFactories() error = %v", err)
	}
	listEntries, err := factorycli.ProjectEffectiveFactoryList(
		context.Background(),
		catalog,
		"beta",
	)
	if err != nil {
		t.Fatalf("ProjectEffectiveFactoryList() error = %v", err)
	}
	nameProjection, err := completionprojection.ProjectFactoryNames(
		context.Background(),
		catalog,
	)
	if err != nil {
		t.Fatalf("ProjectFactoryNames() error = %v", err)
	}

	if calls != 1 {
		t.Fatalf("catalog discovery calls = %d, want one shared service result", calls)
	}
	if len(listEntries) != 2 || len(nameProjection.Candidates) != 2 {
		t.Fatalf(
			"list entries = %#v, candidates = %#v, want two of each",
			listEntries,
			nameProjection.Candidates,
		)
	}
	gotListNames := []string{listEntries[0].Name, listEntries[1].Name}
	gotCandidateNames := []string{
		nameProjection.Candidates[0].Value,
		nameProjection.Candidates[1].Value,
	}
	if !reflect.DeepEqual(gotListNames, gotCandidateNames) {
		t.Fatalf("list names = %v, candidate names = %v", gotListNames, gotCandidateNames)
	}
	if !listEntries[1].Current {
		t.Fatalf("list entries = %#v, want beta marked current", listEntries)
	}
	if listEntries[0].Description != nameProjection.Candidates[0].Description ||
		listEntries[1].Description != nameProjection.Candidates[1].Description {
		t.Fatalf(
			"list descriptions = %#v, candidate descriptions = %#v",
			listEntries,
			nameProjection.Candidates,
		)
	}
}

func TestProvideListFactoriesOperationCallsFactoryDefinitionsOwner(t *testing.T) {
	t.Parallel()

	calls := 0
	definitions, err := provideEffectiveFactoryDefinitionsService(
		func(
			context.Context,
			factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			calls++
			return factorydefinitions.ListEffectiveFactoriesResult{
				Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
					Name:       "owned",
					Definition: &factorydefinitions.FactoryConfig{},
				}},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("provide effective Factory Definitions service: %v", err)
	}
	list := provideListFactoriesOperation(
		definitions,
		func(string) (string, error) { return "", nil },
	)
	var output bytes.Buffer
	if err := list(factorycli.ListConfig{
		Context:     context.Background(),
		ProjectRoot: "project",
		GlobalRoot:  "global",
		JSON:        true,
		Output:      &output,
	}); err != nil {
		t.Fatalf("list operation error = %v", err)
	}

	var entries []factorycli.ListEntry
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	if calls != 1 || len(entries) != 1 || entries[0].Name != "owned" {
		t.Fatalf("service calls = %d, entries = %#v", calls, entries)
	}
}

// TestACPCLIOperationsPersistAndProjectConfiguredWorkers proves the
// Wire-composed ACP CLI operations reach the Operator Settings and Providers
// owner roots rather than joining them inside the CLI transport: adding an
// integration persists it at the service-owned configuration path, listing
// projects it back as a custom ACP worker, and deleting it removes it from
// both the persisted document and the projected catalog.
func TestACPCLIOperationsPersistAndProjectConfiguredWorkers(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	ctx := context.Background()
	settings, providersRoot := newACPCLIOwnerRoots(t)
	operations := provideACPCLIService(
		settings,
		providersRoot,
		provideOperatorSettingsIDGenerator(serviceedges.Edges{}),
	)

	const workerName = "acme-acp"
	if err := operations.Add(ctx, home, workerName, "stdio", acpCLITestCommand); err != nil {
		t.Fatalf("Add(%q) error = %v", workerName, err)
	}
	assertOnlyPersistedACPIntegration(t, settings, home, workerName)

	catalog := listACPCLIWorkers(t, operations, home)
	if !catalog.Custom[providers.ID(workerName)] || len(catalog.Providers) == 0 {
		t.Fatalf(
			"ListWorkers() custom = %#v with %d descriptors, want %q projected alongside the Providers catalog",
			catalog.Custom, len(catalog.Providers), workerName,
		)
	}

	if err := operations.Delete(ctx, home, workerName); err != nil {
		t.Fatalf("Delete(%q) error = %v", workerName, err)
	}
	if remaining := loadPersistedACPIntegrations(t, settings, home); len(remaining) != 0 {
		t.Fatalf("persisted ACP integrations after delete = %#v, want none", remaining)
	}
	if listACPCLIWorkers(t, operations, home).Custom[providers.ID(workerName)] {
		t.Fatalf("ListWorkers() after delete still projects %q as a custom worker", workerName)
	}
}

// TestACPCLIOperationsRequireTheirOwnerRoots proves each composed ACP CLI
// operation refuses to run when Wire has not supplied the owner root it
// delegates to, so an incomplete composition surfaces as a customer-visible
// diagnostic instead of a nil dereference inside a command handler.
func TestACPCLIOperationsRequireTheirOwnerRoots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	home := t.TempDir()
	settings, providersRoot := newACPCLIOwnerRoots(t)
	generateID := provideOperatorSettingsIDGenerator(serviceedges.Edges{})
	withoutSettings := provideACPCLIService(nil, providersRoot, generateID)
	withoutProviders := provideACPCLIService(settings, nil, generateID)
	withoutIDs := provideACPCLIService(settings, providersRoot, nil)

	for _, testCase := range []struct {
		operation string
		call      func() error
		wantOwner string
	}{
		{"ListWorkers without Operator Settings", func() error {
			_, err := withoutSettings.ListWorkers(ctx, home)
			return err
		}, "Operator Settings"},
		{"Add without Operator Settings", func() error {
			return withoutSettings.Add(ctx, home, "acme-acp", "stdio", acpCLITestCommand)
		}, "Operator Settings"},
		{"Delete without Operator Settings", func() error {
			return withoutSettings.Delete(ctx, home, "acme-acp")
		}, "Operator Settings"},
		{"ListWorkers without Providers", func() error {
			_, err := withoutProviders.ListWorkers(ctx, home)
			return err
		}, "Providers"},
		{"Configure without Providers", func() error {
			return withoutProviders.Configure(ctx, nil)
		}, "Providers"},
		{"Add without an ID generator", func() error {
			return withoutIDs.Add(ctx, home, "acme-acp", "stdio", acpCLITestCommand)
		}, "ID generator"},
	} {
		err := testCase.call()
		if err == nil || !strings.Contains(err.Error(), testCase.wantOwner) {
			t.Fatalf("%s error = %v, want a %q diagnostic", testCase.operation, err, testCase.wantOwner)
		}
	}
}

// acpCLITestCommand is the launch command the ACP CLI operation tests persist
// and read back through the Operator Settings owner root.
const acpCLITestCommand = "acme --acp"

// loadPersistedACPIntegrations reads the ACP integrations Operator Settings
// persisted at its own service-owned configuration path for home.
func loadPersistedACPIntegrations(
	t *testing.T,
	settings operatorsettings.Service,
	home string,
) []operatorsettings.ACPIntegration {
	t.Helper()

	configPath := settings.DefaultConfigPath(home)
	loaded, err := settings.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument(%q) error = %v", configPath, err)
	}
	return loaded.Document.Workers.ACP.Integrations
}

// assertOnlyPersistedACPIntegration asserts the named worker is the single
// persisted ACP integration and that Operator Settings assigned it an ID.
func assertOnlyPersistedACPIntegration(
	t *testing.T,
	settings operatorsettings.Service,
	home string,
	name string,
) {
	t.Helper()

	persisted := loadPersistedACPIntegrations(t, settings, home)
	if len(persisted) != 1 {
		t.Fatalf("persisted ACP integrations = %#v, want exactly the added %q worker", persisted, name)
	}
	added := persisted[0]
	if added.Name != name || added.Transport != "stdio" || added.Command != acpCLITestCommand || added.ID == "" {
		t.Fatalf("persisted ACP integration = %#v, want %q on stdio with its command and a generated ID", added, name)
	}
}

// listACPCLIWorkers drives the composed list operation and fails the test on
// any error so callers can assert directly on the projected catalog.
func listACPCLIWorkers(t *testing.T, operations acpcli.Operations, home string) acpcli.WorkerCatalog {
	t.Helper()

	catalog, err := operations.ListWorkers(context.Background(), home)
	if err != nil {
		t.Fatalf("ListWorkers() error = %v", err)
	}
	return catalog
}

// newACPCLIOwnerRoots composes the real Operator Settings and Providers roots
// through their production Wire providers, so tests observe the same owner
// behavior the composed CLI operations reach at runtime.
func newACPCLIOwnerRoots(t *testing.T) (operatorsettings.Service, providers.Service) {
	t.Helper()

	edges := serviceedges.Edges{}
	providersRoot, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	zapLogger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("logging.NewDefaultLogger() error = %v", err)
	}
	settings, err := provideOperatorSettingsService(
		provideOperatorSettingsFileSystem(edges),
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providersRoot),
		provideOperatorConfigDecoder(),
		provideOperatorConfigDiagnosticsDecoder(),
		provideOperatorConfigEncoder(),
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logging.NewZapLogger(zapLogger, false),
	)
	if err != nil {
		t.Fatalf("provideOperatorSettingsService() error = %v", err)
	}
	return settings, providersRoot
}

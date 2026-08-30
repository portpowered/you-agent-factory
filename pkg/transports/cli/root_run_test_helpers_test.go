package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	configcli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/config"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	modelcontract "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	submitcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var runCLI = func(context.Context, runcli.RunConfig) error {
	return fmt.Errorf("run service initializer is required")
}
var flattenFactoryConfig = func(configcli.FactoryConfigFlattenConfig) error {
	return fmt.Errorf("Factory config flatten operation is required")
}
var expandFactoryConfig = func(configcli.FactoryConfigExpandConfig) error {
	return fmt.Errorf("Factory config expand operation is required")
}
var initFactory interfaces.ScaffoldInitializer = func(interfaces.ScaffoldConfig) error {
	return fmt.Errorf("Factory scaffold initializer is required")
}
var submitWork = submitcli.NewSubmit(work.PayloadFileReader(os.ReadFile), rootTestHTTPProtocol())
var submitBatch = submitcli.NewSubmitBatch(rootTestHTTPProtocol(), rootTestBatchPreparation{})
var listWork = workcli.NewList(rootTestHTTPProtocol(), rootTestWorkListPreparation{})
var showWork = workcli.NewShow(rootTestHTTPProtocol())
var moveWork = workcli.NewMove(rootTestHTTPProtocol())
var visualizeWork = func(workcli.VisualizeConfig) error {
	return fmt.Errorf("Work visualization operation is required")
}
var listSessions = sessioncli.NewList(rootTestHTTPProtocol(), rootRequestPreparation{})
var showSession = sessioncli.NewShow(rootTestHTTPProtocol())
var pauseSession = sessioncli.NewPause(rootTestHTTPProtocol())
var resumeSession = sessioncli.NewResume(rootTestHTTPProtocol())
var createSession = sessioncli.NewCreate(rootTestHTTPProtocol())
var deleteSession = sessioncli.NewDelete(rootTestHTTPProtocol())
var rootTestSessionsCLI = func() sessioncli.Service {
	return sessioncli.Bind(sessioncli.Operations{
		List: listSessions, Show: showSession, Pause: pauseSession, Resume: resumeSession,
		Create: createSession, Delete: deleteSession,
	})
}
var queryFactory = factorycli.NewQuery(rootTestHTTPProtocol())

type rootTestBatchPreparation struct{}

func (rootTestBatchPreparation) PrepareFactoryRequestBatch(
	_ context.Context,
	data []byte,
) (work.PreparedFactoryRequestBatch, error) {
	var request work.WorkRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return work.PreparedFactoryRequestBatch{}, err
	}
	return work.PreparedFactoryRequestBatch{Request: request, CanonicalJSON: append([]byte(nil), data...)}, nil
}

type rootTestWorkListPreparation struct{}

func (rootTestWorkListPreparation) PrepareListRequest(
	_ context.Context,
	options work.ListOptions,
) (work.PreparedListRequest, error) {
	return work.PreparedListRequest{Options: options, FilterSummary: "test"}, nil
}

type rootRequestPreparation struct{}

func (rootRequestPreparation) PrepareStart(request factorysessions.StartRequest) (factorysessions.StartRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareControl(request factorysessions.ControlRequest) (factorysessions.ControlRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareApprove(request factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareRetryDispatch(request factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareInterruptDispatch(request factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareListSessions(request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareResult(request factorysessions.ResultRequest) (factorysessions.ResultRequest, error) {
	return request, nil
}
func (rootRequestPreparation) PrepareEventReconnect(request factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error) {
	return request, nil
}

type rootNamedFactoryCatalogFake struct {
	resolve func(string, string, string) (*interfaces.NamedFactoryResolution, error)
}

func (rootNamedFactoryCatalogFake) ListNamedFactories(string) ([]interfaces.NamedFactoryListEntry, error) {
	return nil, nil
}
func (rootNamedFactoryCatalogFake) DeleteNamedFactory(string, string) error { return nil }
func (catalog rootNamedFactoryCatalogFake) ResolveNamedFactoryAcrossRoots(
	projectRoot,
	globalRoot,
	name string,
) (*interfaces.NamedFactoryResolution, error) {
	if catalog.resolve == nil {
		return nil, fmt.Errorf("%w: %s", interfaces.ErrNamedFactoryNotFound, name)
	}
	return catalog.resolve(projectRoot, globalRoot, name)
}

var listFactories = factorycli.NewList(
	func(
		context.Context,
		interfaces.ListEffectiveFactoriesRequest,
	) (interfaces.ListEffectiveFactoriesResult, error) {
		return interfaces.ListEffectiveFactoriesResult{}, nil
	},
	func(string) (string, error) { return "", fs.ErrNotExist },
)
var validateFactory = func(config factorycli.ValidateConfig) error {
	return fmt.Errorf("Factory Definition validator is required")
}
var createFactoryFromFile = factorycli.CreateFromFile
var replaceFactoryCurrent = factorycli.NewReplaceCurrent(rootTestHTTPProtocol())
var updateFactoryFromFile = factorycli.UpdateFromFile
var deleteFactory = factorycli.NewDelete(rootNamedFactoryCatalogFake{})

type rootModelInvocationOperation struct{}

func (rootModelInvocationOperation) ResolveModelInvocationFactoryDir(dir string) (string, error) {
	return dir, nil
}
func (rootModelInvocationOperation) ExportModelInvocationArtifact(string, string) error { return nil }
func (rootModelInvocationOperation) InvokeModel(context.Context, modelscli.InvocationTarget, string, modelcontract.Request) (modelcontract.Result, error) {
	return modelcontract.Result{}, fmt.Errorf("model invocation test operation is not configured")
}

var rootModelsCLI = modelscli.New(rootTestHTTPProtocol(), rootModelInvocationOperation{})

func newLegacyTestRootCommand() *cobra.Command {
	return newLegacyTestRootCommandWithCatalog(rootNamedFactoryCatalogFake{})
}

func withTestInjectedPlatformRoles(factory CommandFactory) CommandFactory {
	if factory.ModelsCLI == nil {
		factory.ModelsCLI = rootModelsCLI
	}
	factory.prepareInvocationInput = rootInvocationInputScript{prepare: func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
		return work.PreparedInvocationInput{}, nil
	}}
	factory.resolveOperatorDefaults = func(string, operatorconfig.Defaults, operatorconfig.FlagOverrides) (operatorconfig.ResolvedDefaults, error) {
		return operatorconfig.ResolvedDefaults{}, nil
	}
	factory.resolveNamedFactoryRoots = func(homeDir, workingDir string) (interfaces.NamedFactoryRoots, error) {
		return interfaces.NamedFactoryRoots{Project: workingDir, Global: homeDir}, nil
	}
	factory.resolveNamedFactoryCandidatePaths = func(projectRoot, globalRoot, _ string) (interfaces.NamedFactoryCandidatePaths, error) {
		return interfaces.NamedFactoryCandidatePaths{Project: projectRoot, Global: globalRoot}, nil
	}
	factory.buildTerminalLogger = func(terminalpolicy.Mode, bool) (*zap.Logger, error) {
		return zap.NewNop(), nil
	}
	factory.resolveCurrentFactoryDir = func(rootDir string) (string, error) {
		return rootDir, nil
	}
	factory.resolveFactoryConfigRoot = resolveTestFactoryConfigRoot
	factory.loadFactoryConfigFile = loadTestFactoryConfigFile
	factory.workRequestFileLoader = loadTestWorkRequestFile
	factory.openRunSelection = func(cfg runcli.RunConfig) startupcli.RunSelection {
		return testRunSelection{cfg: cfg}
	}
	factory.runDefaults = runcli.RunConfig{
		RuntimeLogConfig: logging.RuntimeLogConfig{
			MaxSize: 100, MaxBackups: 20, MaxAge: 30,
		},
		RuntimeMetricsConfig: platformmetrics.RuntimeMetricsConfig{
			MaxSize: 100, MaxBackups: 20, MaxAge: 30,
		},
		RecordingsCLI: recordingscli.New(),
	}
	factory.runDirectoryCreator = testRunDirectoryCreator{}
	factory.browserOpener = func(context.Context, string) error { return nil }
	if factory.completePackagedFactoryNames == nil {
		factory.completePackagedFactoryNames = func(context.Context, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
	if factory.InstallPackagedFactory == nil {
		factory.InstallPackagedFactory = func(factorydefinitionscli.InstallPackagedFactoryConfig) error {
			return errors.New("install packaged factory test operation is not configured")
		}
	}
	return factory
}

// rootInvocationInputScript supplies detached Work-role responses to legacy
// command-boundary tests. Work policy invariants are covered in the owner
// package; these tests exercise only how the CLI maps the returned values.
type rootInvocationInputScript struct {
	prepare func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error)
}

func (root rootInvocationInputScript) PrepareInvocationInput(
	ctx context.Context,
	request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	return root.prepare(ctx, request)
}

type testRunSelection struct{ cfg runcli.RunConfig }

func (selection testRunSelection) Open(context.Context, startupcli.RunIntent) (initializer.RunApplication, error) {
	return runApplicationFuncForCLITest(func(context.Context) error { return nil }), nil
}

type runApplicationFuncForCLITest func(context.Context) error

func (run runApplicationFuncForCLITest) Run(ctx context.Context) error { return run(ctx) }

func testRunConfig(selection startupcli.RunSelection) runcli.RunConfig {
	return selection.(testRunSelection).cfg
}

func resolveTestFactoryConfigRoot(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("factory config path is required")
	}
	resolved, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve factory config path %s: %w", trimmed, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("factory config file not found: %s", trimmed)
		}
		return "", fmt.Errorf("find factory config file %s: %w", trimmed, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("factory config path must be a file: %s", trimmed)
	}
	return filepath.Dir(resolved), nil
}

func loadTestFactoryConfigFile(path string) (*interfaces.FactoryConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("factory config file not found: %s", path)
		}
		return nil, fmt.Errorf("read factory config file %s: %w", path, err)
	}
	cfg, err := factorymapping.NewFactoryConfigMapper().Expand(data)
	if err != nil {
		return nil, fmt.Errorf("parse factory config %s: %w", path, err)
	}
	return cfg, nil
}

func loadTestWorkRequestFile(path string) (work.WorkRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return work.WorkRequest{}, err
	}
	var request work.WorkRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return work.WorkRequest{}, err
	}
	return request, nil
}

type testRunDirectoryCreator struct{}

func (testRunDirectoryCreator) MkdirAll(string, os.FileMode) error { return nil }

func newLegacyTestRootCommandWithCatalog(
	catalog interfaces.NamedFactoryCatalog,
) *cobra.Command {
	return newLegacyTestRootCommandWithCatalogAndOperatorDefaults(catalog, nil)
}

func newLegacyTestRootCommandWithOperatorDefaults(
	resolve operatorconfig.DefaultsResolver,
) *cobra.Command {
	return newLegacyTestRootCommandWithCatalogAndOperatorDefaults(rootNamedFactoryCatalogFake{}, resolve)
}

func expectOperatorDefaultsResolution(
	t *testing.T,
	wantEnvironment operatorconfig.Defaults,
	wantFlags operatorconfig.FlagOverrides,
	result operatorconfig.ResolvedDefaults,
	resultErr error,
) operatorconfig.DefaultsResolver {
	t.Helper()
	return func(_ string, environment operatorconfig.Defaults, flags operatorconfig.FlagOverrides) (operatorconfig.ResolvedDefaults, error) {
		if environment != wantEnvironment {
			t.Fatalf("operator defaults environment = %+v, want %+v", environment, wantEnvironment)
		}
		if flags != wantFlags {
			t.Fatalf("operator defaults flags = %+v, want %+v", flags, wantFlags)
		}
		return result, resultErr
	}
}

func newLegacyTestRootCommandWithCatalogAndOperatorDefaults(
	catalog interfaces.NamedFactoryCatalog,
	resolve operatorconfig.DefaultsResolver,
) *cobra.Command {
	return newLegacyTestRootCommandWithCatalogDefaultsAndInvocation(catalog, resolve, rootInvocationInputScript{})
}

func newLegacyTestRootCommandWithCatalogDefaultsAndInvocation(
	catalog interfaces.NamedFactoryCatalog,
	resolve operatorconfig.DefaultsResolver,
	prepare rootInvocationInputScript,
) *cobra.Command {
	factory := withTestInjectedPlatformRoles(CommandFactory{
		namedFactoryCatalog: catalog,
		SubmitWork:          submitWork, SubmitBatch: submitBatch,
		SessionsCLI:          rootTestSessionsCLI(),
		LocalSessionsCLI:     rootTestSessionsCLI(),
		ModelsCLI:            rootModelsCLI,
		FlattenFactoryConfig: flattenFactoryConfig,
		ExpandFactoryConfig:  expandFactoryConfig, InitFactory: initFactory,
		QueryFactory: queryFactory, ListFactories: listFactories,
		ValidateFactory: validateFactory, CreateFactoryFromFile: createFactoryFromFile,
		ReplaceFactoryCurrent: replaceFactoryCurrent, UpdateFactoryFromFile: updateFactoryFromFile,
		DeleteFactory: deleteFactory,
		ListWork:      listWork, ShowWork: showWork,
		MoveWork: moveWork, VisualizeWork: visualizeWork,
	})
	if resolve != nil {
		factory.resolveOperatorDefaults = resolve
	}
	if prepare.prepare != nil {
		factory.prepareInvocationInput = prepare
	}
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
		RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
			return runCLI(ctx, testRunConfig(selection))
		},
	})
	if workingDirectory, err := os.Getwd(); err == nil {
		root.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))
	}
	return root
}

func newLegacyTestRootCommandWithInvocationInput(prepare rootInvocationInputScript) *cobra.Command {
	return newLegacyTestRootCommandWithCatalogDefaultsAndInvocation(rootNamedFactoryCatalogFake{}, nil, prepare)
}

func newLegacyTestRootCommandWithCatalogAndInvocationInput(catalog interfaces.NamedFactoryCatalog, prepare rootInvocationInputScript) *cobra.Command {
	return newLegacyTestRootCommandWithCatalogDefaultsAndInvocation(catalog, nil, prepare)
}

func programmedInvocationInput(result work.PreparedInvocationInput, err error) rootInvocationInputScript {
	return rootInvocationInputScript{prepare: func(context.Context, work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
		return result, err
	}}
}

func programmedTextInvocationInput(source work.InputSourceLabel, text string) rootInvocationInputScript {
	resolved := &work.ResolvedInput{Source: source, Text: text, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}}}
	return programmedInvocationInput(work.PreparedInvocationInput{Source: source, ResolvedInput: resolved}, nil)
}

func assertDirectCommandCount(t *testing.T, parent *cobra.Command, name string, want int) {
	t.Helper()
	count := 0
	for _, command := range parent.Commands() {
		if command.Name() == name {
			count++
		}
	}
	if count != want {
		t.Fatalf("%s direct %q command count = %d, want %d", parent.CommandPath(), name, count, want)
	}
}

func newComposedTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
		RunFunc: func(_ context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
			err := interfaces.NewBlockingFactoryLoadError(
				interfaces.ValidationResult{
					Targets: []interfaces.ValidationTarget{{
						Code:     "factory.topology.unknownReference",
						Severity: interfaces.ValidationSeverityError,
						Message:  "referenced Factory graph node does not exist",
						Subject: interfaces.ValidationSubject{
							Type: interfaces.ValidationSubjectTypeWorkstation,
							ID:   "missing-workstation",
						},
					}},
				},
			)
			return factoryload.MaybeFormatOperatorError(err, testRunConfig(selection).FactoryConfigPath)
		},
	})
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), t.TempDir()))
	return root
}

func exerciseBatchColdStartCLICharacterization(t *testing.T) {
	t.Helper()
	for _, test := range profileSelectedBatchSystemInitializationCases {
		cmd := &cobra.Command{Use: "run"}
		cmd.SetContext(context.Background())
		if test.changedFlag != "" {
			cmd.Flags().String(test.changedFlag, "", "")
			if err := cmd.Flags().Set(test.changedFlag, "selected"); err != nil {
				t.Fatalf("set changed flag %q: %v", test.changedFlag, err)
			}
		}
		options := CommandFactory{
			initializer: startupcli.Functions{
				InitializeSystemFunc: func(context.Context, string) error { return nil },
			},
		}
		allowed, err := prepareRunSystemInitialization(cmd, &test.cfg, options)
		if err != nil {
			t.Fatalf("prepareRunSystemInitialization(%s) error = %v", test.name, err)
		}
		if got := !allowed; got != test.wantDeferred {
			t.Fatalf("%s deferred = %t, want %t", test.name, got, test.wantDeferred)
		}
	}

	exerciseDeferredBatchSystemInitialization(t)
	exerciseExactFiniteMockBatchCommand(t)
	exerciseInvalidRecordingInputDoesNotActivate(t)
	exerciseDemandedBatchSystemInitialization(t)
	exerciseDemandedBatchCommandDoesNotDispatch(t)
	exerciseBatchColdStartProcessBoundaries(t)
}

func exerciseDeferredBatchSystemInitialization(t *testing.T) {
	t.Helper()
	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	options := CommandFactory{initializer: startupcli.Functions{
		InitializeSystemFunc: func(context.Context, string) error { calls++; return nil },
	}}
	cfg := runcli.RunConfig{
		WorkFile: "one-work.json", MockWorkersEnabled: true, DisableDefaultRecording: true,
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, options, false); err != nil {
		t.Fatalf("prepare deferred startup: %v", err)
	}
	if err := cfg.StartupPreparation(context.Background(), false, nil); err != nil {
		t.Fatalf("deferred StartupPreparation() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("deferred InitializeSystem calls = %d, want 0", calls)
	}
}

func exerciseExactFiniteMockBatchCommand(t *testing.T) {
	t.Helper()
	var initialized int
	var got runcli.RunConfig
	workingDirectory := t.TempDir()
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error { initialized++; return nil },
			RunFunc: func(_ context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
				got = testRunConfig(selection)
				return nil
			},
		},
	)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))
	root.SetArgs([]string{"run", "--work", "one-work.json", "--with-mock-workers=accept.json", "--no-record"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute exact finite mock batch: %v", err)
	}
	if initialized != 0 {
		t.Fatalf("exact batch InitializeSystem calls = %d, want 0", initialized)
	}
	if got.WorkFile != "one-work.json" || !got.MockWorkersEnabled ||
		got.MockWorkersConfigPath != "accept.json" || !got.DisableDefaultRecording {
		t.Fatalf("exact batch config = %+v, want work/mock/config/no-record", got)
	}
}

func exerciseInvalidRecordingInputDoesNotActivate(t *testing.T) {
	t.Helper()
	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	cfg := runcli.RunConfig{
		WorkFile: "one-work.json", RecordPath: "recording", ReplayPath: "replay",
		MockWorkersEnabled: true, DisableDefaultRecording: true,
	}
	allowed, err := prepareRunSystemInitialization(cmd, &cfg, CommandFactory{initializer: startupcli.Functions{
		InitializeSystemFunc: func(context.Context, string) error { calls++; return nil },
	}})
	if err != nil {
		t.Fatalf("invalid recording preflight error = %v, want deferred validation", err)
	}
	if allowed || calls != 0 {
		t.Fatalf("invalid recording preflight allowed/calls = %t/%d, want false/0", allowed, calls)
	}
}

func exerciseDemandedBatchSystemInitialization(t *testing.T) {
	t.Helper()
	wantErr := errors.New("controlled hosted system initialization failure")
	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	options := CommandFactory{initializer: startupcli.Functions{
		InitializeSystemFunc: func(context.Context, string) error { calls++; return wantErr },
	}}
	cfg := runcli.RunConfig{
		WorkFile: "one-work.json", MockWorkersEnabled: true,
		DisableDefaultRecording: true, WithServer: true,
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, options, false); err != nil {
		t.Fatalf("prepare demanded startup: %v", err)
	}
	first := cfg.StartupPreparation(context.Background(), false, nil)
	if !errors.Is(first, wantErr) {
		t.Fatalf("first demanded startup error = %v, want %v", first, wantErr)
	}
	second := cfg.StartupPreparation(context.Background(), false, nil)
	if !errors.Is(second, wantErr) || second != first {
		t.Fatalf("second demanded startup error = %v, want cached %v", second, first)
	}
	if calls != 1 {
		t.Fatalf("demanded InitializeSystem calls = %d, want one", calls)
	}

	var concurrentCalls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	concurrentCmd := &cobra.Command{Use: "run"}
	concurrentCmd.SetContext(context.Background())
	concurrentCfg := runcli.RunConfig{
		WorkFile: "one-work.json", MockWorkersEnabled: true,
		DisableDefaultRecording: true, WithServer: true,
	}
	concurrentOptions := CommandFactory{initializer: startupcli.Functions{
		InitializeSystemFunc: func(ctx context.Context, _ string) error {
			concurrentCalls.Add(1)
			entered <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}}
	if err := prepareRunFactoryStartup(concurrentCmd, &concurrentCfg, concurrentOptions, false); err != nil {
		t.Fatalf("prepare concurrent demanded startup: %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- concurrentCfg.StartupPreparation(context.Background(), false, nil) }()
	}
	awaitBatchColdStartProcessEntries(t, entered, 1)
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent demanded startup error = %v", err)
		}
	}
	if concurrentCalls.Load() != 1 {
		t.Fatalf("concurrent demanded InitializeSystem calls = %d, want one", concurrentCalls.Load())
	}
}

func exerciseDemandedBatchCommandDoesNotDispatch(t *testing.T) {
	t.Helper()
	wantErr := errors.New("controlled demanded command initialization failure")
	initialized, dispatched := 0, 0
	workingDirectory := t.TempDir()
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error { initialized++; return wantErr },
			RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
				dispatched++
				return nil
			},
		},
	)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))
	root.SetArgs([]string{"run", "--work", "one-work.json", "--with-mock-workers=accept.json", "--with-server", "--no-record"})
	err := root.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("demanded command error = %v, want %v", err, wantErr)
	}
	if initialized != 1 || dispatched != 0 {
		t.Fatalf("demanded command initialized/dispatched = %d/%d, want 1/0", initialized, dispatched)
	}
}

func exerciseBatchColdStartProcessBoundaries(t *testing.T) {
	t.Helper()
	exerciseBatchColdStartProcessReuse(t)
	exerciseBatchColdStartProcessConcurrentDemand(t)
	exerciseBatchColdStartProcessCancellation(t)
	exerciseBatchColdStartProcessRecovery(t)
}

type batchColdStartProcessProviderRegistry struct{}

func (batchColdStartProcessProviderRegistry) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}

type batchColdStartProcessLifecycle struct {
	closeCalls atomic.Int32
}

func (lifecycle *batchColdStartProcessLifecycle) Close(context.Context) error {
	lifecycle.closeCalls.Add(1)
	return nil
}

func newBatchColdStartApplicationProcess(
	t *testing.T,
	initializer startupcli.Initializer,
) (*initializerapplication.Process, *batchColdStartProcessLifecycle) {
	t.Helper()
	lifecycle := &batchColdStartProcessLifecycle{}
	process, err := initializerapplication.NewProcess(
		withTestInjectedPlatformRoles(CommandFactory{}), initializer,
		batchColdStartProcessProviderRegistry{}, lifecycle, nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}
	return process, lifecycle
}

func batchColdStartProcessInput(home, workingDirectory string, output *bytes.Buffer, server bool) initializerapplication.Input {
	args := []string{"you", "run", "--work", "one-work.json", "--with-mock-workers=accept.json", "--no-record"}
	if server {
		args = append(args, "--with-server")
	}
	return initializerapplication.Input{
		Args: args, Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		Stdout: output, WorkingDirectory: workingDirectory,
	}
}

func batchColdStartProcessRunFunctions(runCalls *atomic.Int32) startupcli.Functions {
	return startupcli.Functions{RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
		cfg := testRunConfig(selection)
		if cfg.StartupPreparation == nil {
			return errors.New("process characterization startup preparation is missing")
		}
		if err := cfg.StartupPreparation(ctx, false, nil); err != nil {
			return err
		}
		runCalls.Add(1)
		if cfg.Output == nil {
			return nil
		}
		_, err := fmt.Fprintf(cfg.Output, "processed:%s\n", cfg.WorkFile)
		return err
	}}
}

func exerciseBatchColdStartProcessReuse(t *testing.T) {
	t.Helper()
	var runCalls, initCalls atomic.Int32
	process, lifecycle := newBatchColdStartApplicationProcess(t, startupcli.Functions{
		InitializeSystemFunc: func(context.Context, string) error { initCalls.Add(1); return nil },
		RunFunc:              batchColdStartProcessRunFunctions(&runCalls).RunFunc,
	})
	workingDirectory, home := t.TempDir(), t.TempDir()
	var firstOutput, secondOutput bytes.Buffer
	if err := process.Execute(batchColdStartProcessInput(home, workingDirectory, &firstOutput, false)); err != nil {
		t.Fatalf("first reusable batch Execute() error = %v", err)
	}
	if err := process.Execute(batchColdStartProcessInput(home, workingDirectory, &secondOutput, false)); err != nil {
		t.Fatalf("second reusable batch Execute() error = %v", err)
	}
	if initCalls.Load() != 0 || runCalls.Load() != 2 {
		t.Fatalf("reusable batch init/run calls = %d/%d, want 0/2", initCalls.Load(), runCalls.Load())
	}
	if firstOutput.String() != "processed:one-work.json\n" || secondOutput.String() != firstOutput.String() {
		t.Fatalf("reusable batch outputs = %q / %q, want isolated identical results", firstOutput.String(), secondOutput.String())
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("close reusable process: %v", err)
	}
	if lifecycle.closeCalls.Load() != 1 {
		t.Fatalf("reusable process close calls = %d, want 1", lifecycle.closeCalls.Load())
	}
}

func exerciseBatchColdStartProcessConcurrentDemand(t *testing.T) {
	t.Helper()
	var runCalls, initCalls atomic.Int32
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	process, lifecycle := newBatchColdStartApplicationProcess(t, startupcli.Functions{
		InitializeSystemFunc: func(ctx context.Context, _ string) error {
			initCalls.Add(1)
			entered <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		RunFunc: batchColdStartProcessRunFunctions(&runCalls).RunFunc,
	})
	workingDirectory, home := t.TempDir(), t.TempDir()
	var firstOutput, secondOutput bytes.Buffer
	results := make(chan error, 2)
	go func() {
		results <- process.Execute(batchColdStartProcessInput(home, workingDirectory, &firstOutput, true))
	}()
	go func() {
		results <- process.Execute(batchColdStartProcessInput(home, workingDirectory, &secondOutput, true))
	}()
	awaitBatchColdStartProcessEntries(t, entered, 2)
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent demanded Execute() error = %v", err)
		}
	}
	if initCalls.Load() != 2 || runCalls.Load() != 2 {
		t.Fatalf("concurrent demanded init/run calls = %d/%d, want 2/2", initCalls.Load(), runCalls.Load())
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("close concurrent process: %v", err)
	}
	if lifecycle.closeCalls.Load() != 1 {
		t.Fatalf("concurrent process close calls = %d, want 1", lifecycle.closeCalls.Load())
	}
}

func exerciseBatchColdStartProcessCancellation(t *testing.T) {
	t.Helper()
	var runCalls atomic.Int32
	entered := make(chan struct{})
	process, lifecycle := newBatchColdStartApplicationProcess(t, startupcli.Functions{
		InitializeSystemFunc: func(ctx context.Context, _ string) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		},
		RunFunc: batchColdStartProcessRunFunctions(&runCalls).RunFunc,
	})
	home, workingDirectory := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- process.Execute(initializerapplication.Input{
			Args:    []string{"you", "run", "--work", "one-work.json", "--with-mock-workers=accept.json", "--with-server", "--no-record"},
			Env:     []string{"HOME=" + home, "USERPROFILE=" + home},
			Context: ctx, WorkingDirectory: workingDirectory,
		})
	}()
	awaitBatchColdStartProcessEntries(t, entered, 1)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled demanded Execute() error = %v, want context.Canceled", err)
	}
	if runCalls.Load() != 0 {
		t.Fatalf("canceled demanded run calls = %d, want 0", runCalls.Load())
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("close canceled process: %v", err)
	}
	if lifecycle.closeCalls.Load() != 1 {
		t.Fatalf("canceled process close calls = %d, want 1", lifecycle.closeCalls.Load())
	}
}

func exerciseBatchColdStartProcessRecovery(t *testing.T) {
	t.Helper()
	wantErr := errors.New("controlled first process activation failure")
	var initCalls, runCalls atomic.Int32
	process, lifecycle := newBatchColdStartApplicationProcess(t, startupcli.Functions{
		InitializeSystemFunc: func(context.Context, string) error {
			if initCalls.Add(1) == 1 {
				return wantErr
			}
			return nil
		},
		RunFunc: batchColdStartProcessRunFunctions(&runCalls).RunFunc,
	})
	workingDirectory, home := t.TempDir(), t.TempDir()
	var firstOutput, secondOutput bytes.Buffer
	if err := process.Execute(batchColdStartProcessInput(home, workingDirectory, &firstOutput, true)); !errors.Is(err, wantErr) {
		t.Fatalf("first recovery Execute() error = %v, want %v", err, wantErr)
	}
	if firstOutput.Len() != 0 {
		t.Fatalf("failed recovery output = %q, want empty", firstOutput.String())
	}
	if err := process.Execute(batchColdStartProcessInput(home, workingDirectory, &secondOutput, true)); err != nil {
		t.Fatalf("fresh recovery Execute() error = %v", err)
	}
	if initCalls.Load() != 2 || runCalls.Load() != 1 || secondOutput.String() != "processed:one-work.json\n" {
		t.Fatalf("recovery init/run/output = %d/%d/%q, want 2/1/result", initCalls.Load(), runCalls.Load(), secondOutput.String())
	}
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("close recovery process: %v", err)
	}
	if lifecycle.closeCalls.Load() != 1 {
		t.Fatalf("recovery process close calls = %d, want 1", lifecycle.closeCalls.Load())
	}
}

func awaitBatchColdStartProcessEntries(t *testing.T, entries <-chan struct{}, want int) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for range want {
		select {
		case <-entries:
		case <-timer.C:
			t.Fatalf("timed out waiting for %d process initialization entries", want)
		}
	}
}

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	modelcontract "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/factoryload"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
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
var listSessionDispatches = sessioncli.NewDispatches(rootTestHTTPProtocol())
var createSession = sessioncli.NewCreate(rootTestHTTPProtocol())
var deleteSession = sessioncli.NewDelete(rootTestHTTPProtocol())
var rootTestSessionsCLI = func() sessioncli.Service {
	return sessioncli.Bind(sessioncli.Operations{
		List: listSessions, Show: showSession, Pause: pauseSession, Resume: resumeSession,
		ListDispatches: listSessionDispatches, Create: createSession, Delete: deleteSession,
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
func (rootModelInvocationOperation) InvokeModel(context.Context, factorysessions.InvocationTarget, string, modelcontract.Request) (modelcontract.Result, error) {
	return modelcontract.Result{}, fmt.Errorf("model invocation test operation is not configured")
}

var rootModelsCLI = modelscli.New(rootTestHTTPProtocol(), rootModelInvocationOperation{})

func ShowSessionAccessor() func(sessioncli.ShowConfig) error      { return showSession }
func SetShowSessionAccessor(fn func(sessioncli.ShowConfig) error) { showSession = fn }

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

// backendsizecheck:ignore-file consolidated root run and operator-default CLI tests remain together until dedicated CLI test seams split.
// pkgmaintcheck:ignore-file-lines consolidated root run and operator-default CLI tests remain together until dedicated CLI test seams split.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	modelcontract "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
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

// Legacy mutable delegates remain test-only while older command tests migrate
// to CommandFactory. Production command construction has no mutable
// package-level service bindings.
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

var listFactories = factorycli.NewList(rootNamedFactoryCatalogFake{})
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
var listModels = rootModelsCLI.List
var inspectModel = rootModelsCLI.Inspect
var invokeModel = rootModelsCLI.Invoke
var pullModel = rootModelsCLI.Pull

type legacyModelsCLIService struct{}

func (legacyModelsCLIService) List(cfg modelscli.ListConfig) error       { return listModels(cfg) }
func (legacyModelsCLIService) Inspect(cfg modelscli.InspectConfig) error { return inspectModel(cfg) }
func (legacyModelsCLIService) Invoke(cfg modelscli.InvokeConfig) error   { return invokeModel(cfg) }
func (legacyModelsCLIService) Pull(cfg modelscli.PullConfig) error       { return pullModel(cfg) }

func ShowSessionAccessor() func(sessioncli.ShowConfig) error         { return showSession }
func SetShowSessionAccessor(fn func(sessioncli.ShowConfig) error)    { showSession = fn }
func ListModelsAccessor() func(modelscli.ListConfig) error           { return listModels }
func SetListModelsAccessor(fn func(modelscli.ListConfig) error)      { listModels = fn }
func InspectModelAccessor() func(modelscli.InspectConfig) error      { return inspectModel }
func SetInspectModelAccessor(fn func(modelscli.InspectConfig) error) { inspectModel = fn }
func InvokeModelAccessor() func(modelscli.InvokeConfig) error        { return invokeModel }
func SetInvokeModelAccessor(fn func(modelscli.InvokeConfig) error)   { invokeModel = fn }
func PullModelAccessor() func(modelscli.PullConfig) error            { return pullModel }
func SetPullModelAccessor(fn func(modelscli.PullConfig) error)       { pullModel = fn }

func newLegacyTestRootCommand() *cobra.Command {
	return newLegacyTestRootCommandWithCatalog(rootNamedFactoryCatalogFake{})
}

func withTestInjectedPlatformRoles(factory CommandFactory) CommandFactory {
	if factory.ModelsCLI == nil {
		factory.ModelsCLI = legacyModelsCLIService{}
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
	}
	factory.runDirectoryCreator = testRunDirectoryCreator{}
	factory.browserOpener = func(context.Context, string) error { return nil }
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

func TestResolveRunNamedFactorySelectionForwardsFailureToInjectedCandidatePaths(t *testing.T) {
	t.Parallel()

	const (
		homeDir     = "customer-home"
		workingDir  = "customer-repo"
		projectRoot = "detached-project-root"
		globalRoot  = "detached-global-root"
		name        = "@you/goal"
		projectPath = "detached-project-candidate"
		globalPath  = "detached-global-candidate"
	)
	blocking := interfaces.NewBlockingFactoryLoadError(interfaces.ValidationResult{
		Targets: []interfaces.ValidationTarget{{Code: "RULE", Message: "broken"}},
	})
	catalog := rootNamedFactoryCatalogFake{resolve: func(gotProject, gotGlobal, gotName string) (*interfaces.NamedFactoryResolution, error) {
		if gotProject != projectRoot || gotGlobal != globalRoot || gotName != name {
			t.Fatalf("catalog request = (%q, %q, %q), want (%q, %q, %q)", gotProject, gotGlobal, gotName, projectRoot, globalRoot, name)
		}
		return nil, blocking
	}}
	candidateCalls := 0
	resolveCandidates := interfaces.NamedFactoryCandidatePathsResolver(func(gotProject, gotGlobal, gotName string) (interfaces.NamedFactoryCandidatePaths, error) {
		candidateCalls++
		if gotProject != projectRoot || gotGlobal != globalRoot || gotName != name {
			t.Fatalf("candidate request = (%q, %q, %q), want (%q, %q, %q)", gotProject, gotGlobal, gotName, projectRoot, globalRoot, name)
		}
		return interfaces.NamedFactoryCandidatePaths{Project: projectPath, Global: globalPath}, nil
	})
	ctx := startupcli.WithWorkingDirectory(context.Background(), workingDir)
	cfg := &runcli.RunConfig{NamedFactoryName: name}

	err := resolveRunNamedFactorySelection(
		ctx,
		cfg,
		homeDir,
		catalog,
		func(gotHome, gotWorking string) (interfaces.NamedFactoryRoots, error) {
			if gotHome != homeDir || gotWorking != workingDir {
				t.Fatalf("root request = (%q, %q), want (%q, %q)", gotHome, gotWorking, homeDir, workingDir)
			}
			return interfaces.NamedFactoryRoots{Project: projectRoot, Global: globalRoot}, nil
		},
		resolveCandidates,
	)
	operatorErr, ok := factoryload.AsOperatorError(err)
	if !ok {
		t.Fatalf("selection error = %T %v, want OperatorError", err, err)
	}
	if operatorErr.FactoryPath != projectPath {
		t.Fatalf("operator FactoryPath = %q, want detached project candidate %q", operatorErr.FactoryPath, projectPath)
	}
	if candidateCalls != 1 {
		t.Fatalf("candidate resolver calls = %d, want 1", candidateCalls)
	}
}

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
		ListSessions: listSessions, ShowSession: showSession,
		PauseSession: pauseSession, ResumeSession: resumeSession,
		ListSessionDispatches: listSessionDispatches,
		CreateSession:         createSession, DeleteSession: deleteSession,
		ModelsCLI:            legacyModelsCLIService{},
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

func TestNewRootCommandFromSubcommandsAttachesInjectedCommands(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	docs := &cobra.Command{Use: "docs"}
	run := &cobra.Command{Use: "run"}

	got := NewRootCommandFromSubcommands(root, RootSubcommands{
		Commands: []*cobra.Command{docs, run},
	})

	if got != root {
		t.Fatal("root constructor replaced the injected root command")
	}
	if found, _, err := got.Find([]string{"docs"}); err != nil || found != docs {
		t.Fatalf("injected docs command = (%v, %v), want supplied command", found, err)
	}
	if found, _, err := got.Find([]string{"run"}); err != nil || found != run {
		t.Fatalf("injected run command = (%v, %v), want supplied command", found, err)
	}
}

func TestMain(m *testing.M) {
	// Cobra's Windows mousetrap check enumerates processes on every Execute.
	// These tests invoke commands in-process and never exercise Explorer launch
	// behavior, so avoid paying that external-system cost for each command tree.
	cobra.MousetrapHelpText = ""

	homeDir, err := os.MkdirTemp("", "you-cli-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create cli test home: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = os.RemoveAll(homeDir)
	}()

	os.Setenv("HOME", homeDir)
	os.Setenv("USERPROFILE", homeDir)
	os.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	os.Setenv("HOMEPATH", string(os.PathSeparator))

	os.Exit(m.Run())
}

func TestProductionRunSubmitFamilyCutoverEnabled(t *testing.T) {
	root := (CommandFactory{ModelsCLI: legacyModelsCLIService{}}).NewCommand(nil, nil, nil)
	for _, path := range [][]string{{"run"}, {"submit"}, {"submit", "batch"}} {
		cmd, remaining, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if len(remaining) != 0 {
			t.Fatalf("Find(%v) remaining = %v, want none", path, remaining)
		}
		if cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("Find(%v) lifecycle = (%t, %t), want retained PreRunE and RunE", path, cmd.PreRunE != nil, cmd.RunE != nil)
		}
	}

	assertDirectCommandCount(t, root, "run", 1)
	assertDirectCommandCount(t, root, "submit", 1)
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}
	assertDirectCommandCount(t, submitCmd, "batch", 1)
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

func TestRunCommand_VerboseFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	vFlag := runCmd.Flag("verbose")
	if vFlag == nil {
		t.Fatal("expected --verbose flag on run command")
	}
	if vFlag.DefValue != "false" {
		t.Errorf("default verbose = %q, want %q", vFlag.DefValue, "false")
	}
	if vFlag.Shorthand != "v" {
		t.Errorf("verbose shorthand = %q, want %q", vFlag.Shorthand, "v")
	}
}

func TestRootCommand_SharedDiagnosticsFlagsAvailableOnCoveredCommands(t *testing.T) {
	root := newLegacyTestRootCommand()
	commands := [][]string{
		{},
		{"run"},
		{"submit"},
		{"work", "list"},
		{"factory", "query"},
		{"factory", "list"},
		{"factory", "create"},
		{"factory", "replace-current"},
		{"factory", "update", "staging"},
		{"factory", "delete", "staging"},
		{"models", "list"},
		{"models", "inspect"},
		{"models", "invoke"},
		{"models", "pull"},
		{"factory", "config", "flatten"},
		{"factory", "config", "expand"},
		{"factory", "config", "validate"},
		{"init"},
		{"docs", "config"},
	}

	for _, path := range commands {
		cmd := root
		if len(path) > 0 {
			found, _, err := root.Find(path)
			if err != nil {
				t.Fatalf("find %v: %v", path, err)
			}
			cmd = found
		}
		for name, shorthand := range map[string]string{"verbose": "v", "debug": "d"} {
			flag := cmd.Flag(name)
			if flag == nil {
				t.Fatalf("%v missing shared --%s flag", path, name)
			}
			if flag.DefValue != "false" {
				t.Fatalf("%v --%s default = %q, want false", path, name, flag.DefValue)
			}
			if flag.Shorthand != shorthand {
				t.Fatalf("%v --%s shorthand = %q, want %q", path, name, flag.Shorthand, shorthand)
			}
		}
	}
}

func TestRunCommand_RecordFlagsDocumentDefaultRecordingBehavior(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	recordFlag := runCmd.Flags().Lookup("record")
	if recordFlag == nil {
		t.Fatal("expected --record flag on run command")
	}
	if !strings.Contains(recordFlag.Usage, "default live runs record automatically unless --no-record is used") {
		t.Fatalf("--record usage = %q, want default-recording guidance", recordFlag.Usage)
	}
	if !strings.Contains(recordFlag.Usage, "replay artifacts are sensitive") {
		t.Fatalf("--record usage = %q, want sensitivity guidance", recordFlag.Usage)
	}

	noRecordFlag := runCmd.Flags().Lookup("no-record")
	if noRecordFlag == nil {
		t.Fatal("expected --no-record flag on run command")
	}
	if noRecordFlag.DefValue != "false" {
		t.Fatalf("--no-record default = %q, want false", noRecordFlag.DefValue)
	}
	if !strings.Contains(noRecordFlag.Usage, "disable the default replay artifact for this invocation") {
		t.Fatalf("--no-record usage = %q", noRecordFlag.Usage)
	}
	if !strings.Contains(runCmd.Long, "Normal live runs record by default unless you pass --no-record.") {
		t.Fatal("expected run command long help text to document default recording")
	}
	if !strings.Contains(runCmd.Long, "Replay artifacts are sensitive and can contain prompts, payloads, stdout, stderr, and diagnostic metadata.") {
		t.Fatal("expected run command long help text to document replay artifact sensitivity")
	}

	replayFlag := runCmd.Flags().Lookup("replay")
	if replayFlag == nil {
		t.Fatal("expected --replay flag on run command")
	}
	if !strings.Contains(replayFlag.Usage, "existing sensitive replay artifact") {
		t.Fatalf("--replay usage = %q, want sensitivity guidance", replayFlag.Usage)
	}
}

func TestRunCommand_FactoryPromptRejectsEmptyStdinWithStableCode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedInvocationInput(
		work.PreparedInvocationInput{},
		&work.InputError{
			Code:    work.InputErrorCodeEmpty,
			Message: "invocation stdin input is empty",
			Source:  work.InputSourceStdinText,
		},
	))
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected empty stdin rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty stdin code", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for empty factory stdin")
	}
}

func assertRunStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()
	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Factory:",
		"Recording saved",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout = %q, want no %q chatter", stdout, forbidden)
		}
	}
}

func TestRunCommand_FactoryPromptRejectsAmbiguousPositionalAndStdin(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedInvocationInput(
		work.PreparedInvocationInput{},
		&work.InputError{
			Code:               work.InputErrorCodeSourceConflict,
			Message:            "invocation input sources conflict: positional_text, stdin_text",
			ConflictingSources: []work.InputSourceLabel{work.InputSourcePositionalText, work.InputSourceStdinText},
		},
	))
	root.SetIn(strings.NewReader("Fix from stdin\n"))
	root.SetOut(io.Discard)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--quiet", "Fix from args", "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ambiguous positional and stdin prompt rejection")
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
	if runCalled {
		t.Fatal("run should not start for ambiguous factory prompt input")
	}
}

func TestRunCommand_FactoryPromptRejectsWorkFlagConflict(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(
		work.InputSourcePositionalText,
		"Fix the lint issues",
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--work", "work.json", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between positional prompt and --work")
	}
	if !strings.Contains(err.Error(), "cannot be used with --work") {
		t.Fatalf("error = %q, want --work conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when prompt conflicts with --work")
	}
}

func TestRunCommand_PositionalPromptRequiresFactoryFlag(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(
		work.InputSourcePositionalText,
		"Fix the lint issues",
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--dir", "factory", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected positional prompt without --factory to fail")
	}
	if !strings.Contains(err.Error(), "require --factory") {
		t.Fatalf("error = %q, want --factory requirement", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for positional prompt without --factory")
	}
}

func TestRunCommand_CleanInvocationFailureWritesSingleErrorResponseToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeFailed,
			Message: "clean invocation failed: mock worker rejected",
		}
	}

	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var payload struct {
		Code    string `json:"code"`
		Family  string `json:"family"`
		Message string `json:"message"`
	}
	if decodeErr := json.Unmarshal(stderr.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", decodeErr, stderr.String())
	}
	if payload.Code != runcli.InvocationErrorCodeFailed || payload.Family != "INTERNAL_SERVER_ERROR" ||
		payload.Message != "clean invocation failed: mock worker rejected" {
		t.Fatalf("ErrorResponse = %#v", payload)
	}
}

func TestRunCommand_CleanInvocationJSONFailureWritesSingleErrorObjectToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeTimeout,
			Message: "clean invocation timed out",
		}
	}

	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation timeout")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var payload map[string]string
	if decodeErr := json.Unmarshal(stderr.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stderr is not one JSON object: %v\n%s", decodeErr, stderr.String())
	}
	if payload["code"] != runcli.InvocationErrorCodeTimeout {
		t.Fatalf("code = %q, want %q", payload["code"], runcli.InvocationErrorCodeTimeout)
	}
	if payload["message"] != "clean invocation timed out" {
		t.Fatalf("message = %q", payload["message"])
	}
	if payload["family"] != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("family = %q", payload["family"])
	}
}

func TestRootCommand_NoArgsPrintsHelpWithoutRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalls := 0
	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		runCalls++
		return nil
	}

	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}
	if runCalls != 0 {
		t.Fatalf("run calls = %d, want 0", runCalls)
	}
	if output := out.String(); !strings.Contains(output, "Available Commands:") ||
		!strings.Contains(output, "Run and manage CPN-based workflow factories") {
		t.Fatalf("root no-argument output is not concise discovery help:\n%s", output)
	}
}

func TestRootCommand_NoArgsDoesNotChangeExplicitRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var captured []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		captured = append(captured, cfg)
		mode := "batch"
		if cfg.Continuously {
			mode = "continuous"
		}
		if cfg.StartupOutput != nil {
			fmt.Fprintf(
				cfg.StartupOutput,
				"service startup reached: mode=%s bootstrap=%t open-dashboard=%t\n",
				mode,
				cfg.Bootstrap,
				cfg.OpenDashboard,
			)
		}
		return nil
	}

	var rootOut bytes.Buffer
	rootDefault := newLegacyTestRootCommand()
	rootDefault.SetOut(&rootOut)
	rootDefault.SetErr(io.Discard)
	rootDefault.SetArgs([]string{})
	if err := rootDefault.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}

	var explicitOut bytes.Buffer
	explicitRun := newLegacyTestRootCommand()
	explicitRun.SetOut(&explicitOut)
	explicitRun.SetErr(io.Discard)
	explicitRun.SetArgs([]string{"run"})
	if err := explicitRun.Execute(); err != nil {
		t.Fatalf("execute explicit run: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("captured run configs = %d, want 1", len(captured))
	}

	explicit := captured[0]
	if explicit.Continuously || explicit.Bootstrap || explicit.OpenDashboard {
		t.Fatalf("explicit run should not inherit OOTB-only defaults: %#v", explicit)
	}
	if got := rootOut.String(); !strings.Contains(got, "Available Commands:") {
		t.Fatalf("no-args observable output = %q, want discovery help", got)
	}
	if got := explicitOut.String(); !strings.Contains(got, "service startup reached: mode=batch bootstrap=false open-dashboard=false") {
		t.Fatalf("explicit run observable startup output = %q, want explicit service startup", got)
	}
}

func TestRunCommand_DebugFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	dFlag := runCmd.Flag("debug")
	if dFlag == nil {
		t.Fatal("expected --debug flag on run command")
	}
	if dFlag.DefValue != "false" {
		t.Errorf("default debug = %q, want %q", dFlag.DefValue, "false")
	}
	if dFlag.Shorthand != "d" {
		t.Errorf("debug shorthand = %q, want %q", dFlag.Shorthand, "d")
	}
}

func TestRunCommand_DebugImpliesVerboseRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--debug"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --debug: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --debug to imply verbose run behavior")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set debug-capable logger")
	}
}

func TestWorkListCommand_SharedDiagnosticsFlagsMapToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(&stderr)
	root.SetArgs([]string{"work", "list", "--debug"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list --debug: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --debug to imply verbose command diagnostics")
	}
	if !got.Debug {
		t.Fatal("expected debug config")
	}
	if got.Diagnostics != &stderr {
		t.Fatalf("diagnostics writer = %#v, want configured stderr writer", got.Diagnostics)
	}
	if got.Output == nil {
		t.Fatal("expected stdout writer")
	}
}

func TestFactoryQueryCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	queryFactory = func(cfg factorycli.QueryConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: factory query"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, `{"name":"default"}`)
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "factory", "query", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --json --verbose: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if payload["name"] != "default" {
		t.Fatalf("stdout JSON = %#v, want default factory name", payload)
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: factory query") {
		t.Fatalf("stderr = %q, want diagnostics", got)
	}
}

func TestRunCommand_ContinuouslyFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("continuously")
	if flag == nil {
		t.Fatal("expected --continuously flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default continuously = %q, want %q", flag.DefValue, "false")
	}
	if flag.Usage != "keep the factory alive while idle until cancelled" {
		t.Errorf("continuously usage = %q", flag.Usage)
	}
	if runCmd.Long == "" {
		t.Fatal("expected run command long help text")
	}
	if !strings.Contains(runCmd.Long, "you run --work ./docs/examples/startup-work.json") {
		t.Fatal("expected run command long help text to provide explicit default Work")
	}
	if !strings.Contains(runCmd.Long, "factory/inputs/task/default") {
		t.Fatal("expected run command long help text to mention default task input path")
	}
	if !strings.Contains(runCmd.Example, "run --work ./docs/examples/startup-work.json") {
		t.Fatal("expected run command examples to provide explicit default Work")
	}
}

func TestRunCommand_RecordAndReplayFlags(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for _, name := range []string{"record", "replay"} {
		flag := runCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("expected --%s flag on run command", name)
		}
		if flag.DefValue != "" {
			t.Errorf("--%s default = %q, want empty", name, flag.DefValue)
		}
	}
}

func TestRunCommand_WithMockWorkersFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("with-mock-workers")
	if flag == nil {
		t.Fatal("expected --with-mock-workers flag on run command")
	}
	if flag.DefValue != "" {
		t.Errorf("default with-mock-workers = %q, want empty", flag.DefValue)
	}
	if flag.NoOptDefVal == "" {
		t.Error("with-mock-workers should define an internal optional-value default")
	}
	if !strings.Contains(flag.Usage, "optional mock-workers JSON config path") {
		t.Errorf("with-mock-workers usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--with-mock-workers") {
		t.Fatal("expected run command long help text to mention --with-mock-workers")
	}
}

func TestRunCommand_SkipPermissionsFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("skip-permissions")
	if flag == nil {
		t.Fatal("expected --skip-permissions flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default skip-permissions = %q, want false", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "invocation-only unsafe permission bypass") {
		t.Errorf("skip-permissions usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--skip-permissions") {
		t.Fatal("expected run command long help text to mention --skip-permissions")
	}
}

func TestRunCommand_SkipPermissionsFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--skip-permissions"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --skip-permissions: %v", err)
	}
	if got.InvocationSkipPermissionsOverride == nil {
		t.Fatal("expected --skip-permissions to set invocation override")
	}
	if !*got.InvocationSkipPermissionsOverride {
		t.Fatal("expected invocation skip-permissions override to be true")
	}
}

func TestRunCommand_WithoutSkipPermissionsLeavesInvocationOverrideUnset(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if got.InvocationSkipPermissionsOverride != nil {
		t.Fatalf("invocation skip-permissions override = %#v, want nil when flag omitted", got.InvocationSkipPermissionsOverride)
	}
}

func TestRunCommand_WorkflowFlagRejected(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--workflow", "workflow-1"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected run-level --workflow to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown flag: --workflow") {
		t.Fatalf("error = %q, want unknown flag --workflow", err.Error())
	}
	if runCalled {
		t.Fatal("run command should not execute when --workflow is unsupported")
	}
}

func TestRunCommand_RetiredMockExecutionAliasRejected(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	retiredFlag := "--" + strings.Join([]string{"dry", "run"}, "-")
	root.SetArgs([]string{"run", retiredFlag})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected retired mock-execution alias to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown flag: "+retiredFlag) {
		t.Fatalf("error = %q, want unknown retired flag", err.Error())
	}
	if runCalled {
		t.Fatal("run command should not execute when retired mock-execution alias is unsupported")
	}
}

func TestRunCommand_QuietFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("quiet")
	if flag == nil {
		t.Fatal("expected --quiet flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default quiet = %q, want %q", flag.DefValue, "false")
	}
	if flag.Usage != "suppress dashboard output for quiet or CI-oriented runs" {
		t.Errorf("quiet usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--quiet") {
		t.Fatal("expected run command long help text to mention --quiet")
	}
}

func TestWorkListCommand_StateFilterFlagsMapToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json",
		"--server", "http://127.0.0.1:9090",
		"work",
		"list",
		"--state-name", "review",
		"--state-type", "PROCESSING",
		"--sort-by", "state.type",
		"--max-results", "25",
		"--next-token", "cursor-1",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list: %v", err)
	}

	if got.StateName != "review" {
		t.Fatalf("state name = %q, want review", got.StateName)
	}
	if got.StateType != "PROCESSING" {
		t.Fatalf("state type = %q, want PROCESSING", got.StateType)
	}
	if got.SortBy != "state.type" {
		t.Fatalf("sort by = %q, want state.type", got.SortBy)
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
	if got.MaxResults != 25 {
		t.Fatalf("max results = %d, want 25", got.MaxResults)
	}
	if got.NextToken != "cursor-1" {
		t.Fatalf("next token = %q, want cursor-1", got.NextToken)
	}
	if !got.JSON {
		t.Fatal("expected json output flag")
	}
	if got.Output == nil {
		t.Fatal("expected output writer")
	}
}

func TestWorkListCommand_DefaultServerMapsToSharedLocalURI(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list: %v", err)
	}

	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
}

func TestRunCommand_RuntimeLogFlags(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	defaults := logging.DefaultRuntimeLogConfig()
	tests := []struct {
		name    string
		def     string
		usageIn string
	}{
		{name: "runtime-log-dir", def: "", usageIn: "root directory for structured runtime log files grouped by UTC start date"},
		{name: "runtime-log-max-size-mb", def: "100", usageIn: "rotate each runtime log file"},
		{name: "runtime-log-max-backups", def: "20", usageIn: "maximum rotated runtime log files"},
		{name: "runtime-log-max-age-days", def: "30", usageIn: "maximum days to retain rotated runtime log files"},
		{name: "runtime-log-compress", def: "false", usageIn: "compress rotated runtime log files"},
	}
	tests[1].def = strconv.Itoa(defaults.MaxSize)
	tests[2].def = strconv.Itoa(defaults.MaxBackups)
	tests[3].def = strconv.Itoa(defaults.MaxAge)

	for _, tc := range tests {
		flag := runCmd.Flags().Lookup(tc.name)
		if flag == nil {
			t.Fatalf("expected --%s flag on run command", tc.name)
		}
		if flag.DefValue != tc.def {
			t.Fatalf("--%s default = %q, want %q", tc.name, flag.DefValue, tc.def)
		}
		if !strings.Contains(flag.Usage, tc.usageIn) {
			t.Fatalf("--%s usage = %q, want to contain %q", tc.name, flag.Usage, tc.usageIn)
		}
	}
	if got := runCmd.Flags().Lookup("runtime-log-dir").Usage; !strings.Contains(got, "~/.you-agent-factory/logs") {
		t.Fatalf("--runtime-log-dir usage = %q, want canonical default log path", got)
	}
	if !strings.Contains(runCmd.Long, "Runtime logs are structured JSON rolling files grouped by UTC start date under the selected log root") {
		t.Fatal("expected run command long help text to document UTC-grouped runtime log behavior")
	}
	if !strings.Contains(runCmd.Long, "stdout/stderr only on command failures") {
		t.Fatal("expected run command long help text to document command output policy")
	}
}

func TestRunCommand_QuietFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--quiet",
		"--no-record",
		"--dir", "custom-factory",
		"--work", "work.json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --quiet: %v", err)
	}

	if !got.SuppressDashboardRendering {
		t.Fatal("expected --quiet to suppress dashboard rendering")
	}
	if got.Dir != "custom-factory" {
		t.Errorf("dir = %q, want %q", got.Dir, "custom-factory")
	}
	if got.Workflow != "" {
		t.Errorf("workflow = %q, want empty (run-level --workflow removed)", got.Workflow)
	}
	if got.WorkFile != "work.json" {
		t.Errorf("work file = %q, want %q", got.WorkFile, "work.json")
	}
	if !got.DisableDefaultRecording {
		t.Fatal("expected --no-record to disable default recording")
	}
	if got.RecordPath != "" {
		t.Errorf("record path = %q, want empty", got.RecordPath)
	}
	if got.Port != 7437 {
		t.Errorf("port = %d, want %d", got.Port, 7437)
	}
	if !got.AutoPort {
		t.Fatal("expected default --server to enable automatic port resolution")
	}
	if got.BindHost != "localhost" {
		t.Fatalf("bind host = %q, want localhost", got.BindHost)
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestRunCommand_RecordAndNoRecordFlagsCanBePassedTogetherForDeterministicValidation(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--record", "record.replay.json",
		"--no-record",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with --record and --no-record: %v", err)
	}
	if got.RecordPath != "record.replay.json" {
		t.Fatalf("record path = %q, want record.replay.json", got.RecordPath)
	}
	if !got.DisableDefaultRecording {
		t.Fatal("expected --no-record to map into RunConfig for downstream validation")
	}
}

func TestRunCommand_RuntimeLogFlagsMapToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--runtime-log-dir", "logs/runtime",
		"--runtime-log-max-size-mb", "11",
		"--runtime-log-max-backups", "12",
		"--runtime-log-max-age-days", "13",
		"--runtime-log-compress",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with runtime log flags: %v", err)
	}

	if got.RuntimeLogDir != "logs/runtime" {
		t.Fatalf("runtime log dir = %q, want unchanged root logs/runtime", got.RuntimeLogDir)
	}
	want := logging.RuntimeLogConfig{MaxSize: 11, MaxBackups: 12, MaxAge: 13, Compress: true}
	if got.RuntimeLogConfig != want {
		t.Fatalf("runtime log config = %#v, want %#v", got.RuntimeLogConfig, want)
	}
}

func TestRunCommand_OutputResponseStreamFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newTransportNamedFactoryRoot(t, packagedGoalFactoryName)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "@you/goal",
		"--output", "response-stream",
		"ship the goal",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --output response-stream: %v", err)
	}
	if got.InvocationOutputMode != runcli.InvocationOutputResponseStream {
		t.Fatalf("InvocationOutputMode = %q, want %q", got.InvocationOutputMode, runcli.InvocationOutputResponseStream)
	}
}

func TestRunCommand_WithMockWorkersFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--with-mock-workers", "mock-workers.json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --with-mock-workers: %v", err)
	}

	if !got.MockWorkersEnabled {
		t.Fatal("expected --with-mock-workers to enable mock workers")
	}
	if got.MockWorkersConfigPath != "mock-workers.json" {
		t.Fatalf("mock workers config path = %q, want %q", got.MockWorkersConfigPath, "mock-workers.json")
	}
}

func TestRunCommand_WithMockWorkersFlagWithoutPathMapsToDefaultConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--with-mock-workers"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --with-mock-workers without path: %v", err)
	}

	if !got.MockWorkersEnabled {
		t.Fatal("expected --with-mock-workers to enable mock workers")
	}
	if got.MockWorkersConfigPath != "" {
		t.Fatalf("mock workers config path = %q, want empty default path", got.MockWorkersConfigPath)
	}
}

func TestRunCommand_VerboseFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --verbose to enable service verbose logging")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestRunCommand_VerboseDiagnosticsUseStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose run config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		_, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: run startup")
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no diagnostic output", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: run startup") {
		t.Fatalf("stderr = %q, want run diagnostics", got)
	}
}

func TestRunCommand_NamedFactoryResolutionMetadataFlowsIntoRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	projectFactoryDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(projectFactoryDir, interfaces.FactoryConfigFile),
		portableFactoryPayloadWithDefaultHandling(),
		0o600,
	); err != nil {
		t.Fatalf("write detached Factory fixture: %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithCatalog(rootNamedFactoryCatalogFake{
		resolve: func(projectRoot, globalRoot, name string) (*interfaces.NamedFactoryResolution, error) {
			return &interfaces.NamedFactoryResolution{
				Name:               name,
				FactoryDir:         projectFactoryDir,
				Source:             interfaces.NamedFactoryResolutionSourceProjectLocal,
				ProjectRoot:        projectRoot,
				GlobalRoot:         globalRoot,
				PrecedenceDecision: interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal,
			}, nil
		},
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named alpha: %v", err)
	}
	if got.NamedFactoryResolution == nil {
		t.Fatal("expected named-factory resolution metadata")
	}
	if got.Dir != got.NamedFactoryResolution.FactoryDir {
		t.Fatalf("run dir = %q, want resolved named-factory dir %q", got.Dir, got.NamedFactoryResolution.FactoryDir)
	}
	if got.NamedFactoryResolution.Source != interfaces.NamedFactoryResolutionSourceProjectLocal {
		t.Fatalf("resolution source = %q, want %q", got.NamedFactoryResolution.Source, interfaces.NamedFactoryResolutionSourceProjectLocal)
	}
	if got.NamedFactoryResolution.PrecedenceDecision != interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("resolution precedence = %q, want %q", got.NamedFactoryResolution.PrecedenceDecision, interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal)
	}
}

func TestRunCommand_UnknownRunnerFlagReturnsCobraError(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--runner", "codex"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unknown --runner flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "--runner") {
		t.Fatalf("error = %q, want unknown --runner flag error", err.Error())
	}
}

func TestRootAndRunHelp_ShowDefaultWorkerModelFlagsAndHideRunner(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for name, cmd := range map[string]*cobra.Command{"root": root, "run": runCmd} {
		if strings.Contains(cmd.Long, "--runner") {
			t.Fatalf("%s long help still mentions --runner:\n%s", name, cmd.Long)
		}
		if cmd.Root().PersistentFlags().Lookup("default-worker-model-provider") == nil {
			t.Fatalf("%s missing --default-worker-model-provider flag", name)
		}
		if cmd.Root().PersistentFlags().Lookup("default-worker-model") == nil {
			t.Fatalf("%s missing --default-worker-model flag", name)
		}
	}
}

func TestRootCommand_DefaultWorkerModelProviderFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "codex"},
		operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
		},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--default-worker-model-provider", "codex", "run", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root with default provider flag: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag {
		t.Fatalf("provider source = %q, want flag", got.OperatorDefaults.WorkerModelProviderSource)
	}
}

func TestRootCommand_ExplicitEnvironmentIsIsolatedAndFlagsRetainPrecedence(t *testing.T) {
	homeDir := t.TempDir()

	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()
	var got []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = append(got, cfg)
		return nil
	}

	newCommand := func(
		environment map[string]string,
		wantEnvironment operatorconfig.Defaults,
		wantFlags operatorconfig.FlagOverrides,
		result operatorconfig.ResolvedDefaults,
	) *cobra.Command {
		factory := withTestInjectedPlatformRoles(CommandFactory{})
		factory.resolveOperatorDefaults = expectOperatorDefaultsResolution(t, wantEnvironment, wantFlags, result, nil)
		return factory.NewCommand(
			func() (string, error) { return homeDir, nil },
			func(name string) (string, bool) {
				value, ok := environment[name]
				return value, ok
			},
			startupcli.Functions{
				RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
					return runCLI(ctx, testRunConfig(selection))
				},
			},
		)
	}

	first := newCommand(
		map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: "codex"},
		operatorconfig.Defaults{WorkerModelProvider: "codex"},
		operatorconfig.FlagOverrides{WorkerModelProvider: "gemini"},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "GEMINI", WorkerModelProviderSource: operatorconfig.SourceFlag},
	)
	first.SetOut(io.Discard)
	first.SetErr(io.Discard)
	first.SetArgs([]string{"run", "--default-worker-model-provider", "gemini", "--no-record"})
	if err := first.Execute(); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := newCommand(
		map[string]string{
			operatorconfig.EnvDefaultWorkerModelProvider: "codex",
			runcli.ModelCacheDirEnvironment:              "/customer/model-cache",
		},
		operatorconfig.Defaults{WorkerModelProvider: "codex"},
		operatorconfig.FlagOverrides{},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModelProviderSource: operatorconfig.SourceEnv},
	)
	second.SetOut(io.Discard)
	second.SetErr(io.Discard)
	second.SetArgs([]string{"run"})
	if err := second.Execute(); err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	third := newCommand(
		map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: ""},
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CLAUDE", WorkerModelProviderSource: operatorconfig.SourceFile},
	)
	third.SetOut(io.Discard)
	third.SetErr(io.Discard)
	third.SetArgs([]string{"run"})
	if err := third.Execute(); err != nil {
		t.Fatalf("third Execute() error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("run calls = %d, want 3", len(got))
	}
	if got[0].OperatorDefaults.WorkerModelProvider != "GEMINI" || got[0].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag {
		t.Fatalf("first defaults = %+v, want GEMINI from flag", got[0].OperatorDefaults)
	}
	if got[1].OperatorDefaults.WorkerModelProvider != "CODEX" || got[1].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceEnv {
		t.Fatalf("second defaults = %+v, want CODEX from environment", got[1].OperatorDefaults)
	}
	if got[1].ModelCacheDir != "/customer/model-cache" {
		t.Fatalf("second model cache dir = %q, want environment value", got[1].ModelCacheDir)
	}
	if got[2].ModelCacheDir != "" {
		t.Fatalf("third model cache dir = %q, want isolated empty value", got[2].ModelCacheDir)
	}
	if got[2].OperatorDefaults.WorkerModelProvider != "CLAUDE" || got[2].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFile {
		t.Fatalf("third defaults = %+v, want CLAUDE from file", got[2].OperatorDefaults)
	}
}

func TestRootCommand_DefaultWorkerModelFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModel: "gpt-5-codex"},
		operatorconfig.ResolvedDefaults{WorkerModel: "gpt-5-codex", WorkerModelSource: operatorconfig.SourceFlag},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model", "gpt-5-codex", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with default model flag: %v", err)
	}
	if got.OperatorDefaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", got.OperatorDefaults.WorkerModel)
	}
	if got.OperatorDefaults.WorkerModelSource != operatorconfig.SourceFlag {
		t.Fatalf("model source = %q, want flag", got.OperatorDefaults.WorkerModelSource)
	}
}

func TestRootCommand_ExplicitRunHonorsDefaultWorkerModelFlags(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "codex", WorkerModel: "gpt-5-codex"},
		operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model-provider", "codex", "--default-worker-model", "gpt-5-codex"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute explicit run with default model flags: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", got.OperatorDefaults.WorkerModel)
	}
}

func TestRootCommand_DefaultProviderFlagRejectsUnresolvedSymbolicDefault(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "DEFAULT"},
		operatorconfig.ResolvedDefaults{},
		fmt.Errorf("DEFAULT requires a concrete provider"),
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT", "--no-record"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unresolved DEFAULT provider error")
	}
	if !strings.Contains(err.Error(), "DEFAULT requires a concrete provider") {
		t.Fatalf("error = %q, want unresolved DEFAULT guidance", err.Error())
	}
}

func TestRootCommand_DefaultProviderFlagResolvesSymbolicDefaultFromFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "DEFAULT"},
		operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
		},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with DEFAULT provider flag: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag {
		t.Fatalf("provider source = %q, want flag", got.OperatorDefaults.WorkerModelProviderSource)
	}
}

type transportNamedFactoryCatalog map[string]string

func (catalog transportNamedFactoryCatalog) ListNamedFactories(string) ([]interfaces.NamedFactoryListEntry, error) {
	entries := make([]interfaces.NamedFactoryListEntry, 0, len(catalog))
	for name, factoryDir := range catalog {
		entries = append(entries, interfaces.NamedFactoryListEntry{Name: name, FactoryDir: factoryDir})
	}
	return entries, nil
}

func (transportNamedFactoryCatalog) DeleteNamedFactory(string, string) error {
	return nil
}

func (catalog transportNamedFactoryCatalog) ResolveNamedFactoryAcrossRoots(
	projectRoot string,
	globalRoot string,
	name string,
) (*interfaces.NamedFactoryResolution, error) {
	factoryDir, ok := catalog[name]
	if !ok {
		return nil, fmt.Errorf("named factory %q not found", name)
	}
	return &interfaces.NamedFactoryResolution{
		Name:               name,
		FactoryDir:         factoryDir,
		Source:             interfaces.NamedFactoryResolutionSourceGlobal,
		ProjectRoot:        projectRoot,
		GlobalRoot:         globalRoot,
		PrecedenceDecision: interfaces.NamedFactoryPrecedenceDecisionNone,
	}, nil
}

func newTransportNamedFactoryRoot(t *testing.T, names ...string) *cobra.Command {
	return newTransportNamedFactoryRootWithInvocation(t, rootInvocationInputScript{}, names...)
}

func newTransportNamedFactoryRootWithInvocation(
	t *testing.T,
	prepare rootInvocationInputScript,
	names ...string,
) *cobra.Command {
	t.Helper()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	catalog := make(transportNamedFactoryCatalog, len(names))
	for _, name := range names {
		factoryDir := t.TempDir()
		payload := strings.Replace(goalFailureBaselineNamedFactoryJSON, "@you/goal", name, 1)
		if err := os.WriteFile(
			filepath.Join(factoryDir, interfaces.FactoryConfigFile),
			[]byte(payload),
			0o644,
		); err != nil {
			t.Fatalf("write named Factory fixture: %v", err)
		}
		catalog[name] = factoryDir
	}
	return newLegacyTestRootCommandWithCatalogAndInvocationInput(catalog, prepare)
}

func TestRunCommand_VerboseDiagnosticsIncludeOperatorDefaultPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var diagnostics bytes.Buffer
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		_, err := cfg.Diagnostics.Write([]byte(cfg.OperatorDefaults.DiagnosticsLine() + "\n"))
		return err
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "codex", WorkerModel: "gpt-5-codex"},
		operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(&diagnostics)
	root.SetArgs([]string{"run", "--verbose", "--default-worker-model-provider", "codex", "--default-worker-model", "gpt-5-codex", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with verbose operator defaults: %v", err)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"operatorDefaults precedence=file < env < flag",
		"provider=CODEX",
		"providerSource=flag",
		"model=gpt-5-codex",
		"modelSource=flag",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}

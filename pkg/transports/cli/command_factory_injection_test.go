package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"go.uber.org/zap"
)

type injectedModelsCLIService struct {
	list func(modelscli.ListConfig) error
}

func (service injectedModelsCLIService) List(cfg modelscli.ListConfig) error {
	if service.list == nil {
		return fmt.Errorf("models list service is required")
	}
	return service.list(cfg)
}
func (injectedModelsCLIService) Inspect(modelscli.InspectConfig) error { return nil }
func (injectedModelsCLIService) Invoke(modelscli.InvokeConfig) error   { return nil }
func (injectedModelsCLIService) Pull(modelscli.PullConfig) error       { return nil }

func TestNewCommandFactoryDoesNotInstallTransportDefaults(t *testing.T) {
	t.Parallel()

	factory := NewCommandFactory(CommandOperations{})
	if factory.SubmitWork != nil ||
		factory.ListSessions != nil ||
		factory.ModelsCLI != nil ||
		factory.FlattenFactoryConfig != nil ||
		factory.InitFactory != nil ||
		factory.QueryFactory != nil ||
		factory.ListWork != nil ||
		factory.resolveNamedFactoryRoots != nil ||
		factory.resolveNamedFactoryCandidatePaths != nil ||
		factory.resolveCurrentFactoryDir != nil ||
		factory.resolveFactoryConfigRoot != nil ||
		factory.loadFactoryConfigFile != nil ||
		factory.workRequestFileLoader != nil ||
		factory.prepareInvocationInput != nil ||
		factory.openRunSelection != nil ||
		factory.buildTerminalLogger != nil ||
		factory.batchInputFileSystem != nil ||
		factory.runDirectoryCreator != nil ||
		factory.browserOpener != nil ||
		!reflect.DeepEqual(factory.runDefaults, runcli.RunConfig{}) {
		t.Fatalf("factory = %#v, want missing operations to remain missing", factory)
	}
}

func TestNewCommandFactoryPreservesInjectedOperations(t *testing.T) {
	t.Parallel()

	calls := 0
	resolver := interfaces.CurrentFactoryDirectoryResolver(func(rootDir string) (string, error) {
		return rootDir + "/current", nil
	})
	namedRoots := NamedFactoryRootsResolver(func(homeDir, workingDir string) (interfaces.NamedFactoryRoots, error) {
		return interfaces.NamedFactoryRoots{Project: workingDir + "/factory", Global: homeDir + "/factories"}, nil
	})
	namedCandidates := interfaces.NamedFactoryCandidatePathsResolver(func(projectRoot, globalRoot, name string) (interfaces.NamedFactoryCandidatePaths, error) {
		return interfaces.NamedFactoryCandidatePaths{Project: projectRoot + "/candidate", Global: globalRoot + "/candidate"}, nil
	})
	configRootResolver := interfaces.FactoryConfigRootResolver(func(path string) (string, error) {
		return path + "/root", nil
	})
	configLoader := interfaces.FactoryConfigFileLoader(func(string) (*interfaces.FactoryConfig, error) {
		return &interfaces.FactoryConfig{Name: "injected"}, nil
	})
	workLoader := work.RequestFileLoader(func(string) (work.WorkRequest, error) {
		return work.WorkRequest{Type: work.WorkRequestTypeFactoryRequestBatch}, nil
	})
	invocationPreparation := rootInvocationInputScript{}
	openRunSelection := runcli.SelectionFactory(func(runcli.RunConfig) startupcli.RunSelection { return nil })
	loggerBuilder := terminalpolicy.LoggerBuilder(func(terminalpolicy.Mode, bool) (*zap.Logger, error) {
		return zap.NewNop(), nil
	})
	batchFiles := batchInputFileSystemFakeForFactoryTest{}
	directories := runDirectoryCreatorFakeForFactoryTest{}
	browser := func(context.Context, string) error { return nil }
	factory := NewCommandFactory(CommandOperations{
		ResolveNamedFactoryRoots:          namedRoots,
		ResolveNamedFactoryCandidatePaths: namedCandidates,
		ResolveCurrentFactoryDir:          resolver,
		ResolveFactoryConfigRoot:          configRootResolver,
		LoadFactoryConfigFile:             configLoader,
		WorkRequestFileLoader:             workLoader,
		PrepareInvocationInput:            invocationPreparation,
		OpenRunSelection:                  openRunSelection,
		BuildTerminalLogger:               loggerBuilder,
		RunDefaults:                       runcli.RunConfig{Port: 9123},
		BatchInputFileSystem:              batchFiles,
		RunDirectoryCreator:               directories,
		BrowserOpener:                     browser,
		ModelsCLI: injectedModelsCLIService{list: func(modelscli.ListConfig) error {
			calls++
			return nil
		}},
	})
	if roots, err := factory.resolveNamedFactoryRoots("home", "repo"); err != nil || roots.Project != "repo/factory" || roots.Global != "home/factories" {
		t.Fatalf("named Factory roots = %#v, %v", roots, err)
	}
	if candidates, err := factory.resolveNamedFactoryCandidatePaths("project", "global", "alpha"); err != nil || candidates.Project != "project/candidate" || candidates.Global != "global/candidate" {
		t.Fatalf("named Factory candidates = %#v, %v", candidates, err)
	}
	if factory.ModelsCLI == nil {
		t.Fatal("injected Models CLI service is missing")
	}
	if err := factory.ModelsCLI.List(modelscli.ListConfig{Context: context.Background()}); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	resolved, err := factory.resolveCurrentFactoryDir("factory")
	if err != nil || resolved != "factory/current" {
		t.Fatalf("current Factory resolution = %q, %v", resolved, err)
	}
	if logger, err := factory.buildTerminalLogger(terminalpolicy.ModeQuiet, false); err != nil || logger == nil {
		t.Fatalf("terminal logger = %#v, %v", logger, err)
	}
	if resolved, err := factory.resolveFactoryConfigRoot("factory.json"); err != nil || resolved != "factory.json/root" {
		t.Fatalf("config root = %q, %v", resolved, err)
	}
	if loaded, err := factory.loadFactoryConfigFile("factory.json"); err != nil || loaded.Name != "injected" {
		t.Fatalf("config = %#v, %v", loaded, err)
	}
	if loaded, err := factory.workRequestFileLoader("work.json"); err != nil || loaded.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request = %#v, %v", loaded, err)
	}
	if factory.prepareInvocationInput == nil {
		t.Fatal("injected Work invocation-input preparation is missing")
	}
	if factory.openRunSelection == nil {
		t.Fatal("injected run selection operation is missing")
	}
	if factory.runDefaults.Port != 9123 {
		t.Fatalf("run defaults port = %d, want 9123", factory.runDefaults.Port)
	}
	if factory.batchInputFileSystem == nil {
		t.Fatal("injected batch input file system is missing")
	}
	if factory.runDirectoryCreator == nil {
		t.Fatal("injected run directory creator is missing")
	}
	if factory.browserOpener == nil {
		t.Fatal("injected browser opener is missing")
	}
}

type batchInputFileSystemFakeForFactoryTest struct{}

type runDirectoryCreatorFakeForFactoryTest struct{}

func (runDirectoryCreatorFakeForFactoryTest) MkdirAll(string, fs.FileMode) error { return nil }

func (batchInputFileSystemFakeForFactoryTest) Stat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (batchInputFileSystemFakeForFactoryTest) ReadFile(string) ([]byte, error) {
	return nil, fs.ErrNotExist
}

func TestMissingCommandOperationFailsExecutionWithRequiredEdgeError(t *testing.T) {
	t.Parallel()

	root := NewCommandFactory(CommandOperations{ModelsCLI: injectedModelsCLIService{}}).NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "models list service is required") {
		t.Fatalf("error = %v, want explicit required-edge failure", err)
	}
}

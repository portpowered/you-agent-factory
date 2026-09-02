package named_invocation

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const preparationFailureSensitiveValue = "credential-that-must-not-leak"

// runNamedInvocationSharedPreparationFailures keeps every preparation-only
// failure on the package's one immutable root process. Each case owns its
// Factory files, HOME, working directory, and command output, so sharing the
// process cannot hide cross-scenario state.
func runNamedInvocationSharedPreparationFailures(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	tests := []struct {
		name      string
		selection string
		signature map[string]any
		factory   string
		arguments []string
		wantCode  string
		packaged  bool
	}{
		{
			name:      "named/missing_required",
			selection: "named",
			signature: missingRequiredSignature(),
			wantCode:  string(work.ArgumentErrorCodeMissingRequiredInput),
			packaged:  true,
		},
		{
			name:      "file/missing_required",
			selection: "file",
			signature: missingRequiredSignature(),
			wantCode:  string(work.ArgumentErrorCodeMissingRequiredInput),
			packaged:  true,
		},
		{
			name:      "named/reserved_collision",
			selection: "named",
			signature: reservedCollisionSignature(),
			wantCode:  climanifest.CompositionCollisionLongName,
			packaged:  true,
		},
		{
			name:      "file/reserved_collision",
			selection: "file",
			signature: reservedCollisionSignature(),
			wantCode:  climanifest.CompositionCollisionLongName,
			packaged:  true,
		},
		{
			name: "explicit/static_collision",
			factory: `{
  "name": "collision",
  "invocationSignature": {
    "parameters": [
      {
        "name": "credential",
        "sensitive": true,
        "bindings": [{"kind": "POSITIONAL", "position": 1}]
      },
      {
        "name": "reserved",
        "externalName": "quiet",
        "bindings": [{"kind": "NAMED"}]
      }
    ]
  }
}`,
			arguments: []string{preparationFailureSensitiveValue},
			wantCode:  climanifest.CompositionCollisionLongName,
		},
		{
			name: "explicit/sensitive_normalization_failure",
			factory: `{
  "name": "sensitive",
  "invocationSignature": {
    "parameters": [{
      "name": "token",
      "sensitive": true,
      "required": true,
      "choices": ["allowed"],
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  }
}`,
			arguments: []string{preparationFailureSensitiveValue},
			wantCode:  string(work.ArgumentErrorCodeStringValidationMismatch),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runSharedPreparationFailureCase(t, fixture, test)
		})
	}
}

type sharedPreparationFailureCase struct {
	name      string
	selection string
	signature map[string]any
	factory   string
	arguments []string
	wantCode  string
	packaged  bool
}

func (fixture *namedInvocationFixture) edges() serviceedges.Edges {
	return serviceedges.Edges{
		APIServerStarter:                          fixture.api.Start,
		ProviderCommandRunner:                     fixture.provider,
		FactoryDefinitionAuthoredReaderFileSystem: fixture.authoredReader,
		FactorySessionIDGenerator:                 uuid.NewString,
		WorkRequestIDGenerator:                    uuid.NewString,
	}
}

func runSharedPreparationFailureCase(
	t *testing.T,
	fixture *namedInvocationFixture,
	test sharedPreparationFailureCase,
) {
	t.Helper()
	scenario := fixture.newScenario(t, platformprocess.CommandResult{})
	factoryPath := filepath.Join(scenario.workingDirectory, "factory.json")
	if test.packaged {
		factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
		factoryDir = support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
		factoryPath = filepath.Join(factoryDir, "factory.json")
		replaceInvocationSignatureFixture(t, factoryPath, test.signature)
	} else if err := os.WriteFile(factoryPath, []byte(test.factory), 0o600); err != nil {
		t.Fatalf("write Factory fixture: %v", err)
	}

	stdout, stderr, err := executePreparationFailure(
		t, fixture.process, scenario.environment, scenario.workingDirectory,
		preparationFailureArguments(test.selection, factoryPath, test.arguments),
	)
	if err == nil || !strings.Contains(err.Error(), test.wantCode) {
		t.Fatalf("%s error = %v, want stable code %s", test.name, err, test.wantCode)
	}
	observable := errText(err) + stdout + stderr
	if strings.Contains(observable, preparationFailureSensitiveValue) {
		t.Fatalf("%s leaked sensitive input: %s", test.name, observable)
	}
	if scenario.provider.CallCount() != 0 {
		t.Fatalf("%s reached the provider after preparation failed", test.name)
	}
}

func preparationFailureArguments(selection, factoryPath string, arguments []string) []string {
	base := emptyInvocationArguments(selection, factoryPath, customizedNamedGoalFactoryName)
	return append(base, arguments...)
}

func emptyInvocationArguments(selection, factoryPath, factoryName string) []string {
	if selection == "named" {
		return []string{"you", "run", "--named", factoryName, "--no-record", "--quiet"}
	}
	return []string{"you", "run", "--factory", factoryPath, "--no-record", "--quiet"}
}

func executePreparationFailure(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err := process.Execute(root.Input{
		Args:             args,
		Env:              environment,
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

func missingRequiredSignature() map[string]any {
	return map[string]any{"parameters": []any{map[string]any{
		"name": "input", "required": true,
		"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
	}}}
}

func reservedCollisionSignature() map[string]any {
	return map[string]any{"parameters": []any{map[string]any{
		"name": "reserved", "externalName": "quiet",
		"bindings": []any{map[string]any{"kind": "NAMED"}},
	}}}
}

var errCanceledFactoryRootLookup = errors.New("explicit Factory root lookup failed after cancellation")

// runFactoryRootLookupCancellation retains the filesystem cancellation edge
// on the package's shared process. Only the scenario-specific authored-file
// route is mutable; the process and all other edges remain reusable.
func runFactoryRootLookupCancellation(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	scenario := fixture.newScenario(t, platformprocess.CommandResult{})
	workingDirectory := scenario.workingDirectory
	factoryPath := filepath.Join(workingDirectory, "factory.json")
	factory := `{
  "name": "canceled",
  "invocationSignature": {
    "parameters": [{
      "name": "input",
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  }
}`
	if err := os.WriteFile(factoryPath, []byte(factory), 0o600); err != nil {
		t.Fatalf("write Factory fixture: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := fixture.authoredReader.registerCancellation(factoryPath, cancel); err != nil {
		t.Fatalf("register authored-reader cancellation route: %v", err)
	}
	t.Cleanup(func() { fixture.authoredReader.unregisterCancellation(factoryPath) })
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err := fixture.process.Execute(root.Input{
		Args:  []string{"you", "run", "--factory", factoryPath, "--no-record", "--quiet", "draft"},
		Env:   scenario.environment,
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Context: ctx, WorkingDirectory: workingDirectory,
		StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute() error = %v, want context cancellation", err)
	}
	if errors.Is(err, errCanceledFactoryRootLookup) {
		t.Fatalf("Process.Execute() returned lookup failure instead of context cancellation: %v", err)
	}
	observable := errText(err) + stdout.String() + stderr.String()
	if strings.Contains(observable, preparationFailureSensitiveValue) {
		t.Fatalf("cancellation leaked sensitive input: %s", observable)
	}
	if scenario.provider.CallCount() != 0 {
		t.Fatal("canceled Factory lookup reached the provider")
	}
}

type namedInvocationAuthoredReaderRouter struct {
	mu                 sync.RWMutex
	cancellationRoutes map[string]context.CancelFunc
}

func newNamedInvocationAuthoredReaderRouter() *namedInvocationAuthoredReaderRouter {
	return &namedInvocationAuthoredReaderRouter{
		cancellationRoutes: make(map[string]context.CancelFunc),
	}
}

func (router *namedInvocationAuthoredReaderRouter) registerCancellation(
	path string,
	cancel context.CancelFunc,
) error {
	path = normalizeNamedInvocationWorkDir(path)
	if path == "" || cancel == nil {
		return errors.New("named-invocation authored-reader route requires a path and cancel function")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.cancellationRoutes[path]; exists {
		return errors.New("named-invocation authored-reader route is already registered")
	}
	router.cancellationRoutes[path] = cancel
	return nil
}

func (router *namedInvocationAuthoredReaderRouter) unregisterCancellation(path string) {
	router.mu.Lock()
	delete(router.cancellationRoutes, normalizeNamedInvocationWorkDir(path))
	router.mu.Unlock()
}

func (router *namedInvocationAuthoredReaderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.cancellationRoutes)
}

func (router *namedInvocationAuthoredReaderRouter) Stat(path string) (fs.FileInfo, error) {
	router.mu.RLock()
	cancel := router.cancellationRoutes[normalizeNamedInvocationWorkDir(path)]
	router.mu.RUnlock()
	if cancel != nil {
		cancel()
		return nil, errCanceledFactoryRootLookup
	}
	return os.Stat(path)
}

func (router *namedInvocationAuthoredReaderRouter) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

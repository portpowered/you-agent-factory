package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func defaultNamedFactoriesRootForTest() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories"), nil
}

func TestRunCommand_HelpDocumentsSupportedInputPathsAndStdoutModes(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for _, want := range []string{
		"--dir",
		"--named",
		"--factory",
		"trailing positional text or piped stdin text",
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"primary-result-only stdout by default",
		"--output response-stream",
	} {
		if !strings.Contains(runCmd.Long, want) {
			t.Fatalf("run command long help missing %q", want)
		}
	}
	if strings.Contains(runCmd.Long, "run --workflow") {
		t.Fatal("run command long help must not document run-level --workflow")
	}

	for _, want := range []string{
		"run --dir factory",
		"run --named @you/tts",
		"run --factory ./factory.json",
		"echo \"Ship the login bugfix\" | you run --named @you/goal",
		"run --named @you/goal --output response-stream",
	} {
		if !strings.Contains(runCmd.Example, want) {
			t.Fatalf("run command examples missing %q", want)
		}
	}
	if strings.Contains(runCmd.Example, "--workflow") {
		t.Fatal("run command examples must not document --workflow")
	}

	outputFlag := runCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag on run command")
	}
	if !strings.Contains(outputFlag.Usage, "primary (default)") || !strings.Contains(outputFlag.Usage, "response-stream") {
		t.Fatalf("--output usage = %q, want primary default and response-stream guidance", outputFlag.Usage)
	}
}

func TestRunCommand_FactoryFlagDocumentsPortableRun(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("factory")
	if flag == nil {
		t.Fatal("expected --factory flag on run command")
	}
	if flag.DefValue != "" {
		t.Fatalf("--factory default = %q, want empty", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "factory.json") {
		t.Fatalf("--factory usage = %q, want factory.json guidance", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--factory") {
		t.Fatal("expected run command long help text to document --factory")
	}
	if !strings.Contains(runCmd.Long, "trailing positional text or piped stdin text") {
		t.Fatal("expected run command long help text to document invocation input sources")
	}
	if !strings.Contains(runCmd.Long, "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatal("expected run command long help text to document the stable input conflict code")
	}
	if !strings.Contains(runCmd.Long, "you docs run") || !strings.Contains(runCmd.Long, "you docs sessions") {
		t.Fatal("expected run command long help text to point to invocation reference docs")
	}
	if !strings.Contains(runCmd.Example, "run --factory ./factory.json \"Fix the lint issues\"") {
		t.Fatal("expected run command examples to document simplified --factory run")
	}
	if !strings.Contains(flag.Usage, "piped stdin") {
		t.Fatalf("--factory usage = %q, want invocation input guidance", flag.Usage)
	}
}

func TestRunCommand_RunCommandLongHelpDocumentsNamedFactory(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	if !strings.Contains(runCmd.Long, "--named") {
		t.Fatal("expected run command long help text to document --named")
	}
	if !strings.Contains(runCmd.Long, "resolve project-local factories before global built-ins") {
		t.Fatal("expected run command long help text to document local-over-global named resolution")
	}
	if !strings.Contains(runCmd.Long, "materialize lazily into that global root on first use and stay editable on disk") {
		t.Fatal("expected run command long help text to document built-in materialization and editability")
	}
	if !strings.Contains(runCmd.Long, "run --named <factory> --help") || !strings.Contains(runCmd.Long, "run --factory <factory.json> --help") {
		t.Fatal("expected run command long help text to document signature-aware factory help entry points")
	}
	if !strings.Contains(runCmd.Long, "existing run-level flags available") {
		t.Fatal("expected run command long help text to explain that operational flags remain available alongside factory-defined arguments")
	}
	if !strings.Contains(runCmd.Example, "run --named @you/tts") {
		t.Fatal("expected run command examples to document simplified --named run")
	}
	namedFlag := runCmd.Flags().Lookup("named")
	if namedFlag == nil {
		t.Fatal("expected --named flag on run command")
	}
	if !strings.Contains(namedFlag.Usage, "built-ins materialize there on first use and remain editable") {
		t.Fatalf("--named usage = %q, want built-in editability guidance", namedFlag.Usage)
	}
}

func TestRunCommand_NamedFactoryHelpRendersInvocationSignature(t *testing.T) {
	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	projectRoot := filepath.Join(workingDirectory, "factory")
	factoryDir, err := factorydefinitionfixtures.SeedNamedFactory(filepath.Join(projectRoot, "alpha"), portableFactoryPayloadWithInvocationSignature())
	if err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	root := newLegacyTestRootCommandWithCatalog(transportNamedFactoryCatalog{"alpha": factoryDir})
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named alpha --help: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"Factory invocation help",
		"Selected factory: portable (named factory alpha)",
		"Usage:\n  you run --named alpha <input> [--confirm <true|false>] [--mode <value>] [--output <file-path>]",
		"positional 1 <input>",
		"--confirm <true|false>",
		"Named form also accepts bare `--confirm` as `true`.",
		"stdin",
		"Accepted values: fast, safe.",
		"Default: safe.",
		"Path parameter: output",
		"you run --named alpha 'Fix the lint issues' --mode safe --output report.md",
		"printf '%s\\n' 'Fix the lint issues' | you run --named alpha --mode fast",
		"Existing operational flags such as `--no-record`, `--with-mock-workers`, `--server`, and `--json` still apply.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run --named alpha --help missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Load workflow and run the factory engine") {
		t.Fatalf("expected signature-aware help instead of generic Cobra help:\n%s", got)
	}
}

func TestRunCommand_NamedFlagResolvesFactoryRootBeforeRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	globalRoot, err := defaultNamedFactoriesRootForTest()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}

	wantRoot, err := factorydefinitionfixtures.SeedNamedFactory(filepath.Join(globalRoot, "alpha"), portableFactoryPayloadWithDefaultHandling())
	if err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithCatalog(rootNamedFactoryCatalogFake{
		resolve: func(projectRoot, globalRoot, name string) (*interfaces.NamedFactoryResolution, error) {
			return &interfaces.NamedFactoryResolution{
				Name:        name,
				FactoryDir:  wantRoot,
				Source:      interfaces.NamedFactoryResolutionSourceGlobal,
				ProjectRoot: projectRoot,
				GlobalRoot:  globalRoot,
			}, nil
		},
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named: %v", err)
	}
	if testutil.CanonicalPath(got.Dir) != testutil.CanonicalPath(wantRoot) {
		t.Fatalf("dir = %q, want %q", got.Dir, wantRoot)
	}
	if got.NamedFactoryName != "alpha" {
		t.Fatalf("named factory name = %q, want alpha", got.NamedFactoryName)
	}
}

func TestRunCommand_NamedFlagPrefersProjectFactoryOverGlobal(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	homeDirectory := t.TempDir()
	projectDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(projectDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", projectDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	globalRoot, err := defaultNamedFactoriesRootForTest()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	projectRoot := filepath.Join(projectDirectory, "factory")

	if _, err := factorydefinitionfixtures.SeedNamedFactory(filepath.Join(globalRoot, "alpha"), portableFactoryPayloadWithDefaultHandling()); err != nil {
		t.Fatalf("PersistNamedFactory(global alpha): %v", err)
	}

	wantRoot, err := factorydefinitionfixtures.SeedNamedFactory(filepath.Join(projectRoot, "alpha"), portableFactoryPayloadWithDefaultHandling())
	if err != nil {
		t.Fatalf("PersistNamedFactory(local alpha): %v", err)
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
				FactoryDir:         wantRoot,
				Source:             interfaces.NamedFactoryResolutionSourceProjectLocal,
				ProjectRoot:        projectRoot,
				GlobalRoot:         globalRoot,
				PrecedenceDecision: interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal,
			}, nil
		},
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named: %v", err)
	}
	if testutil.CanonicalPath(got.Dir) != testutil.CanonicalPath(wantRoot) {
		t.Fatalf("dir = %q, want project-local %q", got.Dir, wantRoot)
	}
}

func portableFactoryPayloadWithDefaultHandling() []byte {
	return []byte(`{
  "name": "portable",
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workstations": [{
    "name": "ws",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}],
    "onFailure": [{"workType": "story", "state": "failed"}]
  }]
}`)
}

func portableFactoryPayloadWithInvocationSignature() []byte {
	return []byte(`{
  "name": "portable",
  "invocationSignature": {
    "parameters": [
      {
        "name": "input",
        "description": "Primary text input for the portable factory.",
        "required": true,
        "bindings": [{"kind": "POSITIONAL", "position": 1}, {"kind": "STDIN"}]
      },
      {
        "name": "mode",
        "description": "Execution mode for the portable factory.",
        "choices": ["fast", "safe"],
        "defaultValue": "safe",
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "confirm",
        "typeHint": "BOOLEAN_STRING",
        "description": "Request confirmation mode.",
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "output",
        "description": "Optional output file path.",
        "aliases": ["out"],
        "typeHint": "FILE_PATH",
        "bindings": [{"kind": "NAMED"}]
      }
    ],
    "outputContract": {
      "mode": "FILE",
      "pathParameter": "output",
      "contentType": "text/plain",
      "fileExtension": ".txt"
    },
    "examples": [
      {
        "name": "positional-input",
        "argv": ["Fix the lint issues", "--mode", "safe", "--output", "report.md"]
      },
      {
        "name": "stdin-input",
        "argv": ["--mode", "fast"],
        "stdin": "Fix the lint issues"
      }
    ]
  },
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workstations": [{
    "name": "ws",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}],
    "onFailure": [{"workType": "story", "state": "failed"}]
  }]
}`)
}

func TestRunCommand_NamedFactorySignatureArgsPreserveRunFlagsAndNormalizeInputs(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	factoryDir, restore := setupNamedFactoryInvocationTest(t)
	defer restore()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithCatalogAndInvocationInput(transportNamedFactoryCatalog{"alpha": factoryDir}, programmedInvocationInput(work.PreparedInvocationInput{NormalizedArguments: &work.NormalizedArguments{Arguments: map[string]work.NormalizedArgument{
		"input": {Values: []string{"draft"}}, "mode": {Values: []string{"fast"}}, "confirm": {Values: []string{"true"}}, "output": {Values: []string{"result.md"}},
	}}}, nil))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "alpha",
		"--no-record",
		"draft",
		"--mode", "fast",
		"--confirm",
		"--out=result.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named alpha with signature args: %v", err)
	}
	if !got.DisableDefaultRecording {
		t.Fatal("expected --no-record to remain a run-level flag")
	}
	if got.InvocationNormalizedArguments == nil {
		t.Fatal("expected signature-backed invocation arguments to be normalized")
	}
	if got.Output == nil {
		t.Fatal("signature-backed one-shot invocation output = nil, want process stdout")
	}

	wantValues := map[string]string{
		"input":   "draft",
		"mode":    "fast",
		"confirm": "true",
		"output":  "result.md",
	}
	for name, want := range wantValues {
		assertInvocationArgumentValue(t, got, name, want)
	}
}

func setupNamedFactoryInvocationTest(t *testing.T) (string, func()) {
	t.Helper()

	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	globalRoot, err := defaultNamedFactoriesRootForTest()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	factoryDir := filepath.Join(globalRoot, "alpha")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), portableFactoryPayloadWithInvocationSignature(), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	return factoryDir, func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}
}

func assertInvocationArgumentValue(t *testing.T, got runcli.RunConfig, name, want string) {
	t.Helper()

	values := got.InvocationNormalizedArguments.Arguments[name].Values
	if len(values) != 1 || values[0] != want {
		t.Fatalf("%s values = %#v, want [%s]", name, values, want)
	}
}

func TestRunCommand_NamedAndDirFlagsRejectConflict(t *testing.T) {
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
	root.SetArgs([]string{"run", "--named", "alpha", "--dir", "other-factory"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between --named and --dir")
	}
	if !strings.Contains(err.Error(), "--named cannot be used with --dir") {
		t.Fatalf("error = %q, want conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when --named conflicts with --dir")
	}
}

func TestRunCommand_NamedAndFactoryFlagsRejectConflict(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, portableFactoryPayloadWithDefaultHandling(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha", "--factory", factoryPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between --named and --factory")
	}
	if !strings.Contains(err.Error(), "--named cannot be used with --factory") {
		t.Fatalf("error = %q, want conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when --named conflicts with --factory")
	}
}

func TestRunCommand_FactoryFlagResolvesFactoryRootBeforeRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, portableFactoryPayloadWithDefaultHandling(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	wantRoot, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --factory: %v", err)
	}
	if got.Dir != wantRoot {
		t.Fatalf("dir = %q, want %q", got.Dir, wantRoot)
	}
	if got.FactoryConfigPath != factoryPath {
		t.Fatalf("factory config path = %q, want %q", got.FactoryConfigPath, factoryPath)
	}
}

func TestRunCommand_FactoryAndDirFlagsRejectConflict(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(`{"id":"portable"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--dir", "other-factory"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between --factory and --dir")
	}
	if !strings.Contains(err.Error(), "--factory cannot be used with --dir") {
		t.Fatalf("error = %q, want conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when --factory conflicts with --dir")
	}
}

func TestRunCommand_FactoryFlagRejectsMissingConfigFileBeforeRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	missingPath := filepath.Join(t.TempDir(), "missing-factory.json")
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", missingPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing --factory path to fail")
	}
	if !strings.Contains(err.Error(), "factory config file not found") {
		t.Fatalf("error = %q, want not-found message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for missing --factory path")
	}
}

func TestRunCommand_FactoryFlagRejectsDirectoryPathBeforeRun(t *testing.T) {
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
	root.SetArgs([]string{"run", "--factory", t.TempDir()})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected directory --factory path to fail")
	}
	if !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("error = %q, want file requirement message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for directory --factory path")
	}
}

func writePortableFactoryWithDefaultHandling(t *testing.T, dir string) string {
	t.Helper()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(`{
  "name": "portable",
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workstations": [{
    "name": "ws",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}],
    "onFailure": [{"workType": "story", "state": "failed"}]
  }]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return factoryPath
}

func TestRunCommand_FactoryPromptCarriesInvocationText(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(work.InputSourcePositionalText, "Fix the lint issues"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix", "the", "lint", "issues"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --factory with prompt: %v", err)
	}
	if got.InvocationPositionalText == nil {
		t.Fatal("expected invocation positional text for factory prompt run")
	}
	if gotText := *got.InvocationPositionalText; gotText != "Fix the lint issues" {
		t.Fatalf("invocation positional text = %q, want joined prompt text", gotText)
	}
}

func TestRunCommand_FactoryStdinPromptCarriesInvocationText(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(work.InputSourceStdinText, "Fix the stdin path\n"))
	root.SetIn(strings.NewReader("Fix the stdin path\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "-"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --factory with stdin prompt: %v", err)
	}
	if got.InvocationStdinText == nil {
		t.Fatal("expected invocation stdin text for factory prompt run")
	}
	if gotText := *got.InvocationStdinText; gotText != "Fix the stdin path\n" {
		t.Fatalf("invocation stdin text = %q, want raw stdin prompt text", gotText)
	}
}

func TestRunCommand_FactoryPromptSelectsSharedTextInvocationMode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		if cfg.StartupOutput != nil {
			io.WriteString(cfg.StartupOutput, "Factory initiated: unexpected\n")
			io.WriteString(cfg.StartupOutput, "Dashboard URL: unexpected\n")
			io.WriteString(cfg.StartupOutput, "Recording saved: unexpected\n")
		}
		return nil
	}

	var stdout bytes.Buffer
	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(work.InputSourcePositionalText, "Fix the lint issues"))
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "run", "--factory", factoryPath, "Fix the lint issues"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute shared factory prompt run: %v", err)
	}
	if got.InvocationPositionalText == nil {
		t.Fatal("expected invocation positional text for factory prompt run")
	}
	if got.CleanInvocation {
		t.Fatal("expected shared text invocation to keep clean invocation disabled")
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected shared text invocation to suppress dashboard rendering")
	}
	if got.StartupOutput != nil {
		t.Fatalf("startup output = %#v, want nil for shared text invocation mode", got.StartupOutput)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to clean run config")
	}
	if got.Output == nil {
		t.Fatal("expected shared text invocation config to receive stdout writer")
	}
	assertRunStdoutFreeOfOperatorChatter(t, stdout.String())
}

func TestRunCommand_FactoryWorkFileSelectsCleanInvocationMode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)
	workPath := filepath.Join(dir, "work.json")

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--work", workPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute clean factory work-file run: %v", err)
	}
	if !got.CleanInvocation {
		t.Fatal("expected factory work-file batch run to select clean invocation mode")
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected clean invocation mode to suppress dashboard rendering")
	}
	if got.StartupOutput != nil {
		t.Fatalf("startup output = %#v, want nil for clean invocation mode", got.StartupOutput)
	}
}

func TestRunCommand_FactoryContinuousPromptKeepsOperatorOutputMode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--continuously", "Fix the lint issues"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute continuous factory prompt run: %v", err)
	}
	if got.CleanInvocation {
		t.Fatal("continuous factory run should keep operator output mode")
	}
	if got.SuppressDashboardRendering {
		t.Fatal("continuous factory run should not implicitly suppress dashboard rendering")
	}
	if got.StartupOutput == nil {
		t.Fatal("continuous factory run should keep startup output configured")
	}
}

func TestRunCommand_FactoryPromptRejectsEmptyPositionalWithStableCode(t *testing.T) {
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

	root := newLegacyTestRootCommandWithInvocationInput(programmedInvocationInput(work.PreparedInvocationInput{}, &work.InputError{Code: work.InputErrorCodeEmpty, Message: "invocation input is empty", Source: work.InputSourcePositionalText}))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, ""})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected explicit empty positional rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for explicit empty factory positional input")
	}
}

func TestRunCommand_FactoryPromptRejectsWhitespaceOnlyPositionalWithStableCode(t *testing.T) {
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

	root := newLegacyTestRootCommandWithInvocationInput(programmedInvocationInput(work.PreparedInvocationInput{}, &work.InputError{Code: work.InputErrorCodeEmpty, Message: "invocation input is empty", Source: work.InputSourcePositionalText}))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "   "})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected whitespace-only positional rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for whitespace-only factory positional input")
	}
}

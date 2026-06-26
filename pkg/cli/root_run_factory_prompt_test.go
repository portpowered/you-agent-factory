package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestRunCommand_FactoryFlagDocumentsPortableRun(t *testing.T) {
	root := NewRootCommand()
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
	if !strings.Contains(runCmd.Long, "you docs config") || !strings.Contains(runCmd.Long, "you docs sessions") {
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
	root := NewRootCommand()
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

	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}

	wantRoot, err := factoryconfig.PersistNamedFactory(globalRoot, "alpha", portableFactoryPayloadWithDefaultHandling())
	if err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
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

	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	projectRoot, err := factoryconfig.DefaultProjectNamedFactoryRoot(projectDirectory)
	if err != nil {
		t.Fatalf("DefaultProjectNamedFactoryRoot: %v", err)
	}

	if _, err := factoryconfig.PersistNamedFactory(globalRoot, "alpha", portableFactoryPayloadWithDefaultHandling()); err != nil {
		t.Fatalf("PersistNamedFactory(global alpha): %v", err)
	}

	wantRoot, err := factoryconfig.PersistNamedFactory(projectRoot, "alpha", portableFactoryPayloadWithDefaultHandling())
	if err != nil {
		t.Fatalf("PersistNamedFactory(local alpha): %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
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
        "bindings": [{"kind": "POSITIONAL", "position": 1}]
      },
      {
        "name": "mode",
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "confirm",
        "typeHint": "BOOLEAN_STRING",
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "output",
        "aliases": ["out"],
        "bindings": [{"kind": "NAMED"}]
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

	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	segment, err := factoryconfig.NamedFactoryNameToLayoutSegment("alpha")
	if err != nil {
		t.Fatalf("NamedFactoryNameToLayoutSegment(alpha): %v", err)
	}
	factoryDir := filepath.Join(globalRoot, segment)
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), portableFactoryPayloadWithInvocationSignature(), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
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
	if values := got.InvocationNormalizedArguments.Arguments["input"].Values; len(values) != 1 || values[0] != "draft" {
		t.Fatalf("input values = %#v, want [draft]", values)
	}
	if values := got.InvocationNormalizedArguments.Arguments["mode"].Values; len(values) != 1 || values[0] != "fast" {
		t.Fatalf("mode values = %#v, want [fast]", values)
	}
	if values := got.InvocationNormalizedArguments.Arguments["confirm"].Values; len(values) != 1 || values[0] != "true" {
		t.Fatalf("confirm values = %#v, want [true]", values)
	}
	if values := got.InvocationNormalizedArguments.Arguments["output"].Values; len(values) != 1 || values[0] != "result.md" {
		t.Fatalf("output values = %#v, want [result.md]", values)
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

	root := NewRootCommand()
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
	if err := os.WriteFile(factoryPath, []byte(`{"id":"portable"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	root := NewRootCommand()
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
	if err := os.WriteFile(factoryPath, []byte(`{"id":"portable"}`), 0o644); err != nil {
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
	root.SetIn(strings.NewReader("Fix from stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix from args", "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ambiguous positional and stdin prompt rejection")
	}
	for _, want := range []string{
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"positional_text",
		"stdin_text",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

func TestRunCommand_CleanInvocationFailureWritesPlaintextToStderr(t *testing.T) {
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

	root := NewRootCommand()
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
	if got := stderr.String(); got != "RUN_INVOCATION_FAILED: clean invocation failed: mock worker rejected\n" {
		t.Fatalf("stderr = %q", got)
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

	root := NewRootCommand()
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
}

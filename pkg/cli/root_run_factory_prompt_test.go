package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
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
	if !strings.Contains(runCmd.Example, "run --factory ./factory.json \"Fix the lint issues\"") {
		t.Fatal("expected run command examples to document simplified --factory run")
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
	root.SetArgs([]string{"run", "--factory", factoryPath, "--port", "0"})

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

func TestRunCommand_FactoryPromptSubmitsDefaultWorkTypeWorkFile(t *testing.T) {
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
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix", "the", "lint", "issues", "--port", "0"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --factory with prompt: %v", err)
	}
	if got.WorkFile == "" {
		t.Fatal("expected generated work file for factory prompt run")
	}
	t.Cleanup(func() { _ = os.Remove(got.WorkFile) })

	workRequest, err := runcli.LoadWorkFile(got.WorkFile)
	if err != nil {
		t.Fatalf("LoadWorkFile: %v", err)
	}
	if len(workRequest.Works) != 1 || workRequest.Works[0].WorkTypeID != "story" {
		t.Fatalf("works = %#v, want one story work item", workRequest.Works)
	}
	if payload, ok := workRequest.Works[0].Payload.(string); !ok || payload != "Fix the lint issues" {
		t.Fatalf("payload = %#v, want joined prompt text", workRequest.Works[0].Payload)
	}
}

func TestRunCommand_FactoryPromptRejectsEmptyPrompt(t *testing.T) {
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
		t.Fatal("expected empty prompt rejection")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("error = %q, want prompt requirement", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for empty factory prompt")
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

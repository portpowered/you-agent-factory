package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type configInitOutcome struct {
	HomeDir             string
	ConfigPath          string
	NamedFactoriesRoot  string
	SystemConfigOutcome string
}

func executeSessionCommand(
	t testing.TB,
	process support.Process,
	session *builtcliacceptance.Session,
	args ...string,
) (*support.CapturedInputs, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), session.ProcessEnv()...)
	inputs.Input.Stdin = strings.NewReader("")
	inputs.Input.WorkingDirectory = session.WorkDir
	return inputs, process.Execute(inputs.Input)
}

func requireSessionCommandSuccess(
	t testing.TB,
	process support.Process,
	session *builtcliacceptance.Session,
	scenario string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	inputs, err := executeSessionCommand(t, process, session, args...)
	if err != nil {
		t.Fatalf(
			"%s: Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s",
			scenario,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs
}

func initializeConfig(
	t testing.TB,
	ctx context.Context,
	session *builtcliacceptance.Session,
	scenario string,
) (builtcliacceptance.RunResult, configInitOutcome) {
	t.Helper()

	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	namedFactoriesRoot := filepath.Join(session.HomeDir, ".you-agent-factory", "factories")
	outcome := "skipped"
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		outcome = "created"
	}
	missingFactory := filepath.Join(session.WorkDir, "missing-initialization-factory.json")
	result, err := session.Run(ctx, "run", "--factory", missingFactory)
	if err == nil {
		t.Fatalf("%s: run missing Factory error = %v; stdout=%q stderr=%q", scenario, err, result.Stdout, result.Stderr)
	}
	support.RequireSafeCLIDiagnostic(t, result.Stderr)
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("%s: initializer-owned config missing at %s: %v", scenario, configPath, err)
	}
	return result, configInitOutcome{
		HomeDir:             session.HomeDir,
		ConfigPath:          configPath,
		NamedFactoriesRoot:  namedFactoriesRoot,
		SystemConfigOutcome: outcome,
	}
}

func initializeConfigWithProcess(
	t testing.TB,
	process support.Process,
	session *builtcliacceptance.Session,
	scenario string,
) (*support.CapturedInputs, configInitOutcome) {
	t.Helper()

	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	namedFactoriesRoot := filepath.Join(session.HomeDir, ".you-agent-factory", "factories")
	outcome := "skipped"
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		outcome = "created"
	}
	missingFactory := filepath.Join(session.WorkDir, "missing-initialization-factory.json")
	result, err := executeSessionCommand(t, process, session, "run", "--factory", missingFactory)
	if err == nil {
		t.Fatalf("%s: run missing Factory error = %v; stdout=%q stderr=%q", scenario, err, result.Stdout(), result.Stderr())
	}
	support.RequireSafeCLIDiagnostic(t, result.Stderr())
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("%s: initializer-owned config missing at %s: %v", scenario, configPath, err)
	}
	return result, configInitOutcome{
		HomeDir:             session.HomeDir,
		ConfigPath:          configPath,
		NamedFactoriesRoot:  namedFactoriesRoot,
		SystemConfigOutcome: outcome,
	}
}

func TestRootProcessHarness_IsolatesHomeAndLogDirectoriesAcrossSessions(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))

	first := harness.NewSession(t)
	second := harness.NewSession(t)
	process := support.BuildProcess(t, harness.Edges)
	support.CleanupProcess(t, process)

	if first.HomeDir == second.HomeDir || first.LogDir == second.LogDir {
		t.Fatalf("sessions share paths: first home=%q log=%q second home=%q log=%q",
			first.HomeDir, first.LogDir, second.HomeDir, second.LogDir)
	}

	firstResult, firstInit := initializeConfigWithProcess(t, process, first, "first-config-init")
	secondResult, secondInit := initializeConfigWithProcess(t, process, second, "second-config-init")

	firstConfig := firstInit.ConfigPath
	secondConfig := secondInit.ConfigPath
	if firstConfig == secondConfig {
		t.Fatalf("operator config paths collided: %q", firstConfig)
	}
	for _, path := range []string{firstConfig, secondConfig} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("operator config %q: %v", path, err)
		}
	}

	if strings.Contains(firstResult.Stdout()+firstResult.Stderr(), second.HomeDir) {
		t.Fatalf("first session output leaked second home %q:\nstdout=%q\nstderr=%q",
			second.HomeDir, firstResult.Stdout(), firstResult.Stderr())
	}
	if strings.Contains(secondResult.Stdout()+secondResult.Stderr(), first.HomeDir) {
		t.Fatalf("second session output leaked first home %q:\nstdout=%q\nstderr=%q",
			first.HomeDir, secondResult.Stdout(), secondResult.Stderr())
	}
	if _, err := os.Stat(first.LogDir); err != nil {
		t.Fatalf("first log dir %q: %v", first.LogDir, err)
	}
	if _, err := os.Stat(second.LogDir); err != nil {
		t.Fatalf("second log dir %q: %v", second.LogDir, err)
	}
	if info, err := os.Stat(filepath.Join(first.HomeDir, ".you-agent-factory")); err != nil {
		t.Fatalf("first home state dir: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("first home state path = %q, want directory", filepath.Join(first.HomeDir, ".you-agent-factory"))
	}

}

// TestRootProcess_HelpPrintsUsageAndExitsSuccessfully proves the root process exposes usable root help.
func TestRootProcess_HelpPrintsUsageAndExitsSuccessfully(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "-h")
	session.RequireSuccess(t, "cli-help", result, err)
	if !strings.Contains(result.Stdout, "Usage:\n  you [flags]") {
		t.Fatalf("help output did not contain root usage:\n%s", result.Stdout)
	}
}

// TestRootProcess_ConfigAndFactoryAuthoringUseAcceptedInputs proves the built
// executable accepts canonical config and Factory-authoring inputs.
func TestRootProcess_ConfigAndFactoryAuthoringUseAcceptedInputs(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	process := support.BuildProcess(t, harness.Edges)
	support.CleanupProcess(t, process)

	assertRootProcessInitContract(t, process, session)
	sourcePath := testutil.MustRepoPath(
		t,
		"tests/release/testdata/cli_smoke_factory/factory.json",
	)
	createdPath := assertRootProcessFactoryAuthoring(t, process, session, sourcePath)
	assertRootProcessFactoryConfigTransforms(t, process, session, sourcePath, createdPath)
}

func assertRootProcessInitContract(
	t *testing.T,
	process support.Process,
	session *builtcliacceptance.Session,
) {
	t.Helper()
	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	removed, removedErr := executeSessionCommand(t, process, session, "init", "--dir", "legacy-factory")
	if removedErr == nil {
		t.Fatalf(
			"init without packaged selection = (%v, stdout=%q, stderr=%q), want failure",
			removedErr,
			removed.Stdout(),
			removed.Stderr(),
		)
	}
	support.RequireSafeCLIDiagnostic(t, removed.Stderr())
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed init input config stat = %v, want not exist", err)
	}

	requireSessionCommandSuccess(
		t,
		process,
		session,
		"non-interactive-init",
		"init",
		"--provider", "codex",
		"--model", "gpt-5",
	)
	var configured struct {
		Defaults struct {
			WorkerModelProvider string `json:"workerModelProvider"`
			WorkerModel         string `json:"workerModel"`
		} `json:"defaults"`
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read configured operator defaults: %v", err)
	}
	if err := json.Unmarshal(configData, &configured); err != nil {
		t.Fatalf("decode configured operator defaults: %v", err)
	}
	if configured.Defaults.WorkerModelProvider != "codex" ||
		configured.Defaults.WorkerModel != "gpt-5" {
		t.Fatalf("configured defaults = %#v, want codex/gpt-5", configured.Defaults)
	}
}

func assertRootProcessFactoryAuthoring(
	t *testing.T,
	process support.Process,
	session *builtcliacceptance.Session,
	sourcePath string,
) string {
	t.Helper()
	factoryRoot := filepath.Join(session.WorkDir, "named-factories")
	createResult := requireSessionCommandSuccess(
		t,
		process,
		session,
		"factory-create",
		"factory", "create", "acceptance",
		"--from", sourcePath,
		"--dir", factoryRoot,
	)
	if !strings.Contains(createResult.Stdout(), "Created factory acceptance") {
		t.Fatalf("factory create stdout = %q", createResult.Stdout())
	}
	createdPath := filepath.Join(factoryRoot, "acceptance", "factory.json")
	if _, err := os.Stat(createdPath); err != nil {
		t.Fatalf("created Factory config missing: %v", err)
	}

	updateResult := requireSessionCommandSuccess(
		t,
		process,
		session,
		"factory-update",
		"factory", "update", "acceptance",
		"--from", sourcePath,
		"--dir", factoryRoot,
	)
	if !strings.Contains(updateResult.Stdout(), "Updated factory acceptance") {
		t.Fatalf("factory update stdout = %q", updateResult.Stdout())
	}
	return createdPath
}

func assertRootProcessFactoryConfigTransforms(
	t *testing.T,
	process support.Process,
	session *builtcliacceptance.Session,
	sourcePath string,
	createdPath string,
) {
	t.Helper()
	validateResult := requireSessionCommandSuccess(
		t,
		process,
		session,
		"factory-config-validate",
		"factory", "config", "validate", createdPath,
	)
	if !strings.Contains(validateResult.Stdout(), "Factory validation passed.") {
		t.Fatalf("factory validation stdout = %q", validateResult.Stdout())
	}

	flattenResult := requireSessionCommandSuccess(
		t,
		process,
		session,
		"factory-config-flatten",
		"factory", "config", "flatten", filepath.Dir(createdPath),
	)
	var flattened map[string]any
	if err := json.Unmarshal([]byte(flattenResult.Stdout()), &flattened); err != nil {
		t.Fatalf("flattened Factory output is not JSON: %v\n%s", err, flattenResult.Stdout())
	}

	expandDir := filepath.Join(session.WorkDir, "expand-case")
	if err := os.MkdirAll(expandDir, 0o755); err != nil {
		t.Fatalf("create expand fixture directory: %v", err)
	}
	expandSource := filepath.Join(expandDir, "factory.json")
	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read expand source: %v", err)
	}
	if err := os.WriteFile(expandSource, sourceData, 0o600); err != nil {
		t.Fatalf("write expand source: %v", err)
	}
	expandResult := requireSessionCommandSuccess(
		t,
		process,
		session,
		"factory-config-expand",
		"factory", "config", "expand", expandSource,
	)
	if !strings.Contains(expandResult.Stdout(), "Expanded factory config into "+expandDir) {
		t.Fatalf("factory expand stdout = %q, want target %q", expandResult.Stdout(), expandDir)
	}
	if _, err := os.Stat(filepath.Join(expandDir, "workers", "worker-a", "AGENTS.md")); err != nil {
		t.Fatalf("expanded worker instructions missing: %v", err)
	}
}

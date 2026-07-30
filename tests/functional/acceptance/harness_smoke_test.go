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
)

type configInitOutcome struct {
	HomeDir             string
	ConfigPath          string
	NamedFactoriesRoot  string
	SystemConfigOutcome string
}

func initializeConfig(t testing.TB, ctx context.Context, session *builtcliacceptance.Session, scenario string) (builtcliacceptance.RunResult, configInitOutcome) {
	t.Helper()

	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	namedFactoriesRoot := filepath.Join(session.HomeDir, ".you-agent-factory", "factories")
	outcome := "skipped"
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		outcome = "created"
	}
	missingFactory := filepath.Join(session.WorkDir, "missing-initialization-factory.json")
	result, err := session.Run(ctx, "run", "--factory", missingFactory)
	if err == nil || !strings.Contains(result.Stdout+result.Stderr+err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("%s: run missing Factory error = %v; stdout=%q stderr=%q", scenario, err, result.Stdout, result.Stderr)
	}
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

	if first.HomeDir == second.HomeDir || first.LogDir == second.LogDir {
		t.Fatalf("sessions share paths: first home=%q log=%q second home=%q log=%q",
			first.HomeDir, first.LogDir, second.HomeDir, second.LogDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	firstResult, firstInit := initializeConfig(t, ctx, first, "first-config-init")
	_, secondInit := initializeConfig(t, ctx, second, "second-config-init")

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

	if strings.Contains(firstResult.Stdout+firstResult.Stderr, second.HomeDir) {
		t.Fatalf("first session output leaked second home %q:\nstdout=%q\nstderr=%q",
			second.HomeDir, firstResult.Stdout, firstResult.Stderr)
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

func TestRootProcessHarness_NonZeroExitIncludesDiagnostics(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "definitely-not-a-real-subcommand")
	if err == nil {
		t.Fatalf("expected failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for unknown subcommand")
	}

	var failure *builtcliacceptance.ScenarioFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error type = %T, want *builtcliacceptance.ScenarioFailure: %v", err, err)
	}
	if failure.ExitCode == 0 {
		t.Fatalf("failure exit code = 0, want non-zero")
	}
	if strings.TrimSpace(failure.StderrTail) == "" && strings.TrimSpace(failure.StdoutTail) == "" {
		t.Fatalf("failure diagnostics missing stdout/stderr tails: %#v", failure)
	}
	if failure.HomeDir != session.HomeDir || failure.LogDir != session.LogDir {
		t.Fatalf("failure paths = home %q log %q, want home %q log %q",
			failure.HomeDir, failure.LogDir, session.HomeDir, session.LogDir)
	}
	if failure.ProcessBoundary != "root.BuildProcess" {
		t.Fatalf("failure process boundary = %q, want root.BuildProcess", failure.ProcessBoundary)
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
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	assertRootProcessInitContract(t, ctx, session)
	sourcePath := testutil.MustRepoPath(
		t,
		"tests/release/testdata/cli_smoke_factory/factory.json",
	)
	createdPath := assertRootProcessFactoryAuthoring(t, ctx, session, sourcePath)
	assertRootProcessFactoryConfigTransforms(t, ctx, session, sourcePath, createdPath)
}

func assertRootProcessInitContract(
	t *testing.T,
	ctx context.Context,
	session *builtcliacceptance.Session,
) {
	t.Helper()
	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	removed, removedErr := session.Run(ctx, "init", "--dir", "legacy-factory")
	if removedErr == nil ||
		!strings.Contains(removed.Stdout+removed.Stderr+removedErr.Error(), "use --provider") {
		t.Fatalf(
			"init without packaged selection = (%v, stdout=%q, stderr=%q), want provider setup requirement",
			removedErr,
			removed.Stdout,
			removed.Stderr,
		)
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed init input config stat = %v, want not exist", err)
	}

	initResult, initErr := session.Run(
		ctx,
		"init",
		"--provider", "codex",
		"--model", "gpt-5",
	)
	session.RequireSuccess(t, "non-interactive-init", initResult, initErr)
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
	ctx context.Context,
	session *builtcliacceptance.Session,
	sourcePath string,
) string {
	t.Helper()
	factoryRoot := filepath.Join(session.WorkDir, "named-factories")
	createResult, createErr := session.Run(
		ctx,
		"factory", "create", "acceptance",
		"--from", sourcePath,
		"--dir", factoryRoot,
	)
	session.RequireSuccess(t, "factory-create", createResult, createErr)
	if !strings.Contains(createResult.Stdout, "Created factory acceptance") {
		t.Fatalf("factory create stdout = %q", createResult.Stdout)
	}
	createdPath := filepath.Join(factoryRoot, "acceptance", "factory.json")
	if _, err := os.Stat(createdPath); err != nil {
		t.Fatalf("created Factory config missing: %v", err)
	}

	updateResult, updateErr := session.Run(
		ctx,
		"factory", "update", "acceptance",
		"--from", sourcePath,
		"--dir", factoryRoot,
	)
	session.RequireSuccess(t, "factory-update", updateResult, updateErr)
	if !strings.Contains(updateResult.Stdout, "Updated factory acceptance") {
		t.Fatalf("factory update stdout = %q", updateResult.Stdout)
	}
	return createdPath
}

func assertRootProcessFactoryConfigTransforms(
	t *testing.T,
	ctx context.Context,
	session *builtcliacceptance.Session,
	sourcePath string,
	createdPath string,
) {
	t.Helper()
	validateResult, validateErr := session.Run(
		ctx,
		"factory", "config", "validate", createdPath,
	)
	session.RequireSuccess(t, "factory-config-validate", validateResult, validateErr)
	if !strings.Contains(validateResult.Stdout, "Factory validation passed.") {
		t.Fatalf("factory validation stdout = %q", validateResult.Stdout)
	}

	flattenResult, flattenErr := session.Run(
		ctx,
		"factory", "config", "flatten", filepath.Dir(createdPath),
	)
	session.RequireSuccess(t, "factory-config-flatten", flattenResult, flattenErr)
	var flattened map[string]any
	if err := json.Unmarshal([]byte(flattenResult.Stdout), &flattened); err != nil {
		t.Fatalf("flattened Factory output is not JSON: %v\n%s", err, flattenResult.Stdout)
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
	expandResult, expandErr := session.Run(
		ctx,
		"factory", "config", "expand", expandSource,
	)
	session.RequireSuccess(t, "factory-config-expand", expandResult, expandErr)
	if !strings.Contains(expandResult.Stdout, "Expanded factory config into "+expandDir) {
		t.Fatalf("factory expand stdout = %q, want target %q", expandResult.Stdout, expandDir)
	}
	if _, err := os.Stat(filepath.Join(expandDir, "workers", "worker-a", "AGENTS.md")); err != nil {
		t.Fatalf("expanded worker instructions missing: %v", err)
	}
}

func TestRootProcessHarness_WithNoExternalServerReservesUnusedPort(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	flags := session.ServerFlags()
	if len(flags) != 2 || flags[0] != "--server" || flags[1] != session.ServerURL {
		t.Fatalf("server flags = %#v, want --server %q", flags, session.ServerURL)
	}
	if !strings.HasPrefix(session.ServerURL, "http://127.0.0.1:") {
		t.Fatalf("server URL = %q, want local loopback URL", session.ServerURL)
	}
}

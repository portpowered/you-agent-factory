package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

const (
	acceptanceForcedUnwindEnv          = "YOU_FUNCTIONAL_ACCEPTANCE_FORCED_UNWIND"
	acceptanceForcedUnwindReportEnv    = "YOU_FUNCTIONAL_ACCEPTANCE_FORCED_UNWIND_REPORT"
	acceptanceForcedUnwindCloseTimeout = 5 * time.Second
)

type acceptanceForcedUnwindState struct {
	process             support.ApplicationProcess
	rootDir             string
	homeDir             string
	logDir              string
	workDir             string
	configPath          string
	installedFactoryDir string
	reservedServerURL   string
	processClosed       bool
}

type acceptanceForcedUnwindReport struct {
	ProcessClosed          bool   `json:"process_closed"`
	RootDir                string `json:"root_dir,omitempty"`
	RootAbsent             bool   `json:"root_absent"`
	HomeDir                string `json:"home_dir,omitempty"`
	HomeAbsent             bool   `json:"home_absent"`
	LogDir                 string `json:"log_dir,omitempty"`
	LogAbsent              bool   `json:"log_absent"`
	WorkDir                string `json:"work_dir,omitempty"`
	WorkAbsent             bool   `json:"work_absent"`
	ConfigPath             string `json:"config_path,omitempty"`
	ConfigAbsent           bool   `json:"config_absent"`
	InstalledFactoryDir    string `json:"installed_factory_dir,omitempty"`
	InstalledFactoryAbsent bool   `json:"installed_factory_absent"`
	ReservedServerURL      string `json:"reserved_server_url,omitempty"`
	ReservedPortAvailable  bool   `json:"reserved_port_available"`
}

var acceptanceForcedUnwind *acceptanceForcedUnwindState

// TestMain writes the forced-unwind observation after testing has run all
// t.Cleanup callbacks. The report is opt-in so ordinary acceptance runs keep
// their existing output, timing, and top-level test denominator.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := writeAcceptanceForcedUnwindReport(); err != nil {
		fmt.Fprintf(os.Stderr, "write acceptance forced-unwind report: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// failAcceptanceForcedUnwindAfterAssertion acquires the package-owned
// acceptance resources and deliberately fails after their public assertions.
// It is enabled only by the one-shot child characterization command, so it
// does not add a new executable test to the normal acceptance denominator.
func failAcceptanceForcedUnwindAfterAssertion(
	t *testing.T,
	harness *builtcliacceptance.Harness,
	process support.ApplicationProcess,
) {
	t.Helper()
	if os.Getenv(acceptanceForcedUnwindEnv) != "1" {
		return
	}
	if harness == nil || process == nil {
		t.Fatal("acceptance forced-unwind harness or process is unavailable")
	}

	session := harness.NewSession(t).WithNoExternalServer(t)
	_, initialized := initializeConfigWithProcess(t, process, session, "forced-unwind-config-init")
	installedFactoryDir := materializedNamedFactoryDir(t, initialized, packagedGoalFactoryName)
	state := &acceptanceForcedUnwindState{
		process:             process,
		rootDir:             filepath.Dir(session.HomeDir),
		homeDir:             session.HomeDir,
		logDir:              session.LogDir,
		workDir:             session.WorkDir,
		configPath:          initialized.ConfigPath,
		installedFactoryDir: installedFactoryDir,
		reservedServerURL:   session.ServerURL,
	}
	acceptanceForcedUnwind = state
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), acceptanceForcedUnwindCloseTimeout)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("close acceptance forced-unwind process: %v", err)
			return
		}
		state.processClosed = true
	})

	t.Fatal("intentional acceptance forced-unwind characterization failure")
}

func writeAcceptanceForcedUnwindReport() error {
	path := strings.TrimSpace(os.Getenv(acceptanceForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := acceptanceForcedUnwindReport{}
	if state := acceptanceForcedUnwind; state != nil {
		report.ProcessClosed = state.processClosed
		report.RootDir = state.rootDir
		report.RootAbsent = acceptancePathAbsent(state.rootDir)
		report.HomeDir = state.homeDir
		report.HomeAbsent = acceptancePathAbsent(state.homeDir)
		report.LogDir = state.logDir
		report.LogAbsent = acceptancePathAbsent(state.logDir)
		report.WorkDir = state.workDir
		report.WorkAbsent = acceptancePathAbsent(state.workDir)
		report.ConfigPath = state.configPath
		report.ConfigAbsent = acceptancePathAbsent(state.configPath)
		report.InstalledFactoryDir = state.installedFactoryDir
		report.InstalledFactoryAbsent = acceptancePathAbsent(state.installedFactoryDir)
		report.ReservedServerURL = state.reservedServerURL
		report.ReservedPortAvailable = acceptanceReservedPortAvailable(state.reservedServerURL)
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func acceptancePathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func acceptanceReservedPortAvailable(serverURL string) bool {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return false
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	address := net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	return listener.Close() == nil
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

	failAcceptanceForcedUnwindAfterAssertion(t, harness, process)
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

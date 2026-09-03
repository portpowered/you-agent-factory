package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

const (
	regeneratedDeadcodeBaselinePath = "docs/internal/baselines/deadcode-baseline.txt"
	regeneratedUnitBudgetPath       = "docs/internal/baselines/go-unit-lane-latency-budget.v1.json"
	regenerationDeadcodeTool        = "golang.org/x/tools/cmd/deadcode@v0.25.1"
	regenerationRunnerProvider      = "github-actions"
	regenerationRunnerImage         = "ubuntu-24.04"
	regenerationRunnerOS            = "linux"
	regenerationRunnerArchitecture  = "amd64"
)

var (
	runRegenerationDeadcode     = runDeadcodeForRegeneration
	regenerationExecCommand     = exec.Command
	regenerationPositionPattern = regexp.MustCompile(`:(\d+):(\d+):`)
)

type regenerationPaths struct {
	deadcode string
	budget   string
}

func regenerateSharedBaselines(cfg budgetConfig) error {
	paths, err := resolveRegenerationPaths(cfg.root, cfg.budgetPath)
	if err != nil {
		return regenerationError(err)
	}
	if cfg.skipUnitLatency {
		deadcodeReport, err := loadRegenerationDeadcodeReport(cfg.root, cfg.deadcodeReport)
		if err != nil {
			return regenerationError(err)
		}
		if err := publishRegeneratedDeadcode(paths.deadcode, []byte(deadcodeReport)); err != nil {
			return regenerationError(err)
		}
		return nil
	}
	samplePaths, err := splitSamplePaths(cfg.samples)
	if err != nil {
		return regenerationError(err)
	}
	samples, err := loadTimingSamples(samplePaths)
	if err != nil {
		return regenerationError(err)
	}
	if err := validateHostedRegenerationSamples(samples); err != nil {
		return regenerationError(err)
	}

	budget, err := loadLatencyBudget(paths.budget)
	if err != nil {
		return regenerationError(err)
	}
	deadcodeReport, err := loadRegenerationDeadcodeReport(cfg.root, cfg.deadcodeReport)
	if err != nil {
		return regenerationError(err)
	}
	updatedBudget, err := regeneratedBudget(budget, samples[0])
	if err != nil {
		return regenerationError(err)
	}
	budgetData, err := renderRegeneratedBudget(updatedBudget)
	if err != nil {
		return regenerationError(err)
	}
	if err := validateLatencyBudgetDocument(budgetSchemaPath(paths.budget), budgetData); err != nil {
		return regenerationError(fmt.Errorf("generated budget schema validation: %w", err))
	}

	if err := publishRegeneratedBaselines(paths, []byte(deadcodeReport), budgetData); err != nil {
		return regenerationError(err)
	}
	return nil
}

func regenerationError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("shared baseline regeneration failed: %w\nRerun: make regenerate-shared-ci-baselines", err)
}

func resolveRegenerationPaths(root, budgetPath string) (regenerationPaths, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return regenerationPaths{}, fmt.Errorf("resolve repository root %q: %w", root, err)
	}
	expectedBudget := filepath.Join(root, filepath.FromSlash(regeneratedUnitBudgetPath))
	configuredBudget := strings.TrimSpace(budgetPath)
	if configuredBudget == "" {
		return regenerationPaths{}, fmt.Errorf("budget output: expected %s, actual empty", regeneratedUnitBudgetPath)
	}
	if !filepath.IsAbs(configuredBudget) {
		configuredBudget = filepath.Join(root, configuredBudget)
	}
	configuredBudget, err = filepath.Abs(configuredBudget)
	if err != nil {
		return regenerationPaths{}, fmt.Errorf("resolve budget output %q: %w", budgetPath, err)
	}
	if !samePath(configuredBudget, expectedBudget) {
		return regenerationPaths{}, fmt.Errorf("refusing budget output %q; regeneration may write only %s", budgetPath, regeneratedUnitBudgetPath)
	}
	paths := regenerationPaths{
		deadcode: filepath.Join(root, filepath.FromSlash(regeneratedDeadcodeBaselinePath)),
		budget:   configuredBudget,
	}
	for _, output := range []struct {
		name string
		path string
	}{
		{name: "deadcode baseline", path: paths.deadcode},
		{name: "unit budget", path: paths.budget},
	} {
		info, statErr := os.Lstat(output.path)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return regenerationPaths{}, fmt.Errorf("refusing %s symlink %q", output.name, output.path)
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return regenerationPaths{}, fmt.Errorf("inspect %s %q: %w", output.name, output.path, statErr)
		}
	}
	return paths, nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func validateHostedRegenerationSamples(samples []timingSummary) error {
	if _, err := validateBaseline(samples); err != nil {
		return err
	}
	var problems validationProblems
	for index, sample := range samples {
		runner := sample.Run.Runner
		prefix := fmt.Sprintf("sample %d hosted runner", index+1)
		if runner.Provider != regenerationRunnerProvider {
			problems.add("%s provider: expected %q, actual %q", prefix, regenerationRunnerProvider, runner.Provider)
		}
		if runner.Image != regenerationRunnerImage {
			problems.add("%s image: expected %q, actual %q", prefix, regenerationRunnerImage, runner.Image)
		}
		if runner.OS != regenerationRunnerOS {
			problems.add("%s os: expected %q, actual %q", prefix, regenerationRunnerOS, runner.OS)
		}
		if runner.Architecture != regenerationRunnerArchitecture {
			problems.add("%s architecture: expected %q, actual %q", prefix, regenerationRunnerArchitecture, runner.Architecture)
		}
	}
	return problems.err()
}

func regeneratedBudget(budget latencyBudget, sample timingSummary) (latencyBudget, error) {
	packages, tests := inventories(sample)
	if len(packages) == 0 || len(tests) == 0 {
		return latencyBudget{}, fmt.Errorf("reference inventory: expected nonempty package and test inventories")
	}
	updated := budget
	updated.Reference.BaseCommit = sample.Run.Commit
	updated.Reference.RunnerImage = sample.Run.Runner.Image
	updated.Reference.GoVersion = sample.Run.GoVersion
	updated.Reference.UnitDefaultJobs = sample.Run.UnitDefaultJobs
	updated.Reference.ComputedLaneBudget = sample.Run.ComputedLaneBudget
	updated.Reference.PackageInventory = packages
	updated.Reference.TestInventory = tests
	var problems validationProblems
	validateBudgetShape(&problems, updated)
	if err := problems.err(); err != nil {
		return latencyBudget{}, err
	}
	return updated, nil
}

func renderRegeneratedBudget(budget latencyBudget) ([]byte, error) {
	data, err := json.MarshalIndent(budget, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render generated budget: %w", err)
	}
	return append(data, '\n'), nil
}

func loadRegenerationDeadcodeReport(root, reportPath string) (string, error) {
	var (
		report []byte
		err    error
	)
	if strings.TrimSpace(reportPath) != "" {
		path := reportPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		report, err = os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read normalized deadcode report %q: %w", reportPath, err)
		}
	} else {
		var generated string
		generated, err = runRegenerationDeadcode(root)
		if err != nil {
			return "", err
		}
		report = []byte(generated)
	}
	return normalizeRegeneratedDeadcode(string(report)), nil
}

func runDeadcodeForRegeneration(root string) (string, error) {
	command := regenerationExecCommand("go", "run", regenerationDeadcodeTool, "./...")
	command.Dir = root
	command.Env = regenerationDeadcodeEnv()
	var commandOutput bytes.Buffer
	var commandErrors bytes.Buffer
	command.Stdout = &commandOutput
	command.Stderr = &commandErrors
	if err := command.Run(); err != nil {
		diagnostic := strings.TrimSpace(commandErrors.String())
		if diagnostic != "" {
			return "", fmt.Errorf("run deadcode: %w\n%s", err, diagnostic)
		}
		return "", fmt.Errorf("run deadcode: %w", err)
	}
	if commandErrors.Len() > 0 {
		_, _ = io.Copy(stderrWriter, &commandErrors)
	}
	return commandOutput.String(), nil
}

func regenerationDeadcodeEnv() []string {
	env := os.Environ()
	for index, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == "GODEBUG" {
			env[index] = "GODEBUG=" + ensureRegenerationGoTypesAlias(value)
			return env
		}
	}
	return append(env, "GODEBUG=gotypesalias=1")
}

func ensureRegenerationGoTypesAlias(value string) string {
	parts := strings.Split(value, ",")
	flags := make([]string, 0, len(parts)+1)
	found := false
	for _, part := range parts {
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if ok && name == "gotypesalias" {
			flags = append(flags, "gotypesalias=1")
			found = true
			continue
		}
		flags = append(flags, part)
	}
	if !found {
		flags = append(flags, "gotypesalias=1")
	}
	return strings.Join(flags, ",")
}

func normalizeRegeneratedDeadcode(report string) string {
	report = strings.ReplaceAll(report, "\r\n", "\n")
	report = strings.ReplaceAll(report, "\r", "\n")
	report = strings.ReplaceAll(report, "\\", "/")
	report = strings.TrimSpace(report)
	if report == "" {
		return ""
	}
	lines := strings.Split(report, "\n")
	for index, line := range lines {
		lines[index] = regenerationPositionPattern.ReplaceAllString(strings.TrimSpace(line), ":")
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

func publishRegeneratedBaselines(paths regenerationPaths, deadcodeData, budgetData []byte) error {
	currentDeadcode, err := os.ReadFile(paths.deadcode)
	if err != nil {
		return fmt.Errorf("read deadcode baseline before publish: %w", err)
	}
	currentBudget, err := os.ReadFile(paths.budget)
	if err != nil {
		return fmt.Errorf("read unit budget before publish: %w", err)
	}
	changes := []struct {
		name string
		path string
		data []byte
	}{
		{name: "deadcode baseline", path: paths.deadcode, data: deadcodeData},
		{name: "unit budget", path: paths.budget, data: budgetData},
	}
	changed := make([]struct {
		name string
		path string
		data []byte
	}, 0, len(changes))
	for index, candidate := range changes {
		current := currentDeadcode
		if index == 1 {
			current = currentBudget
		}
		if bytes.Equal(current, candidate.data) {
			continue
		}
		changed = append(changed, candidate)
	}
	if len(changed) == 0 {
		fmt.Fprintln(stdoutWriter, "[agent-factory:baselines] shared baselines already match")
		return nil
	}
	for _, candidate := range changed {
		if err := atomicReplaceRegeneratedFile(candidate.path, candidate.data); err != nil {
			return fmt.Errorf("publish %s: %w", candidate.name, err)
		}
	}
	fmt.Fprintf(stdoutWriter, "[agent-factory:baselines] regenerated %d baseline file(s)\n", len(changed))
	return nil
}

func publishRegeneratedDeadcode(path string, data []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read deadcode baseline before publish: %w", err)
	}
	if bytes.Equal(current, data) {
		fmt.Fprintln(stdoutWriter, "[agent-factory:baselines] shared baselines already match")
		return nil
	}
	if err := atomicReplaceRegeneratedFile(path, data); err != nil {
		return fmt.Errorf("publish deadcode baseline: %w", err)
	}
	fmt.Fprintln(stdoutWriter, "[agent-factory:baselines] regenerated 1 baseline file(s)")
	return nil
}

func atomicReplaceRegeneratedFile(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".shared-baseline-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("replace destination: %w (remove existing: %v)", err, removeErr)
		}
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			return fmt.Errorf("rename replacement: %w", retryErr)
		}
	}
	return nil
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRegenerateSharedBaselinesUpdatesOwnedFilesAndIsIdempotent(t *testing.T) {
	fixture, samples := newRegenerationFixture(t)
	for index := range samples {
		samples[index].Run.Commit = strings.Repeat("b", 40)
		samples[index].Packages[1].Tests = append(samples[index].Packages[1].Tests, "TestBeta/new")
		slices.Sort(samples[index].Packages[1].Tests)
		samples[index].TestCount++
	}
	writeRegenerationSamples(t, fixture.samplePaths, samples)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	withRegenerationWriters(t, stdout, stderr)
	originalRunner := runRegenerationDeadcode
	callCount := 0
	runRegenerationDeadcode = func(string) (string, error) {
		callCount++
		return "pkg\\zeta.go:12:4: New finding\r\npkg\\alpha.go:3:2: Old finding\n", nil
	}
	t.Cleanup(func() { runRegenerationDeadcode = originalRunner })

	cfg := regenerationConfig(fixture)
	if err := regenerateSharedBaselines(cfg); err != nil {
		t.Fatalf("first regeneration: %v", err)
	}
	firstDeadcode, err := os.ReadFile(fixture.deadcodePath)
	if err != nil {
		t.Fatalf("read regenerated deadcode baseline: %v", err)
	}
	firstBudget, err := os.ReadFile(fixture.budgetPath)
	if err != nil {
		t.Fatalf("read regenerated unit budget: %v", err)
	}
	if got := string(firstDeadcode); got != "pkg/alpha.go: Old finding\npkg/zeta.go: New finding\n" {
		t.Fatalf("deadcode baseline = %q, want normalized generated report", got)
	}
	updated, err := loadLatencyBudget(fixture.budgetPath)
	if err != nil {
		t.Fatalf("load regenerated unit budget: %v", err)
	}
	packages, tests := inventories(samples[0])
	if !slices.Equal(updated.Reference.PackageInventory, packages) || !slices.Equal(updated.Reference.TestInventory, tests) {
		t.Fatalf("regenerated inventory = %d packages/%d tests, want %d/%d", len(updated.Reference.PackageInventory), len(updated.Reference.TestInventory), len(packages), len(tests))
	}
	if updated.Reference.BaseCommit != strings.Repeat("b", 40) || updated.Reference.RunnerImage != "ubuntu-24.04" || updated.Reference.GoVersion != "go1.25.0" || updated.Reference.UnitDefaultJobs != 2 || updated.Reference.ComputedLaneBudget != 2 {
		t.Fatalf("regenerated identity = %+v, want identity from hosted samples", updated.Reference)
	}
	if !slices.Equal(updated.Reference.Samples, fixture.originalBudget.Reference.Samples) || updated.Reference.MedianWallSeconds != fixture.originalBudget.Reference.MedianWallSeconds || updated.Policy != fixture.originalBudget.Policy {
		t.Fatal("regeneration changed historical timing or policy fields")
	}

	stdout.Reset()
	if err := regenerateSharedBaselines(cfg); err != nil {
		t.Fatalf("second regeneration: %v", err)
	}
	secondDeadcode, _ := os.ReadFile(fixture.deadcodePath)
	secondBudget, _ := os.ReadFile(fixture.budgetPath)
	if !bytes.Equal(firstDeadcode, secondDeadcode) || !bytes.Equal(firstBudget, secondBudget) {
		t.Fatal("second regeneration changed identical baseline inputs")
	}
	if callCount != 2 || !strings.Contains(stdout.String(), "already match") {
		t.Fatalf("second regeneration output = %q, deadcode calls = %d; want idempotent no-diff result", stdout.String(), callCount)
	}
	if stderr.Len() != 0 {
		t.Fatalf("regeneration stderr = %q, want empty", stderr.String())
	}
}

func TestRegenerateSharedBaselinesRejectsInvalidSamplesBeforePublishing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]timingSummary)
		want   string
	}{
		{name: "incomplete", mutate: func(samples []timingSummary) { samples[0].Complete = false }, want: "complete: expected true"},
		{name: "cached", mutate: func(samples []timingSummary) { samples[0].Packages[0].Cache = unitTimingCacheCached }, want: "cache: expected \"executed\""},
		{name: "unknown cache", mutate: func(samples []timingSummary) { samples[0].Packages[0].Cache = unitTimingCacheUnknown }, want: "cache: expected \"executed\""},
		{name: "runner image", mutate: func(samples []timingSummary) {
			for index := range samples {
				samples[index].Run.Runner.Image = "ubuntu-22.04"
			}
		}, want: "hosted runner image"},
		{name: "identity", mutate: func(samples []timingSummary) { samples[1].Run.Commit = strings.Repeat("c", 40) }, want: "identity: expected"},
		{name: "inventory", mutate: func(samples []timingSummary) { samples[1].Packages[0].Tests[0] = "TestDifferent" }, want: "test inventory: expected"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, samples := newRegenerationFixture(t)
			testCase.mutate(samples)
			writeRegenerationSamples(t, fixture.samplePaths, samples)
			originalRunner := runRegenerationDeadcode
			deadcodeCalls := 0
			runRegenerationDeadcode = func(string) (string, error) {
				deadcodeCalls++
				return "new report\n", nil
			}
			t.Cleanup(func() { runRegenerationDeadcode = originalRunner })

			cfg := regenerationConfig(fixture)
			err := regenerateSharedBaselines(cfg)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("regeneration error = %v, want substring %q", err, testCase.want)
			}
			if deadcodeCalls != 0 {
				t.Fatalf("deadcode calls = %d, want validation before generation", deadcodeCalls)
			}
			assertRegenerationFixtureUnchanged(t, fixture)
		})
	}
}

func TestRegenerateSharedBaselinesDeadcodeFailurePreservesBothBaselines(t *testing.T) {
	fixture, samples := newRegenerationFixture(t)
	writeRegenerationSamples(t, fixture.samplePaths, samples)
	originalRunner := runRegenerationDeadcode
	runRegenerationDeadcode = func(string) (string, error) {
		return "", errors.New("deadcode analyzer exited 23")
	}
	t.Cleanup(func() { runRegenerationDeadcode = originalRunner })

	err := regenerateSharedBaselines(regenerationConfig(fixture))
	if err == nil || !strings.Contains(err.Error(), "deadcode analyzer exited 23") {
		t.Fatalf("regeneration error = %v, want deadcode diagnostic", err)
	}
	assertRegenerationFixtureUnchanged(t, fixture)
}

func TestRegenerateSharedBaselinesRejectsOutputOutsideAllowlist(t *testing.T) {
	fixture, samples := newRegenerationFixture(t)
	writeRegenerationSamples(t, fixture.samplePaths, samples)
	originalBudget, err := os.ReadFile(fixture.budgetPath)
	if err != nil {
		t.Fatalf("read fixture budget: %v", err)
	}
	outside := filepath.Join(fixture.root, "outside.json")
	cfg := regenerationConfig(fixture)
	cfg.budgetPath = outside
	if err := regenerateSharedBaselines(cfg); err == nil || !strings.Contains(err.Error(), "may write only") {
		t.Fatalf("regeneration error = %v, want output allowlist failure", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside output stat error = %v, want no output", err)
	}
	currentBudget, _ := os.ReadFile(fixture.budgetPath)
	if !bytes.Equal(originalBudget, currentBudget) {
		t.Fatal("allowlist rejection changed the owned budget")
	}
}

func TestRunDeadcodeForRegenerationUsesPinnedToolAndAliasEnvironment(t *testing.T) {
	originalExec := regenerationExecCommand
	t.Cleanup(func() { regenerationExecCommand = originalExec })
	t.Setenv("GODEBUG", "gocachehash=1,gotypesalias=0")
	t.Setenv("GO_WANT_REGENERATION_DEADCODE_HELPER", "1")
	t.Setenv("REGENERATION_DEADCODE_HELPER_STDOUT", "pkg/foo.go: Example\n")
	var captured *exec.Cmd
	regenerationExecCommand = func(name string, args ...string) *exec.Cmd {
		captured = fakeRegenerationDeadcodeCommand(name, args...)
		return captured
	}

	root := t.TempDir()
	report, err := runDeadcodeForRegeneration(root)
	if err != nil {
		t.Fatalf("runDeadcodeForRegeneration() error = %v", err)
	}
	if report != "pkg/foo.go: Example\n" {
		t.Fatalf("deadcode report = %q, want helper output", report)
	}
	if captured == nil || len(captured.Args) < 5 || captured.Args[len(captured.Args)-4] != "go" || captured.Args[len(captured.Args)-3] != "run" || captured.Args[len(captured.Args)-2] != regenerationDeadcodeTool || captured.Args[len(captured.Args)-1] != "./..." {
		t.Fatalf("deadcode command = %v, want pinned analyzer invocation", captured)
	}
	if captured.Dir != root {
		t.Fatalf("deadcode command directory = %q, want %q", captured.Dir, root)
	}
	if !regenerationEnvContains(captured.Env, "GODEBUG=gocachehash=1,gotypesalias=1") {
		t.Fatalf("deadcode command environment = %v, want gotypesalias enabled", captured.Env)
	}
}

func TestRegenerationDeadcodeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_REGENERATION_DEADCODE_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("REGENERATION_DEADCODE_HELPER_STDOUT"))
	fmt.Fprint(os.Stderr, os.Getenv("REGENERATION_DEADCODE_HELPER_STDERR"))
	os.Exit(0)
}

type regenerationFixture struct {
	root               string
	deadcodePath       string
	budgetPath         string
	samplePaths        []string
	originalDeadcode   []byte
	originalBudget     latencyBudget
	originalBudgetData []byte
}

func newRegenerationFixture(t *testing.T) (regenerationFixture, []timingSummary) {
	t.Helper()
	root := t.TempDir()
	deadcodePath := filepath.Join(root, filepath.FromSlash(regeneratedDeadcodeBaselinePath))
	budgetPath := filepath.Join(root, filepath.FromSlash(regeneratedUnitBudgetPath))
	writeRegenerationSchema(t, budgetPath)
	originalBudget := comparableBudget([]float64{100, 101, 102}, 101)
	originalBudgetData, err := renderRegeneratedBudget(originalBudget)
	if err != nil {
		t.Fatalf("render fixture budget: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(budgetPath), 0o755); err != nil {
		t.Fatalf("create budget directory: %v", err)
	}
	if err := os.WriteFile(budgetPath, originalBudgetData, 0o644); err != nil {
		t.Fatalf("write fixture budget: %v", err)
	}
	originalDeadcode := []byte("pkg/old.go: Old finding\r\n")
	if err := os.MkdirAll(filepath.Dir(deadcodePath), 0o755); err != nil {
		t.Fatalf("create deadcode directory: %v", err)
	}
	if err := os.WriteFile(deadcodePath, originalDeadcode, 0o644); err != nil {
		t.Fatalf("write fixture deadcode baseline: %v", err)
	}
	samplePaths := make([]string, requiredSamples)
	for index := range samplePaths {
		samplePaths[index] = filepath.Join(root, fmt.Sprintf("sample-%d.v2.json", index+1))
	}
	return regenerationFixture{
		root: root, deadcodePath: deadcodePath, budgetPath: budgetPath, samplePaths: samplePaths,
		originalDeadcode: originalDeadcode, originalBudget: originalBudget, originalBudgetData: originalBudgetData,
	}, comparableSamples(10, 10, 10)
}

func writeRegenerationSchema(t *testing.T, budgetPath string) {
	t.Helper()
	source := filepath.Join("..", "..", filepath.FromSlash(latencyBudgetSchemaPath))
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read budget schema: %v", err)
	}
	directory := filepath.Dir(budgetPath)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create budget schema directory: %v", err)
	}
	path := filepath.Join(directory, filepath.Base(filepath.FromSlash(latencyBudgetSchemaPath)))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write budget schema: %v", err)
	}
}

func writeRegenerationSamples(t *testing.T, paths []string, samples []timingSummary) {
	t.Helper()
	for index, sample := range samples {
		data, err := json.MarshalIndent(sample, "", "  ")
		if err != nil {
			t.Fatalf("render sample %d: %v", index+1, err)
		}
		if err := os.WriteFile(paths[index], append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write sample %d: %v", index+1, err)
		}
	}
}

func regenerationConfig(fixture regenerationFixture) budgetConfig {
	return budgetConfig{
		root:       fixture.root,
		budgetPath: regeneratedUnitBudgetPath,
		samples:    strings.Join(fixture.samplePaths, ","),
	}
}

func withRegenerationWriters(t *testing.T, stdout, stderr *bytes.Buffer) {
	t.Helper()
	originalStdout, originalStderr := stdoutWriter, stderrWriter
	stdoutWriter, stderrWriter = stdout, stderr
	t.Cleanup(func() {
		stdoutWriter, stderrWriter = originalStdout, originalStderr
	})
}

func assertRegenerationFixtureUnchanged(t *testing.T, fixture regenerationFixture) {
	t.Helper()
	deadcode, err := os.ReadFile(fixture.deadcodePath)
	if err != nil {
		t.Fatalf("read deadcode baseline after failed regeneration: %v", err)
	}
	if !bytes.Equal(deadcode, fixture.originalDeadcode) {
		t.Fatalf("deadcode after failed regeneration = %q, want original %q", deadcode, fixture.originalDeadcode)
	}
	budget, err := os.ReadFile(fixture.budgetPath)
	if err != nil {
		t.Fatalf("read budget after failed regeneration: %v", err)
	}
	if !bytes.Equal(budget, fixture.originalBudgetData) {
		t.Fatal("unit budget changed after failed regeneration")
	}
}

func fakeRegenerationDeadcodeCommand(name string, args ...string) *exec.Cmd {
	helperArgs := append([]string{"-test.run=TestRegenerationDeadcodeHelperProcess", "--", name}, args...)
	command := exec.Command(os.Args[0], helperArgs...)
	command.Env = regenerationDeadcodeEnv()
	return command
}

func regenerationEnvContains(environment []string, want string) bool {
	for _, entry := range environment {
		if entry == want {
			return true
		}
	}
	return false
}

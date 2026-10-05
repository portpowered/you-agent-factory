package main

import (
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

const modulePath = testlanes.ModulePath

type config struct {
	monolith           bool
	monolithDetails    bool
	monolithPrepare    bool
	monolithPrebuilt   bool
	wiringIntegration  bool
	count              int
	jobs               int
	root               string
	short              bool
	timeout            time.Duration
	vet                bool
	timingOutput       string
	timingCommand      string
	computedLaneBudget int
}

var executeUnitLane = run
var execCommand = exec.Command
var discoverUnitPackages = discoverPackages
var stdoutWriter io.Writer = os.Stdout
var stderrWriter io.Writer = os.Stderr
var exitFunc = os.Exit

// Windows permits a 32,767-character CreateProcess command line. Keep enough
// headroom for the executable and quoting while allowing the current unit
// package inventory to run in one scheduler wave.
const maxGoTestCommandLen = 30000

func main() {
	if err := executeUnitLane(); err != nil {
		fmt.Fprintf(stderrWriter, "%v\n", err)
		exitFunc(1)
	}
}

func run() error {
	cfg := parseConfig()
	if cfg.monolithPrebuilt {
		return runPreparedMonolith(cfg)
	}
	var packages []string
	var err error
	if cfg.wiringIntegration {
		packages, err = discoverWiringPackages(cfg.root)
	} else {
		packages, err = discoverUnitPackages(cfg.root)
	}
	if err != nil {
		return fmt.Errorf("discover unit packages: %w", err)
	}
	if len(packages) == 0 {
		return fmt.Errorf("discover unit packages: no packages found under %s", cfg.root)
	}
	if cfg.monolith || cfg.monolithPrepare {
		return runMonolithUnitTests(cfg, packages)
	}
	if err := runUnitTests(cfg, packages); err != nil {
		return fmt.Errorf("run unit lane: %w", err)
	}
	return nil
}

func parseConfig() config {
	var cfg config
	flag.BoolVar(&cfg.monolith, "monolith", false, "experimental shared Go unit binary; always executes fresh tests and requires Python 3")
	flag.BoolVar(&cfg.monolithDetails, "monolith-details", false, "emit full subtest diagnostics/inventory for the experimental monolith (higher reporting CPU)")
	flag.BoolVar(&cfg.monolithPrepare, "monolith-prepare", false, "prepare all unit binaries and their inventory without executing tests")
	flag.BoolVar(&cfg.monolithPrebuilt, "monolith-prebuilt", false, "execute the explicitly prepared unit suite; skips discovery and build checks, requires re-preparation after changes")
	flag.BoolVar(&cfg.wiringIntegration, "wiring-integration", false, "run retained legacy wiring integration checks instead of unit tests")
	flag.IntVar(&cfg.count, "count", 0, "go test -count value; zero preserves Go's content-addressed test cache")
	flag.IntVar(&cfg.jobs, "jobs", defaultUnitLaneJobs(), "go test -p value")
	flag.StringVar(&cfg.root, "root", "./pkg/...", "go list package pattern for unit test discovery")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.BoolVar(&cfg.vet, "vet", false, "run go test's implicit vet pass")
	flag.StringVar(&cfg.timingOutput, "timing-output", "", "optional path for a deterministic versioned unit package timing summary JSON document")
	flag.StringVar(&cfg.timingCommand, "timing-command", "", "optional command identity to include in the timing summary")
	flag.IntVar(&cfg.computedLaneBudget, "computed-lane-budget", 0, "optional computed lane budget to include in the timing summary")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func discoverPackages(root string) ([]string, error) {
	return discoverPackagesMatching(root, testlanes.IsUnitPackage)
}

func discoverWiringPackages(root string) ([]string, error) {
	return discoverPackagesMatching(root, isWiringIntegrationPackage)
}

func isWiringIntegrationPackage(pkg string) bool {
	lane, owned := testlanes.ForImportPath(pkg)
	return owned && lane == testlanes.LaneIntegration && slices.Contains(strings.Split(pkg, "/"), "wire")
}

func discoverPackagesMatching(root string, include func(string) bool) ([]string, error) {
	rootPath := strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(root)), "/...")
	rootPath = strings.TrimPrefix(rootPath, "./")
	if rootPath == "" || rootPath == "." {
		return nil, fmt.Errorf("unit package root must name a repository directory")
	}
	return discoverPackagesUnderMatching(filepath.FromSlash(rootPath), modulePath+"/"+rootPath, include)
}

func discoverPackagesUnder(rootDir, importPrefix string) ([]string, error) {
	return discoverPackagesUnderMatching(rootDir, importPrefix, testlanes.IsUnitPackage)
}

func discoverPackagesUnderMatching(rootDir, importPrefix string, include func(string) bool) ([]string, error) {
	packageSet := make(map[string]struct{})
	err := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != rootDir && (entry.Name() == "testdata" || entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		buildable, err := build.Default.MatchFile(filepath.Dir(path), entry.Name())
		if err != nil {
			return fmt.Errorf("match build constraints for %s: %w", path, err)
		}
		if !buildable {
			return nil
		}
		relativeDir, err := filepath.Rel(rootDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := importPrefix
		if relativeDir != "." {
			pkg += "/" + filepath.ToSlash(relativeDir)
		}
		if include(pkg) {
			packageSet[pkg] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	slices.Sort(packages)
	return packages, nil
}

func runUnitTests(cfg config, packages []string) error {
	started := time.Now()
	expectedPackages := append([]string(nil), packages...)
	accumulator := newUnitTimingAccumulator(expectedPackages)
	baseArgs := baseGoTestArgs(cfg)
	baseLen := commandArgLen(baseArgs)
	packageArgs := localPackageArguments(packages)
	var laneErr error
	for len(packages) > 0 {
		batch := make([]string, 0, len(packages))
		batchArgs := make([]string, 0, len(packages))
		currentLen := baseLen
		for len(packages) > 0 {
			next := packages[0]
			nextArg := packageArgs[0]
			nextLen := len(nextArg)
			if len(batch) > 0 {
				nextLen++
			}
			if currentLen+nextLen > maxGoTestCommandLen && len(batch) > 0 {
				break
			}
			if currentLen+nextLen > maxGoTestCommandLen {
				return fmt.Errorf("go test command too long for package %q", next)
			}
			batch = append(batch, next)
			batchArgs = append(batchArgs, nextArg)
			currentLen += nextLen
			packages = packages[1:]
			packageArgs = packageArgs[1:]
		}
		capture, err := runGoTest(cfg, batchArgs, batch)
		accumulator.add(capture)
		if err != nil {
			laneErr = err
			break
		}
	}

	run := unitTimingRun{}
	if strings.TrimSpace(cfg.timingOutput) != "" {
		run = unitTimingRunIdentity(cfg)
	}
	summary := accumulator.summaryWithRun(time.Since(started).Seconds(), run)
	if cfg.wiringIntegration {
		summary.Lane = "wiring integration"
	}
	var outputErrs []error
	if err := writeUnitTimingSummary(stdoutWriter, summary); err != nil {
		outputErrs = append(outputErrs, err)
	}
	if err := writeUnitTimingSummaryJSON(cfg.timingOutput, summary); err != nil {
		outputErrs = append(outputErrs, err)
	}
	return errors.Join(laneErr, errors.Join(outputErrs...))
}

func localPackageArguments(packages []string) []string {
	local := make([]string, len(packages))
	for index, pkg := range packages {
		if strings.HasPrefix(pkg, modulePath+"/") {
			local[index] = "." + strings.TrimPrefix(pkg, modulePath)
			continue
		}
		local[index] = pkg
	}
	return local
}

func commandArgLen(args []string) int {
	total := 0
	for i, arg := range args {
		if i > 0 {
			total++
		}
		total += len(arg)
	}
	return total
}

func baseGoTestArgs(cfg config) []string {
	args := []string{"test", fmt.Sprintf("-p=%d", cfg.jobs), "-json"}
	if !cfg.vet {
		args = append(args, "-vet=off")
	}
	if cfg.short {
		args = append(args, "-short")
	}
	return args
}

func runGoTest(cfg config, packages, expectedPackages []string) (unitTimingCapture, error) {
	args := baseGoTestArgs(cfg)
	args = append(args, packages...)
	if cfg.count > 0 {
		args = append(args, fmt.Sprintf("-count=%d", cfg.count))
	}
	args = append(args, fmt.Sprintf("-timeout=%s", cfg.timeout))

	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	cmd.Stderr = stderrWriter
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return unitTimingCapture{}, fmt.Errorf("capture go test JSON output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return unitTimingCapture{}, err
	}
	capture, captureErr := collectUnitTimingCapture(stdout, expectedPackages, stdoutWriter)
	runErr := cmd.Wait()
	return capture, errors.Join(captureErr, runErr)
}

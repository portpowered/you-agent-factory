package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// A prepared suite is an explicit source snapshot, not a Go build-cache hit.
// Execution deliberately does not inspect source files or invoke the compiler.
type preparedMonolithSuite struct {
	Root              string
	WiringIntegration bool
	PreparedAt        time.Time
	Binary            string
	Packages          []string
	Groups            []monolithGroup
	Native            map[string]string
	Identity          unitTimingRun
}

func prepareMonolithSuite(cfg config, packages []string, dir string) error {
	manifest := filepath.Join(dir, "prepared.json")
	if err := os.Remove(manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	binary, groups, excluded, err := prepareMonolithBinary(cfg, packages, dir)
	if err != nil {
		return err
	}
	preparedBinary := filepath.Join(dir, "prepared-"+filepath.Base(binary))
	if err := copyPreparedBinary(binary, preparedBinary); err != nil {
		return err
	}
	native := make(map[string]string, len(excluded))
	for _, pkg := range sortedMonolithKeys(excluded) {
		digest := strings.ReplaceAll(strings.TrimPrefix(pkg, modulePath+"/"), "/", "_")
		path := filepath.Join(dir, digest+".test")
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		args := []string{"test", "-c", fmt.Sprintf("-p=%d", cfg.jobs), "-o=" + path}
		if !cfg.vet {
			args = append(args, "-vet=off")
		}
		args = append(args, localPackageArguments([]string{pkg})...)
		cmd := execCommand("go", args...)
		cmd.Stdout, cmd.Stderr = stderrWriter, stderrWriter
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("prepare native package %s: %w", pkg, err)
		}
		native[pkg] = path
	}
	suite := preparedMonolithSuite{Root: cfg.root, PreparedAt: time.Now().UTC(), Binary: preparedBinary,
		Packages: packages, Groups: groups, Native: native, Identity: unitTimingRunIdentity(cfg), WiringIntegration: cfg.wiringIntegration}
	data, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		return err
	}
	fmt.Fprintf(stdoutWriter, "prepared %d merged packages and %d native binaries; re-prepare after source or build configuration changes\n", len(groups), len(native))
	return nil
}

func sortedMonolithKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func runPreparedMonolith(cfg config) error {
	if cfg.monolithPrepare || cfg.count > 1 {
		return errors.New("prepared execution requires a separate preparation step and supports fresh -count=1 only")
	}
	dir, unlock, err := acquireMonolithWorkspace()
	if err != nil {
		return err
	}
	defer unlock()
	dir, err = monolithScopeWorkspace(cfg, dir)
	if err != nil {
		return err
	}
	var suite preparedMonolithSuite
	if err := readMonolithJSON(filepath.Join(dir, "prepared.json"), &suite); err != nil {
		return fmt.Errorf("read prepared unit suite (run -monolith-prepare first): %w", err)
	}
	if err := validatePreparedMonolith(cfg, suite); err != nil {
		return err
	}
	fmt.Fprintf(stderrWriter, "executing prepared source snapshot from %s; no discovery or build validation\n", suite.PreparedAt.Format(time.RFC3339))
	started := time.Now()
	accumulator := newUnitTimingAccumulator(suite.Packages)
	capture, mergedErr := executeMonolithBinary(cfg, suite.Binary, suite.Groups)
	accumulator.add(capture)
	var nativeErrs []error
	for _, pkg := range sortedMonolithKeys(suite.Native) {
		capture, err := executePreparedNative(cfg, pkg, suite.Native[pkg])
		accumulator.add(capture)
		nativeErrs = append(nativeErrs, err)
	}
	suite.Identity.Command = unitTimingCommand(cfg)
	suite.Identity.EnvironmentInvalidations = append(suite.Identity.EnvironmentInvalidations,
		"explicit prepared source snapshot: "+suite.PreparedAt.Format(time.RFC3339))
	summary := accumulator.summaryWithRun(time.Since(started).Seconds(), suite.Identity)
	if cfg.wiringIntegration {
		summary.Lane = "wiring integration"
	}
	if !cfg.monolithDetails {
		summary.TestInventoryScope = "merged-top-level/native-package"
	}
	return errors.Join(mergedErr, errors.Join(nativeErrs...), writeUnitTimingSummary(stdoutWriter, summary), writeUnitTimingSummaryJSON(cfg.timingOutput, summary))
}

func validatePreparedMonolith(cfg config, suite preparedMonolithSuite) error {
	if suite.Root != cfg.root || suite.WiringIntegration != cfg.wiringIntegration {
		return fmt.Errorf("prepared root %q differs from requested %q; re-prepare", suite.Root, cfg.root)
	}
	if len(suite.Packages) == 0 || suite.PreparedAt.IsZero() {
		return errors.New("prepared suite is empty or incomplete; re-prepare")
	}
	if err := validateMonolithInventory(suite.Packages, suite.Groups, suite.Native); err != nil {
		return err
	}
	paths := append([]string{suite.Binary}, sortedMonolithKeys(suite.Native)...)
	for index, path := range paths {
		if index > 0 {
			path = suite.Native[path]
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("prepared binary unavailable: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("prepared binary is a directory: %s", path)
		}
	}
	return nil
}

func monolithScopeWorkspace(cfg config, dir string) (string, error) {
	if cfg.wiringIntegration {
		dir = filepath.Join(dir, "wiring-integration")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func executePreparedNative(cfg config, pkg, binary string) (unitTimingCapture, error) {
	args := []string{"-test.count=1", "-test.timeout=" + cfg.timeout.String()}
	if cfg.short {
		args = append(args, "-test.short")
	}
	cmd := execCommand(binary, args...)
	cmd.Dir = filepath.FromSlash(strings.TrimPrefix(pkg, modulePath+"/"))
	cmd.Stderr = stderrWriter
	if cfg.monolithDetails {
		args = append([]string{"tool", "test2json", "-t", "-p", pkg, binary, "-test.v=test2json"}, args...)
		cmd = execCommand("go", args...)
		cmd.Dir = filepath.FromSlash(strings.TrimPrefix(pkg, modulePath+"/"))
		cmd.Stderr = stderrWriter
		output, err := cmd.StdoutPipe()
		if err != nil {
			return unitTimingCapture{}, err
		}
		if err := cmd.Start(); err != nil {
			return unitTimingCapture{}, err
		}
		capture, err := collectUnitTimingCapture(output, []string{pkg}, stdoutWriter)
		if !capture.Complete {
			err = errors.Join(err, fmt.Errorf("native package %s did not complete its prepared inventory", pkg))
		}
		waitErr := cmd.Wait()
		reportPreparedNativeCPU(pkg, cmd.ProcessState)
		return capture, errors.Join(err, waitErr)
	}
	cmd.Stdout = stdoutWriter
	started := time.Now()
	err := cmd.Run()
	reportPreparedNativeCPU(pkg, cmd.ProcessState)
	outcome := unitTimingOutcomePass
	if err != nil {
		outcome = unitTimingOutcomeFail
	}
	// Quiet native execution reports package completion, not a subtest inventory.
	// The prepared inventory and native test registration determine what runs.
	return unitTimingCapture{Complete: true, Packages: []unitPackageTiming{{Package: pkg,
		Seconds: time.Since(started).Seconds(), Outcome: outcome, Cache: unitCacheExecuted}}}, err
}

func reportPreparedNativeCPU(pkg string, state *os.ProcessState) {
	if os.Getenv("UNIT_MONOLITH_GROUP_CPU") != "" && state != nil {
		cpu := state.UserTime() + state.SystemTime()
		fmt.Fprintf(stderrWriter, "UNIT_GROUP_CPU {\"package\":%q,\"cpu_seconds\":%.6f}\n", pkg, cpu.Seconds())
	}
}

func copyPreparedBinary(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Close())
}

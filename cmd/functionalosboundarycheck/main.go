// Command functionalosboundarycheck verifies the functional-test OS-boundary
// inventory and its deletion-tolerant per-package spawn baseline.
//
// The checker is deliberately repository-local and read-only. It parses the
// functional Go sources, reconciles direct os/exec launch sites to the
// authored inventory, and never runs a test, binary, or external command.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultBaselinePath  = "docs/internal/baselines/functional-os-spawn-baseline.json"
	defaultInventoryPath = "docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json"
)

type config struct {
	root      string
	baseline  string
	inventory string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.StringVar(&cfg.baseline, "baseline", defaultBaselinePath, "repository-relative OS-spawn baseline")
	flag.StringVar(&cfg.inventory, "inventory", defaultInventoryPath, "repository-relative intentionality inventory")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout, stderr io.Writer) error {
	repoRoot, err := resolveRepositoryRoot(cfg.root)
	if err != nil {
		return err
	}
	baselinePath, err := resolveRepositoryFile(repoRoot, cfg.baseline, defaultBaselinePath)
	if err != nil {
		return fmt.Errorf("resolve baseline: %w", err)
	}
	inventoryPath, err := resolveRepositoryFile(repoRoot, cfg.inventory, defaultInventoryPath)
	if err != nil {
		return fmt.Errorf("resolve inventory: %w", err)
	}

	baseline, err := loadBaseline(baselinePath)
	if err != nil {
		return err
	}
	inventory, err := loadInventory(inventoryPath)
	if err != nil {
		return err
	}
	sites, err := scanFunctionalOSSpawns(repoRoot)
	if err != nil {
		return err
	}
	reconciliation, err := reconcileInventoryWithDiagnostics(sites, inventory)
	if err != nil {
		return reportFindings(stderr, err)
	}
	violations := evaluateBaseline(sites, inventory, baseline)
	if len(violations) > 0 {
		return reportViolations(stderr, violations)
	}

	writeSourceLineDrifts(stdout, reconciliation.sourceLineDrifts)
	writeSuccess(stdout, sites, baseline, inventory)
	return nil
}

func resolveRepositoryRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("stat repository root %s: %w", filepath.ToSlash(absolute), err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository root is not a directory: %s", filepath.ToSlash(absolute))
	}
	return filepath.Clean(absolute), nil
}

func resolveRepositoryFile(repoRoot, requested, defaultPath string) (string, error) {
	if requested == "" {
		requested = defaultPath
	}
	requestedPath := filepath.FromSlash(requested)
	if !filepath.IsAbs(requestedPath) {
		requestedPath = filepath.Join(repoRoot, requestedPath)
	}
	absolute, err := filepath.Abs(requestedPath)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", requested, err)
	}
	relative, err := filepath.Rel(repoRoot, absolute)
	if err != nil {
		return "", fmt.Errorf("check %q is repository-local: %w", requested, err)
	}
	if relative == ".." || len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("%q must be a repository-local path", requested)
	}
	return absolute, nil
}

func reportFindings(stderr io.Writer, err error) error {
	findings := validationFindings(err)
	for _, finding := range findings {
		fmt.Fprintln(stderr, finding)
	}
	fmt.Fprintf(stderr, "LINT_VIOLATION_COUNT: %d\n", len(findings))
	return fmt.Errorf("[agent-factory:functional-os-boundary] inventory reconciliation failed with %d violation(s)", len(findings))
}

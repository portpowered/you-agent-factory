package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// scanBoundaryRepo collects only rules eligible for historical suppression.
func scanBoundaryRepo(cfg config, policy boundaryPolicy) (scanResult, error) {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return scanResult{}, fmt.Errorf("resolve repo root: %w", err)
	}

	scanRoot := filepath.Join(repoRoot, filepath.FromSlash(cfg.packageRoot))
	if isIgnoredRepositoryBoundaryPath(repoRoot, scanRoot) {
		return scanResult{}, nil
	}
	info, err := os.Stat(scanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return scanResult{}, nil
		}
		return scanResult{}, fmt.Errorf("stat scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}
	if !info.IsDir() {
		return scanResult{}, nil
	}

	result := scanResult{}
	if err := scanRetiredPackageRoots(repoRoot, cfg, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryServiceConstruction(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryProductionDefaults(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	return result, nil
}

func scanRetiredPackageRoots(repoRoot string, cfg config, result *scanResult) error {
	for _, retiredRoot := range retiredPackageRoots {
		if !strings.HasPrefix(retiredRoot.packagePath, cfg.packageRoot+"/") {
			continue
		}
		info, statErr := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(retiredRoot.packagePath)))
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return fmt.Errorf("stat retired package root %s: %w", retiredRoot.packagePath, statErr)
		}
		if info.IsDir() {
			result.retiredPackageRootFindings = append(
				result.retiredPackageRootFindings,
				retiredPackageRootFinding{retiredRoot},
			)
		}
	}
	return nil
}

func scanRepositoryServiceConstruction(repoRoot string, result *scanResult) error {
	findings, err := scanProductServiceConstruction(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadServiceConstructionBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.serviceConstructionFindings, result.staleServiceConstructionEntries, err =
		partitionServiceConstructionFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedServiceConstructionFindings = recordedFindingsFromPartition(
		findings,
		result.serviceConstructionFindings,
		func(finding serviceConstructionFinding) string {
			return serviceConstructionKey(finding.filePath, finding.importPath, finding.symbol, finding.class)
		},
	)
	result.serviceConstructionBaselineCount = len(baseline.Entries)
	return nil
}

func scanRepositoryProductionDefaults(repoRoot string, result *scanResult) error {
	findings, err := scanProductionDefaultSelections(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadProductionDefaultBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.productionDefaultFindings, result.staleProductionDefaultEntries, err =
		partitionProductionDefaultFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedProductionDefaultFindings = recordedFindingsFromPartition(
		findings,
		result.productionDefaultFindings,
		func(finding productionDefaultFinding) string {
			return productionDefaultKey(finding.filePath, finding.operation, finding.kind, finding.symbol)
		},
	)
	result.productionDefaultBaselineCount = len(baseline.Entries)
	return nil
}

var repositoryBoundaryIgnoredDirectoryNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"testdata":     {},
	"vendor":       {},
}

// repositoryBoundaryIgnoredRoots contains repository-relative subtrees that
// are policy-owned metadata, generated output, or disposable worktree state.
// Keep artifact/worktree entries root-relative: a tracked production package
// may legitimately contain a directory whose name merely resembles one of
// these transient roots.
var repositoryBoundaryIgnoredRoots = []string{
	".artifacts",
	".claude/worktrees",
	".worktrees",
	"worktrees",
}

func shouldSkipRepositoryWalkDirectory(repoRoot, path string, entry os.DirEntry) bool {
	return entry.IsDir() && isIgnoredRepositoryBoundaryPath(repoRoot, path)
}

func isIgnoredRepositoryBoundaryPath(repoRoot, path string) bool {
	relativePath, err := filepath.Rel(filepath.Clean(repoRoot), filepath.Clean(path))
	if err != nil || relativePath == "." || relativePath == "" {
		return false
	}
	relativePath = filepath.ToSlash(relativePath)
	for _, ignoredRoot := range repositoryBoundaryIgnoredRoots {
		if relativePath == ignoredRoot || strings.HasPrefix(relativePath, ignoredRoot+"/") {
			return true
		}
	}
	for _, directory := range strings.Split(relativePath, "/") {
		if _, ignored := repositoryBoundaryIgnoredDirectoryNames[directory]; ignored {
			return true
		}
	}
	return false
}

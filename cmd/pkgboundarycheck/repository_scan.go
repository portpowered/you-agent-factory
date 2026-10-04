package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	if err := scanRootPackageFamilies(repoRoot, scanRoot, cfg, policy, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryServiceConstruction(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryTransportBoundaries(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryProcessBoundaries(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryProductionDefaults(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryPetriBoundaries(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	sortScanResult(&result)
	return result, nil
}

func scanRootPackageFamilies(
	repoRoot, scanRoot string,
	cfg config,
	policy boundaryPolicy,
	result *scanResult,
) error {
	entries, err := os.ReadDir(scanRoot)
	if err != nil {
		return fmt.Errorf("read scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if isIgnoredRepositoryBoundaryPath(repoRoot, filepath.Join(scanRoot, entry.Name())) {
			continue
		}

		packagePath := filepath.ToSlash(filepath.Join(cfg.packageRoot, entry.Name()))
		if retiredRoot, found := findRetiredPackageRoot(packagePath); found {
			result.retiredPackageRootFindings = append(result.retiredPackageRootFindings, retiredPackageRootFinding{retiredRoot})
			continue
		}
		if isAllowedRootPackageFamily(policy, cfg.packageRoot, packagePath) {
			continue
		}
		result.rootPackageFindings = append(result.rootPackageFindings, rootPackageFinding{packagePath: packagePath})
	}
	for _, retiredRoot := range retiredPackageRoots {
		parent := filepath.ToSlash(filepath.Dir(retiredRoot.packagePath))
		if parent == cfg.packageRoot || !strings.HasPrefix(retiredRoot.packagePath, cfg.packageRoot+"/") {
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

func scanRepositoryTransportBoundaries(repoRoot string, result *scanResult) error {
	var err error
	result.externalImplementationFindings, err = scanConvergedServiceSubpackageImports(repoRoot)
	if err != nil {
		return err
	}
	findings, err := scanTransportBehavior(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadTransportBehaviorBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.transportBehaviorFindings, result.staleTransportBehaviorEntries, err =
		partitionTransportBehaviorFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedTransportBehaviorFindings = recordedFindingsFromPartition(
		findings,
		result.transportBehaviorFindings,
		func(finding transportBehaviorFinding) string {
			return transportBehaviorKey(finding.filePath, finding.kind, finding.symbol)
		},
	)
	result.transportBehaviorBaselineCount = len(baseline.Entries)
	return nil
}

func scanRepositoryProcessBoundaries(repoRoot string, result *scanResult) error {
	var err error
	result.constructedServiceEdgesFindings, err = scanConstructedServiceEdges(repoRoot)
	if err != nil {
		return err
	}
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

func scanRepositoryPetriBoundaries(repoRoot string, result *scanResult) error {
	findings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadPetriPublicSurfaceBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.petriPublicSurfaceFindings, result.stalePetriPublicSurfaceEntries, err =
		partitionPetriPublicSurfaceFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedPetriPublicSurfaceFindings = recordedFindingsFromPartition(
		findings,
		result.petriPublicSurfaceFindings,
		func(finding petriPublicSurfaceFinding) string {
			return petriPublicSurfaceKey(finding.FilePath, finding.Symbol, finding.ImportPath)
		},
	)
	result.petriPublicSurfaceBaselineCount = len(baseline.Entries)
	return nil
}

func sortScanResult(result *scanResult) {
	slices.SortFunc(result.rootPackageFindings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
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

func findRetiredPackageRoot(packagePath string) (retiredPackageRoot, bool) {
	for _, retiredRoot := range retiredPackageRoots {
		if packagePath == retiredRoot.packagePath {
			return retiredRoot, true
		}
	}
	return retiredPackageRoot{}, false
}

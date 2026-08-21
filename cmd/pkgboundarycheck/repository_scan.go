package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func scanRepo(cfg config, policy boundaryPolicy) (scanResult, error) {
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
	if err := scanRepositoryPackageImports(repoRoot, scanRoot, cfg.packageRoot, policy, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryPeerServiceImports(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryTestServiceImports(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositorySupportServiceImports(repoRoot, &result); err != nil {
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
	if err := scanRepositoryTestBehavior(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryProductionDefaults(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryInitializerBehavior(repoRoot, &result); err != nil {
		return scanResult{}, err
	}
	if err := scanRepositoryPetriAndProviderBoundaries(repoRoot, &result); err != nil {
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
		migrationShimFinding, found, err := detectMigrationShimFinding(repoRoot, packagePath)
		if err != nil {
			return err
		}
		if found {
			result.migrationShimFindings = append(result.migrationShimFindings, migrationShimFinding)
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

func scanRepositoryPackageImports(
	repoRoot, scanRoot, packageRoot string,
	policy boundaryPolicy,
	result *scanResult,
) error {
	var err error
	result.applicationGraphImportFindings, err = scanApplicationGraphImports(repoRoot, scanRoot, packageRoot)
	if err != nil {
		return err
	}
	result.retiredPackageImportFindings, err = scanRetiredPackageImports(repoRoot, scanRoot, packageRoot)
	if err != nil {
		return err
	}
	result.handwrittenGeneratedFindings, err = scanHandwrittenGeneratedFiles(repoRoot, policy.generatedCodeExceptions)
	if err != nil {
		return err
	}
	result.domainTransportFindings, err = scanDomainTransportImports(repoRoot, policy.domainTransportExceptions)
	return err
}

func scanRepositoryPeerServiceImports(repoRoot string, result *scanResult) error {
	findings, err := scanPeerServiceImports(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadPeerServiceImportBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.peerServiceImportFindings, result.stalePeerServiceBaselineEntries, err =
		partitionPeerServiceImportFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedPeerServiceImportFindings = recordedFindingsFromPartition(
		findings,
		result.peerServiceImportFindings,
		func(finding peerServiceImportFinding) string {
			return peerServiceImportKey(finding.filePath, finding.importPath, finding.class)
		},
	)
	result.peerServiceBaselineCount = len(baseline.Entries)
	return nil
}

func scanRepositoryTestServiceImports(repoRoot string, result *scanResult) error {
	findings, err := scanTestServiceSubpackageImports(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadTestServiceImportBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.testServiceImportFindings, result.staleTestServiceBaselineEntries, err =
		partitionTestServiceImportFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedTestServiceImportFindings = recordedFindingsFromPartition(
		findings,
		result.testServiceImportFindings,
		func(finding testServiceImportFinding) string {
			return testServiceImportKey(finding.filePath, finding.importPath, finding.class)
		},
	)
	result.testServiceBaselineCount = len(baseline.Entries)
	return nil
}

func scanRepositorySupportServiceImports(repoRoot string, result *scanResult) error {
	findings, err := scanSupportServiceSubpackageImports(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadSupportServiceImportBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.supportServiceImportFindings, result.staleSupportServiceBaselineEntries, err =
		partitionSupportServiceImportFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedSupportServiceImportFindings = recordedFindingsFromPartition(
		findings,
		result.supportServiceImportFindings,
		func(finding supportServiceImportFinding) string {
			return supportServiceImportKey(finding.filePath, finding.importPath, finding.class)
		},
	)
	result.supportServiceBaselineCount = len(baseline.Entries)
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
	result.transportImplementationFindings, err = scanTransportServiceImplementationImports(repoRoot)
	if err != nil {
		return err
	}
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
	result.functionalProcessEdgeFindings, err = scanFunctionalProcessEdges(repoRoot)
	if err != nil {
		return err
	}
	result.constructedServiceEdgesFindings, err = scanConstructedServiceEdges(repoRoot)
	if err != nil {
		return err
	}
	result.testWorkNormalizationFindings, err = scanTestWorkNormalization(repoRoot)
	return err
}

func scanRepositoryTestBehavior(repoRoot string, result *scanResult) error {
	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadTestBehaviorBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.testBehaviorFindings, result.staleTestBehaviorEntries, err =
		partitionTestBehaviorFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedTestBehaviorFindings = recordedFindingsFromPartition(
		findings,
		result.testBehaviorFindings,
		func(finding testBehaviorFinding) string {
			return testBehaviorKey(finding.FilePath, finding.Kind, finding.ImportPath, finding.Symbol)
		},
	)
	result.testBehaviorBaselineCount = len(baseline.Entries)
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

func scanRepositoryInitializerBehavior(repoRoot string, result *scanResult) error {
	findings, err := scanInitializerBehavior(repoRoot)
	if err != nil {
		return err
	}
	baseline, err := loadInitializerBehaviorBaseline(repoRoot)
	if err != nil {
		return err
	}
	result.initializerBehaviorFindings, result.staleInitializerBehaviorEntries, err =
		partitionInitializerBehaviorFindings(findings, baseline)
	if err != nil {
		return err
	}
	result.recordedInitializerBehaviorFindings = recordedFindingsFromPartition(
		findings,
		result.initializerBehaviorFindings,
		func(finding initializerBehaviorFinding) string {
			return initializerBehaviorKey(finding.filePath, finding.kind, finding.symbol)
		},
	)
	result.initializerBehaviorBaselineCount = len(baseline.Entries)
	return nil
}

func scanRepositoryPetriAndProviderBoundaries(repoRoot string, result *scanResult) error {
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
	result.providerEffectOwnershipFindings, err = scanProviderEffectOwnership(repoRoot)
	return err
}

func sortScanResult(result *scanResult) {
	slices.SortFunc(result.rootPackageFindings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	slices.SortFunc(result.migrationShimFindings, func(left, right migrationShimFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	slices.SortFunc(result.retiredPackageImportFindings, func(left, right retiredPackageImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
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

func scanDomainTransportImports(repoRoot string, exceptions []string) ([]domainTransportImportFinding, error) {
	var findings []domainTransportImportFinding
	for _, domainRoot := range protectedTransportIndependentDomainRoots {
		absoluteRoot := filepath.Join(repoRoot, filepath.FromSlash(domainRoot))
		if _, err := os.Stat(absoluteRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat protected domain root %s: %w", domainRoot, err)
		}

		err := filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
				return filepath.SkipDir
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
				return nil
			}
			filePath, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			filePath = filepath.ToSlash(filePath)
			if slices.Contains(exceptions, filePath) {
				return nil
			}
			if isServiceOwnedTransportFile(filePath) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytesContainGeneratedMarker(content) {
				return nil
			}
			parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("parse Factory package imports %s: %w", filePath, err)
			}
			packagePath := filepath.ToSlash(filepath.Dir(filePath))
			for _, importSpec := range parsedFile.Imports {
				importPath, err := strconv.Unquote(importSpec.Path.Value)
				if err == nil && strings.HasPrefix(importPath, transportImportPrefix) {
					findings = append(findings, domainTransportImportFinding{
						packagePath: packagePath,
						importPath:  importPath,
						filePath:    filePath,
						class:       classifyBoundarySource(filePath),
					})
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan protected domain transport imports under %s: %w", domainRoot, err)
		}
	}
	slices.SortFunc(findings, func(left, right domainTransportImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanHandwrittenGeneratedFiles(repoRoot string, exceptions []generatedCodeException) ([]handwrittenGeneratedFinding, error) {
	var findings []handwrittenGeneratedFinding
	for _, exception := range exceptions {
		packageDir := filepath.Join(repoRoot, filepath.FromSlash(exception.packagePath))
		info, err := os.Stat(packageDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat generated-only package %s: %w", exception.packagePath, err)
		}
		if !info.IsDir() {
			continue
		}

		err = filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
				return filepath.SkipDir
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
				return nil
			}
			relativePath, err := filepath.Rel(packageDir, path)
			if err != nil {
				return err
			}
			if exception.scope == generatedCodeExceptionScopeRoot && filepath.Dir(relativePath) != "." {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated-only package file %s: %w", filepath.ToSlash(path), err)
			}
			if ast.IsGenerated(parsedFile) {
				return nil
			}
			filePath, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			findings = append(findings, handwrittenGeneratedFinding{
				filePath:    filepath.ToSlash(filePath),
				packagePath: exception.packagePath,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan generated-only package %s: %w", exception.packagePath, err)
		}
	}
	slices.SortFunc(findings, func(left, right handwrittenGeneratedFinding) int {
		return strings.Compare(left.filePath, right.filePath)
	})
	return findings, nil
}

func findRetiredPackageRoot(packagePath string) (retiredPackageRoot, bool) {
	for _, retiredRoot := range retiredPackageRoots {
		if packagePath == retiredRoot.packagePath {
			return retiredRoot, true
		}
	}
	return retiredPackageRoot{}, false
}

func scanRetiredPackageImports(repoRoot string, scanRoot string, packageRoot string) ([]retiredPackageImportFinding, error) {
	var findings []retiredPackageImportFinding
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read package import file %s: %w", filepath.ToSlash(path), err)
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse package imports %s: %w", filepath.ToSlash(path), err)
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve importing file %s: %w", filepath.ToSlash(path), err)
		}

		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			packagePath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			for _, retiredRoot := range retiredPackageRoots {
				if packagePath != retiredRoot.packagePath && !strings.HasPrefix(packagePath, retiredRoot.packagePath+"/") {
					continue
				}
				findings = append(findings, retiredPackageImportFinding{
					retiredPackageRoot: retiredRoot,
					importPath:         importPath,
					filePath:           filepath.ToSlash(filePath),
					class:              classifyBoundarySource(filepath.ToSlash(filePath)),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s retired package imports: %w", filepath.ToSlash(packageRoot), err)
	}
	return findings, nil
}

func scanApplicationGraphImports(repoRoot string, scanRoot string, packageRoot string) ([]applicationGraphImportFinding, error) {
	var findings []applicationGraphImportFinding
	// The canonical-injector rule applies to every test, including suites outside
	// pkg/. Restricting this walk to packageRoot lets a customer-scale stress or
	// functional test assemble Wire directly. Non-test reusable support remains
	// inventoried separately so its existing exact defects can be migrated.
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		outsidePackageRoot := path != scanRoot && !strings.HasPrefix(path, scanRoot+string(filepath.Separator))
		if outsidePackageRoot && !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read package import file %s: %w", filepath.ToSlash(path), err)
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		if !strings.Contains(string(content), applicationGraphImportPath) {
			return nil
		}

		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse package imports %s: %w", filepath.ToSlash(path), err)
		}
		if !importsApplicationGraph(parsedFile) {
			return nil
		}

		packageDirectory, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("resolve importing package for %s: %w", filepath.ToSlash(path), err)
		}
		packagePath := filepath.ToSlash(packageDirectory)
		if isApprovedApplicationGraphImporter(packagePath) {
			return nil
		}

		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve importing file %s: %w", filepath.ToSlash(path), err)
		}
		findings = append(findings, applicationGraphImportFinding{
			packagePath: packagePath,
			filePath:    filepath.ToSlash(filePath),
			class:       classifyBoundarySource(filepath.ToSlash(filePath)),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s application graph imports: %w", filepath.ToSlash(packageRoot), err)
	}

	slices.SortFunc(findings, func(left, right applicationGraphImportFinding) int {
		return strings.Compare(left.filePath, right.filePath)
	})
	return findings, nil
}

func importsApplicationGraph(parsedFile *ast.File) bool {
	return slices.ContainsFunc(parsedFile.Imports, func(importSpec *ast.ImportSpec) bool {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		return err == nil && (importPath == applicationGraphImportPath ||
			strings.HasPrefix(importPath, applicationGraphImportPath+"/"))
	})
}

func isApprovedApplicationGraphImporter(packagePath string) bool {
	return slices.ContainsFunc(approvedApplicationGraphImporters, func(approvedPath string) bool {
		return packagePath == approvedPath || strings.HasPrefix(packagePath, approvedPath+"/")
	})
}

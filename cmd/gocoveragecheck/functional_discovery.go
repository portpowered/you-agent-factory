package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	functionalDiscoveryParallelThreshold = 8
	functionalDiscoveryMaxJobs           = 4
	functionalGoListErrorFlag            = "-e"
	functionalGoListIdentityJSONFields   = "-json=Dir,ImportPath"
	functionalGoListJSONFields           = "-json=Dir,ImportPath,TestGoFiles,XTestGoFiles"
)

type functionalGoListPackage struct {
	Dir          string
	ImportPath   string
	TestGoFiles  []string
	XTestGoFiles []string
	Error        *functionalGoListPackageError
}

type functionalGoListPackageError struct {
	Pos string
	Err string
}

// discoverFunctionalTestInventory uses go list's build-selected test file
// sets as the source of truth. Parsing those files avoids compiling and
// linking every functional test binary merely to enumerate its top-level
// tests.
func discoverFunctionalTestInventory(packages []string, _ time.Duration, _ bool, jobs int, repoRoot string) (functionalTestInventory, error) {
	requestedPackages := sortedUniqueStrings(packages)
	return discoverFunctionalTestInventoryWithPatternsAndJobs(requestedPackages, requestedPackages, jobs, repoRoot)
}

func discoverFunctionalTestInventoryWithPatterns(listPatterns, packages []string, repoRoot string) (functionalTestInventory, error) {
	return discoverFunctionalTestInventoryWithPatternsAndJobs(listPatterns, packages, defaultCoverageJobs, repoRoot)
}

func discoverFunctionalTestInventoryWithPatternsAndJobs(listPatterns, packages []string, jobs int, repoRoot string) (functionalTestInventory, error) {
	requestedPackages := sortedUniqueStrings(packages)
	if len(requestedPackages) == 0 {
		return functionalTestInventory{}, errors.New("discover functional tests: no packages were selected")
	}

	listedPackages, err := listFunctionalTestPackages(listPatterns, requestedPackages, jobs, repoRoot)
	if err != nil {
		return functionalTestInventory{}, err
	}
	return discoverFunctionalTestInventoryFromListedPackagesWithJobs(requestedPackages, listedPackages, jobs)
}

func discoverFunctionalTestInventoryFromListedPackages(requestedPackages []string, listedPackages []functionalGoListPackage) (functionalTestInventory, error) {
	return discoverFunctionalTestInventoryFromListedPackagesWithJobs(requestedPackages, listedPackages, defaultCoverageJobs)
}

func discoverFunctionalTestInventoryFromListedPackagesWithJobs(requestedPackages []string, listedPackages []functionalGoListPackage, jobs int) (functionalTestInventory, error) {
	requestedPackages = sortedUniqueStrings(requestedPackages)
	if len(requestedPackages) == 0 {
		return functionalTestInventory{}, errors.New("discover functional tests: no packages were selected")
	}

	selectedPackages, err := selectFunctionalPackageSet(requestedPackages, listedPackages)
	if err != nil {
		return functionalTestInventory{}, err
	}

	inventory := functionalTestInventory{
		Packages: make([]string, 0, len(selectedPackages)),
		Tests:    make(map[string][]string, len(selectedPackages)),
	}
	testsByPackage, errorsByPackage := discoverFunctionalPackageTestsInParallel(selectedPackages, jobs)
	for index, pkg := range selectedPackages {
		if err := errorsByPackage[index]; err != nil {
			return functionalTestInventory{}, err
		}
		inventory.Packages = append(inventory.Packages, pkg.ImportPath)
		inventory.Tests[pkg.ImportPath] = testsByPackage[index]
	}
	sort.Strings(inventory.Packages)
	return inventory, nil
}

func discoverFunctionalPackageTestsInParallel(packages []functionalGoListPackage, jobs int) ([][]string, []error) {
	if len(packages) == 0 {
		return nil, nil
	}
	workerCount := min(min(maxFunctionalDiscoveryJobs(jobs), functionalDiscoveryMaxJobs), len(packages))
	testsByPackage := make([][]string, len(packages))
	errorsByPackage := make([]error, len(packages))
	work := make(chan int)
	var waitGroup sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for index := range work {
				testsByPackage[index], errorsByPackage[index] = discoverFunctionalPackageTests(packages[index])
			}
		}()
	}
	for index := range packages {
		work <- index
	}
	close(work)
	waitGroup.Wait()
	return testsByPackage, errorsByPackage
}

func listFunctionalTestPackageMetadata(patterns []string, repoRoot string) ([]functionalGoListPackage, error) {
	patterns = sortedUniqueStrings(patterns)
	if len(patterns) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages matched")
	}

	candidatePaths, usedCurrentTree, err := currentTreeFunctionalPackageCandidates(patterns, repoRoot)
	if err != nil {
		return nil, err
	}
	if usedCurrentTree {
		return listFunctionalTestPackageMetadataForCandidates(candidatePaths, repoRoot)
	}
	return listFunctionalTestPackageMetadataFromPatterns(patterns, repoRoot)
}

func currentTreeFunctionalPackageCandidates(patterns []string, repoRoot string) ([]string, bool, error) {
	if !slices.Equal(patterns, functionalTestPatterns) {
		return nil, false, nil
	}
	root := filepath.Join(repoRoot, "tests", "functional")
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("discover functional tests: inspect current functional tree: %w", err)
	}

	candidates := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("discover functional tests: inspect current functional tree entry %q: %w", filepath.ToSlash(path), walkErr)
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relativeDir, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("discover functional tests: resolve current functional package directory %q: %w", filepath.ToSlash(filepath.Dir(path)), err)
		}
		candidates[modulePath+"/"+filepath.ToSlash(relativeDir)] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, false, nil
	}
	slices.Sort(paths)
	return paths, true, nil
}

func listFunctionalTestPackageMetadataForCandidates(candidatePaths []string, repoRoot string) ([]functionalGoListPackage, error) {
	listedPackages, err := listFunctionalTestPackageBatchWithFieldsAndFlags(
		candidatePaths,
		functionalGoListJSONFields,
		[]string{"-find"},
		repoRoot,
	)
	if err != nil {
		return nil, err
	}
	listedPackages, err = mergeFunctionalGoListPackages(listedPackages)
	if err != nil {
		return nil, err
	}
	runnableCandidates := make([]string, 0, len(candidatePaths))
	for _, candidatePath := range candidatePaths {
		if isFunctionalTestPackage(candidatePath) {
			runnableCandidates = append(runnableCandidates, candidatePath)
		}
	}
	if len(runnableCandidates) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages matched")
	}
	return selectFunctionalPackageSet(runnableCandidates, listedPackages)
}

func listFunctionalTestPackageMetadataFromPatterns(patterns []string, repoRoot string) ([]functionalGoListPackage, error) {
	// Resolve package identities separately from test-file metadata. A wildcard
	// metadata query makes one go list process inspect every test file before it
	// returns. The identity query stays cheap; a single concrete metadata query
	// retains go list's build-selected TestGoFiles/XTestGoFiles authority while
	// letting one go list process share its package-loading cache across the
	// current tree. Splitting this same query into subprocess batches repeats
	// that loading work and increases discovery latency on the coverage runner.
	identityPackages, err := listFunctionalTestPackageBatchWithFieldsAndFlags(
		patterns,
		functionalGoListIdentityJSONFields,
		[]string{"-find"},
		repoRoot,
	)
	if err != nil {
		return nil, err
	}
	functionalPaths := make([]string, 0, len(identityPackages))
	for _, pkg := range identityPackages {
		if isFunctionalTestPackage(pkg.ImportPath) {
			functionalPaths = append(functionalPaths, pkg.ImportPath)
		}
	}
	functionalPaths = sortedUniqueStrings(functionalPaths)
	if len(functionalPaths) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages matched")
	}

	listedPackages, err := listFunctionalTestPackageBatchWithFieldsAndFlags(
		functionalPaths,
		functionalGoListJSONFields,
		[]string{"-find"},
		repoRoot,
	)
	if err != nil {
		return nil, err
	}
	listedPackages, err = mergeFunctionalGoListPackages(listedPackages)
	if err != nil {
		return nil, err
	}
	return selectFunctionalPackageSet(functionalPaths, listedPackages)
}

func resolveFunctionalTestPackagesWithMetadata(cfg config, repoRoot string) ([]string, []functionalGoListPackage, error) {
	if strings.TrimSpace(cfg.packages) != "" {
		return splitList(cfg.packages, " ", true), nil, nil
	}

	listedPackages, err := listFunctionalTestPackageMetadata(functionalTestPatterns, repoRoot)
	if err != nil {
		return nil, nil, err
	}
	packages := make([]string, 0, len(listedPackages))
	for _, pkg := range listedPackages {
		packages = append(packages, pkg.ImportPath)
	}
	slices.Sort(packages)
	return packages, listedPackages, nil
}

func listFunctionalTestPackages(listPatterns, requestedPackages []string, jobs int, repoRoot string) ([]functionalGoListPackage, error) {
	return listFunctionalTestPackagesWithMaxJobs(listPatterns, requestedPackages, jobs, functionalDiscoveryMaxJobs, repoRoot)
}

func listFunctionalTestPackagesWithMaxJobs(listPatterns, requestedPackages []string, jobs, maxJobs int, repoRoot string) ([]functionalGoListPackage, error) {
	patterns := sortedUniqueStrings(listPatterns)
	batches := functionalDiscoveryListBatchesWithMaxJobs(patterns, requestedPackages, jobs, maxJobs)
	if len(batches) == 1 {
		listed, err := listFunctionalTestPackageBatch(batches[0], repoRoot)
		if err != nil {
			return nil, fmt.Errorf("discover functional tests: go list batch %q: %w", strings.Join(batches[0], " "), err)
		}
		return mergeFunctionalGoListPackages(listed)
	}
	results := make([]functionalGoListBatchResult, len(batches))
	var waitGroup sync.WaitGroup
	for index, batch := range batches {
		waitGroup.Add(1)
		go func(index int, batch []string) {
			defer waitGroup.Done()
			listed, err := listFunctionalTestPackageBatch(batch, repoRoot)
			results[index] = functionalGoListBatchResult{packages: listed, err: err, requested: batch}
		}(index, batch)
	}
	waitGroup.Wait()

	listedPackages := make([]functionalGoListPackage, 0)
	for _, result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("discover functional tests: go list batch %q: %w", strings.Join(result.requested, " "), result.err)
		}
		listedPackages = append(listedPackages, result.packages...)
	}
	return mergeFunctionalGoListPackages(listedPackages)
}

type functionalGoListBatchResult struct {
	packages  []functionalGoListPackage
	err       error
	requested []string
}

func functionalDiscoveryListBatches(listPatterns, requestedPackages []string, jobs int) [][]string {
	return functionalDiscoveryListBatchesWithMaxJobs(listPatterns, requestedPackages, jobs, functionalDiscoveryMaxJobs)
}

func functionalDiscoveryListBatchesWithMaxJobs(listPatterns, requestedPackages []string, jobs, maxJobs int) [][]string {
	if len(requestedPackages) < functionalDiscoveryParallelThreshold || !slices.Equal(listPatterns, requestedPackages) {
		return [][]string{listPatterns}
	}
	parallelism := maxFunctionalDiscoveryJobs(jobs)
	if maxJobs < 1 {
		maxJobs = 1
	}
	if parallelism > maxJobs {
		parallelism = maxJobs
	}
	if parallelism > len(requestedPackages) {
		parallelism = len(requestedPackages)
	}
	batchSize := (len(requestedPackages) + parallelism - 1) / parallelism
	batches := make([][]string, 0, parallelism)
	for start := 0; start < len(requestedPackages); start += batchSize {
		end := min(start+batchSize, len(requestedPackages))
		batches = append(batches, append([]string(nil), requestedPackages[start:end]...))
	}
	return batches
}

func listFunctionalTestPackageBatch(packages []string, repoRoot string) ([]functionalGoListPackage, error) {
	return listFunctionalTestPackageBatchWithFields(packages, functionalGoListJSONFields, repoRoot)
}

func listFunctionalTestPackageBatchWithFields(packages []string, jsonFields string, repoRoot string) ([]functionalGoListPackage, error) {
	return listFunctionalTestPackageBatchWithFieldsAndFlags(packages, jsonFields, []string{"-find"}, repoRoot)
}

func listFunctionalTestPackageBatchWithFieldsAndFlags(packages []string, jsonFields string, flags []string, repoRoot string) ([]functionalGoListPackage, error) {
	// The inventory only needs package locations and build-selected test file
	// names. The AST parser below remains responsible for validating the
	// selected source files.
	args := append([]string{"list", functionalGoListErrorFlag, jsonFields}, flags...)
	args = append(args, packages...)
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  repoRoot,
	})
	if err != nil {
		detail := mergeGoTestFailureDetail(stderr, stdout)
		if detail != "" {
			return nil, fmt.Errorf("discover functional tests: go list: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("discover functional tests: go list: %w", err)
	}

	return decodeFunctionalGoListPackages(stdout)
}

func decodeFunctionalGoListPackages(stdout string) ([]functionalGoListPackage, error) {
	decoder := json.NewDecoder(strings.NewReader(stdout))
	packages := make([]functionalGoListPackage, 0)
	for {
		var pkg functionalGoListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				return packages, nil
			}
			return nil, fmt.Errorf("discover functional tests: decode go list package: %w", err)
		}
		if pkg.Error != nil {
			detail := strings.TrimSpace(pkg.Error.Err)
			if detail == "" {
				detail = "package listing failed"
			}
			if position := strings.TrimSpace(pkg.Error.Pos); position != "" {
				return nil, fmt.Errorf("discover functional tests: go list package %q at %s: %s", pkg.ImportPath, position, detail)
			}
			return nil, fmt.Errorf("discover functional tests: go list package %q: %s", pkg.ImportPath, detail)
		}
		if strings.TrimSpace(pkg.ImportPath) == "" {
			return nil, errors.New("discover functional tests: go list returned a package without an import path")
		}
		if strings.TrimSpace(pkg.Dir) == "" {
			return nil, fmt.Errorf("discover functional tests: go list package %q did not include a directory", pkg.ImportPath)
		}
		packages = append(packages, pkg)
	}
}

func mergeFunctionalGoListPackages(packages []functionalGoListPackage) ([]functionalGoListPackage, error) {
	byImportPath := make(map[string]functionalGoListPackage)
	for _, pkg := range packages {
		if previous, exists := byImportPath[pkg.ImportPath]; exists {
			if filepath.Clean(previous.Dir) != filepath.Clean(pkg.Dir) {
				return nil, fmt.Errorf("discover functional tests: go list returned package %q with conflicting directories %q and %q", pkg.ImportPath, previous.Dir, pkg.Dir)
			}
			previous.TestGoFiles = append(previous.TestGoFiles, pkg.TestGoFiles...)
			previous.XTestGoFiles = append(previous.XTestGoFiles, pkg.XTestGoFiles...)
			byImportPath[pkg.ImportPath] = previous
			continue
		}
		byImportPath[pkg.ImportPath] = pkg
	}

	listed := make([]functionalGoListPackage, 0, len(byImportPath))
	for _, pkg := range byImportPath {
		pkg.TestGoFiles = sortedUniqueStrings(pkg.TestGoFiles)
		pkg.XTestGoFiles = sortedUniqueStrings(pkg.XTestGoFiles)
		listed = append(listed, pkg)
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].ImportPath < listed[j].ImportPath })
	return listed, nil
}

func selectFunctionalPackageSet(requested []string, listed []functionalGoListPackage) ([]functionalGoListPackage, error) {
	requestedSet := make(map[string]struct{}, len(requested))
	for _, packagePath := range requested {
		requestedSet[packagePath] = struct{}{}
	}
	listedByImportPath := make(map[string]functionalGoListPackage, len(listed))
	for _, pkg := range listed {
		if _, requested := requestedSet[pkg.ImportPath]; requested {
			listedByImportPath[pkg.ImportPath] = pkg
		}
	}
	selected := make([]functionalGoListPackage, 0, len(requested))
	for _, packagePath := range requested {
		pkg, found := listedByImportPath[packagePath]
		if !found {
			return nil, fmt.Errorf("discover functional tests: go list did not report requested package %q", packagePath)
		}
		selected = append(selected, pkg)
	}
	return selected, nil
}

func discoverFunctionalPackageTests(pkg functionalGoListPackage) ([]string, error) {
	testFiles := append(append([]string(nil), pkg.TestGoFiles...), pkg.XTestGoFiles...)
	testFiles = sortedUniqueStrings(testFiles)
	tests := make([]string, 0)
	for _, listedFile := range testFiles {
		filePath := filepath.Join(pkg.Dir, listedFile)
		source, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("discover functional tests: read package %q file %q: %w", pkg.ImportPath, listedFile, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filePath, source, 0)
		if err != nil {
			return nil, fmt.Errorf("discover functional tests: parse package %q file %q: %w", pkg.ImportPath, listedFile, err)
		}
		tests = append(tests, functionalTestDeclarations(parsed)...)
	}
	return sortedUniqueStrings(tests), nil
}

func functionalTestDeclarations(file *ast.File) []string {
	testingNames := testingImportNames(file)
	tests := make([]string, 0)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || !functionalTestNamePattern.MatchString(function.Name.Name) {
			continue
		}
		if functionalTestSignature(function, testingNames) {
			tests = append(tests, function.Name.Name)
		}
	}
	return tests
}

func testingImportNames(file *ast.File) map[string]struct{} {
	names := make(map[string]struct{})
	for _, importSpec := range file.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil || path != "testing" {
			continue
		}
		if importSpec.Name == nil {
			names["testing"] = struct{}{}
			continue
		}
		if importSpec.Name.Name != "_" {
			names[importSpec.Name.Name] = struct{}{}
		}
	}
	return names
}

func functionalTestSignature(function *ast.FuncDecl, testingNames map[string]struct{}) bool {
	if function.Type.TypeParams != nil || function.Type.Results != nil {
		return false
	}
	if function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	parameter := function.Type.Params.List[0]
	if len(parameter.Names) > 1 {
		return false
	}
	parameterType, ok := unwrapFunctionalParenExpr(parameter.Type).(*ast.StarExpr)
	if !ok {
		return false
	}
	testingType := unwrapFunctionalParenExpr(parameterType.X)
	if selected, ok := testingType.(*ast.SelectorExpr); ok {
		packageName, ok := selected.X.(*ast.Ident)
		if !ok || selected.Sel.Name != "T" {
			return false
		}
		_, imported := testingNames[packageName.Name]
		return imported
	}
	identifier, ok := testingType.(*ast.Ident)
	if !ok || identifier.Name != "T" {
		return false
	}
	_, dotImported := testingNames["."]
	return dotImported
}

func unwrapFunctionalParenExpr(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

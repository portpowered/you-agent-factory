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

type packageNameResolution struct {
	name string
	err  error
}

type importedServiceRoot struct {
	importPath string
	owner      string
}

func scanConvergedServiceSubpackageImports(repoRoot string) ([]transportServiceImplementationFinding, error) {
	packageRoot := filepath.Join(repoRoot, "pkg")
	var findings []transportServiceImplementationFinding
	err := filepath.WalkDir(packageRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		fileFindings, err := scanConvergedServiceSubpackageFile(repoRoot, path)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan converged service subpackage imports: %w", err)
	}
	slices.SortFunc(findings, func(left, right transportServiceImplementationFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanConvergedServiceSubpackageFile(repoRoot, path string) ([]transportServiceImplementationFinding, error) {
	filePath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return nil, err
	}
	filePath = filepath.ToSlash(filePath)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytesContainGeneratedMarker(content) {
		return nil, nil
	}
	parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var findings []transportServiceImplementationFinding
	for _, importSpec := range parsedFile.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}
		repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
		_, isServiceSubpackage := serviceSubpackageOwner(repositoryPath)
		if !isServiceSubpackage {
			continue
		}
		if filePath == "pkg/wire" || strings.HasPrefix(filePath, "pkg/wire/") {
			// Wire is the privileged composition root. Import-direction
			// restrictions for ordinary consumers never apply to it.
			continue
		}
		if isMatchingServiceOwnedTransportConsumer(filePath, repositoryPath) {
			continue
		}
		if isApprovedPeerServiceContractImport(
			filepath.ToSlash(filepath.Dir(filePath)),
			importPath,
		) {
			continue
		}
		_, callerIsService := servicePackageOwner(filePath)
		if callerIsService {
			// Owner-internal imports are allowed here. Peer-service imports
			// are reported by scanPeerServiceImports with baseline support.
			continue
		}

		matchedExplicitPolicy := false
		for privateRoot := range convergedServiceSubpackageRoots {
			if repositoryPath != privateRoot && !strings.HasPrefix(repositoryPath, privateRoot+"/") {
				continue
			}
			matchedExplicitPolicy = true
			findings = append(findings, transportServiceImplementationFinding{
				importPath: importPath,
				filePath:   filePath,
				class:      classifyBoundarySource(filePath),
			})
			break
		}
		if matchedExplicitPolicy {
			continue
		}
		if strings.HasPrefix(filePath, "pkg/transports/") &&
			matchesAnyPackageRoot(repositoryPath, transportPrivateServiceSubpackages) {
			// The transport-specific scanner owns this diagnostic.
			continue
		}
		findings = append(findings, transportServiceImplementationFinding{
			importPath: importPath,
			filePath:   filePath,
			class:      classifyBoundarySource(filePath),
		})
	}
	return findings, nil
}

func scanTestServiceSubpackageImports(repoRoot string) ([]testServiceImportFinding, error) {
	findingsByKey := map[string]testServiceImportFinding{}
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
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		if filePath == "pkg/wire" || strings.HasPrefix(filePath, "pkg/wire/") {
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
			return err
		}
		callerOwner, callerIsService := servicePackageOwner(filePath)
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			importedOwner, isServiceSubpackage := serviceSubpackageOwner(repositoryPath)
			if !isServiceSubpackage {
				continue
			}
			if _, publicTransport := serviceOwnedTransportProtocol(repositoryPath); publicTransport {
				continue
			}
			if callerIsService && callerOwner == importedOwner {
				continue
			}
			if isMatchingServiceOwnedTransportConsumer(filePath, repositoryPath) {
				continue
			}
			if _, publicEffectContract := publicExternalEffectContractImports[importPath]; publicEffectContract {
				continue
			}
			finding := testServiceImportFinding{
				owner:      importedOwner,
				importPath: importPath,
				filePath:   filePath,
				class:      classifyBoundarySource(filePath),
			}
			findingsByKey[testServiceImportKey(filePath, importPath)] = finding
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan test service subpackage imports: %w", err)
	}
	findings := make([]testServiceImportFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, func(left, right testServiceImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanProductServiceConstruction(repoRoot string) ([]serviceConstructionFinding, error) {
	findingsByKey := map[string]serviceConstructionFinding{}
	packageNames := map[string]packageNameResolution{}
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		return scanProductServiceConstructionFile(repoRoot, path, findingsByKey, packageNames)
	})
	if err != nil {
		return nil, fmt.Errorf("scan product service construction: %w", err)
	}
	findings := make([]serviceConstructionFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, func(left, right serviceConstructionFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(left.importPath, right.importPath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.symbol, right.symbol)
	})
	return findings, nil
}

func scanProductServiceConstructionFile(
	repoRoot, path string,
	findingsByKey map[string]serviceConstructionFinding,
	packageNames map[string]packageNameResolution,
) error {
	filePath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return err
	}
	filePath = filepath.ToSlash(filePath)
	if filePath == "pkg/wire" || strings.HasPrefix(filePath, "pkg/wire/") {
		return nil
	}
	callerOwner, callerIsService := servicePackageOwner(filePath)
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytesContainGeneratedMarker(content) {
		return nil
	}
	fset := token.NewFileSet()
	parsedFile, err := parser.ParseFile(fset, path, content, 0)
	if err != nil {
		return err
	}
	importsByName, dotImports, err := serviceConstructionImports(
		repoRoot,
		filePath,
		callerOwner,
		callerIsService,
		parsedFile,
		packageNames,
	)
	if err != nil {
		return err
	}
	recordServiceConstructionExpressions(
		parsedFile,
		fset,
		filePath,
		importsByName,
		dotImports,
		findingsByKey,
	)
	return nil
}

func serviceConstructionImports(
	repoRoot, filePath, callerOwner string,
	callerIsService bool,
	parsedFile *ast.File,
	packageNames map[string]packageNameResolution,
) (map[string]importedServiceRoot, []importedServiceRoot, error) {
	importsByName := map[string]importedServiceRoot{}
	var dotImports []importedServiceRoot
	for _, importSpec := range parsedFile.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}
		owner, servicePackage := serviceRootOwner(importPath)
		if !servicePackage {
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			owner, servicePackage = serviceSubpackageOwner(repositoryPath)
		}
		if !servicePackage {
			continue
		}
		repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
		if isMatchingServiceOwnedTransportConsumer(filePath, repositoryPath) {
			continue
		}
		if callerIsService && callerOwner == owner {
			continue
		}
		name := ""
		if importSpec.Name != nil {
			name = importSpec.Name.Name
		} else {
			name, err = resolveCachedRepositoryImportPackageName(repoRoot, importPath, packageNames)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve package name for %s imported by %s: %w", importPath, filePath, err)
			}
		}
		root := importedServiceRoot{importPath: importPath, owner: owner}
		if name == "." {
			dotImports = append(dotImports, root)
			continue
		}
		if name == "_" {
			continue
		}
		importsByName[name] = root
	}
	return importsByName, dotImports, nil
}

func resolveCachedRepositoryImportPackageName(
	repoRoot, importPath string,
	packageNames map[string]packageNameResolution,
) (string, error) {
	if cached, ok := packageNames[importPath]; ok {
		return cached.name, cached.err
	}
	name, err := resolveRepositoryImportPackageName(repoRoot, importPath)
	packageNames[importPath] = packageNameResolution{name: name, err: err}
	return name, err
}

func recordServiceConstructionExpressions(
	parsedFile *ast.File,
	fset *token.FileSet,
	filePath string,
	importsByName map[string]importedServiceRoot,
	dotImports []importedServiceRoot,
	findingsByKey map[string]serviceConstructionFinding,
) {
	record := func(root importedServiceRoot, symbol string, position token.Pos) {
		class := classifyBoundarySource(filePath)
		key := serviceConstructionKey(filePath, root.importPath, symbol, class)
		finding := findingsByKey[key]
		if finding.count == 0 {
			finding = serviceConstructionFinding{
				owner:      root.owner,
				importPath: root.importPath,
				symbol:     symbol,
				filePath:   filePath,
				line:       fset.Position(position).Line,
				class:      class,
			}
		}
		finding.count++
		findingsByKey[key] = finding
	}
	recordExpression := func(expression ast.Expr) {
		switch selected := expression.(type) {
		case *ast.SelectorExpr:
			identifier, ok := selected.X.(*ast.Ident)
			if !ok {
				return
			}
			root, imported := importsByName[identifier.Name]
			if imported && isProhibitedServiceConstructionSymbol(root.importPath, selected.Sel.Name) {
				record(root, selected.Sel.Name, selected.Sel.Pos())
			}
		case *ast.Ident:
			for _, root := range dotImports {
				if isProhibitedServiceConstructionSymbol(root.importPath, selected.Name) {
					record(root, selected.Name, selected.Pos())
				}
			}
		}
	}
	ast.Inspect(parsedFile, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.CallExpr:
			recordExpression(statement.Fun)
			for _, argument := range statement.Args {
				recordExpression(argument)
			}
		case *ast.ValueSpec:
			for _, value := range statement.Values {
				recordExpression(value)
			}
		case *ast.AssignStmt:
			for _, value := range statement.Rhs {
				recordExpression(value)
			}
		case *ast.ReturnStmt:
			for _, value := range statement.Results {
				recordExpression(value)
			}
		}
		return true
	})
}

func resolveRepositoryImportPackageName(repoRoot, importPath string) (string, error) {
	if !strings.HasPrefix(importPath, repositoryImportPrefix) {
		return filepath.Base(importPath), nil
	}
	relativePath := strings.TrimPrefix(importPath, repositoryImportPrefix)
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return "", fmt.Errorf("read package directory %s: %w", filepath.ToSlash(relativePath), err)
	}
	var packageName string
	for _, entry := range entries {
		if entry.IsDir() ||
			filepath.Ext(entry.Name()) != ".go" ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(packageDir, entry.Name())
		parsed, parseErr := parser.ParseFile(
			token.NewFileSet(),
			path,
			nil,
			parser.PackageClauseOnly,
		)
		if parseErr != nil {
			return "", fmt.Errorf("parse package clause in %s: %w", filepath.ToSlash(path), parseErr)
		}
		if packageName == "" {
			packageName = parsed.Name.Name
			continue
		}
		if parsed.Name.Name != packageName {
			return "", fmt.Errorf(
				"package directory %s declares both %q and %q",
				filepath.ToSlash(relativePath),
				packageName,
				parsed.Name.Name,
			)
		}
	}
	if packageName == "" {
		return "", fmt.Errorf("package directory %s has no non-test Go package clause", filepath.ToSlash(relativePath))
	}
	return packageName, nil
}

func serviceRootOwner(importPath string) (string, bool) {
	const prefix = repositoryImportPrefix + "pkg/services/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	owner := strings.TrimPrefix(importPath, prefix)
	if owner == "" || strings.Contains(owner, "/") {
		return "", false
	}
	return owner, true
}

func isProhibitedServiceConstructionSymbol(importPath, symbol string) bool {
	constructionShaped := false
	for _, prefix := range serviceConstructionPrefixes {
		if symbol == prefix ||
			(strings.HasPrefix(symbol, prefix) &&
				len(symbol) > len(prefix) &&
				symbol[len(prefix)] >= 'A' &&
				symbol[len(prefix)] <= 'Z') {
			constructionShaped = true
			break
		}
	}
	if !constructionShaped {
		return false
	}
	allowedSymbols := allowedServiceValueConstructionSymbols[importPath]
	_, allowed := allowedSymbols[symbol]
	return !allowed
}

func serviceSubpackageOwner(repositoryPath string) (string, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(repositoryPath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(repositoryPath, prefix), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0], true
}

func servicePackageOwner(filePath string) (string, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(filePath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(filePath, prefix), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

func serviceOwnedTransportProtocol(repositoryPath string) (string, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(repositoryPath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(repositoryPath, prefix), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "transports" || parts[2] == "" {
		return "", false
	}
	return parts[2], true
}

func isServiceOwnedTransportFile(filePath string) bool {
	packagePath := filepath.ToSlash(filepath.Dir(filePath))
	_, ok := serviceOwnedTransportProtocol(packagePath)
	return ok
}

func isMatchingServiceOwnedTransportConsumer(filePath, importedRepositoryPath string) bool {
	protocol, ok := serviceOwnedTransportProtocol(importedRepositoryPath)
	if !ok {
		return false
	}
	consumerRoot := "pkg/transports/" + protocol
	return filePath == consumerRoot || strings.HasPrefix(filePath, consumerRoot+"/")
}

func matchesAnyPackageRoot(repositoryPath string, roots []string) bool {
	for _, root := range roots {
		if repositoryPath == root || strings.HasPrefix(repositoryPath, root+"/") {
			return true
		}
	}
	return false
}

func scanTransportServiceImplementationImports(repoRoot string) ([]transportServiceImplementationFinding, error) {
	transportRoot := filepath.Join(repoRoot, "pkg", "transports")
	var findings []transportServiceImplementationFinding
	err := filepath.WalkDir(transportRoot, func(path string, entry os.DirEntry, walkErr error) error {
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
			return err
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			for _, privateRoot := range transportPrivateServiceSubpackages {
				if repositoryPath == privateRoot || strings.HasPrefix(repositoryPath, privateRoot+"/") {
					findings = append(findings, transportServiceImplementationFinding{
						importPath: importPath,
						filePath:   filePath,
						class:      classifyBoundarySource(filePath),
					})
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan transport service implementation imports: %w", err)
	}
	slices.SortFunc(findings, func(left, right transportServiceImplementationFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanPeerServiceImports(repoRoot string) ([]peerServiceImportFinding, error) {
	servicesRoot := filepath.Join(repoRoot, "pkg", "services")
	var findings []peerServiceImportFinding
	err := filepath.WalkDir(servicesRoot, func(path string, entry os.DirEntry, walkErr error) error {
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
		parts := strings.Split(filePath, "/")
		if len(parts) < 4 {
			return nil
		}
		owner := parts[2]
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			servicePrefix := repositoryImportPrefix + "pkg/services/"
			if !strings.HasPrefix(importPath, servicePrefix) {
				continue
			}
			servicePath := strings.TrimPrefix(importPath, servicePrefix)
			serviceParts := strings.Split(servicePath, "/")
			if len(serviceParts) < 2 || serviceParts[0] == owner {
				continue
			}
			if isApprovedPeerServiceContractImport(filepath.ToSlash(filepath.Dir(filePath)), importPath) {
				continue
			}
			findings = append(findings, peerServiceImportFinding{
				owner: owner, peer: serviceParts[0], importPath: importPath, filePath: filePath,
				class: classifyBoundarySource(filePath),
			})
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan peer service implementation imports: %w", err)
	}
	slices.SortFunc(findings, func(left, right peerServiceImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

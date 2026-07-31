package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	// providersLeafEffectContractPackage is the durable Providers Execution
	// leaf that owns the provider inference/process effect contract. Later
	// Providers migration packets move the live Workers path here; this packet
	// only encodes ownership for the checker and fixtures.
	providersLeafEffectContractPackage = "pkg/services/providers/execution/inferencecontract"
	providersServiceRootPrefix         = "pkg/services/providers/"
	providersLeafEffectContractImport  = repositoryImportPrefix + providersLeafEffectContractPackage

	// workersProviderEffectMigrationDebtPackage remains the live declaration
	// site until Providers packets land. It is not the durable normative owner.
	workersProviderEffectMigrationDebtPackage = "pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"

	// workersProviderMigrationDebtPrefix hosts the absorbed Standardized
	// Providers catalog/registry/execution surfaces until Providers packets
	// land. Competing forks outside this prefix and Providers are rejected.
	workersProviderMigrationDebtPrefix = "pkg/services/providers/internal/services/execution/internal/provider/"

	edgesPackagePath = "pkg/services/edges"

	providerEffectPortTypeName = "Provider"
	providerEffectMethodName   = "Infer"

	providerEffectSharedSourceOfTruthRemediation = "enumeration and one-attempt execution share one Providers-owned source of truth; absorb the Standardized Providers protocol, registry, open-config, and testkit model and do not invent a second Providers catalog, registry, conductor, or execution-contract family."
)

var competingProviderAbstractionPackageNames = []string{
	"catalog",
	"registry",
	"conductor",
	"execution",
}

var competingProviderAbstractionTypeNames = map[string]struct{}{
	"Catalog":   {},
	"Registry":  {},
	"Conductor": {},
	"Provider":  {},
}

type providerEffectOwnershipFinding struct {
	kind        string
	packagePath string
	filePath    string
	typeName    string
}

func scanProviderEffectOwnership(repoRoot string) ([]providerEffectOwnershipFinding, error) {
	scanRoot := filepath.Join(repoRoot, "pkg")
	if _, err := os.Stat(scanRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat provider-effect scan root: %w", err)
	}

	var findings []providerEffectOwnershipFinding
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		fileFindings, err := scanProviderEffectOwnershipFile(repoRoot, path, entry, walkErr)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan provider-effect ownership: %w", err)
	}

	slices.SortFunc(findings, func(left, right providerEffectOwnershipFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(left.kind, right.kind); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.typeName, right.typeName)
	})
	return findings, nil
}

func scanProviderEffectOwnershipFile(
	repoRoot string,
	path string,
	entry os.DirEntry,
	walkErr error,
) ([]providerEffectOwnershipFinding, error) {
	if walkErr != nil {
		return nil, walkErr
	}
	if entry.IsDir() {
		if entry.Name() == "testdata" || entry.Name() == "vendor" {
			return nil, filepath.SkipDir
		}
		return nil, nil
	}
	if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
		return nil, nil
	}
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return nil, err
	}
	relative = filepath.ToSlash(relative)
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse provider-effect ownership file %s: %w", relative, err)
	}
	return providerEffectOwnershipFindingsForFile(
		filepath.ToSlash(filepath.Dir(relative)),
		relative,
		file,
	), nil
}

func providerEffectOwnershipFindingsForFile(
	packagePath string,
	filePath string,
	file *ast.File,
) []providerEffectOwnershipFinding {
	imports := importedPackagePaths(file)
	var findings []providerEffectOwnershipFinding
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, specification := range generic.Specs {
			typed, ok := specification.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if finding, hit := providerEffectOwnershipFindingForType(
				packagePath,
				filePath,
				typed,
				imports,
			); hit {
				findings = append(findings, finding)
			}
		}
	}
	return findings
}

func providerEffectOwnershipFindingForType(
	packagePath string,
	filePath string,
	typed *ast.TypeSpec,
	imports map[string]string,
) (providerEffectOwnershipFinding, bool) {
	if packagePath == edgesPackagePath {
		return edgesProviderEffectRedefinition(packagePath, filePath, typed, imports)
	}
	if finding, hit := competingProviderCatalogOrExecutionAbstraction(packagePath, filePath, typed); hit {
		return finding, true
	}
	if isDurableProvidersLeafOwner(packagePath) ||
		packagePath == workersProviderEffectMigrationDebtPackage {
		return providerEffectOwnershipFinding{}, false
	}
	if !isProviderEffectPortDeclaration(typed, imports) &&
		!structFieldsRedefineProviderEffect(typed, imports) {
		return providerEffectOwnershipFinding{}, false
	}
	return providerEffectOwnershipFinding{
		kind:        "durable-owner",
		packagePath: packagePath,
		filePath:    filePath,
		typeName:    typed.Name.Name,
	}, true
}

func isDurableProvidersLeafOwner(packagePath string) bool {
	return packagePath == providersLeafEffectContractPackage
}

func isProvidersServicePackage(packagePath string) bool {
	return packagePath == strings.TrimSuffix(providersServiceRootPrefix, "/") ||
		strings.HasPrefix(packagePath, providersServiceRootPrefix)
}

func isProviderEffectPortDeclaration(typed *ast.TypeSpec, imports map[string]string) bool {
	return declaresProviderEffectMethod(typed, imports) ||
		redefinesProvidersLeafEffectContract(typed, imports)
}

func edgesProviderEffectRedefinition(
	packagePath string,
	filePath string,
	typed *ast.TypeSpec,
	imports map[string]string,
) (providerEffectOwnershipFinding, bool) {
	if typed.Name == nil {
		return providerEffectOwnershipFinding{}, false
	}
	if isProviderEffectPortDeclaration(typed, imports) {
		return providerEffectOwnershipFinding{
			kind:        "edges-redefinition",
			packagePath: packagePath,
			filePath:    filePath,
			typeName:    typed.Name.Name,
		}, true
	}
	if structFieldsRedefineProviderEffect(typed, imports) {
		return providerEffectOwnershipFinding{
			kind:        "edges-redefinition",
			packagePath: packagePath,
			filePath:    filePath,
			typeName:    typed.Name.Name,
		}, true
	}
	return providerEffectOwnershipFinding{}, false
}

func declaresProviderEffectMethod(typed *ast.TypeSpec, imports map[string]string) bool {
	interfaceType, ok := typed.Type.(*ast.InterfaceType)
	if !ok {
		return false
	}
	return interfaceDeclaresProviderEffectMethod(interfaceType, imports)
}

func interfaceDeclaresProviderEffectMethod(
	interfaceType *ast.InterfaceType,
	imports map[string]string,
) bool {
	if interfaceType.Methods == nil {
		return false
	}
	for _, method := range interfaceType.Methods.List {
		for _, name := range method.Names {
			signature, ok := method.Type.(*ast.FuncType)
			if name.Name == providerEffectMethodName &&
				ok &&
				isProviderEffectMethodSignature(signature, imports) {
				return true
			}
		}
	}
	return false
}

func structFieldsRedefineProviderEffect(
	typed *ast.TypeSpec,
	imports map[string]string,
) bool {
	structure, ok := typed.Type.(*ast.StructType)
	if !ok || structure.Fields == nil {
		return false
	}
	for _, field := range structure.Fields.List {
		if referencesProvidersLeafEffectContract(field.Type, imports) {
			continue
		}
		if typeExpressionRedefinesProviderEffect(field.Type, imports) {
			return true
		}
	}
	return false
}

func typeExpressionRedefinesProviderEffect(expression ast.Expr, imports map[string]string) bool {
	redefines := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if redefines || node == nil {
			return false
		}
		typed, ok := node.(ast.Expr)
		if ok && referencesProvidersLeafEffectContract(typed, imports) {
			redefines = true
			return false
		}
		interfaceType, ok := node.(*ast.InterfaceType)
		if ok && interfaceDeclaresProviderEffectMethod(interfaceType, imports) {
			redefines = true
			return false
		}
		return true
	})
	return redefines
}

func isProviderEffectMethodSignature(signature *ast.FuncType, imports map[string]string) bool {
	if fieldCount(signature.Params) != 2 || fieldCount(signature.Results) != 2 {
		return false
	}
	if !isStandardContextType(signature.Params.List[0].Type, imports) {
		return false
	}
	errorResult, ok := signature.Results.List[len(signature.Results.List)-1].Type.(*ast.Ident)
	return ok && errorResult.Name == "error"
}

func isStandardContextType(expression ast.Expr, imports map[string]string) bool {
	switch typed := expression.(type) {
	case *ast.ParenExpr:
		return isStandardContextType(typed.X, imports)
	case *ast.Ident:
		return typed.Name == "Context" && imports["."] == "context"
	case *ast.SelectorExpr:
		if typed.Sel == nil || typed.Sel.Name != "Context" {
			return false
		}
		packageName, ok := typed.X.(*ast.Ident)
		return ok && imports[packageName.Name] == "context"
	default:
		return false
	}
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}

func redefinesProvidersLeafEffectContract(typed *ast.TypeSpec, imports map[string]string) bool {
	if typed.Name == nil {
		return false
	}
	if referencesProvidersLeafEffectContract(typed.Type, imports) {
		return true
	}
	interfaceType, ok := typed.Type.(*ast.InterfaceType)
	if !ok || interfaceType.Methods == nil {
		return false
	}
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) == 0 &&
			referencesProvidersLeafEffectContract(method.Type, imports) {
			return true
		}
	}
	return false
}

func referencesProvidersLeafEffectContract(expression ast.Expr, imports map[string]string) bool {
	parenthesized, ok := expression.(*ast.ParenExpr)
	if ok {
		return referencesProvidersLeafEffectContract(parenthesized.X, imports)
	}
	identifier, ok := expression.(*ast.Ident)
	if ok {
		return identifier.Name == providerEffectPortTypeName &&
			imports["."] == providersLeafEffectContractImport
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != providerEffectPortTypeName {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return imports[packageName.Name] == providersLeafEffectContractImport
}

func importedPackagePaths(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		packageName := pathpkg.Base(importPath)
		if specification.Name != nil {
			packageName = specification.Name.Name
		}
		if packageName == "_" {
			continue
		}
		imports[packageName] = importPath
	}
	return imports
}

func competingProviderCatalogOrExecutionAbstraction(
	packagePath string,
	filePath string,
	typed *ast.TypeSpec,
) (providerEffectOwnershipFinding, bool) {
	if typed.Name == nil {
		return providerEffectOwnershipFinding{}, false
	}
	if isProvidersServicePackage(packagePath) || isAbsorbedWorkersProviderSurface(packagePath) {
		return providerEffectOwnershipFinding{}, false
	}
	if !isCompetingProviderAbstractionPackage(packagePath) {
		return providerEffectOwnershipFinding{}, false
	}
	if _, named := competingProviderAbstractionTypeNames[typed.Name.Name]; !named {
		return providerEffectOwnershipFinding{}, false
	}
	switch typed.Type.(type) {
	case *ast.InterfaceType, *ast.StructType:
		return providerEffectOwnershipFinding{
			kind:        "competing-catalog-or-execution",
			packagePath: packagePath,
			filePath:    filePath,
			typeName:    typed.Name.Name,
		}, true
	default:
		if typed.Assign.IsValid() {
			return providerEffectOwnershipFinding{
				kind:        "competing-catalog-or-execution",
				packagePath: packagePath,
				filePath:    filePath,
				typeName:    typed.Name.Name,
			}, true
		}
		return providerEffectOwnershipFinding{}, false
	}
}

func isAbsorbedWorkersProviderSurface(packagePath string) bool {
	return packagePath == strings.TrimSuffix(workersProviderMigrationDebtPrefix, "/") ||
		strings.HasPrefix(packagePath, workersProviderMigrationDebtPrefix)
}

func isCompetingProviderAbstractionPackage(packagePath string) bool {
	components := strings.Split(strings.Trim(packagePath, "/"), "/")
	for index, component := range components {
		for _, abstraction := range competingProviderAbstractionPackageNames {
			if component == "provider"+abstraction {
				return true
			}
			if (component == "provider" || component == "providers") &&
				index+1 < len(components) &&
				components[index+1] == abstraction {
				return true
			}
		}
	}
	return false
}

func writeProviderEffectOwnershipFindings(writer io.Writer, findings []providerEffectOwnershipFinding) {
	for _, finding := range findings {
		switch finding.kind {
		case "durable-owner":
			fmt.Fprintf(
				writer,
				"[agent-factory:pkg-boundary] prohibited durable provider-effect ownership: %s (%s)\n",
				finding.packagePath,
				finding.filePath,
			)
			fmt.Fprintf(
				writer,
				"  reason: %s declares %s as a durable provider inference/process effect port outside the Providers Execution leaf.\n",
				finding.packagePath,
				finding.typeName,
			)
			fmt.Fprintf(
				writer,
				"  canonical owner: %s\n",
				providersLeafEffectContractPackage,
			)
			fmt.Fprintf(
				writer,
				"  remediation: declare the leaf effect contract in the Providers Execution leaf and keep Workers consuming the Providers root; do not redeclare or alias the port as a peer-owned contract. %s\n",
				providerEffectSharedSourceOfTruthRemediation,
			)
		case "edges-redefinition":
			fmt.Fprintf(
				writer,
				"[agent-factory:pkg-boundary] prohibited provider-effect contract redefinition: %s (%s)\n",
				finding.packagePath,
				finding.filePath,
			)
			fmt.Fprintf(
				writer,
				"  reason: pkg/services/edges declares %s instead of aggregating the exact Providers leaf effect contract unchanged.\n",
				finding.typeName,
			)
			fmt.Fprintf(
				writer,
				"  canonical owner: %s\n",
				providersLeafEffectContractPackage,
			)
			fmt.Fprintf(
				writer,
				"  remediation: aggregate the exact Providers leaf effect contract unchanged as the root/test override bag; do not own, redefine, or alias it in edges. %s\n",
				providerEffectSharedSourceOfTruthRemediation,
			)
		case "competing-catalog-or-execution":
			fmt.Fprintf(
				writer,
				"[agent-factory:pkg-boundary] prohibited competing provider catalog or execution abstraction: %s (%s)\n",
				finding.packagePath,
				finding.filePath,
			)
			fmt.Fprintf(
				writer,
				"  reason: %s declares %s as a parallel provider catalog, registry, conductor, or execution-contract family beside the absorbed Standardized Providers model.\n",
				finding.packagePath,
				finding.typeName,
			)
			fmt.Fprintf(
				writer,
				"  canonical owner: Providers-owned Standardized Providers catalog/execution truth (leaf effects at %s)\n",
				providersLeafEffectContractPackage,
			)
			fmt.Fprintf(
				writer,
				"  remediation: %s\n",
				providerEffectSharedSourceOfTruthRemediation,
			)
		default:
			fmt.Fprintf(
				writer,
				"[agent-factory:pkg-boundary] prohibited provider-effect ownership finding: %s (%s)\n",
				finding.packagePath,
				finding.filePath,
			)
		}
	}
}


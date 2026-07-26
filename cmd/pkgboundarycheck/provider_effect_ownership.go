package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	workersProviderEffectMigrationDebtPackage = "pkg/services/workers/provider/inferencecontract"

	// workersProviderMigrationDebtPrefix hosts the absorbed Standardized
	// Providers catalog/registry/execution surfaces until Providers packets
	// land. Competing forks outside this prefix and Providers are rejected.
	workersProviderMigrationDebtPrefix = "pkg/services/workers/provider/"

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
	var findings []providerEffectOwnershipFinding
	scanRoot := filepath.Join(repoRoot, "pkg")
	if _, err := os.Stat(scanRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat provider-effect scan root: %w", err)
	}

	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		packagePath := filepath.ToSlash(filepath.Dir(relative))

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse provider-effect ownership file %s: %w", relative, err)
		}

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
				if packagePath == edgesPackagePath {
					if finding, hit := edgesProviderEffectRedefinition(packagePath, relative, typed); hit {
						findings = append(findings, finding)
					}
					continue
				}
				if finding, hit := competingProviderCatalogOrExecutionAbstraction(packagePath, relative, typed); hit {
					findings = append(findings, finding)
					continue
				}
				if !isProviderEffectPortDeclaration(typed) {
					continue
				}
				if isDurableProvidersLeafOwner(packagePath) {
					continue
				}
				if packagePath == workersProviderEffectMigrationDebtPackage {
					// Live Workers declaration is migration debt only.
					continue
				}
				findings = append(findings, providerEffectOwnershipFinding{
					kind:        "durable-owner",
					packagePath: packagePath,
					filePath:    relative,
					typeName:    typed.Name.Name,
				})
			}
		}
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

func isDurableProvidersLeafOwner(packagePath string) bool {
	return packagePath == providersLeafEffectContractPackage
}

func isProvidersServicePackage(packagePath string) bool {
	return packagePath == strings.TrimSuffix(providersServiceRootPrefix, "/") ||
		strings.HasPrefix(packagePath, providersServiceRootPrefix)
}

func isProviderEffectPortDeclaration(typed *ast.TypeSpec) bool {
	if typed.Name == nil || typed.Name.Name != providerEffectPortTypeName {
		return false
	}
	switch typed.Type.(type) {
	case *ast.InterfaceType:
		return true
	default:
		return typed.Assign.IsValid()
	}
}

func edgesProviderEffectRedefinition(
	packagePath string,
	filePath string,
	typed *ast.TypeSpec,
) (providerEffectOwnershipFinding, bool) {
	if typed.Name == nil {
		return providerEffectOwnershipFinding{}, false
	}
	if typed.Name.Name == "Edges" {
		return providerEffectOwnershipFinding{}, false
	}
	if isProviderEffectPortDeclaration(typed) ||
		declaresProviderEffectMethod(typed) ||
		aliasesProvidersLeafEffectContract(typed) {
		return providerEffectOwnershipFinding{
			kind:        "edges-redefinition",
			packagePath: packagePath,
			filePath:    filePath,
			typeName:    typed.Name.Name,
		}, true
	}
	return providerEffectOwnershipFinding{}, false
}

func declaresProviderEffectMethod(typed *ast.TypeSpec) bool {
	interfaceType, ok := typed.Type.(*ast.InterfaceType)
	if !ok || interfaceType.Methods == nil {
		return false
	}
	for _, method := range interfaceType.Methods.List {
		for _, name := range method.Names {
			if name.Name == providerEffectMethodName {
				return true
			}
		}
	}
	return false
}

func aliasesProvidersLeafEffectContract(typed *ast.TypeSpec) bool {
	if typed.Name == nil || !typed.Assign.IsValid() {
		return false
	}
	selector, ok := typed.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return selector.Sel != nil && selector.Sel.Name == providerEffectPortTypeName
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

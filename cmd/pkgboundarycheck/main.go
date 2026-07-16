package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const defaultScanRoot = "pkg"
const batch001MigrationShimMarker = "Batch 001 compatibility shim"
const javascriptOrchestratorImportPrefix = "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/"
const applicationGraphImportPath = "github.com/portpowered/infinite-you/pkg/wire"
const transportImportPrefix = "github.com/portpowered/infinite-you/pkg/transports/"
const repositoryImportPrefix = "github.com/portpowered/infinite-you/"

var protectedTransportIndependentDomainRoots = []string{
	"pkg/factory",
	"pkg/work",
}

var factoryRetiredPackageRoots = []retiredPackageRoot{
	{packagePath: "pkg/packagedfactories", canonicalOwner: "pkg/factory/packages"},
	{packagePath: "pkg/factorydefinition", canonicalOwner: "pkg/factory/definition"},
	{packagePath: "pkg/factorysessionexecution", canonicalOwner: "pkg/factory/sessions/execution"},
	{packagePath: "pkg/factorysessions", canonicalOwner: "pkg/factory/sessions"},
	{packagePath: "pkg/petri", canonicalOwner: "pkg/orchestrators/petri"},
}

var retiredPackageRoots = append([]retiredPackageRoot{
	{packagePath: "pkg/api", canonicalOwner: "pkg/transports/http"},
	{packagePath: "pkg/apisurface", canonicalOwner: "pkg/transports/mapping"},
	{packagePath: "pkg/cli", canonicalOwner: "pkg/transports/cli"},
	{packagePath: "pkg/generatedclient", canonicalOwner: "pkg/transports/http/client"},
	{packagePath: "pkg/hostedworkers", canonicalOwner: "pkg/workers/hosted"},
	{packagePath: "pkg/internal/cursorstorage", canonicalOwner: "pkg/platform/cursors"},
	{packagePath: "pkg/internal/metrics", canonicalOwner: "pkg/factory/metrics for domain contracts and pkg/platform/metrics for file-backed recording"},
	{packagePath: "pkg/invocations", canonicalOwner: "pkg/work/invocation, pkg/factory/sessions/invocation, pkg/workers/inference, or pkg/workers/skippermissions, according to the concern"},
	{packagePath: "pkg/interfaces", canonicalOwner: "the defining domain under pkg/factory, pkg/work, pkg/workers, or pkg/models"},
	{packagePath: "pkg/localmodels", canonicalOwner: "pkg/models/local or pkg/models/assets"},
	{packagePath: "pkg/logging", canonicalOwner: "pkg/platform/logging"},
	{packagePath: "pkg/materialize", canonicalOwner: "pkg/work/materialize"},
	{packagePath: "pkg/mcp", canonicalOwner: "pkg/transports/mcp"},
	{packagePath: "pkg/modelhost", canonicalOwner: "pkg/models/host"},
	{packagePath: "pkg/replay", canonicalOwner: "pkg/factory/replay for Factory-event replay policy and pkg/platform/replay for artifact filesystem mechanics"},
	{packagePath: "pkg/sessionpersistence", canonicalOwner: "pkg/platform/cursors"},
	{packagePath: "pkg/testutil", canonicalOwner: "internal/testutil or package-local test helpers"},
	{packagePath: "pkg/timework", canonicalOwner: "pkg/work/timework"},
	{packagePath: "pkg/workcontent", canonicalOwner: "pkg/work/content"},
	{packagePath: "pkg/workgraph", canonicalOwner: "pkg/work/graph"},
	{packagePath: "pkg/workquery", canonicalOwner: "pkg/work/query"},
}, factoryRetiredPackageRoots...)

var approvedApplicationGraphImporters = []string{
	"pkg/initializer",
	"pkg/root",
	"pkg/wire",
}

const (
	generatedCodeExceptionScopeRoot    = "root"
	generatedCodeExceptionScopeSubtree = "subtree"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type boundaryPolicy struct {
	approvedProductPackageFamilies []string
	migrationPackageExceptions     []migrationPackageException
	generatedCodeExceptions        []generatedCodeException
	factoryTransportExceptions     []string
}

type migrationPackageException struct {
	packagePath  string
	targetOwner  string
	workItem     string
	deletionGate string
}

type generatedCodeException struct {
	packagePath string
	scope       string
}

var approvedProductPackageFamilies = []string{
	"pkg/config",
	"pkg/factory",
	"pkg/initializer",
	"pkg/internal",
	"pkg/models",
	"pkg/orchestrators",
	"pkg/platform",
	"pkg/root",
	"pkg/transports",
	"pkg/wire",
	"pkg/work",
	"pkg/workers",
}

const (
	batch006TransportFamilyMove = "Batch 006 — Transport family move"
	batch006WorkFamilyMove      = "Batch 006 — Work family move"
	batch006PlatformFamilyMove  = "Batch 006 — Platform family move"
	batch007And008ServiceMove   = "Batch 007 — Service and Factory Session ownership convergence; Batch 008 — Legacy composition-root deletion"
	batch008CompositionDeletion = "Batch 008 — Legacy composition-root deletion"
)

var documentedMigrationPackageExceptions = []migrationPackageException{
	{packagePath: "pkg/api", targetOwner: "pkg/transports", workItem: batch006TransportFamilyMove, deletionGate: "remove after transport contracts, handlers, and callers move to pkg/transports"},
	{packagePath: "pkg/apisurface", targetOwner: "pkg/transports", workItem: batch006TransportFamilyMove, deletionGate: "remove after boundary mapping and callers move to pkg/transports"},
	{packagePath: "pkg/cli", targetOwner: "pkg/transports", workItem: batch006TransportFamilyMove, deletionGate: "remove after CLI adapters and callers move to pkg/transports"},
	{packagePath: "pkg/mcp", targetOwner: "pkg/transports", workItem: batch006TransportFamilyMove, deletionGate: "remove after MCP adapters and callers move to pkg/transports"},
	{packagePath: "pkg/service", targetOwner: "pkg/wire", workItem: batch007And008ServiceMove, deletionGate: "remove after domain behavior reaches narrow owners and the remaining construction shell moves to pkg/wire"},
	{packagePath: "pkg/runtimehost", targetOwner: "pkg/wire", workItem: batch008CompositionDeletion, deletionGate: "remove after transports and pkg/initializer consume the explicit graph"},
	{packagePath: "pkg/composebridge", targetOwner: "pkg/wire", workItem: batch008CompositionDeletion, deletionGate: "remove after pkg/initializer consumes the explicit graph without service composition internals"},
}

var documentedGeneratedCodeExceptions = []generatedCodeException{
	{packagePath: "pkg/transports/http/client", scope: generatedCodeExceptionScopeRoot},
	{packagePath: "pkg/transports/http/generated", scope: generatedCodeExceptionScopeRoot},
}

func defaultBoundaryPolicy() boundaryPolicy {
	return boundaryPolicy{
		approvedProductPackageFamilies: slices.Clone(approvedProductPackageFamilies),
		migrationPackageExceptions:     slices.Clone(documentedMigrationPackageExceptions),
		generatedCodeExceptions:        slices.Clone(documentedGeneratedCodeExceptions),
		factoryTransportExceptions:     slices.Clone(documentedFactoryTransportExceptions),
	}
}

// documentedFactoryTransportExceptions is a deletion inventory for the
// remaining Factory-owned files that still adapt generated transport values.
// New reverse dependencies are rejected even while these files migrate to
// domain-owned inputs and transport mapping moves outward.
var documentedFactoryTransportExceptions = []string{}

type config struct {
	root        string
	packageRoot string
}

type scanResult struct {
	rootPackageFindings            []rootPackageFinding
	retiredPackageRootFindings     []retiredPackageRootFinding
	retiredPackageImportFindings   []retiredPackageImportFinding
	migrationShimFindings          []migrationShimFinding
	applicationGraphImportFindings []applicationGraphImportFinding
	handwrittenGeneratedFindings   []handwrittenGeneratedFinding
	factoryTransportFindings       []factoryTransportImportFinding
}

type retiredPackageRoot struct {
	packagePath    string
	canonicalOwner string
}

type retiredPackageRootFinding struct {
	retiredPackageRoot
}

type retiredPackageImportFinding struct {
	retiredPackageRoot
	importPath string
	filePath   string
}

type handwrittenGeneratedFinding struct {
	filePath    string
	packagePath string
}

type rootPackageFinding struct {
	packagePath string
}

type migrationShimFinding struct {
	packagePath     string
	marker          string
	canonicalTarget string
}

type applicationGraphImportFinding struct {
	packagePath string
	filePath    string
}

type factoryTransportImportFinding struct {
	packagePath string
	importPath  string
	filePath    string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, stdoutWriter, stderrWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.StringVar(&cfg.packageRoot, "package-root", defaultScanRoot, "repository-relative package root to scan")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	return runWithPolicy(cfg, defaultBoundaryPolicy(), stdout, stderr)
}

func runWithPolicy(cfg config, policy boundaryPolicy, stdout io.Writer, stderr io.Writer) error {
	if strings.TrimSpace(cfg.packageRoot) == "" {
		return fmt.Errorf("package root must not be empty")
	}

	if err := validatePolicy(policy); err != nil {
		return err
	}

	findings, err := scanRepo(cfg, policy)
	if err != nil {
		return err
	}
	blockingViolationCount := len(findings.rootPackageFindings) +
		len(findings.retiredPackageRootFindings) +
		len(findings.retiredPackageImportFindings) +
		len(findings.migrationShimFindings) +
		len(findings.applicationGraphImportFindings) +
		len(findings.handwrittenGeneratedFindings) +
		len(findings.factoryTransportFindings)
	if blockingViolationCount == 0 {
		fmt.Fprintln(stdout, "[agent-factory:pkg-boundary] package boundary passed (no blocking package-boundary violations)")
		writeGeneratedCodeExceptionSummary(stdout, policy)
		return nil
	}

	for _, finding := range findings.rootPackageFindings {
		fmt.Fprintf(stderr, "[agent-factory:pkg-boundary] unapproved root package family: %s\n", finding.packagePath)
		fmt.Fprintf(stderr, "  reason: %s is outside the approved package-family allowlist.\n", finding.packagePath)
		fmt.Fprintln(stderr, "  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.")
	}
	writeRetiredPackageRootFindings(stderr, findings.retiredPackageRootFindings)
	writeRetiredPackageImportFindings(stderr, findings.retiredPackageImportFindings)
	writeMigrationShimBlockingFindings(stderr, findings.migrationShimFindings)
	writeApplicationGraphImportFindings(stderr, findings.applicationGraphImportFindings)
	writeHandwrittenGeneratedFindings(stderr, findings.handwrittenGeneratedFindings)
	writeFactoryTransportImportFindings(stderr, findings.factoryTransportFindings)
	writeGeneratedCodeExceptionSummary(stderr, policy)
	return fmt.Errorf("[agent-factory:pkg-boundary] found %d package-boundary violation(s)", blockingViolationCount)
}

func scanRepo(cfg config, policy boundaryPolicy) (scanResult, error) {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return scanResult{}, fmt.Errorf("resolve repo root: %w", err)
	}

	scanRoot := filepath.Join(repoRoot, filepath.FromSlash(cfg.packageRoot))
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

	entries, err := os.ReadDir(scanRoot)
	if err != nil {
		return scanResult{}, fmt.Errorf("read scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}

	result := scanResult{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packagePath := filepath.ToSlash(filepath.Join(cfg.packageRoot, entry.Name()))
		if retiredRoot, found := findRetiredPackageRoot(packagePath); found {
			result.retiredPackageRootFindings = append(result.retiredPackageRootFindings, retiredPackageRootFinding{retiredRoot})
			continue
		}
		migrationShimFinding, found, err := detectMigrationShimFinding(repoRoot, packagePath)
		if err != nil {
			return scanResult{}, err
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
			return scanResult{}, fmt.Errorf("stat retired package root %s: %w", retiredRoot.packagePath, statErr)
		}
		if info.IsDir() {
			result.retiredPackageRootFindings = append(
				result.retiredPackageRootFindings,
				retiredPackageRootFinding{retiredRoot},
			)
		}
	}

	result.applicationGraphImportFindings, err = scanApplicationGraphImports(repoRoot, scanRoot, cfg.packageRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.retiredPackageImportFindings, err = scanRetiredPackageImports(repoRoot, scanRoot, cfg.packageRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.handwrittenGeneratedFindings, err = scanHandwrittenGeneratedFiles(repoRoot, policy.generatedCodeExceptions)
	if err != nil {
		return scanResult{}, err
	}
	result.factoryTransportFindings, err = scanFactoryTransportImports(repoRoot, policy.factoryTransportExceptions)
	if err != nil {
		return scanResult{}, err
	}

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
	return result, nil
}

func scanFactoryTransportImports(repoRoot string, exceptions []string) ([]factoryTransportImportFinding, error) {
	var findings []factoryTransportImportFinding
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
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
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
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("parse Factory package imports %s: %w", filePath, err)
			}
			packagePath := filepath.ToSlash(filepath.Dir(filePath))
			for _, importSpec := range parsedFile.Imports {
				importPath, err := strconv.Unquote(importSpec.Path.Value)
				if err == nil && strings.HasPrefix(importPath, transportImportPrefix) {
					findings = append(findings, factoryTransportImportFinding{
						packagePath: packagePath,
						importPath:  importPath,
						filePath:    filePath,
					})
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan protected domain transport imports under %s: %w", domainRoot, err)
		}
	}
	slices.SortFunc(findings, func(left, right factoryTransportImportFinding) int {
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
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read package import file %s: %w", filepath.ToSlash(path), err)
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
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read package import file %s: %w", filepath.ToSlash(path), err)
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

func validatePolicy(policy boundaryPolicy) error {
	if err := validateMigrationPackageExceptions(policy); err != nil {
		return err
	}
	for _, exception := range policy.generatedCodeExceptions {
		if strings.TrimSpace(exception.packagePath) == "" {
			return fmt.Errorf("generated-code exception path must not be empty")
		}
		if slices.Contains(policy.approvedProductPackageFamilies, exception.packagePath) {
			return fmt.Errorf("generated-code exception %s must not also be an approved product package family", exception.packagePath)
		}
		if containsMigrationPackageException(policy.migrationPackageExceptions, exception.packagePath) {
			return fmt.Errorf("generated-code exception %s must not also be a migration-only package exception", exception.packagePath)
		}
	}
	return nil
}

func validateMigrationPackageExceptions(policy boundaryPolicy) error {
	for _, exception := range policy.migrationPackageExceptions {
		if strings.TrimSpace(exception.packagePath) == "" {
			return fmt.Errorf("migration-only package exception path must not be empty")
		}
		if slices.Contains(policy.approvedProductPackageFamilies, exception.packagePath) {
			return fmt.Errorf("migration-only package exception %s must not also be an approved product package family", exception.packagePath)
		}
		if strings.TrimSpace(exception.targetOwner) == "" {
			return fmt.Errorf("migration-only package exception %s target owner must not be empty", exception.packagePath)
		}
		if !slices.Contains(policy.approvedProductPackageFamilies, exception.targetOwner) {
			return fmt.Errorf("migration-only package exception %s target owner %s must be an approved product package family", exception.packagePath, exception.targetOwner)
		}
		expectedTarget, active := activeMigrationTarget(exception.workItem)
		if !active {
			return fmt.Errorf("migration-only package exception %s must name an active Batch 006, Batch 007, or Batch 008 work item", exception.packagePath)
		}
		if exception.targetOwner != expectedTarget {
			return fmt.Errorf("migration-only package exception %s work item %q targets %s, not %s", exception.packagePath, exception.workItem, expectedTarget, exception.targetOwner)
		}
		if strings.TrimSpace(exception.deletionGate) == "" {
			return fmt.Errorf("migration-only package exception %s deletion gate must not be empty", exception.packagePath)
		}
	}
	return nil
}

func activeMigrationTarget(workItem string) (string, bool) {
	switch workItem {
	case batch006TransportFamilyMove:
		return "pkg/transports", true
	case batch006WorkFamilyMove:
		return "pkg/work", true
	case batch006PlatformFamilyMove:
		return "pkg/platform", true
	case batch007And008ServiceMove, batch008CompositionDeletion:
		return "pkg/wire", true
	default:
		return "", false
	}
}

func containsMigrationPackageException(exceptions []migrationPackageException, packagePath string) bool {
	return slices.ContainsFunc(exceptions, func(exception migrationPackageException) bool {
		return exception.packagePath == packagePath
	})
}

func isAllowedRootPackageFamily(policy boundaryPolicy, packageRoot string, packagePath string) bool {
	return slices.Contains(policy.approvedProductPackageFamilies, packagePath) ||
		containsMigrationPackageException(policy.migrationPackageExceptions, packagePath) ||
		slices.Contains(directRootGeneratedCodeExceptionPaths(policy, packageRoot), packagePath)
}

func detectMigrationShimFinding(repoRoot string, packagePath string) (migrationShimFinding, bool, error) {
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(packagePath))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return migrationShimFinding{}, false, fmt.Errorf("read migration shim package %s: %w", packagePath, err)
	}

	finding := migrationShimFinding{packagePath: packagePath}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}

		goFilePath := filepath.Join(packageDir, entry.Name())
		marker, canonicalTarget, err := readMigrationShimSignals(goFilePath)
		if err != nil {
			return migrationShimFinding{}, false, err
		}
		if finding.marker == "" {
			finding.marker = marker
		}
		if finding.canonicalTarget == "" {
			finding.canonicalTarget = canonicalTarget
		}
		if finding.marker != "" && finding.canonicalTarget != "" {
			return finding, true, nil
		}
	}

	return finding, finding.marker != "", nil
}

func readMigrationShimSignals(goFilePath string) (string, string, error) {
	content, err := os.ReadFile(goFilePath)
	if err != nil {
		return "", "", fmt.Errorf("read migration shim file %s: %w", filepath.ToSlash(goFilePath), err)
	}

	marker := ""
	if strings.Contains(string(content), batch001MigrationShimMarker) {
		marker = batch001MigrationShimMarker
	}
	return marker, canonicalTargetImport(content), nil
}

func canonicalTargetImport(content []byte) string {
	parsedFile, err := parser.ParseFile(token.NewFileSet(), "", content, parser.ImportsOnly)
	if err != nil {
		return ""
	}

	for _, importSpec := range parsedFile.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}
		if strings.HasPrefix(importPath, javascriptOrchestratorImportPrefix) {
			return importPath
		}
	}
	return ""
}

func directRootGeneratedCodeExceptionPaths(policy boundaryPolicy, packageRoot string) []string {
	var roots []string
	for _, exception := range policy.generatedCodeExceptions {
		exceptionPath := filepath.ToSlash(exception.packagePath)
		if filepath.Dir(exceptionPath) == filepath.ToSlash(packageRoot) {
			roots = append(roots, exceptionPath)
		}
	}
	return roots
}

func writeGeneratedCodeExceptionSummary(writer io.Writer, policy boundaryPolicy) {
	exceptions := generatedCodeExceptionDescriptions(policy)
	if len(exceptions) == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active generated-code exceptions: %s\n", strings.Join(exceptions, ", "))
}

func writeRetiredPackageRootFindings(writer io.Writer, findings []retiredPackageRootFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited retired package root: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  canonical owner: %s\n", finding.canonicalOwner)
		fmt.Fprintf(writer, "  remediation: move the code to %s and delete the retired root.\n", finding.canonicalOwner)
	}
}

func writeRetiredPackageImportFindings(writer io.Writer, findings []retiredPackageImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited retired package import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintf(writer, "  canonical owner: %s\n", finding.canonicalOwner)
		fmt.Fprintf(writer, "  remediation: import %s directly; do not recreate or depend on %s.\n", finding.canonicalOwner, finding.packagePath)
	}
}

func generatedCodeExceptionDescriptions(policy boundaryPolicy) []string {
	descriptions := make([]string, 0, len(policy.generatedCodeExceptions))
	for _, exception := range policy.generatedCodeExceptions {
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", filepath.ToSlash(exception.packagePath), exception.scope))
	}
	return descriptions
}

func writeMigrationShimBlockingFindings(writer io.Writer, findings []migrationShimFinding) {
	if len(findings) == 0 {
		return
	}

	for _, finding := range findings {
		canonicalTarget := finding.canonicalTarget
		if canonicalTarget == "" {
			canonicalTarget = "not detected"
		}
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] blocked migration-only compatibility shim: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  marker: %s\n", finding.marker)
		fmt.Fprintf(writer, "  canonical target: %s\n", canonicalTarget)
		fmt.Fprintln(writer, "  remediation: import the canonical owner directly and do not recreate Batch 001 root compatibility shims.")
	}
}

func writeApplicationGraphImportFindings(writer io.Writer, findings []applicationGraphImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited application composition import: %s (%s)\n", finding.packagePath, finding.filePath)
		fmt.Fprintln(writer, "  reason: pkg/wire is the outward application composition root and must not be imported by domain or transport packages.")
		fmt.Fprintln(writer, "  remediation: depend on a narrow domain-owned contract and inject the collaborator through pkg/root or pkg/initializer.")
	}
}

func writeFactoryTransportImportFindings(writer io.Writer, findings []factoryTransportImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited domain transport import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintf(writer, "  domain owner: %s\n", finding.packagePath)
		fmt.Fprintln(writer, "  reason: protected domain packages must not consume transport contracts or adapters.")
		fmt.Fprintln(writer, "  remediation: define the input at its domain owner and map generated values under pkg/transports/mapping.")
	}
}

func writeHandwrittenGeneratedFindings(writer io.Writer, findings []handwrittenGeneratedFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] handwritten Go file in generated-only package: %s (%s)\n", finding.packagePath, finding.filePath)
		fmt.Fprintln(writer, "  reason: generated-only packages may contain only files with the standard Code generated ... DO NOT EDIT. marker.")
		fmt.Fprintln(writer, "  remediation: move handwritten mapping or policy to pkg/transports/http or pkg/transports/mapping.")
	}
}

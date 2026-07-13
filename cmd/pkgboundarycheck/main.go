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

var retiredTransportImports = map[string]string{
	"github.com/portpowered/infinite-you/pkg/api":             "github.com/portpowered/infinite-you/pkg/transports/http",
	"github.com/portpowered/infinite-you/pkg/apisurface":      "github.com/portpowered/infinite-you/pkg/transports/mapping",
	"github.com/portpowered/infinite-you/pkg/cli":             "github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/mcp":             "github.com/portpowered/infinite-you/pkg/transports/mcp",
	"github.com/portpowered/infinite-you/pkg/generatedclient": "github.com/portpowered/infinite-you/pkg/transports/http/client",
}

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
	"pkg/factorydefinition",
	"pkg/factorysessionexecution",
	"pkg/factorysessions",
	"pkg/hostedworkers",
	"pkg/initializer",
	"pkg/interfaces",
	"pkg/internal",
	"pkg/localmodels",
	"pkg/modelhost",
	"pkg/models",
	"pkg/orchestrators",
	"pkg/packagedfactories",
	"pkg/petri",
	"pkg/platform",
	"pkg/root",
	"pkg/testutil",
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
	{packagePath: "pkg/invocations", targetOwner: "pkg/work", workItem: batch006WorkFamilyMove, deletionGate: "remove after invocation input and return policy move to pkg/work"},
	{packagePath: "pkg/materialize", targetOwner: "pkg/work", workItem: batch006WorkFamilyMove, deletionGate: "remove after materialization behavior and callers move to pkg/work"},
	{packagePath: "pkg/timework", targetOwner: "pkg/work", workItem: batch006WorkFamilyMove, deletionGate: "remove after cron and time-work behavior and callers move to pkg/work"},
	{packagePath: "pkg/workcontent", targetOwner: "pkg/work", workItem: batch006WorkFamilyMove, deletionGate: "remove after Work content behavior and callers move to pkg/work"},
	{packagePath: "pkg/workgraph", targetOwner: "pkg/work", workItem: batch006WorkFamilyMove, deletionGate: "remove after Work graph and lineage behavior and callers move to pkg/work"},
	{packagePath: "pkg/workquery", targetOwner: "pkg/work", workItem: batch006WorkFamilyMove, deletionGate: "remove after Work query behavior and callers move to pkg/work"},
	{packagePath: "pkg/logging", targetOwner: "pkg/platform", workItem: batch006PlatformFamilyMove, deletionGate: "remove after logging infrastructure and callers move to pkg/platform"},
	{packagePath: "pkg/replay", targetOwner: "pkg/platform", workItem: batch006PlatformFamilyMove, deletionGate: "remove after replay and artifact infrastructure and callers move to pkg/platform"},
	{packagePath: "pkg/sessionpersistence", targetOwner: "pkg/platform", workItem: batch006PlatformFamilyMove, deletionGate: "remove after cursor persistence and callers move to pkg/platform"},
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
	}
}

type config struct {
	root        string
	packageRoot string
}

type scanResult struct {
	rootPackageFindings            []rootPackageFinding
	migrationShimFindings          []migrationShimFinding
	applicationGraphImportFindings []applicationGraphImportFinding
	retiredTransportImportFindings []retiredTransportImportFinding
	handwrittenGeneratedFindings   []handwrittenGeneratedFinding
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

type retiredTransportImportFinding struct {
	filePath        string
	importPath      string
	canonicalTarget string
}

type handwrittenGeneratedFinding struct {
	filePath    string
	packagePath string
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
		len(findings.migrationShimFindings) +
		len(findings.applicationGraphImportFindings) +
		len(findings.retiredTransportImportFindings) +
		len(findings.handwrittenGeneratedFindings)
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
	writeMigrationShimBlockingFindings(stderr, findings.migrationShimFindings)
	writeApplicationGraphImportFindings(stderr, findings.applicationGraphImportFindings)
	writeRetiredTransportImportFindings(stderr, findings.retiredTransportImportFindings)
	writeHandwrittenGeneratedFindings(stderr, findings.handwrittenGeneratedFindings)
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

	result.applicationGraphImportFindings, err = scanApplicationGraphImports(repoRoot, scanRoot, cfg.packageRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.retiredTransportImportFindings, result.handwrittenGeneratedFindings, err = scanTransportOwnership(repoRoot, scanRoot, cfg.packageRoot, policy)
	if err != nil {
		return scanResult{}, err
	}

	slices.SortFunc(result.rootPackageFindings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	slices.SortFunc(result.migrationShimFindings, func(left, right migrationShimFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	return result, nil
}

func scanTransportOwnership(
	repoRoot string,
	scanRoot string,
	packageRoot string,
	policy boundaryPolicy,
) ([]retiredTransportImportFinding, []handwrittenGeneratedFinding, error) {
	var retiredImports []retiredTransportImportFinding
	var handwrittenGenerated []handwrittenGeneratedFinding
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read transport ownership file %s: %w", filepath.ToSlash(path), err)
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve transport ownership file %s: %w", filepath.ToSlash(path), err)
		}
		filePath = filepath.ToSlash(filePath)

		if packagePath, generatedOnly := generatedOnlyPackageForFile(repoRoot, path, policy.generatedCodeExceptions); generatedOnly {
			parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated-only package file %s: %w", filePath, err)
			}
			if !ast.IsGenerated(parsedFile) {
				handwrittenGenerated = append(handwrittenGenerated, handwrittenGeneratedFinding{
					filePath:    filePath,
					packagePath: packagePath,
				})
			}
		}

		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse transport ownership imports %s: %w", filePath, err)
		}
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			canonicalTarget, retired := retiredTransportTarget(importPath)
			if !retired {
				continue
			}
			retiredImports = append(retiredImports, retiredTransportImportFinding{
				filePath:        filePath,
				importPath:      importPath,
				canonicalTarget: canonicalTarget,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scan %s transport ownership: %w", filepath.ToSlash(packageRoot), err)
	}

	slices.SortFunc(retiredImports, func(left, right retiredTransportImportFinding) int {
		return strings.Compare(left.filePath+left.importPath, right.filePath+right.importPath)
	})
	slices.SortFunc(handwrittenGenerated, func(left, right handwrittenGeneratedFinding) int {
		return strings.Compare(left.filePath, right.filePath)
	})
	return retiredImports, handwrittenGenerated, nil
}

func retiredTransportTarget(importPath string) (string, bool) {
	for retiredPath, canonicalTarget := range retiredTransportImports {
		if importPath == retiredPath || strings.HasPrefix(importPath, retiredPath+"/") {
			suffix := strings.TrimPrefix(importPath, retiredPath)
			return canonicalTarget + suffix, true
		}
	}
	return "", false
}

func generatedOnlyPackageForFile(repoRoot string, path string, exceptions []generatedCodeException) (string, bool) {
	for _, exception := range exceptions {
		packageDir := filepath.Join(repoRoot, filepath.FromSlash(exception.packagePath))
		relativePath, err := filepath.Rel(packageDir, path)
		if err != nil || relativePath == "." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		if exception.scope == generatedCodeExceptionScopeRoot && filepath.Dir(relativePath) != "." {
			continue
		}
		return exception.packagePath, true
	}
	return "", false
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

func writeRetiredTransportImportFindings(writer io.Writer, findings []retiredTransportImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited legacy transport import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintf(writer, "  reason: %s is a retired transport root.\n", finding.importPath)
		fmt.Fprintf(writer, "  remediation: import the canonical successor %s.\n", finding.canonicalTarget)
	}
}

func writeHandwrittenGeneratedFindings(writer io.Writer, findings []handwrittenGeneratedFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] handwritten Go file in generated-only package: %s (%s)\n", finding.packagePath, finding.filePath)
		fmt.Fprintln(writer, "  reason: generated-only packages may contain only files with the standard Code generated ... DO NOT EDIT. marker.")
		fmt.Fprintln(writer, "  remediation: move handwritten mapping or policy to pkg/transports/http or pkg/transports/mapping.")
	}
}

package main

import (
	"flag"
	"fmt"
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
	generatedCodeExceptions        []generatedCodeException
}

type generatedCodeException struct {
	packagePath string
	scope       string
}

var approvedProductPackageFamilies = []string{
	"pkg/api",
	"pkg/apisurface",
	"pkg/cli",
	"pkg/composebridge",
	"pkg/config",
	"pkg/factory",
	"pkg/factorydefinition",
	"pkg/factorysessionexecution",
	"pkg/factorysessions",
	"pkg/hostedworkers",
	"pkg/initializer",
	"pkg/interfaces",
	"pkg/internal",
	"pkg/invocations",
	"pkg/localmodels",
	"pkg/logging",
	"pkg/materialize",
	"pkg/mcp",
	"pkg/modelhost",
	"pkg/models",
	"pkg/orchestrators",
	"pkg/packagedfactories",
	"pkg/petri",
	"pkg/replay",
	"pkg/runtimehost",
	"pkg/service",
	"pkg/sessionpersistence",
	"pkg/testutil",
	"pkg/timework",
	"pkg/workcontent",
	"pkg/workers",
	"pkg/workgraph",
	"pkg/workquery",
}

var documentedGeneratedCodeExceptions = []generatedCodeException{
	{packagePath: "pkg/generatedclient", scope: generatedCodeExceptionScopeRoot},
	{packagePath: "pkg/api/generated", scope: generatedCodeExceptionScopeSubtree},
}

func defaultBoundaryPolicy() boundaryPolicy {
	return boundaryPolicy{
		approvedProductPackageFamilies: slices.Clone(approvedProductPackageFamilies),
		generatedCodeExceptions:        slices.Clone(documentedGeneratedCodeExceptions),
	}
}

type config struct {
	root        string
	packageRoot string
}

type scanResult struct {
	rootPackageFindings   []rootPackageFinding
	migrationShimFindings []migrationShimFinding
}

type rootPackageFinding struct {
	packagePath string
}

type migrationShimFinding struct {
	packagePath     string
	marker          string
	canonicalTarget string
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
	blockingViolationCount := len(findings.rootPackageFindings) + len(findings.migrationShimFindings)
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

	slices.SortFunc(result.rootPackageFindings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	slices.SortFunc(result.migrationShimFindings, func(left, right migrationShimFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	return result, nil
}

func validatePolicy(policy boundaryPolicy) error {
	for _, exception := range policy.generatedCodeExceptions {
		if strings.TrimSpace(exception.packagePath) == "" {
			return fmt.Errorf("generated-code exception path must not be empty")
		}
		if slices.Contains(policy.approvedProductPackageFamilies, exception.packagePath) {
			return fmt.Errorf("generated-code exception %s must not also be an approved product package family", exception.packagePath)
		}
	}
	return nil
}

func isAllowedRootPackageFamily(policy boundaryPolicy, packageRoot string, packagePath string) bool {
	return slices.Contains(policy.approvedProductPackageFamilies, packagePath) ||
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

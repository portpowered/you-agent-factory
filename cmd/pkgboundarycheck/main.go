package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const defaultScanRoot = "pkg"

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
	temporaryMigrationShimRoots    []string
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

var temporaryMigrationShimRoots = []string{
	"pkg/workflowpolicy",
	"pkg/workflowpreview",
	"pkg/workflowresult",
	"pkg/workflowsource",
	"pkg/workflowvalidation",
}

func defaultBoundaryPolicy() boundaryPolicy {
	return boundaryPolicy{
		approvedProductPackageFamilies: slices.Clone(approvedProductPackageFamilies),
		generatedCodeExceptions:        slices.Clone(documentedGeneratedCodeExceptions),
		temporaryMigrationShimRoots:    slices.Clone(temporaryMigrationShimRoots),
	}
}

type config struct {
	root        string
	packageRoot string
}

type rootPackageFinding struct {
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
	if strings.TrimSpace(cfg.packageRoot) == "" {
		return fmt.Errorf("package root must not be empty")
	}

	policy := defaultBoundaryPolicy()
	if err := validatePolicy(policy); err != nil {
		return err
	}

	findings, err := scanRepo(cfg, policy)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Fprintln(stdout, "[agent-factory:pkg-boundary] package boundary passed (approved root package families and documented exceptions only)")
		writeGeneratedCodeExceptionSummary(stdout, policy)
		return nil
	}

	for _, finding := range findings {
		fmt.Fprintf(stderr, "[agent-factory:pkg-boundary] unapproved root package family: %s\n", finding.packagePath)
		fmt.Fprintf(stderr, "  reason: %s is outside the approved package-family allowlist.\n", finding.packagePath)
		fmt.Fprintln(stderr, "  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.")
	}
	writeGeneratedCodeExceptionSummary(stderr, policy)
	return fmt.Errorf("[agent-factory:pkg-boundary] found %d package-boundary violation(s)", len(findings))
}

func scanRepo(cfg config, policy boundaryPolicy) ([]rootPackageFinding, error) {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	scanRoot := filepath.Join(repoRoot, filepath.FromSlash(cfg.packageRoot))
	info, err := os.Stat(scanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}
	if !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(scanRoot)
	if err != nil {
		return nil, fmt.Errorf("read scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}

	var findings []rootPackageFinding
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packagePath := filepath.ToSlash(filepath.Join(cfg.packageRoot, entry.Name()))
		if isAllowedRootPackageFamily(policy, cfg.packageRoot, packagePath) {
			continue
		}

		findings = append(findings, rootPackageFinding{packagePath: packagePath})
	}

	slices.SortFunc(findings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	return findings, nil
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
		slices.Contains(directRootGeneratedCodeExceptionPaths(policy, packageRoot), packagePath) ||
		slices.Contains(policy.temporaryMigrationShimRoots, packagePath)
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

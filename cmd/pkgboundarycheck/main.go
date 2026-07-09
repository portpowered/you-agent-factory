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

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

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

var documentedGeneratedCodeExceptions = []string{
	"pkg/generatedclient",
	"pkg/api/generated",
}

var temporaryMigrationShimRoots = []string{
	"pkg/workflowpolicy",
	"pkg/workflowpreview",
	"pkg/workflowresult",
	"pkg/workflowsource",
	"pkg/workflowvalidation",
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

	findings, err := scanRepo(cfg)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Fprintln(stdout, "[agent-factory:pkg-boundary] package boundary passed (approved root package families and documented exceptions only)")
		return nil
	}

	for _, finding := range findings {
		fmt.Fprintf(stderr, "[agent-factory:pkg-boundary] unapproved root package family: %s\n", finding.packagePath)
		fmt.Fprintf(stderr, "  reason: %s is outside the approved package-family allowlist.\n", finding.packagePath)
		fmt.Fprintln(stderr, "  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.")
	}
	return fmt.Errorf("[agent-factory:pkg-boundary] found %d package-boundary violation(s)", len(findings))
}

func scanRepo(cfg config) ([]rootPackageFinding, error) {
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
		if isAllowedRootPackageFamily(packagePath) {
			continue
		}

		findings = append(findings, rootPackageFinding{packagePath: packagePath})
	}

	slices.SortFunc(findings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	return findings, nil
}

func isAllowedRootPackageFamily(packagePath string) bool {
	return slices.Contains(approvedProductPackageFamilies, packagePath) ||
		slices.Contains(directRootGeneratedCodeExceptions(), packagePath) ||
		slices.Contains(temporaryMigrationShimRoots, packagePath)
}

func directRootGeneratedCodeExceptions() []string {
	var roots []string
	for _, exception := range documentedGeneratedCodeExceptions {
		if filepath.Dir(filepath.ToSlash(exception)) == defaultScanRoot {
			roots = append(roots, exception)
		}
	}
	return roots
}

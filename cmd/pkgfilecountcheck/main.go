package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/backendsizecheck"
)

const (
	defaultPackageFileLimit = 15
	defaultScanRoot         = "pkg"
	packageFileBaselinePath = "docs/internal/baselines/backend-package-file-count.json"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	root             string
	packageRoot      string
	packageFileLimit int
}

type packageFinding struct {
	packagePath string
	files       []string
	limit       int
}

type packageFileBaseline struct {
	Version int                        `json:"version"`
	Entries []packageFileBaselineEntry `json:"entries"`
}

type packageFileBaselineEntry struct {
	PackagePath   string `json:"packagePath"`
	FileCount     int    `json:"fileCount"`
	Owner         string `json:"owner"`
	RemovalReason string `json:"removalReason"`
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
	flag.IntVar(&cfg.packageFileLimit, "package-file-limit", defaultPackageFileLimit, "maximum hand-maintained Go files per checked package directory")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	if cfg.packageFileLimit <= 0 {
		return fmt.Errorf("package file limit must be positive, got %d", cfg.packageFileLimit)
	}

	findings, err := scanRepo(cfg)
	if err != nil {
		return err
	}
	baseline, err := loadBaseline(cfg)
	if err != nil {
		return err
	}
	if baseline != nil {
		return enforceBaseline(cfg, findings, baseline, stdout, stderr)
	}
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "[agent-factory:pkg-file-count] package file-count passed (package files <= %d)\n", cfg.packageFileLimit)
		return nil
	}

	for _, finding := range findings {
		printFinding(stderr, finding)
	}
	return fmt.Errorf("[agent-factory:pkg-file-count] found %d package file-count violation(s)", len(findings))
}

func loadBaseline(cfg config) (*packageFileBaseline, error) {
	path := filepath.Join(cfg.root, filepath.FromSlash(packageFileBaselinePath))
	source, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read package file-count baseline: %w", err)
	}

	baseline := &packageFileBaseline{}
	if err := json.Unmarshal(source, baseline); err != nil {
		return nil, fmt.Errorf("parse package file-count baseline: %w", err)
	}
	if baseline.Version != 1 {
		return nil, fmt.Errorf("package file-count baseline version must be 1, got %d", baseline.Version)
	}
	previousPath := ""
	for index, entry := range baseline.Entries {
		if entry.PackagePath == "" || !strings.HasPrefix(entry.PackagePath, cfg.packageRoot+"/") {
			return nil, fmt.Errorf("package file-count baseline entry %d has invalid packagePath %q", index, entry.PackagePath)
		}
		if entry.PackagePath <= previousPath {
			return nil, fmt.Errorf("package file-count baseline entries must be unique and sorted by packagePath: %q follows %q", entry.PackagePath, previousPath)
		}
		if entry.FileCount <= cfg.packageFileLimit {
			return nil, fmt.Errorf("package file-count baseline entry %q must exceed limit %d, got %d", entry.PackagePath, cfg.packageFileLimit, entry.FileCount)
		}
		if strings.TrimSpace(entry.Owner) == "" || strings.TrimSpace(entry.RemovalReason) == "" {
			return nil, fmt.Errorf("package file-count baseline entry %q must declare owner and removalReason", entry.PackagePath)
		}
		previousPath = entry.PackagePath
	}
	return baseline, nil
}

func enforceBaseline(cfg config, findings []packageFinding, baseline *packageFileBaseline, stdout io.Writer, stderr io.Writer) error {
	findingsByPath := make(map[string]packageFinding, len(findings))
	for _, finding := range findings {
		findingsByPath[finding.packagePath] = finding
	}
	baselineByPath := make(map[string]packageFileBaselineEntry, len(baseline.Entries))
	violations := 0
	for _, entry := range baseline.Entries {
		baselineByPath[entry.PackagePath] = entry
		finding, exists := findingsByPath[entry.PackagePath]
		if !exists {
			fmt.Fprintf(stderr, "[agent-factory:pkg-file-count] stale baseline entry: %s (recorded %d, now <= %d or absent); remove it\n", entry.PackagePath, entry.FileCount, cfg.packageFileLimit)
			violations++
			continue
		}
		actual := len(finding.files)
		if actual < entry.FileCount {
			fmt.Fprintf(stderr, "[agent-factory:pkg-file-count] reduced baseline entry: %s (recorded %d, now %d); lower it in the same change\n", entry.PackagePath, entry.FileCount, actual)
			violations++
		} else if actual > entry.FileCount {
			fmt.Fprintf(stderr, "[agent-factory:pkg-file-count] package grew beyond baseline: %s (recorded %d, now %d)\n", entry.PackagePath, entry.FileCount, actual)
			violations++
		}
	}
	for _, finding := range findings {
		if _, exists := baselineByPath[finding.packagePath]; exists {
			continue
		}
		printFinding(stderr, finding)
		violations++
	}
	if violations > 0 {
		return fmt.Errorf("[agent-factory:pkg-file-count] found %d package file-count baseline violation(s)", violations)
	}
	if len(baseline.Entries) == 0 {
		fmt.Fprintf(stdout, "[agent-factory:pkg-file-count] package file-count passed (package files <= %d)\n", cfg.packageFileLimit)
		return nil
	}
	fmt.Fprintf(stdout, "[agent-factory:pkg-file-count] package file-count holds (%d exact deletion-only baseline entries remain; unrecorded packages <= %d files)\n", len(baseline.Entries), cfg.packageFileLimit)
	return nil
}

func printFinding(stderr io.Writer, finding packageFinding) {
	fmt.Fprintf(stderr, "[agent-factory:pkg-file-count] oversized package: %s\n", finding.packagePath)
	fmt.Fprintf(stderr, "  package files: %d\n", len(finding.files))
	fmt.Fprintf(stderr, "  limit: %d\n", finding.limit)
	fmt.Fprintln(stderr, "  counted files:")
	for _, file := range finding.files {
		fmt.Fprintf(stderr, "    - %s\n", file)
	}
}

func scanRepo(cfg config) ([]packageFinding, error) {
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

	filesByPackage := map[string][]string{}
	walkErr := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", filepath.ToSlash(path), err)
		}
		if entry.IsDir() {
			if backendsizecheck.ShouldSkipDir(scanRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		generated, err := isGeneratedGoFile(path)
		if err != nil {
			return err
		}
		if generated {
			return nil
		}

		relativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", filepath.ToSlash(path), err)
		}
		relativePath = filepath.ToSlash(relativePath)
		packagePath := filepath.ToSlash(filepath.Dir(relativePath))
		filesByPackage[packagePath] = append(filesByPackage[packagePath], relativePath)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	var findings []packageFinding
	for packagePath, files := range filesByPackage {
		slices.Sort(files)
		if len(files) <= cfg.packageFileLimit {
			continue
		}
		findings = append(findings, packageFinding{
			packagePath: packagePath,
			files:       files,
			limit:       cfg.packageFileLimit,
		})
	}

	slices.SortFunc(findings, func(left, right packageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	return findings, nil
}

func isGeneratedGoFile(path string) (bool, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", filepath.ToSlash(path), err)
	}
	return strings.Contains(string(source), "Code generated") && strings.Contains(string(source), "DO NOT EDIT."), nil
}

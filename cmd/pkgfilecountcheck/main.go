package main

import (
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
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "[agent-factory:pkg-file-count] package file-count passed (package files <= %d)\n", cfg.packageFileLimit)
		return nil
	}

	for _, finding := range findings {
		fmt.Fprintf(stderr, "%s | package files=%d limit=%d counted=%s\n", finding.packagePath, len(finding.files), finding.limit, strings.Join(finding.files, ","))
	}
	return fmt.Errorf("[agent-factory:pkg-file-count] found %d package file-count violation(s)", len(findings))
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

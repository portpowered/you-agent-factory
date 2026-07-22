package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

const modulePath = testlanes.ModulePath

type config struct {
	count   int
	jobs    int
	root    string
	short   bool
	timeout time.Duration
	vet     bool
}

var executeUnitLane = run
var execCommand = exec.Command
var discoverUnitPackages = discoverPackages
var stderrWriter io.Writer = os.Stderr
var exitFunc = os.Exit

func main() {
	if err := executeUnitLane(); err != nil {
		fmt.Fprintf(stderrWriter, "%v\n", err)
		exitFunc(1)
	}
}

func run() error {
	cfg := parseConfig()
	packages, err := discoverUnitPackages(cfg.root)
	if err != nil {
		return fmt.Errorf("discover unit packages: %w", err)
	}
	if len(packages) == 0 {
		return fmt.Errorf("discover unit packages: no packages found under %s", cfg.root)
	}
	if err := runUnitTests(cfg, packages); err != nil {
		return fmt.Errorf("run unit lane: %w", err)
	}
	return nil
}

func parseConfig() config {
	var cfg config
	flag.IntVar(&cfg.count, "count", 0, "go test -count value; zero preserves Go's content-addressed test cache")
	flag.IntVar(&cfg.jobs, "jobs", 32, "go test -p value")
	flag.StringVar(&cfg.root, "root", "./pkg/...", "go list package pattern for unit test discovery")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.BoolVar(&cfg.vet, "vet", false, "run go test's implicit vet pass")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func discoverPackages(root string) ([]string, error) {
	rootPath := strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(root)), "/...")
	rootPath = strings.TrimPrefix(rootPath, "./")
	if rootPath == "" || rootPath == "." {
		return nil, fmt.Errorf("unit package root must name a repository directory")
	}
	return discoverPackagesUnder(filepath.FromSlash(rootPath), modulePath+"/"+rootPath)
}

func discoverPackagesUnder(rootDir, importPrefix string) ([]string, error) {
	packageSet := make(map[string]struct{})
	err := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != rootDir && (entry.Name() == "testdata" || entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relativeDir, err := filepath.Rel(rootDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := importPrefix
		if relativeDir != "." {
			pkg += "/" + filepath.ToSlash(relativeDir)
		}
		if testlanes.IsUnitPackage(pkg) {
			packageSet[pkg] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	slices.Sort(packages)
	return packages, nil
}

func runUnitTests(cfg config, packages []string) error {
	args := []string{"test", fmt.Sprintf("-p=%d", cfg.jobs)}
	if !cfg.vet {
		args = append(args, "-vet=off")
	}
	if cfg.short {
		args = append(args, "-short")
	}
	args = append(args, packages...)
	if cfg.count > 0 {
		args = append(args, fmt.Sprintf("-count=%d", cfg.count))
	}
	args = append(args, fmt.Sprintf("-timeout=%s", cfg.timeout))

	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandError(err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w\n%s", err, detail)
}

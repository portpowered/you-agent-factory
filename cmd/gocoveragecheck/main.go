package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var totalCoveragePattern = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)

const modulePath = "github.com/portpowered/infinite-you"

var (
	defaultCoveragePatterns = []string{"./cmd/factory", "./pkg/..."}
	defaultTestPatterns     = []string{"./cmd/factory", "./pkg/...", "./tests/functional/..."}
)

type config struct {
	covermode string
	coverpkg  string
	min       float64
	packages  string
	profile   string
	short     bool
	timeout   time.Duration
}

func main() {
	cfg := parseConfig()
	actual, err := run(cfg)
	if err != nil {
		failf("%v\n", err)
	}
	if actual < cfg.min {
		failf("go coverage %.1f%% is below minimum %.1f%%\n", actual, cfg.min)
	}
	fmt.Printf("Go coverage %.1f%% meets minimum %.1f%%.\n", actual, cfg.min)
}

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.covermode, "covermode", "count", "go test -covermode value")
	flag.StringVar(&cfg.coverpkg, "coverpkg", "", "comma-separated import paths to measure; defaults to backend-owned packages")
	flag.Float64Var(&cfg.min, "min", 0, "minimum total statement coverage percentage")
	flag.StringVar(&cfg.packages, "packages", "", "space-separated go test package patterns; defaults to backend package tests plus backend-facing functional tests")
	flag.StringVar(&cfg.profile, "profile", "", "coverage profile output path; defaults to a temp file")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.Parse()
	return cfg
}

func run(cfg config) (float64, error) {
	profilePath := cfg.profile
	cleanup := func() error { return nil }
	if profilePath == "" {
		file, err := os.CreateTemp("", "go-coverage-*.out")
		if err != nil {
			return 0, fmt.Errorf("create temp coverage profile: %w", err)
		}
		profilePath = file.Name()
		if err := file.Close(); err != nil {
			return 0, fmt.Errorf("close temp coverage profile: %w", err)
		}
		cleanup = func() error {
			return os.Remove(profilePath)
		}
	}
	defer func() {
		_ = cleanup()
	}()

	coverPackages, testPackages, err := resolveCoverageLane(cfg)
	if err != nil {
		return 0, err
	}

	testArgs := []string{
		"test",
		fmt.Sprintf("-coverpkg=%s", strings.Join(coverPackages, ",")),
	}
	if cfg.short {
		testArgs = append(testArgs, "-short")
	}
	testArgs = append(testArgs,
		fmt.Sprintf("-coverprofile=%s", profilePath),
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)
	testArgs = append(testArgs, testPackages...)

	testCmd := exec.Command("go", testArgs...)
	testCmd.Env = os.Environ()
	testCmd.Stdout = os.Stdout
	testCmd.Stderr = os.Stderr
	if err := testCmd.Run(); err != nil {
		return 0, fmt.Errorf("run go test coverage lane: %w", err)
	}

	coverCmd := exec.Command("go", "tool", "cover", "-func", profilePath)
	coverCmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	coverCmd.Stdout = &stdout
	coverCmd.Stderr = &stderr
	if err := coverCmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return 0, fmt.Errorf("summarize go coverage: %w\n%s", err, detail)
		}
		return 0, fmt.Errorf("summarize go coverage: %w", err)
	}

	report := stdout.String()
	actual, totalLine, err := parseTotalCoverage(report)
	if err != nil {
		return 0, err
	}
	fmt.Println(totalLine)
	return actual, nil
}

func resolveCoverageLane(cfg config) ([]string, []string, error) {
	coverPackages, err := resolveCoverPackages(cfg)
	if err != nil {
		return nil, nil, err
	}
	testPackages, err := resolveTestPackages(cfg)
	if err != nil {
		return nil, nil, err
	}
	return coverPackages, testPackages, nil
}

func resolveCoverPackages(cfg config) ([]string, error) {
	if strings.TrimSpace(cfg.coverpkg) != "" {
		return splitList(cfg.coverpkg, ",", false), nil
	}
	return listGoPackages(defaultCoveragePatterns, isBackendCoveragePackage)
}

func resolveTestPackages(cfg config) ([]string, error) {
	if strings.TrimSpace(cfg.packages) != "" {
		return splitList(cfg.packages, " ", true), nil
	}
	return listGoPackages(defaultTestPatterns, isBackendTestPackage)
}

func listGoPackages(patterns []string, include func(string) bool) ([]string, error) {
	args := append([]string{"list"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	rootDir, err := repoRootDir()
	if err != nil {
		return nil, err
	}
	cmd.Dir = rootDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return nil, fmt.Errorf("list go packages: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("list go packages: %w", err)
	}

	seen := make(map[string]struct{})
	var packages []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		importPath := strings.TrimSpace(line)
		if importPath == "" || !include(importPath) {
			continue
		}
		if _, ok := seen[importPath]; ok {
			continue
		}
		seen[importPath] = struct{}{}
		packages = append(packages, importPath)
	}
	slices.Sort(packages)
	if len(packages) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages matched")
	}
	return packages, nil
}

func repoRootDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("resolve repository root: go.mod not found")
		}
		dir = parent
	}
}

func isBackendCoveragePackage(importPath string) bool {
	switch {
	case importPath == modulePath+"/cmd/factory":
		return true
	case !strings.HasPrefix(importPath, modulePath+"/pkg/"):
		return false
	case importPath == modulePath+"/pkg/api/generated":
		return false
	case importPath == modulePath+"/pkg/generatedclient":
		return false
	case strings.HasPrefix(importPath, modulePath+"/pkg/testutil"):
		return false
	default:
		return true
	}
}

func isBackendTestPackage(importPath string) bool {
	if isBackendCoveragePackage(importPath) {
		return true
	}
	return strings.HasPrefix(importPath, modulePath+"/tests/functional/") &&
		!strings.HasPrefix(importPath, modulePath+"/tests/functional/internal/")
}

func splitList(value string, separator string, filterEmpty bool) []string {
	parts := strings.Split(value, separator)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" && filterEmpty {
			continue
		}
		items = append(items, trimmed)
	}
	return items
}

func parseTotalCoverage(report string) (float64, string, error) {
	matches := totalCoveragePattern.FindStringSubmatch(report)
	if len(matches) != 2 {
		return 0, "", errors.New("parse go coverage total: missing total statements line")
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse go coverage percentage %q: %w", matches[1], err)
	}
	for _, line := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "total:") {
			return value, trimmed, nil
		}
	}
	return value, fmt.Sprintf("total: (statements) %s%%", matches[1]), nil
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var totalCoveragePattern = regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)

type config struct {
	covermode string
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
	flag.Float64Var(&cfg.min, "min", 0, "minimum total statement coverage percentage")
	flag.StringVar(&cfg.packages, "packages", "./...", "go test package pattern")
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

	testArgs := []string{"test"}
	if cfg.short {
		testArgs = append(testArgs, "-short")
	}
	testArgs = append(testArgs,
		fmt.Sprintf("-coverprofile=%s", profilePath),
		fmt.Sprintf("-covermode=%s", cfg.covermode),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
		cfg.packages,
	)

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

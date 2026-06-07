package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/readmecheck"
)

const defaultREADMEPath = "README.md"

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	root       string
	readmePath string
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
	flag.StringVar(&cfg.root, "root", ".", "repository root")
	flag.StringVar(&cfg.readmePath, "readme", defaultREADMEPath, "repository-relative README path")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	readmePath := filepath.Join(repoRoot, filepath.FromSlash(cfg.readmePath))
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("read README %s: %w", filepath.ToSlash(cfg.readmePath), err)
	}

	text := string(content)
	var problems []string

	for _, section := range readmecheck.MissingRequiredSections(text) {
		problems = append(problems, fmt.Sprintf("missing required section: %s", section))
	}

	for _, reference := range readmecheck.LocalReferencePaths(text) {
		target := filepath.Join(repoRoot, filepath.FromSlash(reference))
		info, statErr := os.Stat(target)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				problems = append(problems, fmt.Sprintf("missing local reference: %s", reference))
				continue
			}
			return fmt.Errorf("stat %s: %w", reference, statErr)
		}
		if strings.HasSuffix(reference, "/") && !info.IsDir() {
			problems = append(problems, fmt.Sprintf("local reference is not a directory: %s", reference))
		}
	}

	if len(problems) > 0 {
		slices.Sort(problems)
		for _, problem := range problems {
			fmt.Fprintln(stderr, "[agent-factory:readme-check]", problem)
		}
		return fmt.Errorf("[agent-factory:readme-check] found %d README issue(s)", len(problems))
	}

	fmt.Fprintf(stdout, "[agent-factory:readme-check] README structure and local references passed (%d sections, %d local paths)\n",
		len(readmecheck.TopLevelSections(text)),
		len(readmecheck.LocalReferencePaths(text)),
	)
	return nil
}

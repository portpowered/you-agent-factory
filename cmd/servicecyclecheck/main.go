// Command servicecyclecheck is the derived cross-service cycle ratchet.
//
// It derives the service list from the pkg/services directory tree, builds the
// weighted directed graph of non-test cross-service imports, computes the
// exact minimum feedback arc weight of that graph, and compares the weight
// against a ceiling recorded in a small dedicated baseline file.
//
// The comparison is a ratchet in both directions. A weight above the ceiling
// is a regression and fails. A weight below the ceiling is an unclaimed
// improvement and also fails, with instructions to lower the ceiling, so that
// untangling work is captured permanently instead of being silently undone by
// a later change.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	root        string
	ceilingFile string
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
	flag.StringVar(&cfg.ceilingFile, "ceiling-file", "", "path to the cycle ceiling baseline (defaults to the repository baseline under the scanned root)")
	flag.Parse()
	return cfg
}

// ceilingPath resolves the baseline location, defaulting to the repository
// baseline beneath the scanned root.
func (cfg config) ceilingPath() string {
	if cfg.ceilingFile != "" {
		return cfg.ceilingFile
	}
	return filepath.Join(cfg.root, defaultCeilingRelativePath)
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	graph, err := buildServiceGraph(cfg.root)
	if err != nil {
		return err
	}
	ceiling, err := loadCycleCeiling(cfg.ceilingPath())
	if err != nil {
		return err
	}
	solution, err := minimumFeedbackArcSet(graph.matrix())
	if err != nil {
		return err
	}

	edges := graph.cutSet(solution.ordering)
	if solution.weight == ceiling.Ceiling {
		writeSuccess(stdout, solution.weight, ceiling, edges, len(graph.services))
		return nil
	}
	return reportRatchetFailure(stderr, solution.weight, ceiling, edges)
}

// reportRatchetFailure prints the failing diagnosis, emits the machine
// readable violation count so the CI lint policy never classifies this target
// as unmeasured, and returns the summarizing error.
func reportRatchetFailure(stderr io.Writer, measured int, ceiling cycleCeiling, edges []backEdge) error {
	drift := measured - ceiling.Ceiling
	if drift < 0 {
		drift = -drift
	}
	if measured > ceiling.Ceiling {
		writeRegression(stderr, measured, ceiling, edges)
	} else {
		writeUnclaimedImprovement(stderr, measured, ceiling, edges)
	}
	fmt.Fprintf(stderr, "LINT_VIOLATION_COUNT: %d\n", drift)
	return fmt.Errorf(
		"%s minimum feedback arc weight %d does not match the recorded ceiling %d (drift %d)",
		diagnosticPrefix,
		measured,
		ceiling.Ceiling,
		drift,
	)
}

// Command testsleepcheck is a ratchet against flaky timing in Go tests.
//
// It parses test sources and test helpers with go/ast and finds three kinds of
// sites: time.Sleep calls, short literal wall-clock deadlines (time.After,
// time.NewTimer, time.AfterFunc, context.WithTimeout/WithDeadline with a
// literal duration of five seconds or less), and comparisons of measured
// elapsed time against a literal duration. Sites are counted per file,
// enclosing function, and kind and compared with a checked-in baseline. A count
// above the baseline fails; removing sites never fails. A site can be exempted
// in place with `//nolint:testsleep // reason` on the same or the preceding
// line.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const defaultBaselinePath = "docs/internal/baselines/test-sleep-deadline-baseline.json"

type config struct {
	root       string
	baseline   string
	regenerate bool
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.StringVar(&cfg.baseline, "baseline", defaultBaselinePath, "repository-relative baseline file")
	flag.BoolVar(&cfg.regenerate, "regenerate", false, "rewrite the baseline from the current tree")
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	root, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	baselinePath := filepath.Join(root, filepath.FromSlash(cfg.baseline))
	result, err := scanRepository(root)
	if err != nil {
		return err
	}
	if cfg.regenerate {
		if err := writeBaseline(baselinePath, buildBaseline(result.counts)); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "[agent-factory:test-sleep] wrote baseline with %d entries\n", len(result.counts))
		return nil
	}
	baseline, err := loadBaseline(baselinePath)
	if err != nil {
		return err
	}
	problems := append([]string{}, result.malformed...)
	problems = append(problems, compare(result, baseline)...)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(stderr, p)
		}
		fmt.Fprintf(stderr, "LINT_VIOLATION_COUNT: %d\n", len(problems))
		return fmt.Errorf("[agent-factory:test-sleep] %d new time.Sleep or fixed-deadline site(s); wait for an event or condition, use the injectable clock, or add `//nolint:testsleep // reason`", len(problems))
	}
	fmt.Fprintf(stdout, "[agent-factory:test-sleep] no new sleeps or fixed deadlines (%d baselined function/kind entries)\n", len(baseline))
	return nil
}

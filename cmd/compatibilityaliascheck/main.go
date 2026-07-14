package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

type config struct {
	root string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	terms, err := contractguard.LoadCompatibilityAliasTerms(cfg.root)
	if err != nil {
		return err
	}
	violations, err := contractguard.ScanCompatibilityAliasViolations(cfg.root, terms, contractguard.DefaultCompatibilityAliasBoundaryPrefixes)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		fmt.Fprintln(stdout, "[agent-factory:compatibility-alias] inventoried compatibility aliases are not adopted outside approved boundaries")
		return nil
	}
	for _, violation := range violations {
		fmt.Fprintf(
			stderr,
			"%s:%d:%d: compatibility alias %q (%s) must not be adopted in new internal code; use the canonical successor instead\n",
			violation.FilePath,
			violation.Line,
			violation.Column,
			violation.Term,
			violation.ItemID,
		)
	}
	return fmt.Errorf("[agent-factory:compatibility-alias] found %d prohibited compatibility alias adoption(s)", len(violations))
}

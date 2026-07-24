package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/migrationledgercheck"
)

type config struct {
	root           string
	ledgerPath     string
	checklistPath  string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root")
	flag.StringVar(&cfg.ledgerPath, "ledger", migrationledgercheck.DefaultLedgerPath, "machine-readable migration ledger path")
	flag.StringVar(&cfg.checklistPath, "checklist", migrationledgercheck.DefaultChecklistPath, "destination checklist path")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout, stderr io.Writer) error {
	if err := migrationledgercheck.Check(cfg.root, cfg.ledgerPath, cfg.checklistPath); err != nil {
		return err
	}
	live, err := migrationledgercheck.ScanLiveScenarios(cfg.root)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "[agent-factory:migration-ledger-check] %d customer scenarios mapped with valid destinations and deletion-only batches\n", len(live))
	return err
}

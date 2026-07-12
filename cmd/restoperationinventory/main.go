package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/contractinventory"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	inputPath  string
	outputPath string
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
	flag.StringVar(&cfg.inputPath, "input", "api/openapi.yaml", "OpenAPI YAML input path")
	flag.StringVar(&cfg.outputPath, "output", "-", "inventory JSON output path or - for stdout")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	data, err := os.ReadFile(cfg.inputPath)
	if err != nil {
		return fmt.Errorf("read openapi input %s: %w", cfg.inputPath, err)
	}

	inventory, err := contractinventory.ExtractFromOpenAPIYAML(data)
	if err != nil {
		return err
	}

	payload, err := contractinventory.MarshalCanonicalJSON(inventory)
	if err != nil {
		return err
	}

	if cfg.outputPath == "-" {
		_, err = stdout.Write(payload)
		return err
	}

	if err := os.WriteFile(cfg.outputPath, payload, 0o644); err != nil {
		return fmt.Errorf("write inventory output %s: %w", cfg.outputPath, err)
	}

	fmt.Fprintf(stderr, "[agent-factory:rest-operation-inventory] wrote %d operations to %s\n",
		len(inventory.Operations),
		cfg.outputPath,
	)
	return nil
}

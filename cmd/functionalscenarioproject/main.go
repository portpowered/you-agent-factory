package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/functionalscenarios"
)

type config struct {
	cliPath     string
	openAPIPath string
	mcpPath     string
	outputPath  string
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
	flag.StringVar(&cfg.cliPath, "cli", "contracts/cli/commands.json", "canonical CLI command inventory")
	flag.StringVar(&cfg.openAPIPath, "openapi", "api/openapi.yaml", "bundled OpenAPI contract")
	flag.StringVar(&cfg.mcpPath, "mcp", "contracts/mcp/tools.json", "canonical MCP tool inventory")
	flag.StringVar(&cfg.outputPath, "output", "-", "projection JSON output path or - for stdout")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout, stderr io.Writer) error {
	cliData, err := readInput("CLI", cfg.cliPath)
	if err != nil {
		return err
	}
	openAPIData, err := readInput("OpenAPI", cfg.openAPIPath)
	if err != nil {
		return err
	}
	mcpData, err := readInput("MCP", cfg.mcpPath)
	if err != nil {
		return err
	}
	projection, err := functionalscenarios.Project(cliData, openAPIData, mcpData)
	if err != nil {
		return fmt.Errorf("project functional scenario components: %w", err)
	}
	payload, err := functionalscenarios.MarshalCanonicalJSON(projection)
	if err != nil {
		return err
	}
	if cfg.outputPath == "-" {
		_, err = stdout.Write(payload)
		return err
	}
	if err := os.WriteFile(cfg.outputPath, payload, 0o644); err != nil {
		return fmt.Errorf("write projection %s: %w", cfg.outputPath, err)
	}
	_, _ = fmt.Fprintf(stderr, "[agent-factory:functional-scenario-project] wrote %d components to %s\n", len(projection.Components), cfg.outputPath)
	return nil
}

func readInput(label, path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s inventory %s: %w", label, path, err)
	}
	return data, nil
}

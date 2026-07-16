package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/functionalscenarios"
)

type config struct {
	repositoryRoot string
	cliPath        string
	openAPIPath    string
	mcpPath        string
	outputPath     string
	manifest       bool
	checkPath      string
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
	flag.StringVar(&cfg.repositoryRoot, "root", ".", "repository root used to resolve functional evidence")
	flag.StringVar(&cfg.cliPath, "cli", "contracts/cli/commands.json", "canonical CLI command inventory")
	flag.StringVar(&cfg.openAPIPath, "openapi", "api/openapi.yaml", "bundled OpenAPI contract")
	flag.StringVar(&cfg.mcpPath, "mcp", "contracts/mcp/tools.json", "canonical MCP tool inventory")
	flag.StringVar(&cfg.outputPath, "output", "-", "projection JSON output path or - for stdout")
	flag.BoolVar(&cfg.manifest, "manifest", false, "render the reviewed functional scenario manifest")
	flag.StringVar(&cfg.checkPath, "check", "", "validate a reviewed manifest against current canonical inventories")
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
	if cfg.checkPath != "" {
		return checkManifest(cfg.repositoryRoot, cfg.checkPath, projection, stdout)
	}
	payload, err := renderProjection(projection, cfg.manifest)
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

func checkManifest(repositoryRoot, path string, projection *functionalscenarios.Projection, stdout io.Writer) error {
	data, err := readInput("functional scenario manifest", path)
	if err != nil {
		return err
	}
	manifest, err := functionalscenarios.DecodeManifest(data)
	if err != nil {
		return err
	}
	if err := functionalscenarios.CheckManifest(projection, manifest); err != nil {
		return err
	}
	if err := functionalscenarios.CheckEvidenceReferences(repositoryRoot, manifest); err != nil {
		return err
	}
	canonical, err := functionalscenarios.MarshalCanonicalManifestJSON(manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("check functional scenario manifest %s: bytes are not canonically sorted and serialized; regenerate the reviewed manifest", path)
	}
	_, err = fmt.Fprintf(stdout, "[agent-factory:functional-scenario-check] %d reviewed scenarios are current\n", len(manifest.Scenarios))
	return err
}

func renderProjection(projection *functionalscenarios.Projection, manifest bool) ([]byte, error) {
	if !manifest {
		return functionalscenarios.MarshalCanonicalJSON(projection)
	}
	reviewed, err := functionalscenarios.BuildReviewedManifest(projection)
	if err != nil {
		return nil, err
	}
	return functionalscenarios.MarshalCanonicalManifestJSON(reviewed)
}

func readInput(label, path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s inventory %s: %w", label, path, err)
	}
	return data, nil
}

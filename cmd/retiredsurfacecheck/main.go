package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
	rootapp "github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
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
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	cliInventory, err := loadProductionCLIInventory()
	if err != nil {
		return err
	}
	docsRegistry := loadProductionDocsRegistry()
	mapper := func(factoriesRoot, name string) (string, error) {
		return namedfactorypath.MapDir(factoriesRoot, name)
	}

	violations, err := retiredsurfaceguard.ScanAllViolations(repoRoot, cliInventory, docsRegistry, mapper)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		fmt.Fprintln(stdout, "[agent-factory:retired-surface] retired command/docs surfaces, encoded-path production resolution, and secondary CLI public-shape definitions remain absent")
		return nil
	}
	for _, violation := range violations {
		fmt.Fprintln(stderr, retiredsurfaceguard.FormatViolation(violation))
	}
	return fmt.Errorf("[agent-factory:retired-surface] found %d retired-surface reintroduction violation(s)", len(violations))
}

func loadProductionCLIInventory() (retiredsurfaceguard.CLIInventory, error) {
	var observation cliobservation.Result
	process, err := rootapp.BuildProcess(context.Background(), serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	if err != nil {
		return retiredsurfaceguard.CLIInventory{}, fmt.Errorf("build production process: %w", err)
	}
	if err := process.Execute(rootapp.Input{
		Args: []string{"you"}, Env: os.Environ(), WorkingDirectory: ".",
		Stdout: io.Discard, Stderr: io.Discard,
	}); err != nil {
		return retiredsurfaceguard.CLIInventory{}, fmt.Errorf("observe production CLI tree: %w", err)
	}
	inventory := observation.Snapshot.Commands
	commands := make([]retiredsurfaceguard.CLICommandRecord, 0, len(inventory.Commands))
	for _, record := range inventory.Commands {
		commands = append(commands, retiredsurfaceguard.CLICommandRecord{
			Path:              record.Path,
			Aliases:           append([]string(nil), record.Aliases...),
			Visibility:        record.Visibility,
			Lifecycle:         record.Lifecycle,
			DeprecatedMessage: record.DeprecatedMessage,
		})
	}
	return retiredsurfaceguard.CLIInventory{Commands: commands}, nil
}

func loadProductionDocsRegistry() retiredsurfaceguard.DocsRegistry {
	indexEntries := make([]retiredsurfaceguard.DocsTopicEntry, 0, len(docscli.TopicIndexEntries()))
	for _, entry := range docscli.TopicIndexEntries() {
		indexEntries = append(indexEntries, retiredsurfaceguard.DocsTopicEntry{
			Name:    entry.Name,
			Aliases: append([]string(nil), entry.Aliases...),
		})
	}
	return retiredsurfaceguard.DocsRegistry{
		SupportedTopics:   docscli.SupportedTopics(),
		SupportedCommands: docscli.SupportedTopicCommands(),
		IndexEntries:      indexEntries,
	}
}

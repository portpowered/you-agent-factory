package factory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
)

// UpdateFromFileConfig holds parameters for replacing an existing named factory from disk.
type UpdateFromFileConfig struct {
	Name   string
	From   string
	Dir    string
	JSON   bool
	Output io.Writer
}

// UpdateFromFileResult reports a successful file-based factory update.
type UpdateFromFileResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
}

// UpdateFromFile replaces an existing named factory from a factory.json payload.
func UpdateFromFile(cfg UpdateFromFileConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return fmt.Errorf("factory name is required")
	}
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		return fmt.Errorf("--from is required")
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return fmt.Errorf("factory root is required")
	}

	payload, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read factory config %s: %w", from, err)
	}

	if _, err := configload.LoadFromCanonicalJSON(payload, configload.LoadOptions{}); err != nil {
		return renderUpdateFromFileError(err)
	}

	factoryDir, err := configpersist.ReplaceNamedFactory(cfg.Dir, name, payload)
	if err != nil {
		return renderUpdateFromFileError(err)
	}

	result := UpdateFromFileResult{
		Name:       name,
		FactoryDir: factoryDir,
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	return renderUpdateFromFileSuccess(result, cfg.Output)
}

func renderUpdateFromFileSuccess(result UpdateFromFileResult, output io.Writer) error {
	_, err := fmt.Fprintf(output, "Updated factory %s\nDirectory: %s\n", result.Name, result.FactoryDir)
	return err
}

func renderUpdateFromFileError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("factory not found: %w", err)
	}
	if configload.IsInvalidNamedFactory(err) || configpersist.IsInvalidNamedFactory(err) {
		return fmt.Errorf("invalid factory config: %w", err)
	}
	return err
}

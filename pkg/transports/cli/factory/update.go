package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// UpdateFromFileConfig holds parameters for replacing an existing named factory from disk.
type UpdateFromFileConfig struct {
	Context context.Context
	Name    string
	From    string
	Dir     string
	JSON    bool
	Output  io.Writer
}

// UpdateFromFileResult reports a successful file-based factory update.
type UpdateFromFileResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
}

// UpdateFromFile replaces an existing named factory from a factory.json payload.
func UpdateFromFile(cfg UpdateFromFileConfig) error {
	return UpdateFromFileWithServices(cfg, nil, nil)
}

// UpdateFromFileWithServices validates and persists through injected Factory
// Definitions root capabilities.
func UpdateFromFileWithServices(
	cfg UpdateFromFileConfig,
	persist factorydefinitions.NamedFactoryPersistenceOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) error {
	if cfg.Output == nil {
		return fmt.Errorf("factory update output is required")
	}

	result, err := persistFromFile(persistFromFileConfig{
		Context: cfg.Context,
		Mode:    persistFromFileModeUpdate,
		Name:    cfg.Name,
		From:    cfg.From,
		Dir:     cfg.Dir,
	}, persist, loadSource)
	if err != nil {
		return renderPersistFromFileError(persistFromFileModeUpdate, err)
	}

	updateResult := UpdateFromFileResult{
		Name:       result.Name,
		FactoryDir: result.FactoryDir,
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(updateResult)
	}
	return renderUpdateFromFileSuccess(updateResult, cfg.Output)
}

func renderUpdateFromFileSuccess(result UpdateFromFileResult, output io.Writer) error {
	_, err := fmt.Fprintf(output, "Updated factory %s\nDirectory: %s\n", result.Name, result.FactoryDir)
	return err
}

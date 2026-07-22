package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// CreateFromFileConfig holds parameters for creating a new named factory from disk.
type CreateFromFileConfig struct {
	Context    context.Context
	Name       string
	From       string
	Dir        string
	SetCurrent bool
	JSON       bool
	Output     io.Writer
}

// CreateFromFileResult reports a successful file-based named-factory create.
type CreateFromFileResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
}

// CreateFromFile creates a new named factory from a factory.json payload.
func CreateFromFile(cfg CreateFromFileConfig) error {
	return CreateFromFileWithServices(cfg, nil, nil)
}

// CreateFromFileWithServices validates and persists through injected Factory
// Definitions root capabilities.
func CreateFromFileWithServices(
	cfg CreateFromFileConfig,
	persist factorydefinitions.NamedFactoryPersistenceOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) error {
	if cfg.Output == nil {
		return fmt.Errorf("factory create output is required")
	}

	result, err := persistFromFile(persistFromFileConfig{
		Context:    cfg.Context,
		Mode:       persistFromFileModeCreate,
		Name:       cfg.Name,
		From:       cfg.From,
		Dir:        cfg.Dir,
		SetCurrent: cfg.SetCurrent,
	}, persist, loadSource)
	if err != nil {
		return renderPersistFromFileError(persistFromFileModeCreate, err)
	}

	createResult := CreateFromFileResult{
		Name:       result.Name,
		FactoryDir: result.FactoryDir,
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(createResult)
	}
	return renderCreateFromFileSuccess(createResult, cfg.Output)
}

func renderCreateFromFileSuccess(result CreateFromFileResult, output io.Writer) error {
	_, err := fmt.Fprintf(output, "Created factory %s\nDirectory: %s\n", result.Name, result.FactoryDir)
	return err
}

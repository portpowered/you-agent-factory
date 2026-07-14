package factory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// CreateFromFileConfig holds parameters for creating a new named factory from disk.
type CreateFromFileConfig struct {
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
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	result, err := persistFromFile(persistFromFileConfig{
		Mode:       persistFromFileModeCreate,
		Name:       cfg.Name,
		From:       cfg.From,
		Dir:        cfg.Dir,
		SetCurrent: cfg.SetCurrent,
	})
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

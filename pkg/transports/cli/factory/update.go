package factory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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

	result, err := persistFromFile(persistFromFileConfig{
		Mode: persistFromFileModeUpdate,
		Name: cfg.Name,
		From: cfg.From,
		Dir:  cfg.Dir,
	})
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

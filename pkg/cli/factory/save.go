package factory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// SaveFromFileConfig holds parameters for persisting a new named factory from disk.
type SaveFromFileConfig struct {
	Name       string
	From       string
	Dir        string
	SetCurrent bool
	JSON       bool
	Output     io.Writer
}

// SaveFromFileResult reports a successful file-based factory save.
type SaveFromFileResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDir"`
}

// SaveFromFile creates a new named factory from a factory.json payload.
func SaveFromFile(cfg SaveFromFileConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	result, err := persistFromFile(persistFromFileConfig{
		Mode:       persistFromFileModeSave,
		Name:       cfg.Name,
		From:       cfg.From,
		Dir:        cfg.Dir,
		SetCurrent: cfg.SetCurrent,
	})
	if err != nil {
		return renderPersistFromFileError(persistFromFileModeSave, err)
	}

	saveResult := SaveFromFileResult{
		Name:       result.Name,
		FactoryDir: result.FactoryDir,
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(saveResult)
	}
	return renderSaveFromFileSuccess(saveResult, cfg.Output)
}

func renderSaveFromFileSuccess(result SaveFromFileResult, output io.Writer) error {
	_, err := fmt.Fprintf(output, "Saved factory %s\nDirectory: %s\n", result.Name, result.FactoryDir)
	return err
}

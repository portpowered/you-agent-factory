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
		return renderSaveFromFileError(err)
	}

	factoryDir, err := configpersist.PersistNamedFactory(cfg.Dir, name, payload)
	if err != nil {
		return renderSaveFromFileError(err)
	}

	if cfg.SetCurrent {
		if err := configpersist.WriteCurrentFactoryPointer(cfg.Dir, name); err != nil {
			return err
		}
	}

	result := SaveFromFileResult{
		Name:       name,
		FactoryDir: factoryDir,
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	return renderSaveFromFileSuccess(result, cfg.Output)
}

func renderSaveFromFileSuccess(result SaveFromFileResult, output io.Writer) error {
	_, err := fmt.Fprintf(output, "Saved factory %s\nDirectory: %s\n", result.Name, result.FactoryDir)
	return err
}

func renderSaveFromFileError(err error) error {
	if errors.Is(err, configpersist.ErrNamedFactoryAlreadyExists) {
		return fmt.Errorf("factory already exists: %w", err)
	}
	if configload.IsInvalidNamedFactory(err) || configpersist.IsInvalidNamedFactory(err) {
		return fmt.Errorf("invalid factory config: %w", err)
	}
	return err
}

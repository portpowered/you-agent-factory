package scaffoldfacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type factoryConfigName struct {
	Name string `json:"name"`
}

// NewFactoryNameResolver reads the scaffolded factory aggregate name from the
// materialized factory.json in factoryDir using the injected read operation.
func NewFactoryNameResolver(readFile func(path string) ([]byte, error)) func(string) (string, error) {
	return func(factoryDir string) (string, error) {
		if readFile == nil {
			return "", fmt.Errorf("factory config reader is required")
		}
		path := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
		data, err := readFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		var cfg factoryConfigName
		if err := json.Unmarshal(data, &cfg); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
		name := strings.TrimSpace(cfg.Name)
		if name == "" {
			return "", fmt.Errorf("%s: name is required", path)
		}
		return name, nil
	}
}

// LocalFactoryNameResolver uses os.ReadFile for tests and owner composition.
func LocalFactoryNameResolver() func(string) (string, error) {
	return NewFactoryNameResolver(os.ReadFile)
}

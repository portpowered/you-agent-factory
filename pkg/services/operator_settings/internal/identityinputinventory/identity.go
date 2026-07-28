package identityinputinventory

import (
	"fmt"
	"path/filepath"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// EnsureLocalBackendScope loads backendScopeID from configPath, generates
// local-<uuid> when missing, and persists a newly generated value before returning.
func EnsureLocalBackendScope(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	generateID operatorsettings.IDGenerator,
	decode operatorsettings.ConfigDecoder,
	encode operatorsettings.ConfigEncoder,
	configPath string,
) (operatorsettings.ResolvedBackendScope, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf("system config path is required to resolve backend scope")
	}

	config, err := operatorsettings.LoadFileConfig(files, decode, configPath)
	if err != nil {
		return operatorsettings.ResolvedBackendScope{}, err
	}
	if trimmed := strings.TrimSpace(config.BackendScopeID); trimmed != "" {
		return operatorsettings.ResolvedBackendScope{
			BackendScopeID: trimmed,
			Outcome:        operatorsettings.BackendScopeOutcomeReused,
			ConfigPath:     configPath,
		}, nil
	}

	generated := operatorsettings.GenerateLocalBackendScopeID(generateID)
	config.BackendScopeID = generated
	if err := persistBackendScopeID(files, createTemp, encode, configPath, config); err != nil {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf(
			"persist generated backend scope ID to system config %q: %w; local backends require a stable backendScopeID before exposing session identity",
			configPath,
			err,
		)
	}
	return operatorsettings.ResolvedBackendScope{
		BackendScopeID: generated,
		Outcome:        operatorsettings.BackendScopeOutcomeGenerated,
		ConfigPath:     configPath,
	}, nil
}

func persistBackendScopeID(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	encode operatorsettings.ConfigEncoder,
	configPath string,
	config operatorsettings.Config,
) error {
	backendScopeID := strings.TrimSpace(config.BackendScopeID)
	if backendScopeID == "" {
		return fmt.Errorf("backend scope ID is required")
	}
	if !operatorsettings.IsLocalBackendScopeID(backendScopeID) {
		return fmt.Errorf("backend scope ID %q is not a valid local backend scope", backendScopeID)
	}

	if encode == nil {
		return fmt.Errorf("global config encoder is required")
	}
	data, err := encode(config)
	if err != nil {
		return err
	}
	return writeConfig(files, createTemp, configPath, data)
}

func writeConfig(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	configPath string,
	data []byte,
) error {
	dir := filepath.Dir(configPath)
	if err := files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}

	tmp, err := createTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = files.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync system config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close system config temp file: %w", err)
	}
	if err := files.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set system config temp file permissions: %w", err)
	}
	if err := files.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("replace system config with temp file: %w", err)
	}
	cleanupTemp = false
	return nil
}

package service

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var localBackendScopePattern = regexp.MustCompile(
	`^local-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

func isLocalBackendScopeID(value string) bool {
	return localBackendScopePattern.MatchString(strings.TrimSpace(value))
}

func (s *Service) ensureLocalBackendScope(configPath string) (operatorsettings.ResolvedBackendScope, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf("system config path is required to resolve backend scope")
	}
	if s.files == nil {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf("operator settings filesystem is required")
	}
	config, err := s.LoadFileConfig(configPath)
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
	if s.idGenerator == nil {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf("operator settings ID generator is required")
	}
	generated := operatorsettings.LocalBackendScopePrefix + s.idGenerator()
	config.BackendScopeID = generated
	if err := s.persistBackendScopeID(configPath, config); err != nil {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf(
			"persist generated backend scope ID to system config %q: %w; local backends require a stable backendScopeID before exposing session identity",
			configPath, err,
		)
	}
	return operatorsettings.ResolvedBackendScope{
		BackendScopeID: generated,
		Outcome:        operatorsettings.BackendScopeOutcomeGenerated,
		ConfigPath:     configPath,
	}, nil
}

func (s *Service) persistBackendScopeID(configPath string, config operatorsettings.Config) error {
	if strings.TrimSpace(config.BackendScopeID) == "" {
		return fmt.Errorf("backend scope ID is required")
	}
	if !isLocalBackendScopeID(config.BackendScopeID) {
		return fmt.Errorf("backend scope ID %q is not a valid local backend scope", config.BackendScopeID)
	}
	if s.encoder == nil {
		return fmt.Errorf("global config encoder is required")
	}
	if s.createTemp == nil {
		return fmt.Errorf("operator settings temporary-file creator is required")
	}
	data, err := s.encoder(config)
	if err != nil {
		return err
	}
	dir := filepath.Dir(configPath)
	if err := s.files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}
	tmp, err := s.createTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.files.Remove(tmpPath)
		}
	}()
	if written, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", err)
	} else if written != len(data) {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", io.ErrShortWrite)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync system config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close system config temp file: %w", err)
	}
	if err := s.files.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set system config temp file permissions: %w", err)
	}
	if err := s.files.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("replace system config with temp file: %w", err)
	}
	cleanup = false
	return nil
}

package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"go.uber.org/zap"
)

func ensureServiceBackendScope(cfg *FactoryServiceConfig, logger *zap.Logger) error {
	if cfg == nil {
		return fmt.Errorf("factory service config is required to resolve backend scope")
	}
	if strings.TrimSpace(cfg.ReplayPath) != "" {
		return nil
	}
	if strings.TrimSpace(cfg.BackendScopeID) != "" {
		return nil
	}

	configPath, err := resolveSystemConfigPath(cfg)
	if err != nil {
		return err
	}
	resolved, err := systemconfig.EnsureLocalBackendScope(configPath)
	if err != nil {
		return err
	}
	cfg.BackendScopeID = resolved.BackendScopeID
	if logger != nil {
		logger.Info("resolved backend scope for local backend", zap.String("diagnostics", resolved.DiagnosticsLine()))
	}
	return nil
}

func resolveSystemConfigPath(cfg *FactoryServiceConfig) (string, error) {
	if cfg != nil && strings.TrimSpace(cfg.SystemConfigPath) != "" {
		return strings.TrimSpace(cfg.SystemConfigPath), nil
	}
	homeDir := ""
	if cfg != nil {
		homeDir = strings.TrimSpace(cfg.SystemConfigHomeDir)
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for backend scope system config: %w", err)
		}
	}
	return defaultpaths.OperatorConfigPath(homeDir), nil
}

func serviceBackendScopeID(cfg *FactoryServiceConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.BackendScopeID)
}

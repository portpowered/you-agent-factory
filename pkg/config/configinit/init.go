// Package configinit implements the canonical you config init system bootstrap.
package configinit

import (
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
)

// SystemConfigOutcome reports whether init created or skipped operator/system config.
type SystemConfigOutcome string

const (
	SystemConfigCreated SystemConfigOutcome = "created"
	SystemConfigSkipped SystemConfigOutcome = "skipped"
)

// Result summarizes a successful you config init run.
type Result struct {
	HomeDir             string
	ConfigPath          string
	SystemConfigOutcome SystemConfigOutcome
}

// Init ensures operator/system config exists under homeDir without overwriting
// an already-present config file.
func Init(homeDir string) (Result, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return Result{}, fmt.Errorf("home directory is required")
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if _, err := os.Stat(configPath); err == nil {
		if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("read existing operator config %q: %w", configPath, err)
		}
		return Result{
			HomeDir:             homeDir,
			ConfigPath:          configPath,
			SystemConfigOutcome: SystemConfigSkipped,
		}, nil
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("stat operator config %q: %w", configPath, err)
	}

	if _, err := systemconfig.EnsureLocalBackendScope(configPath); err != nil {
		return Result{}, fmt.Errorf("create system config at %q: %w", configPath, err)
	}
	if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
		return Result{}, fmt.Errorf("validate created operator config %q: %w", configPath, err)
	}

	return Result{
		HomeDir:             homeDir,
		ConfigPath:          configPath,
		SystemConfigOutcome: SystemConfigCreated,
	}, nil
}

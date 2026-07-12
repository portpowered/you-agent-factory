// Package configinit implements the canonical you config init system bootstrap.
package configinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
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

// PackagedFactoryOutcome reports whether init created or skipped one packaged
// default factory.
type PackagedFactoryOutcome string

const (
	PackagedFactoryCreated PackagedFactoryOutcome = "created"
	PackagedFactorySkipped PackagedFactoryOutcome = "skipped"
)

// PackagedFactoryResult summarizes one packaged default factory ensure
// operation during you config init.
type PackagedFactoryResult struct {
	Name       string
	FactoryDir string
	Outcome    PackagedFactoryOutcome
}

// Result summarizes a successful you config init run.
type Result struct {
	HomeDir              string
	ConfigPath           string
	NamedFactoriesRoot   string
	SystemConfigOutcome  SystemConfigOutcome
	PackagedFactories    []PackagedFactoryResult
}

// Init ensures operator/system config exists under homeDir without overwriting
// an already-present config file.
func Init(homeDir string) (Result, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return Result{}, fmt.Errorf("home directory is required")
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)

	if err := ensureSystemConfigParentIsDirectory(configPath); err != nil {
		return Result{}, err
	}

	systemConfigOutcome := SystemConfigCreated
	if _, err := os.Stat(configPath); err == nil {
		if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("read existing operator config %q: %w", configPath, err)
		}
		systemConfigOutcome = SystemConfigSkipped
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("stat operator config %q: %w", configPath, err)
	} else {
		if _, err := systemconfig.EnsureLocalBackendScope(configPath); err != nil {
			return Result{}, fmt.Errorf("create system config at %q: %w", configPath, err)
		}
		if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("validate created operator config %q: %w", configPath, err)
		}
	}

	packagedFactories, err := ensurePackagedDefaultFactories(namedFactoriesRoot)
	if err != nil {
		return Result{}, err
	}

	return Result{
		HomeDir:             homeDir,
		ConfigPath:          configPath,
		NamedFactoriesRoot:  namedFactoriesRoot,
		SystemConfigOutcome: systemConfigOutcome,
		PackagedFactories:   packagedFactories,
	}, nil
}

func ensureSystemConfigParentIsDirectory(configPath string) error {
	parentDir := filepath.Dir(configPath)
	info, err := os.Stat(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("create system config at %s: inspect parent path %s: %w", configPath, parentDir, err)
	}
	if info.IsDir() {
		return nil
	}
	return fmt.Errorf(
		"create system config at %s: parent path %s exists but is not a directory; remove or rename it and retry",
		configPath,
		parentDir,
	)
}

func ensurePackagedDefaultFactories(namedFactoriesRoot string) ([]PackagedFactoryResult, error) {
	factoryResults, err := factoryconfig.EnsureBuiltInNamedFactories(namedFactoriesRoot)
	if err != nil {
		return nil, err
	}

	packagedFactories := make([]PackagedFactoryResult, 0, len(factoryResults))
	for _, factoryResult := range factoryResults {
		outcome := PackagedFactorySkipped
		if factoryResult.Outcome == factoryconfig.BuiltInNamedFactoryCreated {
			outcome = PackagedFactoryCreated
		}
		packagedFactories = append(packagedFactories, PackagedFactoryResult{
			Name:       factoryResult.Name,
			FactoryDir: factoryResult.FactoryDir,
			Outcome:    outcome,
		})
	}
	return packagedFactories, nil
}

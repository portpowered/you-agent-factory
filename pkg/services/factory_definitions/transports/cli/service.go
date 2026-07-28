package cli

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service exposes Factory Definitions CLI command operations to Cobra composition.
type Service interface {
	InstallPackagedFactory(InstallPackagedFactoryConfig) error
}

type service struct {
	install InstallPackagedFactoryOperation
}

// New constructs the Factory Definitions CLI service from the accepted
// Definitions-root packaged-installation operation.
func New(install InstallPackagedFactoryOperation) Service {
	if install == nil {
		return nil
	}
	return &service{install: install}
}

func (service *service) InstallPackagedFactory(cfg InstallPackagedFactoryConfig) error {
	if cfg.Output == nil {
		return fmt.Errorf("packaged factory installation output is required")
	}
	if strings.TrimSpace(cfg.Package) == "" {
		return fmt.Errorf("package name is required")
	}
	rootDir, err := resolveInstallRootDir(cfg)
	if err != nil {
		return err
	}
	format, err := parseInstallFormat(cfg.Format, cfg.FormatChanged)
	if err != nil {
		return err
	}
	diagnosticPrintf(
		cfg.Diagnostics,
		cfg.Verbose,
		"init packaged factory request name=%s rootDir=%s format=%s replace=%t",
		cfg.Package,
		rootDir,
		format,
		cfg.Replace,
	)
	result, err := service.install(
		cfg.Context,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: rootDir,
			Name:    cfg.Package,
			Format:  format,
			Replace: cfg.Replace,
		},
	)
	if err != nil {
		diagnosticPrintf(
			cfg.Diagnostics,
			cfg.Verbose,
			"init packaged factory failed name=%s rootDir=%s",
			cfg.Package,
			rootDir,
		)
		return renderInstallPackagedFactoryError(err)
	}
	diagnosticPrintf(
		cfg.Diagnostics,
		cfg.Verbose,
		"init packaged factory complete name=%s rootDir=%s outcome=%s factoryDir=%s format=%s",
		result.Definition.Name,
		rootDir,
		result.Outcome,
		result.Definition.FactoryDir,
		result.Format,
	)
	return renderInstallPackagedFactorySuccess(cfg, result)
}

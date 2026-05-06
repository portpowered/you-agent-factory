package config

import "github.com/portpowered/infinite-you/pkg/interfaces"

func WriteExpandedFactoryLayoutForTest(
	sourceDir, targetDir string,
	cfg *interfaces.FactoryConfig,
	canonical []byte,
	sourcePath string,
) error {
	_, err := writeExpandedFactoryLayout(sourceDir, targetDir, cfg, canonical, sourcePath)
	return err
}

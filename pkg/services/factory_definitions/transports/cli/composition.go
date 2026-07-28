package cli

// BindInstallPackagedFactory returns the composition-facing operation closure
// that delegates packaged Factory installation to the Definitions-owned CLI
// adapter Service. Wire and other composition roots inject the returned
// function without constructing the Service at the composition boundary.
func BindInstallPackagedFactory(install InstallPackagedFactoryOperation) func(InstallPackagedFactoryConfig) error {
	if install == nil {
		return nil
	}
	return func(cfg InstallPackagedFactoryConfig) error {
		return InstallPackagedFactory(cfg, install)
	}
}

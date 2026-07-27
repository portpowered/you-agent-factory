package factorydefinitions

import "context"

type PackagedFactoryInstallOutcome string

const (
	PackagedFactoryInstallCreated PackagedFactoryInstallOutcome = "created"
	PackagedFactoryInstallSkipped PackagedFactoryInstallOutcome = "skipped"
)

type PackagedFactoryInstallResult struct {
	Name       string
	FactoryDir string
	Outcome    PackagedFactoryInstallOutcome
	Format     PackagedFactoryFormat
}

// PackagedFactoryInstaller owns validation and persistence of the packaged
// Factory catalog into one named-Factory root.
type PackagedFactoryInstaller interface {
	EnsurePackagedFactories(
		context.Context,
		string,
		[]PackagedDefinition,
	) ([]PackagedFactoryInstallResult, error)
}

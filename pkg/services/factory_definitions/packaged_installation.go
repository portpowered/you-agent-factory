package factorydefinitions

import "context"

type PackagedFactoryInstallOutcome string

const (
	PackagedFactoryInstallCreated  PackagedFactoryInstallOutcome = "created"
	PackagedFactoryInstallSkipped  PackagedFactoryInstallOutcome = "skipped"
	PackagedFactoryInstallReplaced PackagedFactoryInstallOutcome = "replaced"
)

// PackagedFactoryInstallParams carries one Definitions-owned packaged
// installation request after catalog selection has returned a detached
// definition.
type PackagedFactoryInstallParams struct {
	NamedFactoriesRoot string
	Definition         PackagedDefinition
	Format             PackagedFactoryFormat
	Replace            bool
}

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

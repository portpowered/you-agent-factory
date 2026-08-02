package factorydefinitions

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

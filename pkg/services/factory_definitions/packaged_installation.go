package factorydefinitions

import "context"

type PackagedFactoryInstallOutcome string

const (
	PackagedFactoryInstallCreated          PackagedFactoryInstallOutcome = "created"
	PackagedFactoryInstallSkipped          PackagedFactoryInstallOutcome = "skipped"
	PackagedFactoryInstallReplaced         PackagedFactoryInstallOutcome = "replaced"
	PackagedFactoryInstallCurrent          PackagedFactoryInstallOutcome = "current"
	PackagedFactoryInstallRefreshed        PackagedFactoryInstallOutcome = "refreshed"
	PackagedFactoryInstallCustomerModified PackagedFactoryInstallOutcome = "customer-modified"
	PackagedFactoryInstallFailed           PackagedFactoryInstallOutcome = "failed"
)

// PackagedFactoryInstallParams carries one Definitions-owned packaged
// installation request after catalog selection has returned a detached
// definition.
type PackagedFactoryInstallParams struct {
	NamedFactoriesRoot string
	BackendScopeID     string
	Definition         PackagedDefinition
	Format             PackagedFactoryFormat
	Replace            bool
	// ManagedRefresh is set only by the system-initialization ensure route.
	// Direct installation retains its historical create/skip/replace behavior;
	// managed installation may reconcile stale materialized content.
	ManagedRefresh bool
}

type PackagedFactoryInstallResult struct {
	Name               string
	FactoryDir         string
	Outcome            PackagedFactoryInstallOutcome
	Format             PackagedFactoryFormat
	BackupDir          string
	PublishedContentID string
	InstalledContentID string
}

// PackagedFactoryInstaller owns validation and persistence of the packaged
// Factory catalog into one named-Factory root.
type PackagedFactoryInstaller interface {
	EnsurePackagedFactories(
		context.Context,
		string,
		string,
		[]PackagedDefinition,
	) ([]PackagedFactoryInstallResult, error)
}

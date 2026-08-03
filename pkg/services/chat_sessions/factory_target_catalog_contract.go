package chatsessions

import "context"

// FactoryTargetCatalogService is the singular Chat Sessions detached
// operation that resolves the allowed, installed FACTORY target catalog for
// an ACP client picker. It combines the effective Operator Settings ACP
// Agent profile with Factory Definitions' public installed-catalog behavior
// so callers never assemble this cross-service policy themselves. This
// package publishes only the interface and its detached request/result
// values; it has no implementation, persistence, or dependency-injection
// wiring.
//
// Every resolution reads current collaborator state: it never persists,
// caches, or returns a result that can be affected by caller mutation of the
// returned value or by anything but the next call's collaborator facts.
type FactoryTargetCatalogService interface {
	// ResolveFactoryTargetCatalog returns the FACTORY targets that are both
	// allowed by the effective ACP Agent profile and currently installed,
	// plus exactly one current/default target drawn from that same set. It
	// reports *FactoryTargetCatalogError when the effective profile cannot
	// be resolved, the installed catalog cannot be read, or no requested or
	// configured current target belongs to the allowed/installed
	// intersection.
	ResolveFactoryTargetCatalog(ctx context.Context, req ResolveFactoryTargetCatalogRequest) (ResolveFactoryTargetCatalogResult, error)
}

// FactoryDiscoveryRoots selects the project-local and global roots used to
// read the installed Factory catalog, mirroring Factory Definitions'
// ListEffectiveFactoriesRequest root pair.
type FactoryDiscoveryRoots struct {
	ProjectRoot string
	GlobalRoot  string
}

// ResolveFactoryTargetCatalogRequest carries the inputs required to resolve
// one Factory target catalog.
type ResolveFactoryTargetCatalogRequest struct {
	// OperatorSettingsPath identifies the operator document the effective
	// ACP Agent profile is resolved from.
	OperatorSettingsPath string
	// FactoryDiscovery selects the roots the installed Factory catalog is
	// read from.
	FactoryDiscovery FactoryDiscoveryRoots
	// ClientWorkingRoot is the ACP client's own working root, used to
	// validate a Factory that pins an incompatible working root.
	ClientWorkingRoot string
	// CurrentTarget, when non-blank, is a caller-supplied canonical
	// unversioned factory:<ref> validated in place of the effective
	// profile's configured default target.
	CurrentTarget string
}

// FactoryTargetCatalogChoice is one selectable FACTORY target. Value is its
// canonical unversioned factory:<ref> and Name is a non-empty stable display
// name derived from Factory Definitions-owned canonical identity facts.
type FactoryTargetCatalogChoice struct {
	Value string
	Name  string
}

// ResolveFactoryTargetCatalogResult carries the deterministic, deduplicated,
// stably ordered FACTORY choice list and the one current/default target
// selected from it. It is detached from collaborator-owned slices: mutating
// it never affects a subsequent resolution.
type ResolveFactoryTargetCatalogResult struct {
	Choices       []FactoryTargetCatalogChoice
	CurrentTarget string
}

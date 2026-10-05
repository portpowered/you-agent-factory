package main

import (
	"fmt"
	"slices"
	"strings"
)

const defaultScanRoot = "pkg"
const applicationGraphImportPath = "github.com/portpowered/infinite-you/pkg/wire"
const repositoryImportPrefix = "github.com/portpowered/infinite-you/"
const serviceConstructionBaselinePath = "docs/internal/baselines/service-construction-baseline.json"
const serviceConstructionBaselineStage = "wire-injection-full-blow"
const serviceConstructionDeletionGate = "inject the already-constructed service role from pkg/wire or move the invariant to the owning service"

var serviceConstructionPrefixes = []string{"New", "Build", "Create", "Ensure", "Open", "Provide"}

// allowedServiceValueConstructionSymbols is an exact, reviewed inventory of
// pure values, errors, IDs, results, and projections whose names happen to
// look like dependency-graph construction. Any other construction-shaped
// service-root symbol is denied by default outside its owner and pkg/wire.
var allowedServiceValueConstructionSymbols = map[string]map[string]struct{}{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions": {
		"NewBlockingFactoryLoadError": {},
		"NewFactoryEvent":             {},
		"NewFactorySnapshot":          {},
	},
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime": {
		"NewEngineStateSnapshot": {},
	},
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions": {
		"BuildProjectionContext":          {},
		"BuildTargetFromConfig":           {},
		"NewLogicalTargetValidationError": {},
		"NewSessionID":                    {},
	},
	"github.com/portpowered/infinite-you/pkg/services/operator_settings": {
		"EnsureLocalBackendScope": {},
	},
	"github.com/portpowered/infinite-you/pkg/services/recordings": {
		"BuildFactoryWorldWorkstationRequestProjectionSlice": {},
		"BuildPortableRecording":                             {},
	},
	"github.com/portpowered/infinite-you/pkg/services/workers": {
		"NewCapabilities":           {},
		"NewEmptyMockWorkersConfig": {},
		"NewProviderError":          {},
	},
	"github.com/portpowered/infinite-you/pkg/services/providers/wire": {
		"NewCapabilitySet": {},
	},
}

var factoryRetiredPackageRoots = []retiredPackageRoot{
	{packagePath: "pkg/factory", canonicalOwner: "pkg/services/factory_definitions, pkg/services/factory_sessions, pkg/services/factory_runtime, or pkg/services/recordings according to ownership"},
	{packagePath: "pkg/packagedfactories", canonicalOwner: "pkg/services/factory_definitions/internal/services/distribution"},
	{packagePath: "pkg/factorydefinition", canonicalOwner: "pkg/services/factory_definitions/definition"},
	{packagePath: "pkg/factorysessionexecution", canonicalOwner: "pkg/services/factory_sessions"},
	{packagePath: "pkg/factorysessions", canonicalOwner: "pkg/services/factory_sessions"},
	{packagePath: "pkg/petri", canonicalOwner: "pkg/services/factory_runtime"},
}

var retiredPackageRoots = append([]retiredPackageRoot{
	{packagePath: "pkg/api", canonicalOwner: "pkg/transports/http"},
	{packagePath: "pkg/apisurface", canonicalOwner: "pkg/transports/mapping"},
	{packagePath: "pkg/cli", canonicalOwner: "pkg/transports/cli"},
	{packagePath: "pkg/transports/cli/startup", canonicalOwner: "pkg/initializer/process"},
	{packagePath: "pkg/services/factory_definitions/contracts", canonicalOwner: "pkg/services/factory_definitions"},
	{packagePath: "pkg/platform/namedfactorypath", canonicalOwner: "pkg/services/factory_definitions"},
	{packagePath: "pkg/platform/defaultpaths", canonicalOwner: "the defining service owner, or pkg/platform/internal/runtimeartifact for policy-free artifact mechanics"},
	{packagePath: "pkg/wire/runtimeproviders", canonicalOwner: "focused provider files in pkg/wire"},
	{packagePath: "pkg/generatedclient", canonicalOwner: "pkg/transports/http/client"},
	{packagePath: "pkg/hostedworkers", canonicalOwner: "Automation Hosted Sources (hosted polling / observation, secret resolution for observation, poll/restart/checkpoint, observation normalization, and commanding Work admission) or Workers Hosted Runner (remote Work execution request/result, execution lifecycle observation, cancellation, and normalized execution outcome under the Runner contract); transitional pkg/services/workers/services/hosted_logic location alone is not durable ownership"},
	{packagePath: "pkg/internal/cursorstorage", canonicalOwner: "pkg/services/provider_sessions/internal/services/cursor_reader/internal/cursor"},
	{packagePath: "pkg/internal/metrics", canonicalOwner: "pkg/services/factory_runtime/internal/services/orchestration/metrics for domain contracts and pkg/platform/metrics for file-backed recording"},
	{packagePath: "pkg/platform/runtimeinput", canonicalOwner: "bounded owner requests assembled by pkg/wire"},
	{packagePath: "pkg/invocations", canonicalOwner: "pkg/services/work, pkg/services/factory_sessions, or pkg/services/workers, according to the concern"},
	{packagePath: "pkg/interfaces", canonicalOwner: "the defining domain under pkg/services"},
	{packagePath: "pkg/localmodels", canonicalOwner: "pkg/services/models"},
	{packagePath: "pkg/logging", canonicalOwner: "pkg/platform/logging"},
	{packagePath: "pkg/materialize", canonicalOwner: "pkg/services/work"},
	{packagePath: "pkg/mcp", canonicalOwner: "pkg/transports/mcp"},
	{packagePath: "pkg/modelhost", canonicalOwner: "pkg/services/models"},
	{packagePath: "pkg/models", canonicalOwner: "pkg/services/models"},
	{packagePath: "pkg/orchestrators", canonicalOwner: "pkg/services/factory_runtime"},
	{packagePath: "pkg/replay", canonicalOwner: "pkg/services/recordings/replay for Factory-event replay policy and pkg/platform/replay for artifact filesystem mechanics"},
	{packagePath: "pkg/service", canonicalOwner: "pkg/services for product services and pkg/wire for composition"},
	{packagePath: "pkg/sessionpersistence", canonicalOwner: "pkg/services/factory_sessions/internal/cursors/persistence"},
	{packagePath: "pkg/services/provider_sessions/cursor/persistence", canonicalOwner: "pkg/services/factory_sessions/internal/cursors/persistence"},
	{packagePath: "pkg/services/factory_sessions/internal/execution/testharness", canonicalOwner: "owner-local _test.go construction in pkg/services/factory_sessions/internal/execution"},
	{packagePath: "pkg/testutil", canonicalOwner: "internal/testutil or package-local test helpers"},
	{packagePath: "pkg/timework", canonicalOwner: "pkg/services/automations/internal/services/cron"},
	{packagePath: "pkg/services/automations/timework", canonicalOwner: "pkg/services/automations/internal/services/cron"},
	{packagePath: "pkg/work", canonicalOwner: "pkg/services/work"},
	{packagePath: "pkg/workcontent", canonicalOwner: "pkg/services/work"},
	{packagePath: "pkg/workers", canonicalOwner: "pkg/services/workers"},
	{packagePath: "pkg/services/automation", canonicalOwner: "pkg/services/automations"},
	{packagePath: "pkg/services/bundle", canonicalOwner: "exact service-root or Factory Sessions consumer contracts supplied by pkg/wire"},
	{packagePath: "pkg/services/factory_runtime/resource", canonicalOwner: "pkg/services/factory_definitions/internal/services/catalog/resource"},
	{packagePath: "pkg/services/models/provider", canonicalOwner: "pkg/services/models"},
	{packagePath: "pkg/services/provider_sessions/cursor", canonicalOwner: "pkg/services/provider_sessions/internal/services/cursor_reader"},
	{packagePath: "pkg/services/provider_sessions/cursor/session", canonicalOwner: "pkg/services/factory_sessions/internal/cursors"},
	{packagePath: "pkg/services/workers/application", canonicalOwner: "pkg/services/workers/service with flat constructor parameters"},
	{packagePath: "pkg/workgraph", canonicalOwner: "pkg/services/work"},
	{packagePath: "pkg/workquery", canonicalOwner: "pkg/services/work"},
}, factoryRetiredPackageRoots...)

const (
	generatedCodeExceptionScopeRoot = "root"
)

type boundaryPolicy struct {
	generatedCodeExceptions   []generatedCodeException
	domainTransportExceptions []string
}

type generatedCodeException struct {
	packagePath string
	scope       string
}

var documentedGeneratedCodeExceptions = []generatedCodeException{
	{packagePath: "pkg/transports/http/client", scope: generatedCodeExceptionScopeRoot},
	{packagePath: "pkg/transports/http/generated", scope: generatedCodeExceptionScopeRoot},
}

func defaultBoundaryPolicy() boundaryPolicy {
	return boundaryPolicy{
		generatedCodeExceptions:   slices.Clone(documentedGeneratedCodeExceptions),
		domainTransportExceptions: slices.Clone(documentedDomainTransportExceptions),
	}
}

// documentedDomainTransportExceptions remains as a deletion-only inventory
// hook. It is intentionally empty now that every protected domain uses
// domain-owned inputs and outward transport mapping.
var documentedDomainTransportExceptions []string

func validatePolicy(policy boundaryPolicy) error {
	for _, exception := range policy.generatedCodeExceptions {
		if strings.TrimSpace(exception.packagePath) == "" {
			return fmt.Errorf("generated-code exception path must not be empty")
		}
	}
	return nil
}

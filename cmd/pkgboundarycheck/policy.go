package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const defaultScanRoot = "pkg"
const batch001MigrationShimMarker = "Batch 001 compatibility shim"
const factoryRuntimeImportPath = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
const applicationGraphImportPath = "github.com/portpowered/infinite-you/pkg/wire"
const transportImportPrefix = "github.com/portpowered/infinite-you/pkg/transports/"
const repositoryImportPrefix = "github.com/portpowered/infinite-you/"
const peerServiceImportBaselinePath = "service-cross-import-baseline.json"
const peerServiceImportBaselineStage = "wire-injection-full-blow"
const peerServiceImportDeletionGate = "replace the peer implementation import with the exact pkg/services/<peer> root contract and delete this exact entry"
const testServiceImportBaselinePath = "docs/internal/baselines/test-service-import-baseline.json"
const testServiceImportBaselineStage = "wire-injection-full-blow"
const testServiceImportDeletionGate = "replace the concrete service subpackage import with the owning service root contract, an owner-local test, or root.BuildProcess"
const supportServiceImportBaselinePath = "support-service-import-baseline.json"
const supportServiceImportBaselineStage = "wire-injection-full-blow"
const supportServiceImportDeletionGate = "replace reusable support composition with service-root contracts, typed edge fakes, package-local owner fixtures, or root.BuildProcess"
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

var protectedTransportIndependentDomainRoots = []string{
	"pkg/services",
}

var transportPrivateServiceSubpackages = []string{
	"pkg/services/factory_runtime/internal/services/orchestration/engine",
	"pkg/services/factory_runtime/internal/services/orchestration/runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/scheduler",
	"pkg/services/factory_runtime/internal/services/orchestration/state",
	"pkg/services/factory_runtime/internal/services/orchestration/subsystems",
	"pkg/services/factory_runtime/internal/services/orchestration/throttle",
	"pkg/services/factory_runtime/internal/services/orchestration/token",
	"pkg/services/factory_runtime/internal/services/orchestration/token_transformer",
	"pkg/services/factory_runtime/internal/services/orchestration/context",
	"pkg/services/factory_runtime/internal/services/orchestration/definitionmapping",
	"pkg/services/factory_runtime/internal/services/orchestration/metrics",
	"pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner",
	"pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract",
	"pkg/services/factory_runtime/internal/services/orchestration/replayhooks",
	"pkg/services/factory_runtime/internal/services/orchestration/runtimecontract",
	"pkg/services/factory_runtime/internal/services/orchestration/javascript",
	"pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior",
	"pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/catalog",
	"pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity",
	"pkg/services/factory_sessions/internal/runtime",
	"pkg/services/factory_sessions/internal/runtimebinding",
	"pkg/services/factory_sessions/internal/sessionservice",
	"pkg/services/recordings/events",
	"pkg/services/recordings/internal/events",
	"pkg/services/workers/runner",
	"pkg/services/workers/service",
	"pkg/services/workers/services",
}

// convergedServiceSubpackageRoots records service implementation/compatibility
// paths whose external callers have been migrated to the owning service root.
// Packages within the same service may continue using these paths internally.
var convergedServiceSubpackageRoots = map[string]string{
	"pkg/services/factory_definitions/internal/contracts":                                           "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope":         "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation":  "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput":         "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype":       "factory_definitions",
	"pkg/services/factory_definitions/internal/services/compilation/loadedsource":                   "factory_definitions",
	"pkg/services/factory_definitions/internal/services/catalog/persistence":                        "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy":             "factory_definitions",
	"pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig":         "factory_definitions",
	"pkg/services/factory_definitions/internal/services/catalog/resource":                           "factory_definitions",
	"pkg/services/factory_definitions/internal/services/snapshots_portability/capture":              "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability":         "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution":     "factory_definitions",
	"pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation":          "factory_definitions",
	"pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers":           "factory_definitions",
	"pkg/services/factory_definitions/internal/services/validation/authoredmodel/taxonomy":          "factory_definitions",
	"pkg/services/factory_runtime/internal/services/orchestration/state":                            "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/token":                            "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/metrics":                          "factory_runtime",
	"pkg/services/factory_runtime/internal/services/instance_host/build":                            "factory_runtime",
	"pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore":   "factory_runtime",
	"pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptsummary": "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/context":                          "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/definitionmapping":                "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/engine":                           "factory_runtime",
	"pkg/services/automations/internal/services/filesystem_watchers":                                "automations",
	"pkg/services/automations/internal/services/filesystem_watchers/internal/service":               "automations",
	"pkg/services/automations/internal/services/filesystem_watchers/wire":                           "automations",
	"pkg/services/factory_runtime/internal":                                                         "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner":               "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract":             "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/replayhooks":                      "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/runtime":                          "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/runtimecontract":                  "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/scheduler":                        "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/subsystems":                       "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/throttle":                         "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/token_transformer":                "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/tooling":                          "factory_runtime",
	"pkg/services/factory_sessions/internal/invocation":                                             "factory_sessions",
	"pkg/services/factory_sessions/internal/cursors":                                                "factory_sessions",
	"pkg/services/factory_sessions/internal/execution":                                              "factory_sessions",
	"pkg/services/factory_sessions/internal/logicaltarget":                                          "factory_sessions",
	"pkg/services/factory_sessions/internal/responseevents":                                         "factory_sessions",
	"pkg/services/factory_sessions/internal/responseeventstore":                                     "factory_sessions",
	"pkg/services/factory_sessions/internal/responsestream":                                         "factory_sessions",
	"pkg/services/factory_sessions/internal/runtime":                                                "factory_sessions",
	"pkg/services/factory_sessions/internal/runtimebinding":                                         "factory_sessions",
	"pkg/services/factory_definitions/internal/services/validation/impl":                            "factory_definitions",
	"pkg/services/factory_definitions/internal/services/snapshots_portability/editable":             "factory_definitions",
	"pkg/services/factory_definitions/scaffold":                                                     "factory_definitions",
	"pkg/services/factory_definitions/service":                                                      "factory_definitions",
	"pkg/services/recordings/events":                                                                "recordings",
	"pkg/services/recordings/internal/events":                                                       "recordings",
	"pkg/services/recordings/internal/projections":                                                  "recordings",
	"pkg/services/recordings/internal/projections/dashboard":                                        "recordings",
	"pkg/services/recordings/internal/artifacts":                                                    "recordings",
	"pkg/services/recordings/internal/replay":                                                       "recordings",
	"pkg/services/recordings/artifacts":                                                             "recordings",
	"pkg/services/recordings/replay":                                                                "recordings",
	"pkg/services/recordings/service":                                                               "recordings",
	"pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty":               "providers",
	"pkg/services/workers/invocation":                                                               "workers",
	"pkg/services/workers/prompting":                                                                "workers",
	"pkg/services/automations/internal/services/hosted_sources":                                     "automations",
	"pkg/services/workers/services/testing":                                                         "workers",
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

var approvedApplicationGraphImporters = []string{
	"pkg/root",
	"pkg/wire",
}

// pkg/services/edges is the canonical process-edge aggregator. It may name
// only contracts owned by the leaf packages that directly perform these
// external effects. This is deliberately not a general service-subpackage
// exception.
//
// Provider inference/process effects belong to the Providers Execution leaf
// (providersLeafEffectContractImport). Workers may retain only its
// request-scoped compatibility adapter and must not add a provider protocol,
// catalog, adapter, session, or native execution owner.
var approvedPeerServiceContractImports = map[string]struct{}{
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty": {},
	"pkg/platform/pty\x00github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty":   {},
	"pkg/services/edges\x00" + providersLeafEffectContractImport:                                                                                {},
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                     {},
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/automations":                                                        {},
	"pkg/wire\x00github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire":                            {},
	"pkg/services/factory_runtime\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                           {},
	"pkg/services/factory_runtime/internal/services/instance_host/build\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":     {},
	"pkg/services/recordings\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                {},
	"pkg/services/recordings/internal/artifacts\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                             {},
	"pkg/services/recordings/internal/replay\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                {},
}

// publicExternalEffectContractImports are intentionally declared beside the
// leaf adapter that crosses the process, network, filesystem, clock, or host
// boundary. Tests may import these exact ports to supply edges.Edges values;
// they are not permission to construct the owning service implementation.
// Providers owns the durable public effect port; Workers consumes it through
// its request-scoped compatibility adapter.
var publicExternalEffectContractImports = map[string]struct{}{
	providersLeafEffectContractImport: {},
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty": {},
	"github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                     {},
	"github.com/portpowered/infinite-you/pkg/services/automations":                                                        {},
}

const (
	generatedCodeExceptionScopeRoot = "root"
)

type boundaryPolicy struct {
	approvedProductPackageFamilies []string
	migrationPackageExceptions     []migrationPackageException
	generatedCodeExceptions        []generatedCodeException
	domainTransportExceptions      []string
}

type migrationPackageException struct {
	packagePath  string
	targetOwner  string
	workItem     string
	deletionGate string
}

type generatedCodeException struct {
	packagePath string
	scope       string
}

var approvedProductPackageFamilies = []string{
	"pkg/config",
	"pkg/initializer",
	"pkg/internal",
	"pkg/platform",
	"pkg/root",
	"pkg/services",
	"pkg/transports",
	"pkg/wire",
}

const (
	batch006TransportFamilyMove = "Batch 006 — Transport family move"
	batch006WorkFamilyMove      = "Batch 006 — Work family move"
	batch006PlatformFamilyMove  = "Batch 006 — Platform family move"
)

var documentedMigrationPackageExceptions []migrationPackageException

var documentedGeneratedCodeExceptions = []generatedCodeException{
	{packagePath: "pkg/transports/http/client", scope: generatedCodeExceptionScopeRoot},
	{packagePath: "pkg/transports/http/generated", scope: generatedCodeExceptionScopeRoot},
}

func defaultBoundaryPolicy() boundaryPolicy {
	return boundaryPolicy{
		approvedProductPackageFamilies: slices.Clone(approvedProductPackageFamilies),
		migrationPackageExceptions:     slices.Clone(documentedMigrationPackageExceptions),
		generatedCodeExceptions:        slices.Clone(documentedGeneratedCodeExceptions),
		domainTransportExceptions:      slices.Clone(documentedDomainTransportExceptions),
	}
}

// documentedDomainTransportExceptions remains as a deletion-only inventory
// hook. It is intentionally empty now that every protected domain uses
// domain-owned inputs and outward transport mapping.
var documentedDomainTransportExceptions []string

func validatePolicy(policy boundaryPolicy) error {
	if err := validateMigrationPackageExceptions(policy); err != nil {
		return err
	}
	for _, exception := range policy.generatedCodeExceptions {
		if strings.TrimSpace(exception.packagePath) == "" {
			return fmt.Errorf("generated-code exception path must not be empty")
		}
		if slices.Contains(policy.approvedProductPackageFamilies, exception.packagePath) {
			return fmt.Errorf("generated-code exception %s must not also be an approved product package family", exception.packagePath)
		}
		if containsMigrationPackageException(policy.migrationPackageExceptions, exception.packagePath) {
			return fmt.Errorf("generated-code exception %s must not also be a migration-only package exception", exception.packagePath)
		}
	}
	return nil
}

func validateMigrationPackageExceptions(policy boundaryPolicy) error {
	for _, exception := range policy.migrationPackageExceptions {
		if strings.TrimSpace(exception.packagePath) == "" {
			return fmt.Errorf("migration-only package exception path must not be empty")
		}
		if slices.Contains(policy.approvedProductPackageFamilies, exception.packagePath) {
			return fmt.Errorf("migration-only package exception %s must not also be an approved product package family", exception.packagePath)
		}
		if strings.TrimSpace(exception.targetOwner) == "" {
			return fmt.Errorf("migration-only package exception %s target owner must not be empty", exception.packagePath)
		}
		if !slices.Contains(policy.approvedProductPackageFamilies, exception.targetOwner) &&
			!containsMigrationPackageException(policy.migrationPackageExceptions, exception.targetOwner) {
			return fmt.Errorf("migration-only package exception %s target owner %s must be an approved or documented migration package family", exception.packagePath, exception.targetOwner)
		}
		expectedTarget, active := activeMigrationTarget(exception.workItem)
		if !active {
			return fmt.Errorf("migration-only package exception %s must name a recognized active work item", exception.packagePath)
		}
		if exception.targetOwner != expectedTarget {
			return fmt.Errorf("migration-only package exception %s work item %q targets %s, not %s", exception.packagePath, exception.workItem, expectedTarget, exception.targetOwner)
		}
		if strings.TrimSpace(exception.deletionGate) == "" {
			return fmt.Errorf("migration-only package exception %s deletion gate must not be empty", exception.packagePath)
		}
	}
	return nil
}

func activeMigrationTarget(workItem string) (string, bool) {
	switch workItem {
	case batch006TransportFamilyMove:
		return "pkg/transports", true
	case batch006WorkFamilyMove:
		return "pkg/services", true
	case batch006PlatformFamilyMove:
		return "pkg/platform", true
	default:
		return "", false
	}
}

func containsMigrationPackageException(exceptions []migrationPackageException, packagePath string) bool {
	return slices.ContainsFunc(exceptions, func(exception migrationPackageException) bool {
		return exception.packagePath == packagePath
	})
}

func isAllowedRootPackageFamily(policy boundaryPolicy, packageRoot string, packagePath string) bool {
	return slices.Contains(policy.approvedProductPackageFamilies, packagePath) ||
		containsMigrationPackageException(policy.migrationPackageExceptions, packagePath) ||
		slices.Contains(directRootGeneratedCodeExceptionPaths(policy, packageRoot), packagePath)
}

func detectMigrationShimFinding(repoRoot string, packagePath string) (migrationShimFinding, bool, error) {
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(packagePath))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return migrationShimFinding{}, false, fmt.Errorf("read migration shim package %s: %w", packagePath, err)
	}

	finding := migrationShimFinding{packagePath: packagePath}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}

		goFilePath := filepath.Join(packageDir, entry.Name())
		marker, canonicalTarget, err := readMigrationShimSignals(goFilePath)
		if err != nil {
			return migrationShimFinding{}, false, err
		}
		if finding.marker == "" {
			finding.marker = marker
		}
		if finding.canonicalTarget == "" {
			finding.canonicalTarget = canonicalTarget
		}
		if finding.marker != "" && finding.canonicalTarget != "" {
			return finding, true, nil
		}
	}

	return finding, finding.marker != "", nil
}

func readMigrationShimSignals(goFilePath string) (string, string, error) {
	content, err := os.ReadFile(goFilePath)
	if err != nil {
		return "", "", fmt.Errorf("read migration shim file %s: %w", filepath.ToSlash(goFilePath), err)
	}

	marker := ""
	if strings.Contains(string(content), batch001MigrationShimMarker) {
		marker = batch001MigrationShimMarker
	}
	return marker, canonicalTargetImport(content), nil
}

func canonicalTargetImport(content []byte) string {
	parsedFile, err := parser.ParseFile(token.NewFileSet(), "", content, parser.ImportsOnly)
	if err != nil {
		return ""
	}

	for _, importSpec := range parsedFile.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}
		if importPath == factoryRuntimeImportPath {
			return importPath
		}
	}
	return ""
}

func directRootGeneratedCodeExceptionPaths(policy boundaryPolicy, packageRoot string) []string {
	var roots []string
	for _, exception := range policy.generatedCodeExceptions {
		exceptionPath := filepath.ToSlash(exception.packagePath)
		if filepath.Dir(exceptionPath) == filepath.ToSlash(packageRoot) {
			roots = append(roots, exceptionPath)
		}
	}
	return roots
}

// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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
const testServiceImportBaselinePath = "test-service-import-baseline.json"
const testServiceImportBaselineStage = "wire-injection-full-blow"
const testServiceImportDeletionGate = "replace the concrete service subpackage import with the owning service root contract, an owner-local test, or root.BuildProcess"
const supportServiceImportBaselinePath = "support-service-import-baseline.json"
const supportServiceImportBaselineStage = "wire-injection-full-blow"
const supportServiceImportDeletionGate = "replace reusable support composition with service-root contracts, typed edge fakes, package-local owner fixtures, or root.BuildProcess"
const serviceConstructionBaselinePath = "service-construction-baseline.json"
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
	"github.com/portpowered/infinite-you/pkg/services/work": {
		"NewSelection": {},
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
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract": {
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
	"pkg/services/factory_runtime/service",
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
	"pkg/services/work/service",
	"pkg/services/workers/diagnostics",
	"pkg/services/workers/executor",
	"pkg/services/workers/provider",
	"pkg/services/workers/runner",
	"pkg/services/workers/service",
	"pkg/services/workers/services",
}

// convergedServiceSubpackageRoots records service implementation/compatibility
// paths whose external callers have been migrated to the owning service root.
// Packages within the same service may continue using these paths internally.
var convergedServiceSubpackageRoots = map[string]string{
	"pkg/services/factory_definitions/internal/contracts":       "factory_definitions",
	"pkg/services/factory_definitions/decisionenvelope":         "factory_definitions",
	"pkg/services/factory_definitions/invocationinterpolation":  "factory_definitions",
	"pkg/services/factory_definitions/invocationoutput":         "factory_definitions",
	"pkg/services/factory_definitions/invocationworktype":       "factory_definitions",
	"pkg/services/factory_definitions/loadedsource":             "factory_definitions",
	"pkg/services/factory_definitions/loading":                  "factory_definitions",
	"pkg/services/factory_definitions/portableconfig":           "factory_definitions",
	"pkg/services/factory_definitions/persistence":              "factory_definitions",
	"pkg/services/factory_definitions/quorumpolicy":             "factory_definitions",
	"pkg/services/factory_definitions/replayconfig":             "factory_definitions",
	"pkg/services/factory_definitions/resource":                 "factory_definitions",
	"pkg/services/factory_definitions/runtimeconfig":            "factory_definitions",
	"pkg/services/factory_definitions/snapshotcapture":          "factory_definitions",
	"pkg/services/factory_definitions/ttsobservability":         "factory_definitions",
	"pkg/services/factory_definitions/workstationexecution":     "factory_definitions",
	"pkg/services/factory_definitions/workpropagation":          "factory_definitions",
	"pkg/services/factory_definitions/workers":                  "factory_definitions",
	"pkg/services/factory_definitions/workers/taxonomy":         "factory_definitions",
	"pkg/services/factory_runtime/internal/services/orchestration/state":                        "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/token":                        "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/metrics":                      "factory_runtime",
	"pkg/services/factory_runtime/build":                        "factory_runtime",
	"pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptstore":  "factory_runtime",
	"pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptsummary": "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/context":                      "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/definitionmapping":            "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/engine":                       "factory_runtime",
	"pkg/services/automations/internal/services/filesystem_watchers":                       "automations",
	"pkg/services/automations/internal/services/filesystem_watchers/internal/service":      "automations",
	"pkg/services/automations/internal/services/filesystem_watchers/wire":                  "automations",
	"pkg/services/factory_runtime/internal":                     "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner":           "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract":         "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/replayhooks":                  "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/runtime":                      "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/runtimecontract":              "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/scheduler":                    "factory_runtime",
	"pkg/services/factory_runtime/service":                      "factory_runtime",
	"pkg/services/factory_runtime/service/host":                 "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/subsystems":                   "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/throttle":                     "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/token_transformer":            "factory_runtime",
	"pkg/services/factory_runtime/internal/services/orchestration/tooling": "factory_runtime",
	"pkg/services/factory_sessions/internal/invocation":         "factory_sessions",
	"pkg/services/factory_sessions/internal/cursors":            "factory_sessions",
	"pkg/services/factory_sessions/internal/execution":          "factory_sessions",
	"pkg/services/factory_sessions/internal/logicaltarget":      "factory_sessions",
	"pkg/services/factory_sessions/internal/responseevents":     "factory_sessions",
	"pkg/services/factory_sessions/internal/responseeventstore": "factory_sessions",
	"pkg/services/factory_sessions/internal/responsestream":     "factory_sessions",
	"pkg/services/factory_sessions/internal/runtime":            "factory_sessions",
	"pkg/services/factory_sessions/internal/runtimebinding":     "factory_sessions",
	"pkg/services/factory_definitions/validation":               "factory_definitions",
	"pkg/services/factory_definitions/editable":                 "factory_definitions",
	"pkg/services/factory_definitions/packages":                 "factory_definitions",
	"pkg/services/factory_definitions/scaffold":                 "factory_definitions",
	"pkg/services/factory_definitions/service":                  "factory_definitions",
	"pkg/services/recordings/events":                            "recordings",
	"pkg/services/recordings/projections/dashboard":             "recordings",
	"pkg/services/recordings/replay":                            "recordings",
	"pkg/services/recordings/service":                           "recordings",
	"pkg/services/work/service":                                 "work",
	"pkg/services/workers/agypty":                               "workers",
	"pkg/services/workers/execution/recording":                  "workers",
	"pkg/services/workers/invocation":                           "workers",
	"pkg/services/workers/prompting":                            "workers",
	"pkg/services/workers/provider":                             "workers",
	"pkg/services/automations/internal/services/hosted_sources": "automations",
	"pkg/services/workers/services/testing":                     "workers",
}

var factoryRetiredPackageRoots = []retiredPackageRoot{
	{packagePath: "pkg/factory", canonicalOwner: "pkg/services/factory_definitions, pkg/services/factory_sessions, pkg/services/factory_runtime, or pkg/services/recordings according to ownership"},
	{packagePath: "pkg/packagedfactories", canonicalOwner: "pkg/services/factory_definitions/packages"},
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
	{packagePath: "pkg/services/factory_runtime/resource", canonicalOwner: "pkg/services/factory_definitions/resource"},
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
// Provider inference/process effects: the durable owner is the Providers
// Execution leaf (providersLeafEffectContractImport). Workers
// provider/inferencecontract entries remain only as migration debt until later
// Providers packets land; they are not the durable normative owner.
var approvedPeerServiceContractImports = map[string]struct{}{
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/workers/agypty":                                                        {},
	"pkg/platform/pty\x00github.com/portpowered/infinite-you/pkg/services/workers/agypty":                                                          {},
	"pkg/services/edges\x00" + providersLeafEffectContractImport:                                                                                   {},
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                                    {},
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/automations":                                         {},
	"pkg/wire\x00github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire": {},
	"pkg/services/factory_runtime\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                          {},
	"pkg/services/factory_runtime/build\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                    {},
	"pkg/services/factory_runtime/service\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                  {},
	"pkg/services/recordings\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                               {},
	"pkg/services/recordings/replay\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                        {},
	"pkg/services/recordings/service\x00github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":                       {},
}

// publicExternalEffectContractImports are intentionally declared beside the
// leaf adapter that crosses the process, network, filesystem, clock, or host
// boundary. Tests may import these exact ports to supply edges.Edges values;
// they are not permission to construct the owning service implementation.
// Workers provider/inferencecontract remains migration debt; Providers leaf is
// the durable public effect port.
var publicExternalEffectContractImports = map[string]struct{}{
	providersLeafEffectContractImport:                                                       {},
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty":                       {},
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":   {},
	"github.com/portpowered/infinite-you/pkg/services/automations":        {},
}

const (
	generatedCodeExceptionScopeRoot = "root"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
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

type config struct {
	root                              string
	packageRoot                       string
	writeTestServiceImportBaseline    bool
	writeSupportServiceImportBaseline bool
	writeTransportBehaviorBaseline    bool
	writeProductionDefaultBaseline    bool
	writeTestBehaviorBaseline         bool
	writePetriPublicSurfaceBaseline   bool
}

type scanResult struct {
	rootPackageFindings                []rootPackageFinding
	retiredPackageRootFindings         []retiredPackageRootFinding
	retiredPackageImportFindings       []retiredPackageImportFinding
	migrationShimFindings              []migrationShimFinding
	applicationGraphImportFindings     []applicationGraphImportFinding
	handwrittenGeneratedFindings       []handwrittenGeneratedFinding
	domainTransportFindings            []domainTransportImportFinding
	peerServiceImportFindings          []peerServiceImportFinding
	stalePeerServiceBaselineEntries    []peerServiceImportBaselineEntry
	peerServiceBaselineCount           int
	testServiceImportFindings          []testServiceImportFinding
	staleTestServiceBaselineEntries    []testServiceImportBaselineEntry
	testServiceBaselineCount           int
	supportServiceImportFindings       []supportServiceImportFinding
	staleSupportServiceBaselineEntries []supportServiceImportBaselineEntry
	supportServiceBaselineCount        int
	serviceConstructionFindings        []serviceConstructionFinding
	staleServiceConstructionEntries    []serviceConstructionBaselineEntry
	serviceConstructionBaselineCount   int
	transportImplementationFindings    []transportServiceImplementationFinding
	externalImplementationFindings     []transportServiceImplementationFinding
	transportBehaviorFindings          []transportBehaviorFinding
	staleTransportBehaviorEntries      []transportBehaviorBaselineEntry
	transportBehaviorBaselineCount     int
	functionalProcessEdgeFindings      []functionalProcessEdgeFinding
	constructedServiceEdgesFindings    []constructedServiceEdgesFinding
	testWorkNormalizationFindings      []testWorkNormalizationFinding
	productionDefaultFindings          []productionDefaultFinding
	staleProductionDefaultEntries      []productionDefaultBaselineEntry
	productionDefaultBaselineCount     int
	initializerBehaviorFindings        []initializerBehaviorFinding
	staleInitializerBehaviorEntries    []initializerBehaviorBaselineEntry
	initializerBehaviorBaselineCount   int
	testBehaviorFindings               []testBehaviorFinding
	staleTestBehaviorEntries           []testBehaviorBaselineEntry
	testBehaviorBaselineCount          int
	petriPublicSurfaceFindings         []petriPublicSurfaceFinding
	stalePetriPublicSurfaceEntries     []petriPublicSurfaceBaselineEntry
	petriPublicSurfaceBaselineCount    int
	providerEffectOwnershipFindings    []providerEffectOwnershipFinding
}

type retiredPackageRoot struct {
	packagePath    string
	canonicalOwner string
}

type retiredPackageRootFinding struct {
	retiredPackageRoot
}

type retiredPackageImportFinding struct {
	retiredPackageRoot
	importPath string
	filePath   string
}

type handwrittenGeneratedFinding struct {
	filePath    string
	packagePath string
}

type rootPackageFinding struct {
	packagePath string
}

type migrationShimFinding struct {
	packagePath     string
	marker          string
	canonicalTarget string
}

type applicationGraphImportFinding struct {
	packagePath string
	filePath    string
}

type domainTransportImportFinding struct {
	packagePath string
	importPath  string
	filePath    string
}

type peerServiceImportFinding struct {
	owner      string
	peer       string
	importPath string
	filePath   string
}

type peerServiceImportBaseline struct {
	Version int                              `json:"version"`
	Entries []peerServiceImportBaselineEntry `json:"entries"`
}

type peerServiceImportBaselineEntry struct {
	Owner        string `json:"owner"`
	Peer         string `json:"peer"`
	ImportPath   string `json:"importPath"`
	FilePath     string `json:"filePath"`
	TargetRoot   string `json:"targetRoot"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

type transportServiceImplementationFinding struct {
	importPath string
	filePath   string
}

type testServiceImportFinding struct {
	owner      string
	importPath string
	filePath   string
}

type testServiceImportBaseline struct {
	Version int                              `json:"version"`
	Entries []testServiceImportBaselineEntry `json:"entries"`
}

type testServiceImportBaselineEntry struct {
	Owner        string `json:"owner"`
	ImportPath   string `json:"importPath"`
	FilePath     string `json:"filePath"`
	TargetRoot   string `json:"targetRoot"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

type serviceConstructionFinding struct {
	owner      string
	importPath string
	symbol     string
	filePath   string
	line       int
	count      int
}

type serviceConstructionBaseline struct {
	Version int                                `json:"version"`
	Entries []serviceConstructionBaselineEntry `json:"entries"`
}

type serviceConstructionBaselineEntry struct {
	Owner        string `json:"owner"`
	ImportPath   string `json:"importPath"`
	Symbol       string `json:"symbol"`
	FilePath     string `json:"filePath"`
	Count        int    `json:"count"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

func main() {
	cfg := parseConfig()
	if cfg.writeTestServiceImportBaseline {
		if err := createTestServiceImportBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeSupportServiceImportBaseline {
		if err := createSupportServiceImportBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeTransportBehaviorBaseline {
		if err := createTransportBehaviorBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeProductionDefaultBaseline {
		if err := createProductionDefaultBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeTestBehaviorBaseline {
		if err := createTestBehaviorBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writePetriPublicSurfaceBaseline {
		if err := createPetriPublicSurfaceBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if err := run(cfg, stdoutWriter, stderrWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.StringVar(&cfg.packageRoot, "package-root", defaultScanRoot, "repository-relative package root to scan")
	flag.BoolVar(
		&cfg.writeTestServiceImportBaseline,
		"create-test-service-import-baseline",
		false,
		"create the deletion-only test service import baseline; fails when the file already exists",
	)
	flag.BoolVar(
		&cfg.writeProductionDefaultBaseline,
		"create-production-default-selection-baseline",
		false,
		"create the deletion-only production default-selection baseline; fails when the file already exists or no debt exists",
	)
	flag.BoolVar(
		&cfg.writeSupportServiceImportBaseline,
		"create-support-service-import-baseline",
		false,
		"create the deletion-only reusable-support service import baseline; fails when the file already exists",
	)
	flag.BoolVar(
		&cfg.writeTransportBehaviorBaseline,
		"create-transport-behavior-baseline",
		false,
		"create the deletion-only transport behavior baseline; fails when the file already exists",
	)
	flag.BoolVar(
		&cfg.writeTestBehaviorBaseline,
		"create-test-behavior-boundary-baseline",
		false,
		"create the exact deletion-only test behavior baseline; fails when the file exists or no debt exists",
	)
	flag.BoolVar(
		&cfg.writePetriPublicSurfaceBaseline,
		"create-petri-public-surface-baseline",
		false,
		"create the exact deletion-only Petri public-surface baseline; fails when the file exists or no debt exists",
	)
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	return runWithPolicy(cfg, defaultBoundaryPolicy(), stdout, stderr)
}

func runWithPolicy(cfg config, policy boundaryPolicy, stdout io.Writer, stderr io.Writer) error {
	if strings.TrimSpace(cfg.packageRoot) == "" {
		return fmt.Errorf("package root must not be empty")
	}

	if err := validatePolicy(policy); err != nil {
		return err
	}

	findings, err := scanRepo(cfg, policy)
	if err != nil {
		return err
	}
	blockingViolationCount := len(findings.rootPackageFindings) +
		len(findings.retiredPackageRootFindings) +
		len(findings.retiredPackageImportFindings) +
		len(findings.migrationShimFindings) +
		len(findings.applicationGraphImportFindings) +
		len(findings.handwrittenGeneratedFindings) +
		len(findings.domainTransportFindings) +
		len(findings.peerServiceImportFindings) +
		len(findings.stalePeerServiceBaselineEntries) +
		len(findings.testServiceImportFindings) +
		len(findings.staleTestServiceBaselineEntries) +
		len(findings.supportServiceImportFindings) +
		len(findings.staleSupportServiceBaselineEntries) +
		len(findings.serviceConstructionFindings) +
		len(findings.staleServiceConstructionEntries) +
		len(findings.transportImplementationFindings) +
		len(findings.externalImplementationFindings) +
		len(findings.transportBehaviorFindings) +
		len(findings.staleTransportBehaviorEntries) +
		len(findings.functionalProcessEdgeFindings) +
		len(findings.constructedServiceEdgesFindings) +
		len(findings.testWorkNormalizationFindings) +
		len(findings.productionDefaultFindings) +
		len(findings.staleProductionDefaultEntries) +
		len(findings.initializerBehaviorFindings) +
		len(findings.staleInitializerBehaviorEntries)
	blockingViolationCount += len(findings.testBehaviorFindings) +
		len(findings.staleTestBehaviorEntries)
	blockingViolationCount += len(findings.petriPublicSurfaceFindings) +
		len(findings.stalePetriPublicSurfaceEntries)
	blockingViolationCount += len(findings.providerEffectOwnershipFindings)
	if blockingViolationCount == 0 {
		fmt.Fprintln(stdout, "[agent-factory:pkg-boundary] package boundary passed (no blocking package-boundary violations)")
		writePeerServiceBaselineSummary(stdout, findings.peerServiceBaselineCount)
		writeTestServiceBaselineSummary(stdout, findings.testServiceBaselineCount)
		writeSupportServiceBaselineSummary(stdout, findings.supportServiceBaselineCount)
		writeServiceConstructionBaselineSummary(stdout, findings.serviceConstructionBaselineCount)
		writeTransportBehaviorBaselineSummary(stdout, findings.transportBehaviorBaselineCount)
		writeProductionDefaultBaselineSummary(stdout, findings.productionDefaultBaselineCount)
		writeInitializerBehaviorBaselineSummary(stdout, findings.initializerBehaviorBaselineCount)
		writeTestBehaviorBaselineSummary(stdout, findings.testBehaviorBaselineCount)
		writePetriPublicSurfaceBaselineSummary(stdout, findings.petriPublicSurfaceBaselineCount)
		writeGeneratedCodeExceptionSummary(stdout, policy)
		return nil
	}

	for _, finding := range findings.rootPackageFindings {
		fmt.Fprintf(stderr, "[agent-factory:pkg-boundary] unapproved root package family: %s\n", finding.packagePath)
		fmt.Fprintf(stderr, "  reason: %s is outside the approved package-family allowlist.\n", finding.packagePath)
		fmt.Fprintln(stderr, "  remediation: move the code under an approved owner or deliberately update the allowlist with ownership rationale.")
	}
	writeRetiredPackageRootFindings(stderr, findings.retiredPackageRootFindings)
	writeRetiredPackageImportFindings(stderr, findings.retiredPackageImportFindings)
	writeMigrationShimBlockingFindings(stderr, findings.migrationShimFindings)
	writeApplicationGraphImportFindings(stderr, findings.applicationGraphImportFindings)
	writeHandwrittenGeneratedFindings(stderr, findings.handwrittenGeneratedFindings)
	writeDomainTransportImportFindings(stderr, findings.domainTransportFindings)
	writePeerServiceImportFindings(stderr, findings.peerServiceImportFindings)
	writeStalePeerServiceBaselineEntries(stderr, findings.stalePeerServiceBaselineEntries)
	writePeerServiceBaselineSummary(stderr, findings.peerServiceBaselineCount)
	writeTestServiceImportFindings(stderr, findings.testServiceImportFindings)
	writeStaleTestServiceBaselineEntries(stderr, findings.staleTestServiceBaselineEntries)
	writeTestServiceBaselineSummary(stderr, findings.testServiceBaselineCount)
	writeSupportServiceImportFindings(stderr, findings.supportServiceImportFindings)
	writeStaleSupportServiceBaselineEntries(stderr, findings.staleSupportServiceBaselineEntries)
	writeSupportServiceBaselineSummary(stderr, findings.supportServiceBaselineCount)
	writeServiceConstructionFindings(stderr, findings.serviceConstructionFindings)
	writeStaleServiceConstructionBaselineEntries(stderr, findings.staleServiceConstructionEntries)
	writeServiceConstructionBaselineSummary(stderr, findings.serviceConstructionBaselineCount)
	writeTransportServiceImplementationFindings(stderr, findings.transportImplementationFindings)
	writeExternalServiceImplementationFindings(stderr, findings.externalImplementationFindings)
	writeTransportBehaviorFindings(stderr, findings.transportBehaviorFindings)
	writeStaleTransportBehaviorBaselineEntries(stderr, findings.staleTransportBehaviorEntries)
	writeFunctionalProcessEdgeFindings(stderr, findings.functionalProcessEdgeFindings)
	writeConstructedServiceEdgesFindings(stderr, findings.constructedServiceEdgesFindings)
	writeTestWorkNormalizationFindings(stderr, findings.testWorkNormalizationFindings)
	writeTransportBehaviorBaselineSummary(stderr, findings.transportBehaviorBaselineCount)
	writeProductionDefaultFindings(stderr, findings.productionDefaultFindings)
	writeStaleProductionDefaultBaselineEntries(stderr, findings.staleProductionDefaultEntries)
	writeProductionDefaultBaselineSummary(stderr, findings.productionDefaultBaselineCount)
	writeInitializerBehaviorFindings(stderr, findings.initializerBehaviorFindings)
	writeStaleInitializerBehaviorBaselineEntries(stderr, findings.staleInitializerBehaviorEntries)
	writeInitializerBehaviorBaselineSummary(stderr, findings.initializerBehaviorBaselineCount)
	writeTestBehaviorFindings(stderr, findings.testBehaviorFindings)
	writeStaleTestBehaviorBaselineEntries(stderr, findings.staleTestBehaviorEntries)
	writeTestBehaviorBaselineSummary(stderr, findings.testBehaviorBaselineCount)
	writePetriPublicSurfaceFindings(stderr, findings.petriPublicSurfaceFindings)
	writeStalePetriPublicSurfaceBaselineEntries(stderr, findings.stalePetriPublicSurfaceEntries)
	writePetriPublicSurfaceBaselineSummary(stderr, findings.petriPublicSurfaceBaselineCount)
	writeProviderEffectOwnershipFindings(stderr, findings.providerEffectOwnershipFindings)
	writeGeneratedCodeExceptionSummary(stderr, policy)
	return fmt.Errorf("[agent-factory:pkg-boundary] found %d package-boundary violation(s)", blockingViolationCount)
}

func scanRepo(cfg config, policy boundaryPolicy) (scanResult, error) {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return scanResult{}, fmt.Errorf("resolve repo root: %w", err)
	}

	scanRoot := filepath.Join(repoRoot, filepath.FromSlash(cfg.packageRoot))
	info, err := os.Stat(scanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return scanResult{}, nil
		}
		return scanResult{}, fmt.Errorf("stat scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}
	if !info.IsDir() {
		return scanResult{}, nil
	}

	entries, err := os.ReadDir(scanRoot)
	if err != nil {
		return scanResult{}, fmt.Errorf("read scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}

	result := scanResult{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packagePath := filepath.ToSlash(filepath.Join(cfg.packageRoot, entry.Name()))
		if retiredRoot, found := findRetiredPackageRoot(packagePath); found {
			result.retiredPackageRootFindings = append(result.retiredPackageRootFindings, retiredPackageRootFinding{retiredRoot})
			continue
		}
		migrationShimFinding, found, err := detectMigrationShimFinding(repoRoot, packagePath)
		if err != nil {
			return scanResult{}, err
		}
		if found {
			result.migrationShimFindings = append(result.migrationShimFindings, migrationShimFinding)
		}

		if isAllowedRootPackageFamily(policy, cfg.packageRoot, packagePath) {
			continue
		}

		result.rootPackageFindings = append(result.rootPackageFindings, rootPackageFinding{packagePath: packagePath})
	}
	for _, retiredRoot := range retiredPackageRoots {
		parent := filepath.ToSlash(filepath.Dir(retiredRoot.packagePath))
		if parent == cfg.packageRoot || !strings.HasPrefix(retiredRoot.packagePath, cfg.packageRoot+"/") {
			continue
		}
		info, statErr := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(retiredRoot.packagePath)))
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return scanResult{}, fmt.Errorf("stat retired package root %s: %w", retiredRoot.packagePath, statErr)
		}
		if info.IsDir() {
			result.retiredPackageRootFindings = append(
				result.retiredPackageRootFindings,
				retiredPackageRootFinding{retiredRoot},
			)
		}
	}

	result.applicationGraphImportFindings, err = scanApplicationGraphImports(repoRoot, scanRoot, cfg.packageRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.retiredPackageImportFindings, err = scanRetiredPackageImports(repoRoot, scanRoot, cfg.packageRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.handwrittenGeneratedFindings, err = scanHandwrittenGeneratedFiles(repoRoot, policy.generatedCodeExceptions)
	if err != nil {
		return scanResult{}, err
	}
	result.domainTransportFindings, err = scanDomainTransportImports(repoRoot, policy.domainTransportExceptions)
	if err != nil {
		return scanResult{}, err
	}
	peerServiceFindings, err := scanPeerServiceImports(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	peerServiceBaseline, err := loadPeerServiceImportBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.peerServiceImportFindings, result.stalePeerServiceBaselineEntries, err =
		partitionPeerServiceImportFindings(peerServiceFindings, peerServiceBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.peerServiceBaselineCount = len(peerServiceBaseline.Entries)
	testServiceFindings, err := scanTestServiceSubpackageImports(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	testServiceBaseline, err := loadTestServiceImportBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.testServiceImportFindings, result.staleTestServiceBaselineEntries, err =
		partitionTestServiceImportFindings(testServiceFindings, testServiceBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.testServiceBaselineCount = len(testServiceBaseline.Entries)
	supportServiceFindings, err := scanSupportServiceSubpackageImports(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	supportServiceBaseline, err := loadSupportServiceImportBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.supportServiceImportFindings, result.staleSupportServiceBaselineEntries, err =
		partitionSupportServiceImportFindings(supportServiceFindings, supportServiceBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.supportServiceBaselineCount = len(supportServiceBaseline.Entries)
	serviceConstructionFindings, err := scanProductServiceConstruction(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	serviceConstructionBaseline, err := loadServiceConstructionBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.serviceConstructionFindings, result.staleServiceConstructionEntries, err =
		partitionServiceConstructionFindings(serviceConstructionFindings, serviceConstructionBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.serviceConstructionBaselineCount = len(serviceConstructionBaseline.Entries)
	result.transportImplementationFindings, err = scanTransportServiceImplementationImports(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.externalImplementationFindings, err = scanConvergedServiceSubpackageImports(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	transportBehaviorFindings, err := scanTransportBehavior(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	transportBehaviorBaseline, err := loadTransportBehaviorBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.transportBehaviorFindings, result.staleTransportBehaviorEntries, err =
		partitionTransportBehaviorFindings(transportBehaviorFindings, transportBehaviorBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.transportBehaviorBaselineCount = len(transportBehaviorBaseline.Entries)
	result.functionalProcessEdgeFindings, err = scanFunctionalProcessEdges(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.constructedServiceEdgesFindings, err = scanConstructedServiceEdges(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.testWorkNormalizationFindings, err = scanTestWorkNormalization(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	testBehaviorFindings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	testBehaviorBaseline, err := loadTestBehaviorBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.testBehaviorFindings, result.staleTestBehaviorEntries, err =
		partitionTestBehaviorFindings(testBehaviorFindings, testBehaviorBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.testBehaviorBaselineCount = len(testBehaviorBaseline.Entries)
	productionDefaultFindings, err := scanProductionDefaultSelections(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	productionDefaultBaseline, err := loadProductionDefaultBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.productionDefaultFindings, result.staleProductionDefaultEntries, err =
		partitionProductionDefaultFindings(productionDefaultFindings, productionDefaultBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.productionDefaultBaselineCount = len(productionDefaultBaseline.Entries)
	initializerBehaviorFindings, err := scanInitializerBehavior(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	initializerBehaviorBaseline, err := loadInitializerBehaviorBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.initializerBehaviorFindings, result.staleInitializerBehaviorEntries, err =
		partitionInitializerBehaviorFindings(initializerBehaviorFindings, initializerBehaviorBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.initializerBehaviorBaselineCount = len(initializerBehaviorBaseline.Entries)
	petriPublicSurfaceFindings, err := scanPetriPublicSurface(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	petriPublicSurfaceBaseline, err := loadPetriPublicSurfaceBaseline(repoRoot)
	if err != nil {
		return scanResult{}, err
	}
	result.petriPublicSurfaceFindings, result.stalePetriPublicSurfaceEntries, err =
		partitionPetriPublicSurfaceFindings(petriPublicSurfaceFindings, petriPublicSurfaceBaseline)
	if err != nil {
		return scanResult{}, err
	}
	result.petriPublicSurfaceBaselineCount = len(petriPublicSurfaceBaseline.Entries)
	result.providerEffectOwnershipFindings, err = scanProviderEffectOwnership(repoRoot)
	if err != nil {
		return scanResult{}, err
	}

	slices.SortFunc(result.rootPackageFindings, func(left, right rootPackageFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	slices.SortFunc(result.migrationShimFindings, func(left, right migrationShimFinding) int {
		return strings.Compare(left.packagePath, right.packagePath)
	})
	slices.SortFunc(result.retiredPackageImportFindings, func(left, right retiredPackageImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return result, nil
}

func scanConvergedServiceSubpackageImports(repoRoot string) ([]transportServiceImplementationFinding, error) {
	packageRoot := filepath.Join(repoRoot, "pkg")
	var findings []transportServiceImplementationFinding
	err := filepath.WalkDir(packageRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			_, isServiceSubpackage := serviceSubpackageOwner(repositoryPath)
			if !isServiceSubpackage {
				continue
			}
			if filePath == "pkg/wire" || strings.HasPrefix(filePath, "pkg/wire/") {
				// Wire is the privileged composition root. Import-direction
				// restrictions for ordinary consumers never apply to it.
				continue
			}
			if isMatchingServiceOwnedTransportConsumer(filePath, repositoryPath) {
				continue
			}
			if isApprovedPeerServiceContractImport(
				filepath.ToSlash(filepath.Dir(filePath)),
				importPath,
			) {
				continue
			}
			_, callerIsService := servicePackageOwner(filePath)
			if callerIsService {
				// Owner-internal imports are allowed here. Peer-service imports
				// are reported by scanPeerServiceImports with baseline support.
				continue
			}

			matchedExplicitPolicy := false
			for privateRoot := range convergedServiceSubpackageRoots {
				if repositoryPath != privateRoot && !strings.HasPrefix(repositoryPath, privateRoot+"/") {
					continue
				}
				matchedExplicitPolicy = true
				findings = append(findings, transportServiceImplementationFinding{
					importPath: importPath,
					filePath:   filePath,
				})
				break
			}
			if matchedExplicitPolicy {
				continue
			}
			if strings.HasPrefix(filePath, "pkg/transports/") &&
				matchesAnyPackageRoot(repositoryPath, transportPrivateServiceSubpackages) {
				// The transport-specific scanner owns this diagnostic.
				continue
			}
			findings = append(findings, transportServiceImplementationFinding{
				importPath: importPath,
				filePath:   filePath,
			})
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan converged service subpackage imports: %w", err)
	}
	slices.SortFunc(findings, func(left, right transportServiceImplementationFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanTestServiceSubpackageImports(repoRoot string) ([]testServiceImportFinding, error) {
	findingsByKey := map[string]testServiceImportFinding{}
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != repoRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		if filePath == "pkg/wire" || strings.HasPrefix(filePath, "pkg/wire/") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return err
		}
		callerOwner, callerIsService := servicePackageOwner(filePath)
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			importedOwner, isServiceSubpackage := serviceSubpackageOwner(repositoryPath)
			if !isServiceSubpackage {
				continue
			}
			if _, publicTransport := serviceOwnedTransportProtocol(repositoryPath); publicTransport {
				continue
			}
			if callerIsService && callerOwner == importedOwner {
				continue
			}
			if isMatchingServiceOwnedTransportConsumer(filePath, repositoryPath) {
				continue
			}
			if _, publicEffectContract := publicExternalEffectContractImports[importPath]; publicEffectContract {
				continue
			}
			finding := testServiceImportFinding{
				owner:      importedOwner,
				importPath: importPath,
				filePath:   filePath,
			}
			findingsByKey[testServiceImportKey(filePath, importPath)] = finding
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan test service subpackage imports: %w", err)
	}
	findings := make([]testServiceImportFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, func(left, right testServiceImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func loadTestServiceImportBaseline(repoRoot string) (testServiceImportBaseline, error) {
	path := filepath.Join(repoRoot, testServiceImportBaselinePath)
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return testServiceImportBaseline{}, nil
		}
		return testServiceImportBaseline{}, fmt.Errorf("read test service import baseline: %w", err)
	}
	var baseline testServiceImportBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return testServiceImportBaseline{}, fmt.Errorf("decode test service import baseline: %w", err)
	}
	if baseline.Version != 1 {
		return testServiceImportBaseline{}, fmt.Errorf(
			"test service import baseline version = %d, want 1",
			baseline.Version,
		)
	}
	if err := requireNonEmptyMigrationBaseline(testServiceImportBaselinePath, len(baseline.Entries)); err != nil {
		return testServiceImportBaseline{}, err
	}
	return baseline, nil
}

func partitionTestServiceImportFindings(
	findings []testServiceImportFinding,
	baseline testServiceImportBaseline,
) ([]testServiceImportFinding, []testServiceImportBaselineEntry, error) {
	baselineByKey := make(map[string]testServiceImportBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateTestServiceImportBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := testServiceImportKey(entry.FilePath, entry.ImportPath)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf(
				"duplicate test service import baseline entry: %s -> %s",
				entry.FilePath,
				entry.ImportPath,
			)
		}
		baselineByKey[key] = entry
	}

	var blocking []testServiceImportFinding
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		key := testServiceImportKey(finding.filePath, finding.importPath)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded {
			blocking = append(blocking, finding)
			continue
		}
		if entry.Owner != finding.owner {
			return nil, nil, fmt.Errorf(
				"test service import baseline edge %s -> %s declares owner %s; detected %s",
				entry.FilePath,
				entry.ImportPath,
				entry.Owner,
				finding.owner,
			)
		}
	}
	var stale []testServiceImportBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right testServiceImportBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return blocking, stale, nil
}

func validateTestServiceImportBaselineEntry(entry testServiceImportBaselineEntry) error {
	if strings.TrimSpace(entry.Owner) == "" ||
		strings.TrimSpace(entry.ImportPath) == "" ||
		strings.TrimSpace(entry.FilePath) == "" ||
		strings.TrimSpace(entry.TargetRoot) == "" ||
		strings.TrimSpace(entry.Stage) == "" ||
		strings.TrimSpace(entry.DeletionGate) == "" {
		return fmt.Errorf("test service import baseline entry is incomplete: %#v", entry)
	}
	for _, value := range []string{entry.Owner, entry.ImportPath, entry.FilePath, entry.TargetRoot} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("test service import baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	if entry.Stage != testServiceImportBaselineStage {
		return fmt.Errorf(
			"test service import baseline entry %s -> %s stage = %q, want %q",
			entry.FilePath,
			entry.ImportPath,
			entry.Stage,
			testServiceImportBaselineStage,
		)
	}
	wantTarget := "pkg/services/" + entry.Owner
	if entry.TargetRoot != wantTarget {
		return fmt.Errorf(
			"test service import baseline entry %s -> %s target = %q, want %q",
			entry.FilePath,
			entry.ImportPath,
			entry.TargetRoot,
			wantTarget,
		)
	}
	if entry.DeletionGate != testServiceImportDeletionGate {
		return fmt.Errorf(
			"test service import baseline entry %s -> %s has an unrecognized deletion gate",
			entry.FilePath,
			entry.ImportPath,
		)
	}
	return nil
}

func createTestServiceImportBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	path := filepath.Join(repoRoot, testServiceImportBaselinePath)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing test service import baseline: %s", testServiceImportBaselinePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat test service import baseline: %w", err)
	}
	findings, err := scanTestServiceSubpackageImports(repoRoot)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return fmt.Errorf("refusing to create empty test service import baseline: no migration debt exists")
	}
	baseline := testServiceImportBaseline{
		Version: 1,
		Entries: make([]testServiceImportBaselineEntry, 0, len(findings)),
	}
	for _, finding := range findings {
		baseline.Entries = append(baseline.Entries, testServiceImportBaselineEntry{
			Owner:        finding.owner,
			ImportPath:   finding.importPath,
			FilePath:     finding.filePath,
			TargetRoot:   "pkg/services/" + finding.owner,
			Stage:        testServiceImportBaselineStage,
			DeletionGate: testServiceImportDeletionGate,
		})
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create test service import baseline: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode test service import baseline: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close test service import baseline: %w", err)
	}
	fmt.Fprintf(
		stdoutWriter,
		"[agent-factory:pkg-boundary] created %s with %d deletion-only edge(s)\n",
		testServiceImportBaselinePath,
		len(baseline.Entries),
	)
	return nil
}

func testServiceImportKey(filePath, importPath string) string {
	return filepath.ToSlash(filePath) + "\x00" + importPath
}

func scanProductServiceConstruction(repoRoot string) ([]serviceConstructionFinding, error) {
	findingsByKey := map[string]serviceConstructionFinding{}
	type packageNameResolution struct {
		name string
		err  error
	}
	packageNames := map[string]packageNameResolution{}
	resolvePackageName := func(importPath string) (string, error) {
		if cached, ok := packageNames[importPath]; ok {
			return cached.name, cached.err
		}
		name, err := resolveRepositoryImportPackageName(repoRoot, importPath)
		packageNames[importPath] = packageNameResolution{name: name, err: err}
		return name, err
	}
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != repoRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		if filePath == "pkg/wire" || strings.HasPrefix(filePath, "pkg/wire/") {
			return nil
		}
		callerOwner, callerIsService := servicePackageOwner(filePath)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		parsedFile, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			return err
		}
		type importedServiceRoot struct {
			importPath string
			owner      string
		}
		importsByName := map[string]importedServiceRoot{}
		var dotImports []importedServiceRoot
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			owner, servicePackage := serviceRootOwner(importPath)
			if !servicePackage {
				repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
				owner, servicePackage = serviceSubpackageOwner(repositoryPath)
			}
			if !servicePackage {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			if isMatchingServiceOwnedTransportConsumer(filePath, repositoryPath) {
				continue
			}
			if callerIsService && callerOwner == owner {
				continue
			}
			name := ""
			if importSpec.Name != nil {
				name = importSpec.Name.Name
			} else {
				name, err = resolvePackageName(importPath)
				if err != nil {
					return fmt.Errorf("resolve package name for %s imported by %s: %w", importPath, filePath, err)
				}
			}
			root := importedServiceRoot{importPath: importPath, owner: owner}
			if name == "." {
				dotImports = append(dotImports, root)
				continue
			}
			if name == "_" {
				continue
			}
			importsByName[name] = root
		}
		record := func(root importedServiceRoot, symbol string, position token.Pos) {
			key := serviceConstructionKey(filePath, root.importPath, symbol)
			finding := findingsByKey[key]
			if finding.count == 0 {
				finding = serviceConstructionFinding{
					owner:      root.owner,
					importPath: root.importPath,
					symbol:     symbol,
					filePath:   filePath,
					line:       fset.Position(position).Line,
				}
			}
			finding.count++
			findingsByKey[key] = finding
		}
		recordExpression := func(expression ast.Expr) {
			switch selected := expression.(type) {
			case *ast.SelectorExpr:
				identifier, ok := selected.X.(*ast.Ident)
				if !ok {
					return
				}
				root, imported := importsByName[identifier.Name]
				if imported && isProhibitedServiceConstructionSymbol(root.importPath, selected.Sel.Name) {
					record(root, selected.Sel.Name, selected.Sel.Pos())
				}
			case *ast.Ident:
				for _, root := range dotImports {
					if isProhibitedServiceConstructionSymbol(root.importPath, selected.Name) {
						record(root, selected.Name, selected.Pos())
					}
				}
			}
		}
		ast.Inspect(parsedFile, func(node ast.Node) bool {
			switch statement := node.(type) {
			case *ast.CallExpr:
				recordExpression(statement.Fun)
				for _, argument := range statement.Args {
					recordExpression(argument)
				}
			case *ast.ValueSpec:
				for _, value := range statement.Values {
					recordExpression(value)
				}
			case *ast.AssignStmt:
				for _, value := range statement.Rhs {
					recordExpression(value)
				}
			case *ast.ReturnStmt:
				for _, value := range statement.Results {
					recordExpression(value)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan product service construction: %w", err)
	}
	findings := make([]serviceConstructionFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, func(left, right serviceConstructionFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(left.importPath, right.importPath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.symbol, right.symbol)
	})
	return findings, nil
}

func resolveRepositoryImportPackageName(repoRoot, importPath string) (string, error) {
	if !strings.HasPrefix(importPath, repositoryImportPrefix) {
		return filepath.Base(importPath), nil
	}
	relativePath := strings.TrimPrefix(importPath, repositoryImportPrefix)
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return "", fmt.Errorf("read package directory %s: %w", filepath.ToSlash(relativePath), err)
	}
	var packageName string
	for _, entry := range entries {
		if entry.IsDir() ||
			filepath.Ext(entry.Name()) != ".go" ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(packageDir, entry.Name())
		parsed, parseErr := parser.ParseFile(
			token.NewFileSet(),
			path,
			nil,
			parser.PackageClauseOnly,
		)
		if parseErr != nil {
			return "", fmt.Errorf("parse package clause in %s: %w", filepath.ToSlash(path), parseErr)
		}
		if packageName == "" {
			packageName = parsed.Name.Name
			continue
		}
		if parsed.Name.Name != packageName {
			return "", fmt.Errorf(
				"package directory %s declares both %q and %q",
				filepath.ToSlash(relativePath),
				packageName,
				parsed.Name.Name,
			)
		}
	}
	if packageName == "" {
		return "", fmt.Errorf("package directory %s has no non-test Go package clause", filepath.ToSlash(relativePath))
	}
	return packageName, nil
}

func loadServiceConstructionBaseline(repoRoot string) (serviceConstructionBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, serviceConstructionBaselinePath))
	if err != nil {
		if os.IsNotExist(err) {
			return serviceConstructionBaseline{}, nil
		}
		return serviceConstructionBaseline{}, fmt.Errorf("read service construction baseline: %w", err)
	}
	var baseline serviceConstructionBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return serviceConstructionBaseline{}, fmt.Errorf("decode service construction baseline: %w", err)
	}
	if baseline.Version != 1 {
		return serviceConstructionBaseline{}, fmt.Errorf("service construction baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(serviceConstructionBaselinePath, len(baseline.Entries)); err != nil {
		return serviceConstructionBaseline{}, err
	}
	return baseline, nil
}

func partitionServiceConstructionFindings(
	findings []serviceConstructionFinding,
	baseline serviceConstructionBaseline,
) ([]serviceConstructionFinding, []serviceConstructionBaselineEntry, error) {
	baselineByKey := make(map[string]serviceConstructionBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateServiceConstructionBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := serviceConstructionKey(entry.FilePath, entry.ImportPath, entry.Symbol)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate service construction baseline entry: %s -> %s.%s", entry.FilePath, entry.ImportPath, entry.Symbol)
		}
		baselineByKey[key] = entry
	}
	var blocking []serviceConstructionFinding
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		key := serviceConstructionKey(finding.filePath, finding.importPath, finding.symbol)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != finding.count {
			blocking = append(blocking, finding)
			continue
		}
		if entry.Owner != finding.owner {
			return nil, nil, fmt.Errorf("service construction baseline %s -> %s.%s declares owner %s; detected %s", entry.FilePath, entry.ImportPath, entry.Symbol, entry.Owner, finding.owner)
		}
	}
	var stale []serviceConstructionBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right serviceConstructionBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(left.ImportPath, right.ImportPath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.Symbol, right.Symbol)
	})
	return blocking, stale, nil
}

func validateServiceConstructionBaselineEntry(entry serviceConstructionBaselineEntry) error {
	if strings.TrimSpace(entry.Owner) == "" ||
		strings.TrimSpace(entry.ImportPath) == "" ||
		strings.TrimSpace(entry.Symbol) == "" ||
		strings.TrimSpace(entry.FilePath) == "" ||
		strings.TrimSpace(entry.Stage) == "" ||
		strings.TrimSpace(entry.DeletionGate) == "" ||
		entry.Count < 1 {
		return fmt.Errorf("service construction baseline entry is incomplete: %#v", entry)
	}
	for _, value := range []string{entry.Owner, entry.ImportPath, entry.Symbol, entry.FilePath} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("service construction baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	wantOwner, servicePackage := serviceRootOwner(entry.ImportPath)
	if !servicePackage {
		repositoryPath := strings.TrimPrefix(entry.ImportPath, repositoryImportPrefix)
		wantOwner, servicePackage = serviceSubpackageOwner(repositoryPath)
	}
	if !servicePackage {
		return fmt.Errorf("service construction baseline entry %s names a non-service import %s", entry.FilePath, entry.ImportPath)
	}
	if !isProhibitedServiceConstructionSymbol(entry.ImportPath, entry.Symbol) {
		return fmt.Errorf("service construction baseline entry %s names an allowed or non-construction symbol %s.%s", entry.FilePath, entry.ImportPath, entry.Symbol)
	}
	if entry.Owner != wantOwner {
		return fmt.Errorf("service construction baseline entry %s owner = %q, want %q", entry.FilePath, entry.Owner, wantOwner)
	}
	if entry.Stage != serviceConstructionBaselineStage || entry.DeletionGate != serviceConstructionDeletionGate {
		return fmt.Errorf("service construction baseline entry %s has an unrecognized migration stage or deletion gate", entry.FilePath)
	}
	return nil
}

func serviceConstructionKey(filePath, importPath, symbol string) string {
	return filepath.ToSlash(filePath) + "\x00" + importPath + "\x00" + symbol
}

func serviceRootOwner(importPath string) (string, bool) {
	const prefix = repositoryImportPrefix + "pkg/services/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	owner := strings.TrimPrefix(importPath, prefix)
	if owner == "" || strings.Contains(owner, "/") {
		return "", false
	}
	return owner, true
}

func isProhibitedServiceConstructionSymbol(importPath, symbol string) bool {
	constructionShaped := false
	for _, prefix := range serviceConstructionPrefixes {
		if symbol == prefix ||
			(strings.HasPrefix(symbol, prefix) &&
				len(symbol) > len(prefix) &&
				symbol[len(prefix)] >= 'A' &&
				symbol[len(prefix)] <= 'Z') {
			constructionShaped = true
			break
		}
	}
	if !constructionShaped {
		return false
	}
	allowedSymbols := allowedServiceValueConstructionSymbols[importPath]
	_, allowed := allowedSymbols[symbol]
	return !allowed
}

func serviceSubpackageOwner(repositoryPath string) (string, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(repositoryPath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(repositoryPath, prefix), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0], true
}

func servicePackageOwner(filePath string) (string, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(filePath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(filePath, prefix), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

func serviceOwnedTransportProtocol(repositoryPath string) (string, bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(repositoryPath, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(repositoryPath, prefix), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "transports" || parts[2] == "" {
		return "", false
	}
	return parts[2], true
}

func isServiceOwnedTransportFile(filePath string) bool {
	packagePath := filepath.ToSlash(filepath.Dir(filePath))
	_, ok := serviceOwnedTransportProtocol(packagePath)
	return ok
}

func isMatchingServiceOwnedTransportConsumer(filePath, importedRepositoryPath string) bool {
	protocol, ok := serviceOwnedTransportProtocol(importedRepositoryPath)
	if !ok {
		return false
	}
	consumerRoot := "pkg/transports/" + protocol
	return filePath == consumerRoot || strings.HasPrefix(filePath, consumerRoot+"/")
}

func matchesAnyPackageRoot(repositoryPath string, roots []string) bool {
	for _, root := range roots {
		if repositoryPath == root || strings.HasPrefix(repositoryPath, root+"/") {
			return true
		}
	}
	return false
}

func scanTransportServiceImplementationImports(repoRoot string) ([]transportServiceImplementationFinding, error) {
	transportRoot := filepath.Join(repoRoot, "pkg", "transports")
	var findings []transportServiceImplementationFinding
	err := filepath.WalkDir(transportRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return err
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			for _, privateRoot := range transportPrivateServiceSubpackages {
				if repositoryPath == privateRoot || strings.HasPrefix(repositoryPath, privateRoot+"/") {
					findings = append(findings, transportServiceImplementationFinding{
						importPath: importPath,
						filePath:   filePath,
					})
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan transport service implementation imports: %w", err)
	}
	slices.SortFunc(findings, func(left, right transportServiceImplementationFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanPeerServiceImports(repoRoot string) ([]peerServiceImportFinding, error) {
	servicesRoot := filepath.Join(repoRoot, "pkg", "services")
	var findings []peerServiceImportFinding
	err := filepath.WalkDir(servicesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		parts := strings.Split(filePath, "/")
		if len(parts) < 4 {
			return nil
		}
		owner := parts[2]
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			servicePrefix := repositoryImportPrefix + "pkg/services/"
			if !strings.HasPrefix(importPath, servicePrefix) {
				continue
			}
			servicePath := strings.TrimPrefix(importPath, servicePrefix)
			serviceParts := strings.Split(servicePath, "/")
			if len(serviceParts) < 2 || serviceParts[0] == owner {
				continue
			}
			if isApprovedPeerServiceContractImport(filepath.ToSlash(filepath.Dir(filePath)), importPath) {
				continue
			}
			findings = append(findings, peerServiceImportFinding{
				owner: owner, peer: serviceParts[0], importPath: importPath, filePath: filePath,
			})
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan peer service implementation imports: %w", err)
	}
	slices.SortFunc(findings, func(left, right peerServiceImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func isApprovedPeerServiceContractImport(packagePath string, importPath string) bool {
	_, approved := approvedPeerServiceContractImports[packagePath+"\x00"+importPath]
	return approved
}

func loadPeerServiceImportBaseline(repoRoot string) (peerServiceImportBaseline, error) {
	path := filepath.Join(repoRoot, peerServiceImportBaselinePath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return peerServiceImportBaseline{}, nil
		}
		return peerServiceImportBaseline{}, fmt.Errorf("read peer service import baseline: %w", err)
	}
	var baseline peerServiceImportBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return peerServiceImportBaseline{}, fmt.Errorf("decode peer service import baseline: %w", err)
	}
	if baseline.Version != 1 {
		return peerServiceImportBaseline{}, fmt.Errorf(
			"decode peer service import baseline: version %d is unsupported; want 1",
			baseline.Version,
		)
	}
	if err := requireNonEmptyMigrationBaseline(peerServiceImportBaselinePath, len(baseline.Entries)); err != nil {
		return peerServiceImportBaseline{}, err
	}
	return baseline, nil
}

func requireNonEmptyMigrationBaseline(path string, entryCount int) error {
	if entryCount == 0 {
		return fmt.Errorf("migration baseline %s is empty; delete the file to record zero debt", path)
	}
	return nil
}

func partitionPeerServiceImportFindings(
	findings []peerServiceImportFinding,
	baseline peerServiceImportBaseline,
) ([]peerServiceImportFinding, []peerServiceImportBaselineEntry, error) {
	baselineByKey := make(map[string]peerServiceImportBaselineEntry, len(baseline.Entries))
	for index, entry := range baseline.Entries {
		if err := validatePeerServiceImportBaselineEntry(entry); err != nil {
			return nil, nil, fmt.Errorf("peer service import baseline entry %d: %w", index, err)
		}
		key := peerServiceImportKey(entry.FilePath, entry.ImportPath)
		if _, exists := baselineByKey[key]; exists {
			return nil, nil, fmt.Errorf(
				"peer service import baseline entry %d: duplicate file/import edge %s -> %s",
				index,
				entry.FilePath,
				entry.ImportPath,
			)
		}
		baselineByKey[key] = entry
	}

	remaining := make(map[string]struct{}, len(findings))
	var violations []peerServiceImportFinding
	for _, finding := range findings {
		key := peerServiceImportKey(finding.filePath, finding.importPath)
		entry, approved := baselineByKey[key]
		if !approved {
			violations = append(violations, finding)
			continue
		}
		if entry.Owner != finding.owner || entry.Peer != finding.peer {
			return nil, nil, fmt.Errorf(
				"peer service import baseline edge %s -> %s declares owner/peer %s/%s; detected %s/%s",
				entry.FilePath,
				entry.ImportPath,
				entry.Owner,
				entry.Peer,
				finding.owner,
				finding.peer,
			)
		}
		remaining[key] = struct{}{}
	}

	var stale []peerServiceImportBaselineEntry
	for key, entry := range baselineByKey {
		if _, exists := remaining[key]; !exists {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right peerServiceImportBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return violations, stale, nil
}

func validatePeerServiceImportBaselineEntry(entry peerServiceImportBaselineEntry) error {
	for field, value := range map[string]string{
		"owner": entry.Owner, "peer": entry.Peer, "importPath": entry.ImportPath,
		"filePath": entry.FilePath, "targetRoot": entry.TargetRoot, "stage": entry.Stage,
		"deletionGate": entry.DeletionGate,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("%s must be exact and cannot contain wildcards", field)
		}
	}
	if entry.Stage != peerServiceImportBaselineStage || entry.DeletionGate != peerServiceImportDeletionGate {
		return fmt.Errorf("stage or deletionGate is not the recognized peer-service migration contract")
	}
	if entry.Owner == entry.Peer {
		return fmt.Errorf("owner and peer must differ")
	}
	expectedImportPrefix := repositoryImportPrefix + "pkg/services/" + entry.Peer + "/"
	if !strings.HasPrefix(entry.ImportPath, expectedImportPrefix) {
		return fmt.Errorf("importPath %q is not a subpackage of peer %q", entry.ImportPath, entry.Peer)
	}
	expectedTargetRoot := "pkg/services/" + entry.Peer
	if entry.TargetRoot != expectedTargetRoot {
		return fmt.Errorf("targetRoot %q must be %q", entry.TargetRoot, expectedTargetRoot)
	}
	expectedOwnerPrefix := "pkg/services/" + entry.Owner
	if entry.FilePath != expectedOwnerPrefix && !strings.HasPrefix(entry.FilePath, expectedOwnerPrefix+"/") {
		return fmt.Errorf("filePath %q is not owned by %q", entry.FilePath, entry.Owner)
	}
	return nil
}

func peerServiceImportKey(filePath, importPath string) string {
	return filepath.ToSlash(filePath) + "\x00" + importPath
}

func scanDomainTransportImports(repoRoot string, exceptions []string) ([]domainTransportImportFinding, error) {
	var findings []domainTransportImportFinding
	for _, domainRoot := range protectedTransportIndependentDomainRoots {
		absoluteRoot := filepath.Join(repoRoot, filepath.FromSlash(domainRoot))
		if _, err := os.Stat(absoluteRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat protected domain root %s: %w", domainRoot, err)
		}

		err := filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			filePath, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			filePath = filepath.ToSlash(filePath)
			if slices.Contains(exceptions, filePath) {
				return nil
			}
			if isServiceOwnedTransportFile(filePath) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("parse Factory package imports %s: %w", filePath, err)
			}
			packagePath := filepath.ToSlash(filepath.Dir(filePath))
			for _, importSpec := range parsedFile.Imports {
				importPath, err := strconv.Unquote(importSpec.Path.Value)
				if err == nil && strings.HasPrefix(importPath, transportImportPrefix) {
					findings = append(findings, domainTransportImportFinding{
						packagePath: packagePath,
						importPath:  importPath,
						filePath:    filePath,
					})
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan protected domain transport imports under %s: %w", domainRoot, err)
		}
	}
	slices.SortFunc(findings, func(left, right domainTransportImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanHandwrittenGeneratedFiles(repoRoot string, exceptions []generatedCodeException) ([]handwrittenGeneratedFinding, error) {
	var findings []handwrittenGeneratedFinding
	for _, exception := range exceptions {
		packageDir := filepath.Join(repoRoot, filepath.FromSlash(exception.packagePath))
		info, err := os.Stat(packageDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat generated-only package %s: %w", exception.packagePath, err)
		}
		if !info.IsDir() {
			continue
		}

		err = filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
				return nil
			}
			relativePath, err := filepath.Rel(packageDir, path)
			if err != nil {
				return err
			}
			if exception.scope == generatedCodeExceptionScopeRoot && filepath.Dir(relativePath) != "." {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated-only package file %s: %w", filepath.ToSlash(path), err)
			}
			if ast.IsGenerated(parsedFile) {
				return nil
			}
			filePath, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			findings = append(findings, handwrittenGeneratedFinding{
				filePath:    filepath.ToSlash(filePath),
				packagePath: exception.packagePath,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan generated-only package %s: %w", exception.packagePath, err)
		}
	}
	slices.SortFunc(findings, func(left, right handwrittenGeneratedFinding) int {
		return strings.Compare(left.filePath, right.filePath)
	})
	return findings, nil
}

func findRetiredPackageRoot(packagePath string) (retiredPackageRoot, bool) {
	for _, retiredRoot := range retiredPackageRoots {
		if packagePath == retiredRoot.packagePath {
			return retiredRoot, true
		}
	}
	return retiredPackageRoot{}, false
}

func scanRetiredPackageImports(repoRoot string, scanRoot string, packageRoot string) ([]retiredPackageImportFinding, error) {
	var findings []retiredPackageImportFinding
	err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read package import file %s: %w", filepath.ToSlash(path), err)
		}
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse package imports %s: %w", filepath.ToSlash(path), err)
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve importing file %s: %w", filepath.ToSlash(path), err)
		}

		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			packagePath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			for _, retiredRoot := range retiredPackageRoots {
				if packagePath != retiredRoot.packagePath && !strings.HasPrefix(packagePath, retiredRoot.packagePath+"/") {
					continue
				}
				findings = append(findings, retiredPackageImportFinding{
					retiredPackageRoot: retiredRoot,
					importPath:         importPath,
					filePath:           filepath.ToSlash(filePath),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s retired package imports: %w", filepath.ToSlash(packageRoot), err)
	}
	return findings, nil
}

func scanApplicationGraphImports(repoRoot string, scanRoot string, packageRoot string) ([]applicationGraphImportFinding, error) {
	var findings []applicationGraphImportFinding
	// The canonical-injector rule applies to every test, including suites outside
	// pkg/. Restricting this walk to packageRoot lets a customer-scale stress or
	// functional test assemble Wire directly. Non-test reusable support remains
	// inventoried separately so its existing exact defects can be migrated.
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != repoRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		outsidePackageRoot := path != scanRoot && !strings.HasPrefix(path, scanRoot+string(filepath.Separator))
		if outsidePackageRoot && !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read package import file %s: %w", filepath.ToSlash(path), err)
		}
		if !strings.Contains(string(content), applicationGraphImportPath) {
			return nil
		}

		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse package imports %s: %w", filepath.ToSlash(path), err)
		}
		if !importsApplicationGraph(parsedFile) {
			return nil
		}

		packageDirectory, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("resolve importing package for %s: %w", filepath.ToSlash(path), err)
		}
		packagePath := filepath.ToSlash(packageDirectory)
		if isApprovedApplicationGraphImporter(packagePath) {
			return nil
		}

		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve importing file %s: %w", filepath.ToSlash(path), err)
		}
		findings = append(findings, applicationGraphImportFinding{
			packagePath: packagePath,
			filePath:    filepath.ToSlash(filePath),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s application graph imports: %w", filepath.ToSlash(packageRoot), err)
	}

	slices.SortFunc(findings, func(left, right applicationGraphImportFinding) int {
		return strings.Compare(left.filePath, right.filePath)
	})
	return findings, nil
}

func importsApplicationGraph(parsedFile *ast.File) bool {
	return slices.ContainsFunc(parsedFile.Imports, func(importSpec *ast.ImportSpec) bool {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		return err == nil && (importPath == applicationGraphImportPath ||
			strings.HasPrefix(importPath, applicationGraphImportPath+"/"))
	})
}

func isApprovedApplicationGraphImporter(packagePath string) bool {
	return slices.ContainsFunc(approvedApplicationGraphImporters, func(approvedPath string) bool {
		return packagePath == approvedPath || strings.HasPrefix(packagePath, approvedPath+"/")
	})
}

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

func writeGeneratedCodeExceptionSummary(writer io.Writer, policy boundaryPolicy) {
	exceptions := generatedCodeExceptionDescriptions(policy)
	if len(exceptions) == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active generated-code exceptions: %s\n", strings.Join(exceptions, ", "))
}

func writeRetiredPackageRootFindings(writer io.Writer, findings []retiredPackageRootFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited retired package root: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  canonical owner: %s\n", finding.canonicalOwner)
		fmt.Fprintf(writer, "  remediation: move the code to %s and delete the retired root.\n", finding.canonicalOwner)
	}
}

func writeRetiredPackageImportFindings(writer io.Writer, findings []retiredPackageImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited retired package import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintf(writer, "  canonical owner: %s\n", finding.canonicalOwner)
		fmt.Fprintf(writer, "  remediation: import %s directly; do not recreate or depend on %s.\n", finding.canonicalOwner, finding.packagePath)
	}
}

func generatedCodeExceptionDescriptions(policy boundaryPolicy) []string {
	descriptions := make([]string, 0, len(policy.generatedCodeExceptions))
	for _, exception := range policy.generatedCodeExceptions {
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", filepath.ToSlash(exception.packagePath), exception.scope))
	}
	return descriptions
}

func writeMigrationShimBlockingFindings(writer io.Writer, findings []migrationShimFinding) {
	if len(findings) == 0 {
		return
	}

	for _, finding := range findings {
		canonicalTarget := finding.canonicalTarget
		if canonicalTarget == "" {
			canonicalTarget = "not detected"
		}
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] blocked migration-only compatibility shim: %s\n", finding.packagePath)
		fmt.Fprintf(writer, "  marker: %s\n", finding.marker)
		fmt.Fprintf(writer, "  canonical target: %s\n", canonicalTarget)
		fmt.Fprintln(writer, "  remediation: import the canonical owner directly and do not recreate Batch 001 root compatibility shims.")
	}
}

func writeApplicationGraphImportFindings(writer io.Writer, findings []applicationGraphImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited application composition import: %s (%s)\n", finding.packagePath, finding.filePath)
		fmt.Fprintln(writer, "  reason: pkg/wire is the outward application composition root and must not be imported by domain or transport packages.")
		fmt.Fprintln(writer, "  remediation: depend on a narrow domain-owned contract and inject the collaborator through pkg/root or pkg/initializer.")
	}
}

func writeDomainTransportImportFindings(writer io.Writer, findings []domainTransportImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited domain transport import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintf(writer, "  domain owner: %s\n", finding.packagePath)
		fmt.Fprintln(writer, "  reason: protected domain packages must not consume transport contracts or adapters.")
		fmt.Fprintln(writer, "  remediation: define the input at its domain owner and map generated values under pkg/transports/mapping.")
	}
}

func writePeerServiceImportFindings(writer io.Writer, findings []peerServiceImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited peer service subpackage import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintf(writer, "  service owner: pkg/services/%s; peer owner: pkg/services/%s\n", finding.owner, finding.peer)
		fmt.Fprintf(writer, "  remediation: publish the required value or capability at pkg/services/%s and import only that peer root.\n", finding.peer)
	}
}

func writeStalePeerServiceBaselineEntries(writer io.Writer, entries []peerServiceImportBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale peer service import baseline entry: %s -> %s\n",
			entry.FilePath,
			entry.ImportPath,
		)
		fmt.Fprintln(writer, "  reason: the recorded bypass edge no longer exists.")
		fmt.Fprintln(writer, "  remediation: remove this entry from service-cross-import-baseline.json in the same change.")
	}
}

func writePeerServiceBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] active peer-service root-contract migration baseline: %d edge(s)\n",
		count,
	)
	fmt.Fprintln(writer, "  deletion gate: migrate every edge to an exact pkg/services/<peer> root import, then delete the baseline.")
}

func writeTestServiceImportFindings(writer io.Writer, findings []testServiceImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] prohibited test import of service internals: %s (%s)\n",
			finding.importPath,
			finding.filePath,
		)
		fmt.Fprintf(writer, "  service owner: pkg/services/%s\n", finding.owner)
		fmt.Fprintln(writer, "  remediation: use the service root contract, move the invariant to the owning service, or exercise cross-service behavior through root.BuildProcess.")
	}
}

func writeStaleTestServiceBaselineEntries(writer io.Writer, entries []testServiceImportBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale test service import baseline entry: %s -> %s\n",
			entry.FilePath,
			entry.ImportPath,
		)
		fmt.Fprintln(writer, "  reason: the concrete cross-owner test import no longer exists.")
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", testServiceImportBaselinePath)
	}
}

func writeTestServiceBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] active test service-internal migration baseline: %d edge(s)\n",
		count,
	)
	fmt.Fprintln(writer, "  deletion gate: move each invariant to its service owner, use a service-root fake, or enter through root.BuildProcess; then delete the exact baseline entry.")
}

func writeServiceConstructionFindings(writer io.Writer, findings []serviceConstructionFinding) {
	for _, finding := range findings {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] prohibited product-service construction: %s.%s (%s:%d, %d selection(s))\n",
			finding.importPath,
			finding.symbol,
			finding.filePath,
			finding.line,
			finding.count,
		)
		fmt.Fprintf(writer, "  service owner: pkg/services/%s\n", finding.owner)
		fmt.Fprintln(writer, "  remediation: construct the collaborator in pkg/wire and inject its service-root role; owner-local invariants may construct it inside the owning service.")
	}
}

func writeStaleServiceConstructionBaselineEntries(writer io.Writer, entries []serviceConstructionBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale service construction baseline entry: %s -> %s.%s\n",
			entry.FilePath,
			entry.ImportPath,
			entry.Symbol,
		)
		fmt.Fprintln(writer, "  reason: the recorded construction selection no longer exists.")
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", serviceConstructionBaselinePath)
	}
}

func writeServiceConstructionBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:pkg-boundary] active product-service construction migration baseline: %d exact file/symbol edge(s)\n",
		count,
	)
	fmt.Fprintln(writer, "  deletion gate: inject each service role from pkg/wire or move the invariant to its owning service, then delete the exact baseline entry.")
}

func writeTransportServiceImplementationFindings(writer io.Writer, findings []transportServiceImplementationFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited transport service implementation import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintln(writer, "  reason: transports may consume only service root contracts or explicitly public service subservices.")
		fmt.Fprintln(writer, "  remediation: publish the required capability at its service boundary and keep representation mapping in the transport.")
	}
}

func writeExternalServiceImplementationFindings(writer io.Writer, findings []transportServiceImplementationFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited external service subpackage import: %s (%s)\n", finding.importPath, finding.filePath)
		fmt.Fprintln(writer, "  reason: service subpackages are owner-internal for ordinary consumers; pkg/wire is the unrestricted composition-root exception.")
		fmt.Fprintln(writer, "  remediation: import the exact pkg/services/<service-name> root and use its published contract.")
	}
}

func writeHandwrittenGeneratedFindings(writer io.Writer, findings []handwrittenGeneratedFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] handwritten Go file in generated-only package: %s (%s)\n", finding.packagePath, finding.filePath)
		fmt.Fprintln(writer, "  reason: generated-only packages may contain only files with the standard Code generated ... DO NOT EDIT. marker.")
		fmt.Fprintln(writer, "  remediation: move handwritten mapping or policy to pkg/transports/http or pkg/transports/mapping.")
	}
}

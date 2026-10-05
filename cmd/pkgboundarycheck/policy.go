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

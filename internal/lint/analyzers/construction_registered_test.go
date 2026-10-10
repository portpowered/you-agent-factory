package analyzers

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// Compiler-valid source exercises the actual production policy, including the
// explicit captured reader and HTTP logger inputs that result inference misses.
func TestConstructionProviderSessionsProductionPolicy(t *testing.T) {
	useFixtures(t)
	const module = "github.com/portpowered/infinite-you"
	modulePrefix = module + "/"
	const root = module + "/pkg/services/provider_sessions"
	files := map[string]string{
		module + "/pkg/services/recordings/reader.go": "package recordings\ntype WorkerCapturedActivityReader interface { Read() }\n",
		"go.uber.org/zap/logger.go":                   "package zap\ntype Logger struct{}\n",
		root + "/contract.go":                         "package provider_sessions\ntype Service interface { Observe() }\n",
		root + "/internal/service/service.go": fmt.Sprintf(`package service
import recordings %q
import sessions %q
type inspectionService struct{ reader recordings.WorkerCapturedActivityReader }
func New(reader recordings.WorkerCapturedActivityReader) (sessions.Service, error) {
 if reader == nil { return nil, nil } // want "required-dependency-guard:.*New.*New"
 return &inspectionService{reader: reader}, nil
}
func (s *inspectionService) Observe() { s.reader.Read() }
func Operation(reader recordings.WorkerCapturedActivityReader) { New(reader) } // want "registered-construction:.*Operation.*internal/service.New"
`, module+"/pkg/services/recordings", root),
		root + "/wire/provider.go": fmt.Sprintf(`package wire
import recordings %q
import sessions %q
import service %q
func NewService(reader recordings.WorkerCapturedActivityReader) (sessions.Service, error) { return service.New(reader) }
`, module+"/pkg/services/recordings", root, root+"/internal/service"),
		root + "/transports/http/adapter.go": fmt.Sprintf(`package http
import sessions %q
import zap "go.uber.org/zap"
type Adapter struct{ sessions sessions.Service }
type Handler struct{ adapter *Adapter; logger *zap.Logger }
func NewAdapter(peer sessions.Service) *Adapter { return &Adapter{sessions: peer} }
func NewHandler(adapter *Adapter, logger *zap.Logger) *Handler {
 if logger == nil { return nil } // want "required-dependency-guard:.*NewHandler.*NewHandler"
 return &Handler{adapter: adapter, logger: logger}
}
func (a *Adapter) Observe() { if a.sessions != nil { a.sessions.Observe() } } // want "required-dependency-guard:.*Observe.*NewAdapter"
func (a *Adapter) Peer() sessions.Service { return a.sessions } // want Peer:"construction-getter=service-getter-locator"
func (a *Adapter) Use() { a.Peer().Observe() } // want "service-getter-locator:.*Use.*\\(Adapter\\).Peer"
func (a *Adapter) Escape() any { return a.Peer } // want "unresolved-service-getter-reference:.*Escape.*\\(Adapter\\).Peer"
func (a *Adapter) View(id string) sessions.Service { _ = id; return a.sessions }
`, root),
		module + "/pkg/services/new_caller/caller.go": fmt.Sprintf(`package new_caller
import wire %q
import recordings %q
func Run(reader recordings.WorkerCapturedActivityReader) { wire.NewService(reader) } // want "registered-construction:.*Run.*provider_sessions/wire.NewService"
`, root+"/wire", module+"/pkg/services/recordings"),
		module + "/pkg/wire/provider.go": fmt.Sprintf(`package wire
import sessions %q
import transport %q
import zap "go.uber.org/zap"
func Provide(peer sessions.Service, logger *zap.Logger) *transport.Handler {
 return transport.NewHandler(transport.NewAdapter(peer), logger)
}
`, root, root+"/transports/http"),
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, registeredConstructionAnalyzer(RepositoryConstructionRegistry()),
		root+"/internal/service", root+"/wire", root+"/transports/http", module+"/pkg/services/new_caller", module+"/pkg/wire")
}

func registeredFixtureRegistry(mode ConstructionMode) ConstructionRegistry {
	const owner = "m/pkg/registeredowner"
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "fixture", OwnerTask: "T20", Mode: mode}},
		Types: []ConstructionType{
			{Symbol: service, CapabilitySet: "fixture", Kind: ConstructionBehavior},
			{Symbol: ConstructionSymbol{ImportPath: owner, Name: "Domain"}, CapabilitySet: "fixture", Kind: ConstructionDomain},
		},
	}
	for _, symbol := range []ConstructionSymbol{
		{ImportPath: owner, Name: "New"}, {ImportPath: owner, Name: "Generic"},
		{ImportPath: owner, Receiver: "Maker", Name: "Construct"},
	} {
		registry.Constructors = append(registry.Constructors, ConstructionConstructor{
			Symbol: symbol, CapabilitySet: "fixture", Results: []ConstructionSymbol{service},
			RequiredParameters: []ConstructionParameter{{Index: 0, TypeExpr: owner + ".Dependency"}},
		})
	}
	registry.Constructors = append(registry.Constructors, ConstructionConstructor{
		Symbol: ConstructionSymbol{ImportPath: owner, Name: "Value"}, CapabilitySet: "fixture",
		Results: []ConstructionSymbol{{ImportPath: owner, Name: "Domain"}},
	})
	return registry
}

func TestConstructionRegisteredMetadataErrors(t *testing.T) {
	useFixtures(t)
	results := analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionReport)), "m/pkg/registeredowner")
	if len(results) != 1 {
		t.Fatalf("analysis results = %d, want one defining unit", len(results))
	}
	tests := []struct {
		name   string
		change func(*ConstructionRegistry)
		want   string
	}{
		{"ownerless set", func(r *ConstructionRegistry) { r.CapabilitySets[0].OwnerTask = "" }, "invalid capability set"},
		{"duplicate set", func(r *ConstructionRegistry) { r.CapabilitySets = append(r.CapabilitySets, r.CapabilitySets[0]) }, "duplicate capability set"},
		{"duplicate type", func(r *ConstructionRegistry) { r.Types = append(r.Types, r.Types[0]) }, "duplicate type"},
		{"missing type", func(r *ConstructionRegistry) { r.Types[0].Symbol.Name = "Missing" }, "missing type declaration"},
		{"invalid kind", func(r *ConstructionRegistry) { r.Types[0].Kind = "invalid" }, "invalid construction kind"},
		{"duplicate constructor", func(r *ConstructionRegistry) { r.Constructors = append(r.Constructors, r.Constructors[0]) }, "duplicate constructor"},
		{"missing constructor", func(r *ConstructionRegistry) { r.Constructors[0].Symbol.Name = "Missing" }, "missing constructor declaration"},
		{"wildcard constructor", func(r *ConstructionRegistry) { r.Constructors[0].Symbol.ImportPath = "m/*" }, "invalid constructor metadata"},
		{"unknown set", func(r *ConstructionRegistry) { r.Constructors[0].CapabilitySet = "missing" }, "invalid constructor metadata"},
		{"parameter index", func(r *ConstructionRegistry) { r.Constructors[0].RequiredParameters[0].Index = 9 }, "invalid or duplicate parameter index"},
		{"duplicate parameter", func(r *ConstructionRegistry) {
			r.Constructors[0].RequiredParameters = append(r.Constructors[0].RequiredParameters, r.Constructors[0].RequiredParameters[0])
		}, "invalid or duplicate parameter index"},
		{"result mismatch", func(r *ConstructionRegistry) { r.Constructors[0].Results[0].Name = "Domain" }, "result type mismatch"},
		{"unclassified result", func(r *ConstructionRegistry) { r.Constructors[0].Results[0].Name = "Missing" }, "matching type classification"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := registeredFixtureRegistry(ConstructionReport)
			test.change(&registry)
			err := validateRegisteredConstruction(results[0].Pass, registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestConstructionRegisteredTypedCallsAndReferences(t *testing.T) {
	useFixtures(t)
	registry := registeredFixtureRegistry(ConstructionEnforce)
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry),
		"m/pkg/registeredowner", "m/pkg/registeredcalls", "m/pkg/registereddot", "m/pkg/registeredexcluded")
	analysistest.Run(t, registeredProviderTestData(t, "pkg/registeredallowed"), registeredConstructionAnalyzer(registry), "m/pkg/wire")
}

func TestConstructionRegisteredReportObservations(t *testing.T) {
	useFixtures(t)
	results := analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionReport)), "m/pkg/registeredreport")
	for _, result := range results {
		findings := result.Result.([]ConstructionFinding)
		if len(findings) != 1 {
			t.Fatalf("findings = %#v, want one report-only observation", findings)
		}
		finding := findings[0]
		if finding.Rule != "registered-construction" || finding.Mode != ConstructionReport || finding.CapabilitySet != "fixture" ||
			finding.Callee.String() != "m/pkg/registeredowner.New" || finding.Caller.String() != "m/pkg/registeredreport.Report" ||
			finding.FilePath != "pkg/registeredreport/report.go" || finding.Line != 5 {
			t.Fatalf("observation lost identity: %#v", finding)
		}
	}
}

func TestConstructionRegisteredBaseline(t *testing.T) {
	useFixtures(t,
		"registered-construction|pkg/registeredlisted|m/pkg/registeredlisted.Listed->m/pkg/registeredowner.New",
		"registered-construction|pkg/registeredstale|m/pkg/registeredstale.Listed->m/pkg/registeredowner.New",
	)
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionEnforce)), "m/pkg/registeredlisted", "m/pkg/registeredstale")
}

// Real compiler objects prove that omission from the constructor list cannot
// exempt an already classified result, even within the owning service.
func TestConstructionUnlistedClassifiedResults(t *testing.T) {
	useFixtures(t)
	const owner = "m/pkg/services/unlisted"
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{
			{Name: "unlisted", OwnerTask: "T29", Mode: ConstructionEnforce},
			{Name: "reporting", OwnerTask: "T29", Mode: ConstructionReport},
		},
		Types: []ConstructionType{{
			Symbol:        ConstructionSymbol{ImportPath: owner, Name: "Reporting"},
			CapabilitySet: "reporting", Kind: ConstructionBehavior,
		}},
	}
	for name, kind := range map[string]ConstructionKind{
		"Service": ConstructionBehavior, "Dependency": ConstructionEffect,
		"Domain": ConstructionDomain, "Scope": ConstructionState, "Resource": ConstructionResource,
	} {
		registry.Types = append(registry.Types, ConstructionType{
			Symbol: ConstructionSymbol{ImportPath: owner, Name: name}, CapabilitySet: "unlisted", Kind: kind,
		})
	}
	files := map[string]string{
		owner + "/owner.go": `package unlisted
type Dependency interface { Run() }
type Service struct{}
type Reporting struct{}
type Alias = Service
type Domain struct{}
type Scope struct{}
type Resource struct{}
func NewAlternate(dep Dependency) *Alias { return &Service{} }
func NewGeneric[T any](dep Dependency) (*Service, error) { return &Service{}, nil }
func NewMixed() (*Reporting, *Service) { return &Reporting{}, &Service{} }
func NewGuarded(dep Dependency) *Service {
 if dep == nil { return nil } // want "required-dependency-guard:.*NewGuarded"
 return &Service{}
}
func NewDomain(payload *Domain) Domain { if payload == nil { return Domain{} }; return *payload }
func NewScope(previous *Scope) *Scope { if previous == nil { return &Scope{} }; return previous }
func OpenResource(previous *Resource) *Resource { if previous == nil { return &Resource{} }; return previous }
func Run(dep Dependency) {
 NewAlternate(dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewAlternate"
 NewGeneric[int](dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewGeneric"
 NewMixed() // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewMixed"
 _ = NewDomain(nil); _ = NewScope(nil); _ = OpenResource(nil)
}
`,
		"m/pkg/services/unrelated/consumer.go": `package unrelated
import owner "m/pkg/services/unlisted"
func Run(dep owner.Dependency) {
 owner.NewAlternate(dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewAlternate"
 create := owner.NewGeneric[int]
 create(dep) // want "registered-construction:.*Run.*m/pkg/services/unlisted.NewGeneric"
 _ = owner.NewDomain(nil); _ = owner.NewScope(nil); _ = owner.OpenResource(nil)
}
func Escape() any { return owner.NewAlternate } // want "unresolved-construction-reference:.*Escape.*NewAlternate"
func Shadowed() { owner := struct { NewAlternate func() }{func(){}}; owner.NewAlternate() }
`,
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, registeredConstructionAnalyzer(registry), owner, "m/pkg/services/unrelated")
}

func TestConstructionRegisteredSignatureValidation(t *testing.T) {
	useFixtures(t)
	registry := registeredFixtureRegistry(ConstructionReport)
	registry.Constructors = registry.Constructors[:1]
	registry.Constructors[0].Symbol.ImportPath = "m/pkg/registeredinvalid"
	registry.Constructors[0].Results[0].ImportPath = "m/pkg/registeredinvalid"
	registry.Types = registry.Types[:1]
	registry.Types[0].Symbol.ImportPath = "m/pkg/registeredinvalid"
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registry), "m/pkg/registeredinvalid")
}

func TestConstructionRegisteredTagged(t *testing.T) {
	useFixtures(t)
	t.Setenv("GOFLAGS", "-tags=backendconformance")
	analysistest.Run(t, analysistest.TestData(), registeredConstructionAnalyzer(registeredFixtureRegistry(ConstructionEnforce)), "m/pkg/registeredtagged")
}

func TestConstructionCompositionBoundaries(t *testing.T) {
	useFixtures(t)
	const owner = "m/pkg/services/catalog/internal/service"
	service := ConstructionSymbol{ImportPath: owner, Name: "Service"}
	registry := ConstructionRegistry{
		CapabilitySets: []ConstructionCapabilitySet{{Name: "catalog", OwnerTask: "T29", Mode: ConstructionEnforce}},
		Types:          []ConstructionType{{Symbol: service, CapabilitySet: "catalog", Kind: ConstructionBehavior}},
	}
	files := map[string]string{
		"github.com/google/wire/wire.go": `package wire
func NewSet(...any) any { return nil }
func Build(...any) string { return "" }
`,
		owner + "/service.go": `package service
type Service struct{}
func New() *Service { return &Service{} }
func Operation() { New() } // want "registered-construction:.*Operation.*catalog/internal/service.New"
`,
		"m/pkg/services/catalog/wire/provider.go": `package wire
import service "m/pkg/services/catalog/internal/service"
func NewCatalog() *service.Service { return service.New() }
func Operation() { service.New() } // want "registered-construction:.*Operation.*catalog/internal/service.New"
type Maker struct{}
func (Maker) NewCatalog() { service.New() } // want "registered-construction:.*Maker.*NewCatalog.*catalog/internal/service.New"
var invalid = service.New() // want "registered-construction:.*<package>.*catalog/internal/service.New"
`,
		"m/pkg/services/other/wire/provider.go": `package wire
import catalog "m/pkg/services/catalog/wire"
func NewOther() { catalog.NewCatalog() } // want "registered-construction:.*NewOther.*catalog/wire.NewCatalog"
`,
		"m/pkg/wire/provider.go": `package wire
import catalog "m/pkg/services/catalog/wire"
import generator "github.com/google/wire"
var providers = generator.NewSet(catalog.NewCatalog)
func BuildCatalog() { generator.Build(catalog.NewCatalog) }
func provideCatalog() { catalog.NewCatalog() }
func Operation() { catalog.NewCatalog() } // want "registered-construction:.*Operation.*catalog/wire.NewCatalog"
func provideDeferred() { defer catalog.NewCatalog() } // want "registered-construction:.*provideDeferred.*catalog/wire.NewCatalog"
func Shadowed() {
 generator := struct { NewSet func(...any) any }{func(...any) any { return nil }}
 generator.NewSet(catalog.NewCatalog) // want "unresolved-construction-reference:.*Shadowed.*catalog/wire.NewCatalog"
}
`,
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, registeredConstructionAnalyzer(registry), owner,
		"m/pkg/services/catalog/wire", "m/pkg/services/other/wire", "m/pkg/wire")
}

// The production registry must reject regressions in the delivered Events
// owner, while preserving retention policy and per-topic state allocation.
func TestConstructionRepositoryEvents(t *testing.T) {
	useFixtures(t)
	const module = "github.com/portpowered/infinite-you"
	modulePrefix = module + "/"
	const owner = module + "/pkg/services/events/internal/service"
	files := map[string]string{
		module + "/pkg/platform/logging/logger.go": `package logging
type Logger interface { Observe() }
`,
		module + "/pkg/services/events/service.go": `package events
type Service interface { Observe() }
`,
		owner + "/store.go": `package service
import "github.com/portpowered/infinite-you/pkg/platform/logging"
import "github.com/portpowered/infinite-you/pkg/services/events"
type topicState struct { values map[string]string }
type Store struct { logger logging.Logger; topics map[string]*topicState; peer events.Service }
func NewWithRetention(retention int, logger logging.Logger) *Store {
 if logger == nil { return nil } // want "required-dependency-guard:.*NewWithRetention"
 if retention <= 0 { retention = 10000 }
 return &Store{logger: logger, topics: make(map[string]*topicState)}
}
func (s *Store) Observe() {
 if s.logger != nil { s.logger.Observe() } // want "required-dependency-guard:.*Observe.*NewWithRetention"
}
func (s *Store) Topic() { s.topics["topic"] = &topicState{values: make(map[string]string)} }
func NewAlternate(logger logging.Logger) *Store { return &Store{logger: logger} }
func NewGeneric[T any](value T, logger logging.Logger) *Store { _ = value; return &Store{logger: logger} }
func NewPeer(peer events.Service) *Store {
 if peer == nil { return nil } // want "required-dependency-guard:.*NewPeer"
 return &Store{peer: peer}
}
func (s *Store) Peer() events.Service { return s.peer } // want Peer:"construction-getter=service-getter-locator"
func (s *Store) UsePeer() {
 if s.peer != nil { s.peer.Observe() } // want "required-dependency-guard:.*UsePeer.*NewPeer"
 s.Peer() // want "service-getter-locator:.*UsePeer.*Peer"
}
func (s *Store) EscapePeer() any { return s.Peer } // want "unresolved-service-getter-reference:.*EscapePeer.*Peer"
func (s *Store) PeerView(_ string) events.Service { return s.peer }
func (s *Store) View() { s.PeerView("scope") }
func Operation(logger logging.Logger) {
 NewWithRetention(0, logger) // want "registered-construction:.*Operation.*NewWithRetention"
 NewAlternate(logger) // want "registered-construction:.*Operation.*NewAlternate"
 NewGeneric("scope", logger) // want "registered-construction:.*Operation.*NewGeneric"
}
`,
		module + "/pkg/services/events/wire/provider.go": `package wire
import "github.com/portpowered/infinite-you/pkg/platform/logging"
import "github.com/portpowered/infinite-you/pkg/services/events"
import "github.com/portpowered/infinite-you/pkg/services/events/internal/service"
func NewService(logger logging.Logger) (events.Service, error) {
 if logger == nil { return nil, nil } // want "required-dependency-guard:.*NewService"
 return service.NewWithRetention(0, logger), nil
}
`,
		module + "/pkg/services/events/internal/consumer/consumer.go": `package consumer
import "github.com/portpowered/infinite-you/pkg/services/events/internal/service"
type View struct { *service.Store }
func Use(view *View) {
 view.Peer() // want "service-getter-locator:.*Use.*\\(Store\\).Peer"
 view.PeerView("scope")
}
func Escape(view *View) any { return view.Peer } // want "unresolved-service-getter-reference:.*Escape.*\\(Store\\).Peer"
`,
		module + "/pkg/services/events/internal/consumer/dot.go": `package consumer
import . "github.com/portpowered/infinite-you/pkg/services/events/internal/service"
import "github.com/portpowered/infinite-you/pkg/platform/logging"
func Dot(logger logging.Logger) {
 NewWithRetention(0, logger) // want "registered-construction:.*Dot.*events/internal/service.NewWithRetention"
}
func Shadow(logger logging.Logger) {
 NewWithRetention := func(_ int, _ logging.Logger) int { return 1 }
 _ = NewWithRetention(0, logger)
}
`,
		module + "/pkg/services/other/consumer.go": `package other
import "github.com/portpowered/infinite-you/pkg/platform/logging"
import eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
func Operation(logger logging.Logger) {
 eventswire.NewService(logger) // want "registered-construction:.*Operation.*events/wire.NewService"
}
`,
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, registeredConstructionAnalyzer(RepositoryConstructionRegistry()), owner,
		module+"/pkg/services/events/wire", module+"/pkg/services/events/internal/consumer", module+"/pkg/services/other")
}

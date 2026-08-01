package wire_test

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

type missingConstructionDependencyCase struct {
	name   string
	mutate func(*constructionPorts)
	want   string
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	base := validConstructionPorts(t)
	for _, test := range missingConstructionDependencyCases() {
		t.Run(test.name, func(t *testing.T) {
			ports := base
			test.mutate(&ports)
			service, err := newServiceForConstructionPorts(ports)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService() error = %v, want %q", err, test.want)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func missingConstructionDependencyCases() []missingConstructionDependencyCase {
	return append(
		missingCoreConstructionDependencyCases(),
		missingCatalogConstructionDependencyCases()...,
	)
}

func missingCoreConstructionDependencyCases() []missingConstructionDependencyCase {
	return []missingConstructionDependencyCase{
		{name: "session host", mutate: func(p *constructionPorts) { p.sessionHost = nil }, want: "session host is required"},
		{name: "activation gateway", mutate: func(p *constructionPorts) { p.activationGateway = nil }, want: "activation gateway is required"},
		{name: "validator", mutate: func(p *constructionPorts) { p.validator = nil }, want: "validator is required"},
		{name: "persistence", mutate: func(p *constructionPorts) { p.persistence = nil }, want: "persistence is required"},
		{name: "loader", mutate: func(p *constructionPorts) { p.loader = nil }, want: "loader is required"},
		{name: "portable bundled files applier", mutate: func(p *constructionPorts) { p.applySupportedFiles = nil }, want: "portable bundled files applier is required"},
		{name: "starter Work applier", mutate: func(p *constructionPorts) { p.applyStarterWork = nil }, want: "starter Work applier is required"},
		{name: "named path resolver", mutate: func(p *constructionPorts) { p.namedPaths = nil }, want: "named path resolver is required"},
		{name: "named Factory catalog filesystem", mutate: func(p *constructionPorts) { p.namedFactoryCatalogFileSystem = nil }, want: "named Factory catalog filesystem is required"},
		{name: "clock", mutate: func(p *constructionPorts) { p.clock = nil }, want: "clock is required"},
	}
}

func missingCatalogConstructionDependencyCases() []missingConstructionDependencyCase {
	return []missingConstructionDependencyCase{
		{name: "version filesystem", mutate: func(p *constructionPorts) { p.versionFileSystem = nil }, want: "version filesystem is required"},
		{name: "effective Factory catalog", mutate: func(p *constructionPorts) { p.listEffective = nil }, want: "effective Factory catalog is required"},
		{name: "packaged Factory catalog list operation", mutate: func(p *constructionPorts) { p.packagedCatalog.List = nil }, want: "packaged Factory catalog list operation is required"},
		{name: "packaged Factory catalog resolve operation", mutate: func(p *constructionPorts) { p.packagedCatalog.Resolve = nil }, want: "packaged Factory catalog resolve operation is required"},
		{name: "packaged Factory installer", mutate: func(p *constructionPorts) { p.packagedInstaller.Install = nil }, want: "packaged Factory installer is required"},
		{name: "required tool checker", mutate: func(p *constructionPorts) { p.requiredToolChecker = nil }, want: "required tool checker is required"},
		{name: "orchestrator definition validator", mutate: func(p *constructionPorts) { p.orchestratorValidator = nil }, want: "orchestrator definition validator is required"},
		{name: "portable filesystem", mutate: func(p *constructionPorts) { p.portableFileSystem = nil }, want: "portable filesystem is required"},
		{name: "directory replacement store", mutate: func(p *constructionPorts) { p.directoryReplacementStore = nil }, want: "directory replacement store is required"},
	}
}

func newServiceForConstructionPorts(ports constructionPorts) (factorydefinitions.Service, error) {
	return factorydefinitionswire.NewService(
		ports.sessionHost, ports.activationGateway, ports.validator, ports.persistence,
		ports.loader, ports.applySupportedFiles, ports.applyStarterWork, ports.namedPaths,
		ports.namedFactoryCatalogFileSystem, ports.clock, ports.versionFileSystem,
		ports.listEffective, ports.packagedCatalog, ports.packagedInstaller,
		ports.requiredToolChecker, ports.orchestratorValidator, ports.portableFileSystem,
		ports.directoryReplacementStore,
	)
}

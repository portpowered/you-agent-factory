package factorydefinitions_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (p fakeDefinitionsPeer) ListBuiltInPackagedFactories(
	_ context.Context,
	_ factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	entries := append([]factorydefinitions.BuiltInPackagedFactoryEntry(nil), p.builtIns...)
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: entries}, nil
}

func (p fakeDefinitionsPeer) InstallPackagedFactory(
	_ context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" || name == "@you/missing" {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
	}
	if name == "@you/fail-install" {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrFactoryDistributeFailed
	}
	factoryDir := p.authoredFactoryDir
	if factoryDir == "" {
		factoryDir = "/factories/" + strings.TrimPrefix(name, "@you/")
	}
	return factorydefinitions.InstallPackagedFactoryResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name:       name,
			FactoryDir: factoryDir,
		},
		Outcome: factorydefinitions.PackagedFactoryInstallCreated,
	}, nil
}

func (p fakeDefinitionsPeer) CreateFactoryScaffold(
	_ context.Context,
	request factorydefinitions.CreateFactoryScaffoldRequest,
) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	targetDir := strings.TrimSpace(request.TargetDir)
	if targetDir == "" || request.Type == "unsupported" {
		return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
	}
	name := "scaffold"
	if request.Type != "" {
		name = request.Type
	}
	factoryDir := targetDir
	if p.authoredFactoryDir != "" {
		factoryDir = p.authoredFactoryDir
	}
	return factorydefinitions.CreateFactoryScaffoldResult{
		Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
			Name:       name,
			FactoryDir: factoryDir,
		},
		ScaffoldType: name,
	}, nil
}

func TestRootService_DistributeSlice_InstallAndScaffoldSameAggregateFacts(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{
		authoredFactoryDir: "/factories/goal",
		builtIns: []factorydefinitions.BuiltInPackagedFactoryEntry{
			{Name: "@you/goal", Project: "builtin-goal"},
		},
	}

	listed, err := service.ListBuiltInPackagedFactories(
		context.Background(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/goal" {
		t.Fatalf("ListBuiltInPackagedFactories result = %#v, want @you/goal entry", listed)
	}

	installed, err := service.InstallPackagedFactory(
		context.Background(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/goal",
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory: %v", err)
	}
	if installed.Definition.Name != "@you/goal" ||
		installed.Definition.FactoryDir != "/factories/goal" ||
		installed.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("InstallPackagedFactory result = %#v, want goal aggregate facts", installed)
	}

	scaffolded, err := service.CreateFactoryScaffold(
		context.Background(),
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: "/factories/goal",
			Type:      string(factorydefinitions.DefaultScaffoldType),
			Executor:  factorydefinitions.DefaultStarterExecutor,
		},
	)
	if err != nil {
		t.Fatalf("CreateFactoryScaffold: %v", err)
	}
	if scaffolded.Definition.FactoryDir != installed.Definition.FactoryDir {
		t.Fatalf(
			"install and scaffold FactoryDir diverge: install=%q scaffold=%q",
			installed.Definition.FactoryDir,
			scaffolded.Definition.FactoryDir,
		)
	}
	if scaffolded.Definition.Name == "" || scaffolded.ScaffoldType == "" {
		t.Fatalf("CreateFactoryScaffold result = %#v, want aggregate identity facts", scaffolded)
	}
}

func TestRootService_DistributeSlice_TypedUnknownIdentityAndDistributeFailure(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}

	_, unknownErr := service.InstallPackagedFactory(
		context.Background(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/missing",
		},
	)
	if !errors.Is(unknownErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf(
			"InstallPackagedFactory unknown-identity error = %v, want %v",
			unknownErr,
			factorydefinitions.ErrUnknownPackagedFactoryIdentity,
		)
	}

	_, installErr := service.InstallPackagedFactory(
		context.Background(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/fail-install",
		},
	)
	if !errors.Is(installErr, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf(
			"InstallPackagedFactory distribute failure = %v, want %v",
			installErr,
			factorydefinitions.ErrFactoryDistributeFailed,
		)
	}
	if errors.Is(installErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatal("install failure must not also match ErrUnknownPackagedFactoryIdentity")
	}

	_, scaffoldErr := service.CreateFactoryScaffold(
		context.Background(),
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: "/factories/alpha",
			Type:      "unsupported",
		},
	)
	if !errors.Is(scaffoldErr, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf(
			"CreateFactoryScaffold distribute failure = %v, want %v",
			scaffoldErr,
			factorydefinitions.ErrFactoryDistributeFailed,
		)
	}
	if errors.Is(scaffoldErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatal("scaffold failure must not also match ErrUnknownPackagedFactoryIdentity")
	}
}

func TestRootService_AllSixSlices_ReachableThroughSingularService(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"name":"alpha"}`)
	var service factorydefinitions.Service = fakeDefinitionsPeer{
		entries: []factorydefinitions.NamedFactoryListEntry{
			{Name: "alpha", FactoryDir: "/factories/alpha", Current: true},
		},
		authoredCanonical:  payload,
		authoredFactoryDir: "/factories/alpha",
		builtIns: []factorydefinitions.BuiltInPackagedFactoryEntry{
			{Name: "@you/goal", Project: "builtin-goal"},
		},
	}

	if _, err := service.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: "/factories"},
	); err != nil {
		t.Fatalf("catalog slice: %v", err)
	}
	if _, err := service.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	); err != nil {
		t.Fatalf("authoring slice: %v", err)
	}
	if _, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  payload,
			FactoryDir: "/factories/alpha",
		},
	); err != nil {
		t.Fatalf("compile slice: %v", err)
	}
	if _, err := service.ValidateStructuralFactoryDefinition(
		context.Background(),
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{Canonical: payload},
	); err != nil {
		t.Fatalf("validate slice: %v", err)
	}
	if _, err := service.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/alpha",
			Canonical:  payload,
			Name:       "alpha",
		},
	); err != nil {
		t.Fatalf("snapshot slice: %v", err)
	}
	if _, err := service.InstallPackagedFactory(
		context.Background(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/goal",
		},
	); err != nil {
		t.Fatalf("distribute slice: %v", err)
	}
}

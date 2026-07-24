package factorydefinitions_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// fakeDefinitionsPeer is a peer-owned stand-in that depends only on the
// Factory Definitions root package. It proves cross-service consumers can
// satisfy the singular root Service without importing Definitions
// implementation subpackages.
type fakeDefinitionsPeer struct {
	entries            []factorydefinitions.NamedFactoryListEntry
	authoredCanonical  []byte
	authoredFactoryDir string
}

func (fakeDefinitionsPeer) ActivateNamedFactory(context.Context, string) error {
	return nil
}

func (fakeDefinitionsPeer) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, nil
}

func (fakeDefinitionsPeer) GetCurrentNamedFactory(
	context.Context,
) (*factorydefinitions.FactorySnapshot, error) {
	return nil, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fakeDefinitionsPeer) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fakeDefinitionsPeer) CurrentFactoryDefinitionVersionAtRoot(
	string,
	string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (p fakeDefinitionsPeer) ListNamedFactories(
	_ context.Context,
	_ factorydefinitions.ListNamedFactoriesRequest,
) (factorydefinitions.ListNamedFactoriesResult, error) {
	entries := append([]factorydefinitions.NamedFactoryListEntry(nil), p.entries...)
	return factorydefinitions.ListNamedFactoriesResult{Entries: entries}, nil
}

func (p fakeDefinitionsPeer) GetNamedFactory(
	_ context.Context,
	request factorydefinitions.GetNamedFactoryRequest,
) (factorydefinitions.GetNamedFactoryResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	for _, entry := range p.entries {
		if entry.Name == request.Name {
			return factorydefinitions.GetNamedFactoryResult{Entry: entry}, nil
		}
	}
	return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (p fakeDefinitionsPeer) ResolveNamedFactory(
	_ context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	for _, entry := range p.entries {
		if entry.Name == request.Name {
			return factorydefinitions.ResolveNamedFactoryResult{
				Resolution: factorydefinitions.NamedFactoryResolution{
					Name:               entry.Name,
					FactoryDir:         entry.FactoryDir,
					Source:             factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
					ProjectRoot:        request.ProjectRoot,
					GlobalRoot:         request.GlobalRoot,
					PrecedenceDecision: factorydefinitions.NamedFactoryPrecedenceDecisionNone,
				},
			}, nil
		}
	}
	return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fakeDefinitionsPeer) DeleteNamedFactory(
	_ context.Context,
	request factorydefinitions.DeleteNamedFactoryRequest,
) (factorydefinitions.DeleteNamedFactoryResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.DeleteNamedFactoryResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	return factorydefinitions.DeleteNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (p fakeDefinitionsPeer) GetCurrentFactoryPointer(
	_ context.Context,
	_ factorydefinitions.GetCurrentFactoryPointerRequest,
) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	for _, entry := range p.entries {
		if entry.Current {
			return factorydefinitions.GetCurrentFactoryPointerResult{
				Name:       entry.Name,
				FactoryDir: entry.FactoryDir,
			}, nil
		}
	}
	return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fakeDefinitionsPeer) SetCurrentFactoryPointer(
	_ context.Context,
	request factorydefinitions.SetCurrentFactoryPointerRequest,
) (factorydefinitions.SetCurrentFactoryPointerResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	return factorydefinitions.SetCurrentFactoryPointerResult{Name: request.Name}, nil
}

func (p fakeDefinitionsPeer) PrepareFactoryLayout(
	_ context.Context,
	request factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	if len(request.Payload) == 0 || string(request.Payload) == "{" {
		return factorydefinitions.PrepareFactoryLayoutResult{}, factorydefinitions.ErrMalformedFactoryLayoutPayload
	}
	canonical := append([]byte(nil), request.Payload...)
	if len(p.authoredCanonical) > 0 {
		canonical = append([]byte(nil), p.authoredCanonical...)
	}
	return factorydefinitions.PrepareFactoryLayoutResult{
		Prepared: factorydefinitions.PreparedFactoryLayoutPayload{Canonical: canonical},
	}, nil
}

func (p fakeDefinitionsPeer) FlattenFactoryLayout(
	_ context.Context,
	_ factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	return factorydefinitions.FlattenFactoryLayoutResult{
		Canonical: append([]byte(nil), p.authoredCanonical...),
	}, nil
}

func (p fakeDefinitionsPeer) ExpandFactoryLayout(
	_ context.Context,
	_ factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	return factorydefinitions.ExpandFactoryLayoutResult{
		FactoryDir: p.authoredFactoryDir,
		Report:     factorydefinitions.LayoutExpansionReport{},
	}, nil
}

func (p fakeDefinitionsPeer) CreateNamedFactory(
	_ context.Context,
	request factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	if request.Name == "fail-write" {
		return factorydefinitions.CreateNamedFactoryResult{}, &factorydefinitions.AtomicFactoryWriteFailure{
			Name:              request.Name,
			FactoryDir:        p.authoredFactoryDir,
			PreviousPreserved: true,
		}
	}
	return factorydefinitions.CreateNamedFactoryResult{
		Name:       request.Name,
		FactoryDir: p.authoredFactoryDir,
	}, nil
}

func (p fakeDefinitionsPeer) ReplaceNamedFactory(
	_ context.Context,
	request factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	if request.Name == "fail-write" {
		return factorydefinitions.ReplaceNamedFactoryResult{}, &factorydefinitions.AtomicFactoryWriteFailure{
			Name:              request.Name,
			FactoryDir:        p.authoredFactoryDir,
			PreviousPreserved: true,
		}
	}
	return factorydefinitions.ReplaceNamedFactoryResult{
		Name:       request.Name,
		FactoryDir: p.authoredFactoryDir,
	}, nil
}

func (p fakeDefinitionsPeer) CompileEffectiveFactorySource(
	_ context.Context,
	request factorydefinitions.CompileEffectiveFactorySourceRequest,
) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	canonical := strings.TrimSpace(string(request.Canonical))
	if canonical == "" || canonical == "{" {
		return factorydefinitions.CompileEffectiveFactorySourceResult{}, factorydefinitions.ErrInvalidAuthoredFactorySource
	}
	if strings.Contains(canonical, `"$unresolved"`) {
		return factorydefinitions.CompileEffectiveFactorySourceResult{}, factorydefinitions.ErrUnresolvedDefinitionReference
	}
	factoryDir := request.FactoryDir
	if factoryDir == "" {
		factoryDir = p.authoredFactoryDir
	}
	return factorydefinitions.CompileEffectiveFactorySourceResult{
		Effective: factorydefinitions.EffectiveFactorySource{
			FactoryDir:      factoryDir,
			RuntimeBaseDir:  factoryDir,
			ContentIdentity: canonical,
		},
	}, nil
}

func (fakeDefinitionsPeer) ValidateStructuralFactoryDefinition(
	_ context.Context,
	request factorydefinitions.ValidateStructuralFactoryDefinitionRequest,
) (factorydefinitions.ValidateStructuralFactoryDefinitionResult, error) {
	canonical := strings.TrimSpace(string(request.Canonical))
	if canonical == "" || canonical == "{" {
		return factorydefinitions.ValidateStructuralFactoryDefinitionResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	if strings.Contains(canonical, `"invalidTopology"`) ||
		strings.Contains(canonical, `"invalidTool"`) ||
		strings.Contains(canonical, `"invalidStrategy"`) ||
		strings.Contains(canonical, `"invalidLayout"`) {
		code := factorydefinitions.ValidationCodeFactoryPayloadInvalid
		switch {
		case strings.Contains(canonical, `"invalidLayout"`):
			code = factorydefinitions.ValidationCodeLayoutInvalidValue
		case strings.Contains(canonical, `"invalidTopology"`):
			code = "factory.topology.invalid"
		case strings.Contains(canonical, `"invalidTool"`):
			code = "factory.tool.required"
		case strings.Contains(canonical, `"invalidStrategy"`):
			code = "factory.strategy.invalid"
		}
		return factorydefinitions.ValidateStructuralFactoryDefinitionResult{}, &factorydefinitions.FactoryDefinitionValidationFailure{
			Validation: factorydefinitions.ValidationResult{
				Targets: []factorydefinitions.ValidationTarget{{
					Code:     code,
					Severity: factorydefinitions.ValidationSeverityError,
					Message:  "definition validation failed",
					Subject: factorydefinitions.ValidationSubject{
						Type:     factorydefinitions.ValidationSubjectTypeFactory,
						Location: factorydefinitions.ValidationSubjectLocationDefinition,
					},
				}},
			},
		}
	}
	return factorydefinitions.ValidateStructuralFactoryDefinitionResult{
		Validation: factorydefinitions.ValidationResult{},
	}, nil
}

func (fakeDefinitionsPeer) ValidateEffectiveFactoryDefinition(
	_ context.Context,
	request factorydefinitions.ValidateEffectiveFactoryDefinitionRequest,
) (factorydefinitions.ValidateEffectiveFactoryDefinitionResult, error) {
	canonical := strings.TrimSpace(string(request.Canonical))
	if canonical == "" {
		canonical = strings.TrimSpace(request.Effective.ContentIdentity)
	}
	if canonical == "" || canonical == "{" {
		return factorydefinitions.ValidateEffectiveFactoryDefinitionResult{}, factorydefinitions.ErrInvalidFactoryDefinitionPayload
	}
	if strings.Contains(canonical, `"invalidTopology"`) {
		return factorydefinitions.ValidateEffectiveFactoryDefinitionResult{}, &factorydefinitions.FactoryDefinitionValidationFailure{
			Validation: factorydefinitions.ValidationResult{
				Targets: []factorydefinitions.ValidationTarget{{
					Code:     "factory.topology.invalid",
					Severity: factorydefinitions.ValidationSeverityError,
					Message:  "effective definition validation failed",
					Subject: factorydefinitions.ValidationSubject{
						Type:     factorydefinitions.ValidationSubjectTypeFactory,
						Location: factorydefinitions.ValidationSubjectLocationDefinition,
					},
				}},
			},
		}
	}
	return factorydefinitions.ValidateEffectiveFactoryDefinitionResult{
		Validation: factorydefinitions.ValidationResult{},
	}, nil
}

func (p fakeDefinitionsPeer) CaptureFactorySnapshot(
	_ context.Context,
	request factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	canonical := strings.TrimSpace(string(request.Canonical))
	if canonical == "" || canonical == "{" || !strings.HasPrefix(canonical, "{") {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	name := request.Name
	if name == "" {
		name = "alpha"
	}
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name":             name,
		"factoryDirectory": request.FactoryDir,
	})
	if err != nil {
		return factorydefinitions.CaptureFactorySnapshotResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	return factorydefinitions.CaptureFactorySnapshotResult{Snapshot: snapshot}, nil
}

func (p fakeDefinitionsPeer) PrepareFactorySnapshotImport(
	_ context.Context,
	request factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	payload := strings.TrimSpace(string(request.Payload))
	if payload == "" || payload == "{" || !strings.HasPrefix(payload, "{") {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name": "alpha",
	})
	if err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	factoryDir := p.authoredFactoryDir
	if factoryDir == "" {
		factoryDir = "/factories/alpha"
	}
	return factorydefinitions.PrepareFactorySnapshotImportResult{
		Snapshot: snapshot,
		Name:     "alpha",
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: factoryDir,
			Assets: []factorydefinitions.PortableSnapshotAssetFact{
				{TargetPath: "factory/docs/README.md"},
			},
		},
	}, nil
}

func (p fakeDefinitionsPeer) MaterializeFactorySnapshot(
	_ context.Context,
	request factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	targetDir := strings.TrimSpace(request.TargetDir)
	if targetDir == "" || request.Snapshot == nil || strings.Contains(targetDir, "..") {
		return factorydefinitions.MaterializeFactorySnapshotResult{}, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize
	}
	return factorydefinitions.MaterializeFactorySnapshotResult{
		TargetDir: targetDir,
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: targetDir,
			Assets: []factorydefinitions.PortableSnapshotAssetFact{
				{TargetPath: "factory/docs/README.md"},
			},
		},
	}, nil
}

func TestRootService_FakePeerReadPath_TypedNotFound(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}
	snapshot, err := service.GetCurrentNamedFactory(context.Background())
	if snapshot != nil {
		t.Fatalf("GetCurrentNamedFactory snapshot = %#v, want nil", snapshot)
	}
	if !errors.Is(err, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentNamedFactory error = %v, want %v",
			err,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}
}

func TestRootService_CatalogSlice_SuccessDetachedEntries(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{
		entries: []factorydefinitions.NamedFactoryListEntry{
			{
				Name:       "alpha",
				FactoryDir: "/factories/alpha",
				Current:    true,
			},
		},
	}

	listed, err := service.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: "/factories"},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "alpha" {
		t.Fatalf("ListNamedFactories result = %#v, want alpha entry", listed)
	}

	got, err := service.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: "/factories", Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("GetNamedFactory: %v", err)
	}
	if got.Entry.Name != "alpha" || got.Entry.FactoryDir != "/factories/alpha" {
		t.Fatalf("GetNamedFactory result = %#v, want alpha identity facts", got)
	}

	pointer, err := service.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: "/factories"},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != "/factories/alpha" {
		t.Fatalf("GetCurrentFactoryPointer result = %#v, want alpha current pointer", pointer)
	}
}

func TestRootService_CatalogSlice_TypedInvalidNameAndMissing(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}

	_, invalidErr := service.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: "/factories", Name: "../evil"},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatalf(
			"GetNamedFactory invalid-name error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidNamedFactoryName,
		)
	}

	_, missingErr := service.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: "/factories", Name: "missing"},
	)
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"GetNamedFactory missing error = %v, want %v",
			missingErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}
	if errors.Is(missingErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatal("missing named Factory must not also match ErrInvalidNamedFactoryName")
	}
}

func TestRootService_AuthoringSlice_PrepareFlattenExpandRoundTrip(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"name":"alpha"}`)
	var service factorydefinitions.Service = fakeDefinitionsPeer{
		authoredCanonical:  payload,
		authoredFactoryDir: "/factories/alpha",
	}

	prepared, err := service.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if string(prepared.Prepared.Canonical) != string(payload) {
		t.Fatalf("PrepareFactoryLayout canonical = %q, want %q", prepared.Prepared.Canonical, payload)
	}

	flattened, err := service.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}
	if string(flattened.Canonical) != string(payload) {
		t.Fatalf("FlattenFactoryLayout canonical = %q, want %q", flattened.Canonical, payload)
	}

	expanded, err := service.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir != "/factories/alpha" {
		t.Fatalf("ExpandFactoryLayout factoryDir = %q, want /factories/alpha", expanded.FactoryDir)
	}

	created, err := service.CreateNamedFactory(
		context.Background(),
		factorydefinitions.CreateNamedFactoryRequest{
			RootDir:  "/factories",
			Name:     "alpha",
			Prepared: prepared.Prepared,
		},
	)
	if err != nil {
		t.Fatalf("CreateNamedFactory: %v", err)
	}
	if created.Name != "alpha" || created.FactoryDir != "/factories/alpha" {
		t.Fatalf("CreateNamedFactory result = %#v, want alpha identity facts", created)
	}

	replaced, err := service.ReplaceNamedFactory(
		context.Background(),
		factorydefinitions.ReplaceNamedFactoryRequest{
			RootDir:  "/factories",
			Name:     "alpha",
			Prepared: prepared.Prepared,
		},
	)
	if err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}
	if replaced.Name != "alpha" || replaced.FactoryDir != "/factories/alpha" {
		t.Fatalf("ReplaceNamedFactory result = %#v, want alpha identity facts", replaced)
	}
}

func TestRootService_AuthoringSlice_TypedMalformedAndAtomicWriteFailure(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{
		authoredFactoryDir: "/factories/alpha",
	}

	_, malformedErr := service.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: []byte("{")},
	)
	if !errors.Is(malformedErr, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatalf(
			"PrepareFactoryLayout malformed error = %v, want %v",
			malformedErr,
			factorydefinitions.ErrMalformedFactoryLayoutPayload,
		)
	}

	_, createErr := service.CreateNamedFactory(
		context.Background(),
		factorydefinitions.CreateNamedFactoryRequest{RootDir: "/factories", Name: "fail-write"},
	)
	var writeFailure *factorydefinitions.AtomicFactoryWriteFailure
	if !errors.As(createErr, &writeFailure) {
		t.Fatalf("CreateNamedFactory error = %v, want AtomicFactoryWriteFailure", createErr)
	}
	if !writeFailure.PreviousPreserved {
		t.Fatal("AtomicFactoryWriteFailure.PreviousPreserved = false, want true")
	}
	if !errors.Is(createErr, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatalf(
			"CreateNamedFactory error = %v, want %v",
			createErr,
			factorydefinitions.ErrAtomicFactoryWriteFailed,
		)
	}
	if errors.Is(createErr, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatal("atomic write failure must not also match ErrMalformedFactoryLayoutPayload")
	}
}

func TestRootService_CompileSlice_EquivalentInputsSameEffectiveIdentity(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{
		authoredFactoryDir: "/factories/alpha",
	}

	first, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`  {"name":"alpha"}  `),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource first: %v", err)
	}

	second, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`{"name":"alpha"}`),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource second: %v", err)
	}

	if first.Effective.ContentIdentity == "" {
		t.Fatal("CompileEffectiveFactorySource ContentIdentity is empty")
	}
	if first.Effective.ContentIdentity != second.Effective.ContentIdentity {
		t.Fatalf(
			"equivalent inputs produced different ContentIdentity: %q vs %q",
			first.Effective.ContentIdentity,
			second.Effective.ContentIdentity,
		)
	}
	if first.Effective.FactoryDir != "/factories/alpha" ||
		first.Effective.RuntimeBaseDir != "/factories/alpha" {
		t.Fatalf("CompileEffectiveFactorySource effective = %#v, want alpha identity facts", first.Effective)
	}
}

func TestRootService_CompileSlice_TypedInvalidSourceAndUnresolvedReference(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}

	_, invalidErr := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"CompileEffectiveFactorySource invalid-source error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidAuthoredFactorySource,
		)
	}

	_, unresolvedErr := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"worker":"$unresolved"}`),
		},
	)
	if !errors.Is(unresolvedErr, factorydefinitions.ErrUnresolvedDefinitionReference) {
		t.Fatalf(
			"CompileEffectiveFactorySource unresolved error = %v, want %v",
			unresolvedErr,
			factorydefinitions.ErrUnresolvedDefinitionReference,
		)
	}
	if errors.Is(unresolvedErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatal("unresolved definition reference must not also match ErrInvalidAuthoredFactorySource")
	}
}

func TestRootService_ValidateSlice_ValidDefinitionNoErrorFindings(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}
	payload := []byte(`{"name":"alpha"}`)

	structural, err := service.ValidateStructuralFactoryDefinition(
		context.Background(),
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: payload,
			Profile:   factorydefinitions.ValidationProfilePrePersist,
		},
	)
	if err != nil {
		t.Fatalf("ValidateStructuralFactoryDefinition: %v", err)
	}
	if structural.Validation.HasBlockingTargets() {
		t.Fatalf("ValidateStructuralFactoryDefinition findings = %#v, want none", structural.Validation)
	}

	effective, err := service.ValidateEffectiveFactoryDefinition(
		context.Background(),
		factorydefinitions.ValidateEffectiveFactoryDefinitionRequest{
			Canonical: payload,
			Effective: factorydefinitions.EffectiveFactorySource{
				FactoryDir:      "/factories/alpha",
				RuntimeBaseDir:  "/factories/alpha",
				ContentIdentity: string(payload),
			},
		},
	)
	if err != nil {
		t.Fatalf("ValidateEffectiveFactoryDefinition: %v", err)
	}
	if effective.Validation.HasBlockingTargets() {
		t.Fatalf("ValidateEffectiveFactoryDefinition findings = %#v, want none", effective.Validation)
	}
}

func TestRootService_ValidateSlice_TypedInvalidPayloadAndFindings(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}

	_, invalidErr := service.ValidateStructuralFactoryDefinition(
		context.Background(),
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition invalid-payload error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidFactoryDefinitionPayload,
		)
	}

	_, findingsErr := service.ValidateStructuralFactoryDefinition(
		context.Background(),
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"invalidLayout":true}`),
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	var validationFailure *factorydefinitions.FactoryDefinitionValidationFailure
	if !errors.As(findingsErr, &validationFailure) {
		t.Fatalf("ValidateStructuralFactoryDefinition error = %v, want FactoryDefinitionValidationFailure", findingsErr)
	}
	if !errors.Is(findingsErr, factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition error = %v, want %v",
			findingsErr,
			factorydefinitions.ErrFactoryDefinitionValidationFailed,
		)
	}
	if errors.Is(findingsErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatal("validation findings must not also match ErrInvalidFactoryDefinitionPayload")
	}
	if len(validationFailure.Validation.Targets) == 0 {
		t.Fatal("FactoryDefinitionValidationFailure must carry validation targets")
	}
	hasErrorFinding := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Severity == factorydefinitions.ValidationSeverityError {
			hasErrorFinding = true
		}
		if strings.Contains(strings.ToLower(target.Code), "petri") ||
			strings.Contains(strings.ToLower(target.Message), "petri") {
			t.Fatalf("published validation findings must not use Petri vocabulary: %#v", target)
		}
	}
	if !hasErrorFinding {
		t.Fatal("FactoryDefinitionValidationFailure must carry at least one error-severity finding")
	}
}

func TestRootService_SnapshotSlice_CaptureAndImportMaterializeSuccess(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{
		authoredFactoryDir: "/factories/alpha",
	}
	payload := []byte(`{"name":"alpha"}`)

	captured, err := service.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{
			FactoryDir: "/factories/alpha",
			Canonical:  payload,
			Name:       "alpha",
		},
	)
	if err != nil {
		t.Fatalf("CaptureFactorySnapshot: %v", err)
	}
	if captured.Snapshot == nil {
		t.Fatal("CaptureFactorySnapshot snapshot is nil")
	}
	var capturedObject map[string]any
	if decodeErr := captured.Snapshot.Decode(&capturedObject); decodeErr != nil {
		t.Fatalf("CaptureFactorySnapshot decode: %v", decodeErr)
	}
	if capturedObject["name"] != "alpha" {
		t.Fatalf("CaptureFactorySnapshot name = %#v, want alpha", capturedObject["name"])
	}

	imported, err := service.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactorySnapshotImport: %v", err)
	}
	if imported.Snapshot == nil || imported.Name != "alpha" {
		t.Fatalf("PrepareFactorySnapshotImport result = %#v, want alpha snapshot facts", imported)
	}
	if imported.Portable.FactoryDir != "/factories/alpha" || len(imported.Portable.Assets) == 0 {
		t.Fatalf("PrepareFactorySnapshotImport portable = %#v, want portable success facts", imported.Portable)
	}

	materialized, err := service.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "/factories/alpha",
			Snapshot:  imported.Snapshot,
		},
	)
	if err != nil {
		t.Fatalf("MaterializeFactorySnapshot: %v", err)
	}
	if materialized.TargetDir != "/factories/alpha" ||
		materialized.Portable.FactoryDir != "/factories/alpha" ||
		len(materialized.Portable.Assets) == 0 {
		t.Fatalf("MaterializeFactorySnapshot result = %#v, want portable success facts", materialized)
	}
}

func TestRootService_SnapshotSlice_TypedInvalidPayloadAndUnsafeMaterialize(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}

	_, invalidErr := service.PrepareFactorySnapshotImport(
		context.Background(),
		factorydefinitions.PrepareFactorySnapshotImportRequest{Payload: []byte(`["not-object"]`)},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"PrepareFactorySnapshotImport invalid-payload error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}

	_, captureInvalidErr := service.CaptureFactorySnapshot(
		context.Background(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"string"`)},
	)
	if !errors.Is(captureInvalidErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"CaptureFactorySnapshot invalid-payload error = %v, want %v",
			captureInvalidErr,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}

	_, unsafeErr := service.MaterializeFactorySnapshot(
		context.Background(),
		factorydefinitions.MaterializeFactorySnapshotRequest{
			TargetDir: "../outside",
			Snapshot:  nil,
		},
	)
	if !errors.Is(unsafeErr, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf(
			"MaterializeFactorySnapshot unsafe error = %v, want %v",
			unsafeErr,
			factorydefinitions.ErrUnsafeFactorySnapshotMaterialize,
		)
	}
	if errors.Is(unsafeErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatal("unsafe materialize must not also match ErrInvalidFactorySnapshotPayload")
	}
}

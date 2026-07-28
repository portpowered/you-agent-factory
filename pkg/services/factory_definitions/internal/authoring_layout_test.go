package internal_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

type stubAuthoringLayout struct{}

func (stubAuthoringLayout) PrepareFactoryLayout(
	_ context.Context,
	request factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return factorydefinitions.PrepareFactoryLayoutResult{
		Prepared: factorydefinitions.PreparedFactoryLayoutPayload{
			Canonical: request.Payload,
		},
	}, nil
}

func (stubAuthoringLayout) FlattenFactoryLayout(
	_ context.Context,
	request factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	return factorydefinitions.FlattenFactoryLayoutResult{Canonical: []byte(request.Path)}, nil
}

func (stubAuthoringLayout) ExpandFactoryLayout(
	_ context.Context,
	request factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	return factorydefinitions.ExpandFactoryLayoutResult{FactoryDir: request.Path}, nil
}

func (stubAuthoringLayout) CreateNamedFactory(
	_ context.Context,
	request factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	return factorydefinitions.CreateNamedFactoryResult{
		Name:       request.Name,
		FactoryDir: request.RootDir + "/" + request.Name,
	}, nil
}

func (stubAuthoringLayout) ReplaceNamedFactory(
	_ context.Context,
	request factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	return factorydefinitions.ReplaceNamedFactoryResult{
		Name:       request.Name,
		FactoryDir: request.RootDir + "/" + request.Name,
	}, nil
}

var _ authoringlayout.Service = stubAuthoringLayout{}

func TestAttachAuthoringLayout_DelegatesCTRDEFAuthoringSlice(t *testing.T) {
	t.Parallel()

	base := factorydefinition.New(nil)
	attached, err := factoryinternal.AttachAuthoringLayout(base, stubAuthoringLayout{})
	if err != nil {
		t.Fatalf("AttachAuthoringLayout: %v", err)
	}

	payload := []byte(`{"name":"alpha"}`)
	prepared, err := attached.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if string(prepared.Prepared.Canonical) != string(payload) {
		t.Fatalf("PrepareFactoryLayout canonical = %q, want %q", prepared.Prepared.Canonical, payload)
	}

	flattened, err := attached.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil || string(flattened.Canonical) != "/factories/alpha" {
		t.Fatalf("FlattenFactoryLayout = %#v, %v", flattened, err)
	}

	expanded, err := attached.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil || expanded.FactoryDir != "/factories/alpha" {
		t.Fatalf("ExpandFactoryLayout = %#v, %v", expanded, err)
	}

	created, err := attached.CreateNamedFactory(
		context.Background(),
		factorydefinitions.CreateNamedFactoryRequest{
			RootDir:  "/factories",
			Name:     "alpha",
			Prepared: prepared.Prepared,
		},
	)
	if err != nil || created.FactoryDir != "/factories/alpha" {
		t.Fatalf("CreateNamedFactory = %#v, %v", created, err)
	}

	replaced, err := attached.ReplaceNamedFactory(
		context.Background(),
		factorydefinitions.ReplaceNamedFactoryRequest{
			RootDir:  "/factories",
			Name:     "alpha",
			Prepared: prepared.Prepared,
		},
	)
	if err != nil || replaced.FactoryDir != "/factories/alpha" {
		t.Fatalf("ReplaceNamedFactory = %#v, %v", replaced, err)
	}
}

func TestAttachAuthoringLayout_RejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := factoryinternal.AttachAuthoringLayout(nil, stubAuthoringLayout{}); err == nil {
		t.Fatal("AttachAuthoringLayout(nil service) expected error")
	}
	if _, err := factoryinternal.AttachAuthoringLayout(
		factorydefinition.New(nil),
		nil,
	); err == nil {
		t.Fatal("AttachAuthoringLayout(nil authoring) expected error")
	}
}

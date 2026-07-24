package service_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
)

func TestAuthoringLayoutOwnsPrepareCreateReplaceAndFlattenExpand(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"name":"alpha"}`)
	prepared := factorydefinitions.PreparedFactoryLayoutPayload{Canonical: payload}
	var prepareCalls, flattenCalls, expandCalls, createCalls, replaceCalls int

	svc, err := authoringlayoutwire.NewService(authoringlayout.Ports{
		Prepare: func(_ context.Context, name string, got []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			prepareCalls++
			if name != "alpha" || string(got) != string(payload) {
				t.Fatalf("Prepare ports got name=%q payload=%q", name, got)
			}
			return prepared, nil
		},
		Flatten: func(path string) ([]byte, error) {
			flattenCalls++
			if path != "/factories/alpha" {
				t.Fatalf("Flatten path = %q", path)
			}
			return payload, nil
		},
		Expand: func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			expandCalls++
			if path != "/factories/alpha" {
				t.Fatalf("Expand path = %q", path)
			}
			return "/factories/alpha", factorydefinitions.LayoutExpansionReport{FactoryConfigPaths: 1}, nil
		},
		Create: func(rootDir, name string, got factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			createCalls++
			if rootDir != "/factories" || name != "alpha" || string(got.Canonical) != string(payload) {
				t.Fatalf("Create got root=%q name=%q prepared=%#v", rootDir, name, got)
			}
			return "/factories/alpha", nil
		},
		Replace: func(rootDir, name string, got factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			replaceCalls++
			if rootDir != "/factories" || name != "alpha" || string(got.Canonical) != string(payload) {
				t.Fatalf("Replace got root=%q name=%q prepared=%#v", rootDir, name, got)
			}
			return "/factories/alpha", nil
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	preparedResult, err := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: payload,
	})
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if string(preparedResult.Prepared.Canonical) != string(payload) {
		t.Fatalf("prepared canonical = %q", preparedResult.Prepared.Canonical)
	}

	created, err := svc.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: preparedResult.Prepared,
	})
	if err != nil {
		t.Fatalf("CreateNamedFactory: %v", err)
	}
	if created.Name != "alpha" || created.FactoryDir != "/factories/alpha" {
		t.Fatalf("CreateNamedFactory result = %#v", created)
	}

	replaced, err := svc.ReplaceNamedFactory(ctx, factorydefinitions.ReplaceNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: preparedResult.Prepared,
	})
	if err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}
	if replaced.Name != "alpha" || replaced.FactoryDir != "/factories/alpha" {
		t.Fatalf("ReplaceNamedFactory result = %#v", replaced)
	}

	flattened, err := svc.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{
		Path: "/factories/alpha",
	})
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}
	if string(flattened.Canonical) != string(payload) {
		t.Fatalf("flattened = %q", flattened.Canonical)
	}

	expanded, err := svc.ExpandFactoryLayout(ctx, factorydefinitions.ExpandFactoryLayoutRequest{
		Path: "/factories/alpha",
	})
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir != "/factories/alpha" || expanded.Report.FactoryConfigPaths != 1 {
		t.Fatalf("ExpandFactoryLayout result = %#v", expanded)
	}

	if prepareCalls != 1 || flattenCalls != 1 || expandCalls != 1 || createCalls != 1 || replaceCalls != 1 {
		t.Fatalf(
			"port calls prepare=%d flatten=%d expand=%d create=%d replace=%d",
			prepareCalls, flattenCalls, expandCalls, createCalls, replaceCalls,
		)
	}
}

func TestNewServiceRejectsMissingPorts(t *testing.T) {
	t.Parallel()

	svc, err := authoringlayoutwire.NewService(authoringlayout.Ports{})
	if err == nil || svc != nil {
		t.Fatalf("NewService(empty ports) = (%v, %v), want deterministic dependency error", svc, err)
	}
}

func TestAuthoringLayoutPreservesRoundTripAuthoredSemantics(t *testing.T) {
	t.Parallel()

	// In-memory authored layout store proves prepare → create/replace → flatten/expand
	// preserves identity and content through the private authoring_layout path.
	store := map[string][]byte{}
	factoryPath := func(rootDir, name string) string {
		return rootDir + "/" + name
	}

	authoredPayload := []byte(`{"name":"alpha","workStations":[{"id":"intake"}]}`)
	preparedCanonical := []byte(`{"name":"alpha","workStations":[{"id":"intake"}]}`)

	svc, err := authoringlayoutwire.NewService(authoringlayout.Ports{
		Prepare: func(_ context.Context, name string, got []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			if name != "alpha" || string(got) != string(authoredPayload) {
				t.Fatalf("Prepare got name=%q payload=%q", name, got)
			}
			return factorydefinitions.PreparedFactoryLayoutPayload{
				Canonical: append([]byte(nil), preparedCanonical...),
			}, nil
		},
		Create: func(rootDir, name string, prepared factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			path := factoryPath(rootDir, name)
			store[path] = append([]byte(nil), prepared.Canonical...)
			return path, nil
		},
		Replace: func(rootDir, name string, prepared factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			path := factoryPath(rootDir, name)
			if _, ok := store[path]; !ok {
				t.Fatalf("Replace missing prior content at %q", path)
			}
			store[path] = append([]byte(nil), prepared.Canonical...)
			return path, nil
		},
		Flatten: func(path string) ([]byte, error) {
			canonical, ok := store[path]
			if !ok {
				return nil, errors.New("missing authored layout")
			}
			return append([]byte(nil), canonical...), nil
		},
		Expand: func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			if _, ok := store[path]; !ok {
				return "", factorydefinitions.LayoutExpansionReport{}, errors.New("missing authored layout")
			}
			return path, factorydefinitions.LayoutExpansionReport{FactoryConfigPaths: 1}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	prepared, err := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: authoredPayload,
	})
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if string(prepared.Prepared.Canonical) != string(preparedCanonical) {
		t.Fatalf(
			"prepared canonical = %q, want CTR-DEF prepared outcome %q",
			prepared.Prepared.Canonical,
			preparedCanonical,
		)
	}

	created, err := svc.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: prepared.Prepared,
	})
	if err != nil {
		t.Fatalf("CreateNamedFactory: %v", err)
	}
	if created.Name != "alpha" || created.FactoryDir != "/factories/alpha" {
		t.Fatalf("CreateNamedFactory result = %#v, want Definitions-owned identity facts", created)
	}

	flattenedAfterCreate, err := svc.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{
		Path: created.FactoryDir,
	})
	if err != nil {
		t.Fatalf("FlattenFactoryLayout after create: %v", err)
	}
	if string(flattenedAfterCreate.Canonical) != string(prepared.Prepared.Canonical) {
		t.Fatalf(
			"flatten after create = %q, want prepared canonical %q",
			flattenedAfterCreate.Canonical,
			prepared.Prepared.Canonical,
		)
	}

	expanded, err := svc.ExpandFactoryLayout(ctx, factorydefinitions.ExpandFactoryLayoutRequest{
		Path: created.FactoryDir,
	})
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir != created.FactoryDir || expanded.Report.FactoryConfigPaths != 1 {
		t.Fatalf("ExpandFactoryLayout result = %#v, want factory directory identity", expanded)
	}

	replaced, err := svc.ReplaceNamedFactory(ctx, factorydefinitions.ReplaceNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: prepared.Prepared,
	})
	if err != nil {
		t.Fatalf("ReplaceNamedFactory: %v", err)
	}
	if replaced.Name != "alpha" || replaced.FactoryDir != "/factories/alpha" {
		t.Fatalf("ReplaceNamedFactory result = %#v, want Definitions-owned identity facts", replaced)
	}

	flattenedAfterReplace, err := svc.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{
		Path: replaced.FactoryDir,
	})
	if err != nil {
		t.Fatalf("FlattenFactoryLayout after replace: %v", err)
	}
	if string(flattenedAfterReplace.Canonical) != string(prepared.Prepared.Canonical) {
		t.Fatalf(
			"flatten after replace = %q, want prepared canonical %q",
			flattenedAfterReplace.Canonical,
			prepared.Prepared.Canonical,
		)
	}
}

func TestRootAuthoringSurfaceSucceedsThroughPrivateOwnership(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"name":"alpha"}`)
	authoring, err := authoringlayoutwire.NewService(authoringlayout.Ports{
		Prepare: func(_ context.Context, _ string, got []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return factorydefinitions.PreparedFactoryLayoutPayload{Canonical: append([]byte(nil), got...)}, nil
		},
		Flatten: func(string) ([]byte, error) {
			return payload, nil
		},
		Expand: func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return path, factorydefinitions.LayoutExpansionReport{}, nil
		},
		Create: func(_, name string, prepared factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "/factories/" + name, nil
		},
		Replace: func(_, name string, _ factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "/factories/" + name, nil
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var root factorydefinitions.Service = rootAuthoringAdapter{authoring: authoring}
	ctx := context.Background()

	prepared, err := root.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: payload,
	})
	if err != nil {
		t.Fatalf("root PrepareFactoryLayout: %v", err)
	}
	created, err := root.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: prepared.Prepared,
	})
	if err != nil {
		t.Fatalf("root CreateNamedFactory: %v", err)
	}
	if created.FactoryDir != "/factories/alpha" {
		t.Fatalf("root CreateNamedFactory factoryDir = %q", created.FactoryDir)
	}
	replaced, err := root.ReplaceNamedFactory(ctx, factorydefinitions.ReplaceNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: prepared.Prepared,
	})
	if err != nil {
		t.Fatalf("root ReplaceNamedFactory: %v", err)
	}
	if replaced.FactoryDir != "/factories/alpha" {
		t.Fatalf("root ReplaceNamedFactory factoryDir = %q", replaced.FactoryDir)
	}

	flattened, err := root.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{
		Path: "/factories/alpha",
	})
	if err != nil {
		t.Fatalf("root FlattenFactoryLayout: %v", err)
	}
	if string(flattened.Canonical) != string(payload) {
		t.Fatalf("root FlattenFactoryLayout = %q", flattened.Canonical)
	}
	expanded, err := root.ExpandFactoryLayout(ctx, factorydefinitions.ExpandFactoryLayoutRequest{
		Path: "/factories/alpha",
	})
	if err != nil {
		t.Fatalf("root ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir != "/factories/alpha" {
		t.Fatalf("root ExpandFactoryLayout factoryDir = %q", expanded.FactoryDir)
	}
}

func TestAuthoringLayoutMapsMalformedAndAtomicWriteFailures(t *testing.T) {
	t.Parallel()

	svc, err := authoringlayoutwire.NewService(authoringlayout.Ports{
		Prepare: func(context.Context, string, []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return factorydefinitions.PreparedFactoryLayoutPayload{}, errors.New("decode failed")
		},
		Flatten: func(string) ([]byte, error) { return nil, errors.New("unused") },
		Expand: func(string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return "", factorydefinitions.LayoutExpansionReport{}, errors.New("unused")
		},
		Create: func(string, string, factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "", errors.New("disk full")
		},
		Replace: func(string, string, factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "", errors.New("unused")
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	_, prepareErr := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: []byte("{"),
	})
	if !errors.Is(prepareErr, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatalf("PrepareFactoryLayout error = %v, want ErrMalformedFactoryLayoutPayload", prepareErr)
	}

	_, createErr := svc.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha",
	})
	var writeFailure *factorydefinitions.AtomicFactoryWriteFailure
	if !errors.As(createErr, &writeFailure) {
		t.Fatalf("CreateNamedFactory error = %v, want AtomicFactoryWriteFailure", createErr)
	}
	if !writeFailure.PreviousPreserved {
		t.Fatal("PreviousPreserved = false, want true")
	}
	if !errors.Is(createErr, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatalf("CreateNamedFactory error = %v, want ErrAtomicFactoryWriteFailed", createErr)
	}
	if errors.Is(createErr, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatal("atomic write failure must not also match ErrMalformedFactoryLayoutPayload")
	}
}

// rootAuthoringAdapter proves the Definitions root authoring surface can succeed
// only through the private authoring_layout subservice, not a second authority.
type rootAuthoringAdapter struct {
	factorydefinitions.UnimplementedService
	authoring authoringlayout.Service
}

func (r rootAuthoringAdapter) ActivateNamedFactory(context.Context, string) error {
	return errors.New("activate is outside authoring_layout ownership")
}

func (r rootAuthoringAdapter) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, errors.New("save is outside authoring_layout ownership")
}

func (r rootAuthoringAdapter) GetCurrentNamedFactory(context.Context) (*factorydefinitions.FactorySnapshot, error) {
	return nil, errors.New("current named factory is outside authoring_layout ownership")
}

func (r rootAuthoringAdapter) GetCurrentFactoryForSession(context.Context, string) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, errors.New("session current factory is outside authoring_layout ownership")
}

func (r rootAuthoringAdapter) CurrentFactoryDefinitionVersionAtRoot(string, string) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, errors.New("definition version is outside authoring_layout ownership")
}

func (r rootAuthoringAdapter) PrepareFactoryLayout(
	ctx context.Context,
	request factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return r.authoring.PrepareFactoryLayout(ctx, request)
}

func (r rootAuthoringAdapter) FlattenFactoryLayout(
	ctx context.Context,
	request factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	return r.authoring.FlattenFactoryLayout(ctx, request)
}

func (r rootAuthoringAdapter) ExpandFactoryLayout(
	ctx context.Context,
	request factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	return r.authoring.ExpandFactoryLayout(ctx, request)
}

func (r rootAuthoringAdapter) CreateNamedFactory(
	ctx context.Context,
	request factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	return r.authoring.CreateNamedFactory(ctx, request)
}

func (r rootAuthoringAdapter) ReplaceNamedFactory(
	ctx context.Context,
	request factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	return r.authoring.ReplaceNamedFactory(ctx, request)
}

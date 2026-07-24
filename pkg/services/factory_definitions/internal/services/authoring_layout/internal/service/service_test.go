package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestAuthoringLayoutPublicSurfaceRejectsForbiddenOwnershipLeaks(t *testing.T) {
	t.Parallel()

	// Service contract is authoring-only: no catalog, compilation, validation,
	// snapshot/portability, or distribution ownership methods.
	serviceType := reflect.TypeOf((*authoringlayout.Service)(nil)).Elem()
	// reflect reports interface methods in lexicographic order.
	wantMethods := []string{
		"CreateNamedFactory",
		"ExpandFactoryLayout",
		"FlattenFactoryLayout",
		"PrepareFactoryLayout",
		"ReplaceNamedFactory",
	}
	gotMethods := make([]string, 0, serviceType.NumMethod())
	for i := 0; i < serviceType.NumMethod(); i++ {
		gotMethods = append(gotMethods, serviceType.Method(i).Name)
	}
	if !reflect.DeepEqual(gotMethods, wantMethods) {
		t.Fatalf("Service methods = %v, want authoring-only %v", gotMethods, wantMethods)
	}
	for _, forbidden := range []string{
		"ListNamedFactories",
		"DeleteNamedFactory",
		"SetCurrentNamedFactoryPointer",
		"CompileEffectiveFactorySource",
		"ValidateStructuralFactoryDefinition",
		"ValidateEffectiveFactoryDefinition",
		"CaptureFactorySnapshot",
		"PrepareFactorySnapshotImport",
		"MaterializeFactorySnapshot",
		"InstallPackagedFactory",
		"CreateFactoryScaffold",
		"ActivateNamedFactory",
		"Save",
	} {
		if _, ok := serviceType.MethodByName(forbidden); ok {
			t.Fatalf("Service exposes out-of-lease method %q", forbidden)
		}
	}

	// Ports accept only exact injected effect funcs with Definitions-owned
	// vocabulary — not peer implementations, Runtime/Petri, or Wire/root types.
	portsType := reflect.TypeOf(authoringlayout.Ports{})
	wantPorts := []string{"Prepare", "Flatten", "Expand", "Create", "Replace"}
	if portsType.NumField() != len(wantPorts) {
		t.Fatalf("Ports fields = %d, want %d exact effect ports", portsType.NumField(), len(wantPorts))
	}
	for i, wantName := range wantPorts {
		field := portsType.Field(i)
		if field.Name != wantName {
			t.Fatalf("Ports field[%d] = %q, want %q", i, field.Name, wantName)
		}
		if field.Type.Kind() != reflect.Func {
			t.Fatalf("Ports.%s type = %v, want func effect port", field.Name, field.Type)
		}
		assertAuthoringLayoutSurfaceTypesAllowed(t, "Ports."+field.Name, field.Type)
	}

	// Construction API takes only Ports — callers cannot select peer services
	// or Wire/root composition ownership through authoring_layout wire.
	ctorType := reflect.TypeOf(authoringlayoutwire.NewService)
	if ctorType.NumIn() != 1 || ctorType.In(0) != portsType {
		t.Fatalf("NewService inputs = %v, want exactly Ports", ctorType)
	}
	if ctorType.NumOut() != 2 ||
		ctorType.Out(0) != serviceType ||
		ctorType.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("NewService outputs = %v, want (Service, error)", ctorType)
	}

	// Observable runtime proof: construct and exercise authoring through exact
	// ports alone, with no peer/Wire/root collaborator injected.
	payload := []byte(`{"name":"surface"}`)
	prepared := factorydefinitions.PreparedFactoryLayoutPayload{Canonical: payload}
	var prepareCalls int
	svc, err := authoringlayoutwire.NewService(authoringlayout.Ports{
		Prepare: func(_ context.Context, name string, got []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			prepareCalls++
			if name != "surface" || string(got) != string(payload) {
				t.Fatalf("Prepare got name=%q payload=%q", name, got)
			}
			return prepared, nil
		},
		Flatten: func(string) ([]byte, error) { return payload, nil },
		Expand: func(string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return "/factories/surface", factorydefinitions.LayoutExpansionReport{}, nil
		},
		Create:  func(string, string, factorydefinitions.PreparedFactoryLayoutPayload) (string, error) { return "/factories/surface", nil },
		Replace: func(string, string, factorydefinitions.PreparedFactoryLayoutPayload) (string, error) { return "/factories/surface", nil },
	})
	if err != nil {
		t.Fatalf("NewService from exact ports: %v", err)
	}
	result, err := svc.PrepareFactoryLayout(context.Background(), factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "surface", Payload: payload,
	})
	if err != nil {
		t.Fatalf("PrepareFactoryLayout through exact ports: %v", err)
	}
	if string(result.Prepared.Canonical) != string(payload) || prepareCalls != 1 {
		t.Fatalf("prepare via exact ports = %#v calls=%d", result, prepareCalls)
	}
}

func assertAuthoringLayoutSurfaceTypesAllowed(t *testing.T, path string, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Func:
		for i := 0; i < typ.NumIn(); i++ {
			assertAuthoringLayoutSurfaceTypesAllowed(t, path+".in", typ.In(i))
		}
		for i := 0; i < typ.NumOut(); i++ {
			assertAuthoringLayoutSurfaceTypesAllowed(t, path+".out", typ.Out(i))
		}
		return
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan, reflect.Map:
		if typ.Kind() == reflect.Map {
			assertAuthoringLayoutSurfaceTypesAllowed(t, path+".key", typ.Key())
		}
		assertAuthoringLayoutSurfaceTypesAllowed(t, path+".elem", typ.Elem())
		return
	case reflect.Interface:
		if typ == reflect.TypeOf((*error)(nil)).Elem() || typ == reflect.TypeOf((*context.Context)(nil)).Elem() {
			return
		}
		t.Fatalf("%s exposes interface type %v outside Definitions-owned effect vocabulary", path, typ)
		return
	}

	pkg := typ.PkgPath()
	if authoringLayoutSurfacePackageAllowed(pkg) {
		// Definitions-owned value types are opaque here; do not walk nested fields.
		return
	}
	for _, forbiddenPrefix := range []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
		"github.com/portpowered/infinite-you/pkg/services/workers",
		"github.com/portpowered/infinite-you/pkg/services/automations",
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/root",
	} {
		if pkg == forbiddenPrefix || strings.HasPrefix(pkg, forbiddenPrefix+"/") {
			t.Fatalf("%s exposes type %v from forbidden ownership package %q", path, typ, pkg)
		}
	}
	t.Fatalf("%s exposes type %v from non-Definitions package %q", path, typ, pkg)
}

func authoringLayoutSurfacePackageAllowed(pkg string) bool {
	const definitionsRoot = "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	const definitionsContracts = definitionsRoot + "/contracts"
	switch {
	case pkg == "", pkg == "context", pkg == definitionsRoot:
		return true
	case pkg == definitionsContracts, strings.HasPrefix(pkg, definitionsContracts+"/"):
		return true
	default:
		return false
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

func TestPublicRootPreservesCTRDEFAuthoringSuccessEquivalence(t *testing.T) {
	t.Parallel()

	// Peer-facing subject is the public Definitions root Service only; private
	// authoring_layout stays behind the adapter and is not a peer import surface.
	payload := []byte(`{"name":"alpha"}`)
	var root factorydefinitions.Service = newPublicRootDelegatingToAuthoringLayout(t, authoringlayout.Ports{
		Prepare: func(_ context.Context, _ string, got []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return factorydefinitions.PreparedFactoryLayoutPayload{Canonical: append([]byte(nil), got...)}, nil
		},
		Flatten: func(string) ([]byte, error) {
			return append([]byte(nil), payload...), nil
		},
		Expand: func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return path, factorydefinitions.LayoutExpansionReport{}, nil
		},
		Create: func(_, name string, _ factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "/factories/" + name, nil
		},
		Replace: func(_, name string, _ factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			return "/factories/" + name, nil
		},
	})

	prepared, err := root.PrepareFactoryLayout(
		context.Background(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout: %v", err)
	}
	if string(prepared.Prepared.Canonical) != string(payload) {
		t.Fatalf("PrepareFactoryLayout canonical = %q, want %q", prepared.Prepared.Canonical, payload)
	}

	flattened, err := root.FlattenFactoryLayout(
		context.Background(),
		factorydefinitions.FlattenFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("FlattenFactoryLayout: %v", err)
	}
	if string(flattened.Canonical) != string(payload) {
		t.Fatalf("FlattenFactoryLayout canonical = %q, want %q", flattened.Canonical, payload)
	}

	expanded, err := root.ExpandFactoryLayout(
		context.Background(),
		factorydefinitions.ExpandFactoryLayoutRequest{Path: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("ExpandFactoryLayout: %v", err)
	}
	if expanded.FactoryDir != "/factories/alpha" {
		t.Fatalf("ExpandFactoryLayout factoryDir = %q, want /factories/alpha", expanded.FactoryDir)
	}

	created, err := root.CreateNamedFactory(
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

	replaced, err := root.ReplaceNamedFactory(
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

func TestPublicRootPreservesCTRDEFAuthoringTypedFailureEquivalence(t *testing.T) {
	t.Parallel()

	var root factorydefinitions.Service = newPublicRootDelegatingToAuthoringLayout(t, authoringlayout.Ports{
		Prepare: func(_ context.Context, _ string, payload []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			if len(payload) == 0 || string(payload) == "{" {
				return factorydefinitions.PreparedFactoryLayoutPayload{}, errors.New("decode failed")
			}
			return factorydefinitions.PreparedFactoryLayoutPayload{Canonical: append([]byte(nil), payload...)}, nil
		},
		Flatten: func(string) ([]byte, error) {
			return nil, errors.New("unused")
		},
		Expand: func(string) (string, factorydefinitions.LayoutExpansionReport, error) {
			return "", factorydefinitions.LayoutExpansionReport{}, errors.New("unused")
		},
		Create: func(_, name string, _ factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			if name == "fail-write" {
				return "/factories/alpha", errors.New("disk full")
			}
			return "/factories/" + name, nil
		},
		Replace: func(_, name string, _ factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
			if name == "fail-write" {
				return "/factories/alpha", errors.New("disk full")
			}
			return "/factories/" + name, nil
		},
	})

	_, malformedErr := root.PrepareFactoryLayout(
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

	_, createErr := root.CreateNamedFactory(
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

func newPublicRootDelegatingToAuthoringLayout(t *testing.T, ports authoringlayout.Ports) factorydefinitions.Service {
	t.Helper()
	authoring, err := authoringlayoutwire.NewService(ports)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return rootAuthoringAdapter{authoring: authoring}
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

func TestAuthoringLayoutPreservesPriorContentOnFailedAtomicWrite(t *testing.T) {
	t.Parallel()

	// Stateful layout store: create succeeds, replace fails without mutating prior bytes.
	store := map[string][]byte{}
	factoryPath := func(rootDir, name string) string {
		return rootDir + "/" + name
	}
	priorCanonical := []byte(`{"name":"alpha","workStations":[{"id":"intake"}]}`)
	attemptedCanonical := []byte(`{"name":"alpha","workStations":[{"id":"corrupt"}]}`)

	svc, err := authoringlayoutwire.NewService(authoringlayout.Ports{
		Prepare: func(_ context.Context, _ string, got []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error) {
			if string(got) == "{" {
				return factorydefinitions.PreparedFactoryLayoutPayload{}, errors.New("decode failed")
			}
			return factorydefinitions.PreparedFactoryLayoutPayload{
				Canonical: append([]byte(nil), got...),
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
			if string(prepared.Canonical) != string(attemptedCanonical) {
				t.Fatalf("Replace prepared = %q, want attempted %q", prepared.Canonical, attemptedCanonical)
			}
			// Fail without writing: prior on-disk (in-memory) content stays unchanged.
			return path, errors.New("atomic rename failed")
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
	preparedPrior, err := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: priorCanonical,
	})
	if err != nil {
		t.Fatalf("PrepareFactoryLayout prior: %v", err)
	}
	created, err := svc.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: preparedPrior.Prepared,
	})
	if err != nil {
		t.Fatalf("CreateNamedFactory: %v", err)
	}

	baseline, err := svc.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{
		Path: created.FactoryDir,
	})
	if err != nil {
		t.Fatalf("FlattenFactoryLayout baseline: %v", err)
	}
	if string(baseline.Canonical) != string(priorCanonical) {
		t.Fatalf("baseline = %q, want prior %q", baseline.Canonical, priorCanonical)
	}

	_, prepareMalformedErr := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: []byte("{"),
	})
	if !errors.Is(prepareMalformedErr, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatalf(
			"PrepareFactoryLayout malformed error = %v, want ErrMalformedFactoryLayoutPayload",
			prepareMalformedErr,
		)
	}
	if errors.Is(prepareMalformedErr, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatal("malformed failure must not also match ErrAtomicFactoryWriteFailed")
	}

	preparedAttempt, err := svc.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{
		Name: "alpha", Payload: attemptedCanonical,
	})
	if err != nil {
		t.Fatalf("PrepareFactoryLayout attempted: %v", err)
	}

	_, replaceErr := svc.ReplaceNamedFactory(ctx, factorydefinitions.ReplaceNamedFactoryRequest{
		RootDir: "/factories", Name: "alpha", Prepared: preparedAttempt.Prepared,
	})
	var writeFailure *factorydefinitions.AtomicFactoryWriteFailure
	if !errors.As(replaceErr, &writeFailure) {
		t.Fatalf("ReplaceNamedFactory error = %v, want AtomicFactoryWriteFailure", replaceErr)
	}
	if !writeFailure.PreviousPreserved {
		t.Fatal("PreviousPreserved = false, want true")
	}
	if writeFailure.Name != "alpha" {
		t.Fatalf("AtomicFactoryWriteFailure.Name = %q, want alpha", writeFailure.Name)
	}
	if !errors.Is(replaceErr, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatalf("ReplaceNamedFactory error = %v, want ErrAtomicFactoryWriteFailed", replaceErr)
	}
	if errors.Is(replaceErr, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatal("atomic write failure must not also match ErrMalformedFactoryLayoutPayload")
	}

	afterFailure, err := svc.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{
		Path: created.FactoryDir,
	})
	if err != nil {
		t.Fatalf("FlattenFactoryLayout after failed replace: %v", err)
	}
	if string(afterFailure.Canonical) != string(baseline.Canonical) {
		t.Fatalf(
			"after failed replace = %q, want unchanged prior baseline %q",
			afterFailure.Canonical,
			baseline.Canonical,
		)
	}
	if string(afterFailure.Canonical) == string(attemptedCanonical) {
		t.Fatal("failed replace must not leave attempted canonical bytes on disk")
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

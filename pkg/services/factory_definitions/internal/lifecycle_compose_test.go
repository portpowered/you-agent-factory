package internal_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	_ "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition"
)

func TestComposedLifecycleHostExercisesVersionSurface(t *testing.T) {
	t.Parallel()

	service := &lifecycle.Service{}
	next := service.NextEditableFactoryVersion(nil, time.Unix(42, 0).UTC())
	if next.Logical != 1 {
		t.Fatalf("NextEditableFactoryVersion logical = %d, want 1", next.Logical)
	}
	if !next.Physical.Equal(time.Unix(42, 0).UTC()) {
		t.Fatalf("NextEditableFactoryVersion physical = %v, want unix 42", next.Physical)
	}

	current := factorydefinitions.FactoryVersion{Logical: 3, Physical: time.Unix(100, 0).UTC()}
	base := factorydefinitions.FactoryVersion{Logical: 4, Physical: time.Unix(101, 0).UTC()}
	if err := service.RequireFreshEditableFactoryVersion(&base, current); err != nil {
		t.Fatalf("RequireFreshEditableFactoryVersion() error = %v, want success", err)
	}
}

func TestCompletedLifecycleDelegatesRuntimeSnapshot(t *testing.T) {
	t.Parallel()
	called := false
	service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		nil, nil, nil, nil, nil, nil,
		snapshotOperation(func(ctx context.Context, request factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
			called = true
			return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: factorydefinitions.RuntimeSnapshot{FactoryDir: request.FactoryDir}}, nil
		}),
		factorydefinitions.UnimplementedService{}, nil,
		factorydefinitions.UnimplementedService{}.ListEffectiveFactories,
		factorydefinitions.UnimplementedService{},
	)
	if called {
		t.Fatal("construction queried snapshot owner")
	}
	result, err := service.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{FactoryDir: "/factories/alpha"})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot: %v", err)
	}
	if !called || result.Snapshot.FactoryDir != "/factories/alpha" {
		t.Fatalf("delegation = called %t result %#v", called, result)
	}
}

type snapshotOperation factorydefinitions.RuntimeSnapshotOperation

func (operation snapshotOperation) ResolveRuntimeSnapshot(ctx context.Context, request factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return operation(ctx, request)
}

func TestCompletedLifecycleDelegatesCompilation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"success", nil}, {"invalid source", factorydefinitions.ErrInvalidAuthoredFactorySource}, {"canceled", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operationError := tc.err
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if errors.Is(operationError, context.Canceled) {
				cancel()
			}
			request := factorydefinitions.CompileEffectiveFactorySourceRequest{FactoryDir: "/factories/alpha", Canonical: []byte(`{"name":"alpha"}`)}
			expected := factorydefinitions.CompileEffectiveFactorySourceResult{}
			if operationError == nil {
				expected.Effective.ContentIdentity = "alpha-identity"
			}
			calls := 0
			owner := compilationOperation(func(gotContext context.Context, gotRequest factorydefinitions.CompileEffectiveFactorySourceRequest) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
				calls++
				if gotContext != ctx || !reflect.DeepEqual(gotRequest, request) {
					t.Fatal("compilation request or context changed")
				}
				return expected, operationError
			})
			service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
				nil, nil, nil, nil, nil, nil,
				factorydefinitions.UnimplementedService{}, owner, nil,
				factorydefinitions.UnimplementedService{}.ListEffectiveFactories,
				factorydefinitions.UnimplementedService{},
			)
			if calls != 0 {
				t.Fatal("construction invoked compilation")
			}
			result, err := service.CompileEffectiveFactorySource(ctx, request)
			if calls != 1 || !reflect.DeepEqual(result, expected) || err != operationError {
				t.Fatalf("compilation calls=%d result=%#v error=%v; want %#v, %v", calls, result, err, expected, operationError)
			}
		})
	}
}

type compilationOperation func(context.Context, factorydefinitions.CompileEffectiveFactorySourceRequest) (factorydefinitions.CompileEffectiveFactorySourceResult, error)

func (operation compilationOperation) CompileEffectiveFactorySource(ctx context.Context, request factorydefinitions.CompileEffectiveFactorySourceRequest) (factorydefinitions.CompileEffectiveFactorySourceResult, error) {
	return operation(ctx, request)
}

func TestCompletedLifecycleDelegatesEffectiveCatalog(t *testing.T) {
	t.Parallel()
	for _, operationError := range []error{nil, errors.New("catalog unavailable"), context.Canceled} {
		t.Run(fmt.Sprint(operationError), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if errors.Is(operationError, context.Canceled) {
				cancel()
			}
			request := factorydefinitions.ListEffectiveFactoriesRequest{ProjectRoot: "/project", GlobalRoot: "/global"}
			expected := factorydefinitions.ListEffectiveFactoriesResult{}
			if operationError == nil {
				expected.Entries = []factorydefinitions.EffectiveFactoryCatalogEntry{{Name: "alpha"}}
			}
			calls := 0
			listEffective := func(gotContext context.Context, gotRequest factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
				calls++
				if gotContext != ctx || gotRequest != request {
					t.Fatal("catalog request or context changed")
				}
				return expected, operationError
			}
			disabled := factorydefinitions.UnimplementedService{}
			service := lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
				nil, nil, disabled, disabled, disabled, disabled,
				disabled, disabled, nil, listEffective,
				factorydefinitions.UnimplementedService{},
			)
			if calls != 0 {
				t.Fatal("construction queried effective catalog")
			}
			result, err := service.ListEffectiveFactories(ctx, request)
			if calls != 1 || !reflect.DeepEqual(result, expected) || err != operationError {
				t.Fatalf("catalog calls=%d result=%#v error=%v; want %#v, %v", calls, result, err, expected, operationError)
			}
		})
	}
}

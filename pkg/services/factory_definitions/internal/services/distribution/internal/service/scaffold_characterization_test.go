package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// The canonical root currently disables scaffold creation. Preserve its public
// error precedence when the disabled role becomes an explicit operation.
func TestUnavailableScaffoldPrecedesRequestAndContextValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		request  factorydefinitions.CreateFactoryScaffoldRequest
		canceled bool
	}{
		{name: "valid", request: factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: "/customer/factory"}},
		{name: "invalid", request: factorydefinitions.CreateFactoryScaffoldRequest{Type: "unsupported"}},
		{name: "canceled valid", request: factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: "/customer/factory"}, canceled: true},
		{name: "canceled invalid", canceled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			effects := 0
			installer := factorydefinitions.PackagedFactoryInstallationOperations{
				Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
					effects++
					return factorydefinitions.PackagedFactoryInstallResult{}, nil
				},
			}
			svc := newDistributionServiceWithScaffold(t, unusedScaffoldCatalog(t), installer, nil, func(string) (string, error) {
				effects++
				return "factory", nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.canceled {
				cancel()
			}
			result, err := svc.CreateFactoryScaffold(ctx, test.request)
			if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) || err.Error() != "factory distribute failed: scaffold collaborator is required" {
				t.Fatalf("disabled scaffold error = %v, want canonical unavailable failure", err)
			}
			if !reflect.DeepEqual(result, factorydefinitions.CreateFactoryScaffoldResult{}) || effects != 0 {
				t.Fatalf("disabled scaffold result=%#v effects=%d, want empty result and no effects", result, effects)
			}
		})
	}
}

func TestSupportedScaffoldCancellationPrecedesInvalidRequestWithoutWrites(t *testing.T) {
	t.Parallel()
	calls := 0
	svc := newDistributionServiceWithScaffold(t, unusedScaffoldCatalog(t), factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
			t.Fatal("scaffold called packaged installer")
			return factorydefinitions.PackagedFactoryInstallResult{}, nil
		},
	}, func(factorydefinitions.ScaffoldConfig) error {
		calls++
		return nil
	}, scaffoldNameResolver("factory"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := svc.CreateFactoryScaffold(ctx, factorydefinitions.CreateFactoryScaffoldRequest{})
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("canceled scaffold error=%v writes=%d, want context cancellation and no writes", err, calls)
	}
}

func unusedScaffoldCatalog(t *testing.T) factorydefinitions.PackagedFactoryCatalogOperations {
	t.Helper()
	return factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(context.Context, factorydefinitions.ListBuiltInPackagedFactoriesRequest) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			t.Fatal("scaffold called packaged catalog list")
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
		},
		Resolve: func(context.Context, factorydefinitions.ResolveBuiltInPackagedFactoryRequest) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			t.Fatal("scaffold called packaged catalog resolve")
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, nil
		},
	}
}

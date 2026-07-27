package systeminitialization_test

import (
	"context"
	"testing"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// rootServiceFake is a peer-shaped System Bootstrap root Service that uses only
// Bootstrap-owned request, result, value, and typed-error contracts. It never
// imports Operator Settings implementation packages, Factory Definitions
// implementation subpackages, concrete filesystem collaborator ports, or
// pkg/initializer lifecycle types.
type rootServiceFake struct {
	result systeminitialization.Result
	err    error

	lastRequest systeminitialization.Request
}

var _ systeminitialization.Service = (*rootServiceFake)(nil)

func (fake *rootServiceFake) Initialize(
	_ context.Context,
	request systeminitialization.Request,
) (systeminitialization.Result, error) {
	fake.lastRequest = request
	if fake.err != nil {
		return systeminitialization.Result{}, fake.err
	}
	return fake.result, nil
}

func TestRootService_Characterization_FakeImplementsSingularSeam(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{
		result: systeminitialization.Result{
			HomeDir:             "/home/peer",
			ConfigPath:          "/home/peer/.you/config.json",
			NamedFactoriesRoot:  "/home/peer/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigCreated,
			PackagedFactories: []systeminitialization.PackagedFactoryResult{{
				Name:       "@you/goal",
				FactoryDir: "goal",
				Outcome:    systeminitialization.PackagedFactoryCreated,
			}},
		},
	}

	var service systeminitialization.Service = fake
	result, err := service.Initialize(context.Background(), systeminitialization.Request{
		HomeDir: "/home/peer",
	})
	if err != nil {
		t.Fatalf("Initialize() = %v", err)
	}
	if fake.lastRequest.HomeDir != "/home/peer" {
		t.Fatalf("fake recorded request = %#v", fake.lastRequest)
	}
	if result.HomeDir != "/home/peer" ||
		result.SystemConfigOutcome != systeminitialization.SystemConfigCreated ||
		len(result.PackagedFactories) != 1 ||
		result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("Initialize() result = %#v", result)
	}
}

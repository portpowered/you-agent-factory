package service_test

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// operatorSettingsFake is a focused Operator Settings root fake exercising
// only ResolveACPAgentProfile, the sole collaborator method the Factory
// target-catalog operation depends on.
type operatorSettingsFake struct {
	operatorsettings.Service

	resolveACPAgentProfile func(string) (operatorsettings.ACPAgentProfile, error)
}

func (fake *operatorSettingsFake) ResolveACPAgentProfile(path string) (operatorsettings.ACPAgentProfile, error) {
	if fake.resolveACPAgentProfile != nil {
		return fake.resolveACPAgentProfile(path)
	}
	return operatorsettings.ACPAgentProfile{}, errUnexpectedCall
}

// factoryDefinitionsFake is a focused Factory Definitions root fake
// exercising only ListEffectiveFactories, the sole collaborator method the
// Factory target-catalog operation depends on.
type factoryDefinitionsFake struct {
	factorydefinitions.Service

	listEffectiveFactories func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error)
}

func (fake *factoryDefinitionsFake) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	if fake.listEffectiveFactories != nil {
		return fake.listEffectiveFactories(ctx, request)
	}
	return factorydefinitions.ListEffectiveFactoriesResult{}, errUnexpectedCall
}

type unexpectedCallError struct{}

func (unexpectedCallError) Error() string { return "unexpected fake call" }

var errUnexpectedCall = unexpectedCallError{}

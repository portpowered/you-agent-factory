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
// exercising only ListEffectiveFactories and ResolveNamedFactory, the
// collaborator methods the Factory target-catalog operation depends on.
// resolveNamedFactory is only exercised when a test supplies a
// ClientWorkingRoot, since the operation only calls it in that case.
type factoryDefinitionsFake struct {
	factorydefinitions.Service

	listEffectiveFactories func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error)
	resolveNamedFactory    func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error)
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

func (fake *factoryDefinitionsFake) ResolveNamedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	if fake.resolveNamedFactory != nil {
		return fake.resolveNamedFactory(ctx, request)
	}
	return factorydefinitions.ResolveNamedFactoryResult{}, errUnexpectedCall
}

type unexpectedCallError struct{}

func (unexpectedCallError) Error() string { return "unexpected fake call" }

var errUnexpectedCall = unexpectedCallError{}

// installedFactoryEntry returns an EffectiveFactoryCatalogEntry representing
// a materialized (installed) Factory. Factory Definitions' contract leaves
// Location nil for packaged definitions that have not been materialized, so
// every fixture standing in for an installed Factory must set it explicitly.
func installedFactoryEntry(name, displayName string) factorydefinitions.EffectiveFactoryCatalogEntry {
	location := "/factories/" + name
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:       name,
		Location:   &location,
		Definition: &factorydefinitions.FactoryConfig{Name: displayName},
	}
}

// packagedOnlyFactoryEntry returns an EffectiveFactoryCatalogEntry
// representing a packaged Factory definition that has not been materialized:
// it is effective/listable but never counts as installed.
func packagedOnlyFactoryEntry(name, displayName string) factorydefinitions.EffectiveFactoryCatalogEntry {
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:       name,
		Definition: &factorydefinitions.FactoryConfig{Name: displayName},
	}
}

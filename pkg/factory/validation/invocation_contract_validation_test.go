package validation_test

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func invocationSignatureValidationConfig(mutate func(*interfaces.FactoryConfig)) *interfaces.FactoryConfig {
	cfg := &interfaces.FactoryConfig{
		Name: "invocation-signature-validation",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{
			Name: "worker-a",
			Type: interfaces.WorkerTypeInference,
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
	if mutate != nil {
		mutate(cfg)
	}
	return cfg
}

func TestValidate_InvocationReturnExplicitValid(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        string(factoryapi.InvocationReturnPolicyExplicit),
			WorkTypeName:  "story",
			TerminalState: "complete",
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if strings.HasPrefix(target.Code, "factory.invocationReturn.") {
			t.Fatalf("targets = %#v, want no invocationReturn findings", result.Targets)
		}
	}
}

func TestValidate_InvocationReturnExplicitMissingWorkType(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        string(factoryapi.InvocationReturnPolicyExplicit),
			TerminalState: "complete",
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationReturnMissingWorkTypeName)
}

func TestValidate_InvocationReturnExplicitInvalidTerminalState(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        string(factoryapi.InvocationReturnPolicyExplicit),
			WorkTypeName:  "story",
			TerminalState: "review",
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "review", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationReturnInvalidTerminalState)
}

func TestValidate_InvocationReturnOmitted(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if strings.HasPrefix(target.Code, "factory.invocationReturn.") {
			t.Fatalf("targets = %#v, want omitted invocationReturn to stay valid", result.Targets)
		}
	}
}

func TestValidate_InvocationSignatureValid(t *testing.T) {
	t.Parallel()

	cfg := invocationSignatureValidationConfig(func(cfg *interfaces.FactoryConfig) {
		cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{
			UnknownNamedArgumentPolicy: string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyReject),
			Parameters: []interfaces.InvocationParameterConfig{
				{
					Name:         "input",
					ExternalName: "input",
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1},
					},
				},
				{
					Name:         "output",
					ExternalName: "output",
					TypeHint:     string(factoryapi.FactoryInvocationParameterTypeHintPath),
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
					},
				},
			},
			OutputContract: &interfaces.InvocationOutputContractConfig{
				Mode:          string(factoryapi.FactoryInvocationOutputContractModeFile),
				PathParameter: "output",
			},
		}
		cfg.Workers[0].Model = "${input}"
		cfg.Workstations[0].Body = "Render ${input}"
		cfg.Workstations[0].WorkingDirectory = "/tmp/${output}"
	})

	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if strings.HasPrefix(target.Code, "factory.invocationSignature.") {
			t.Fatalf("targets = %#v, want no invocationSignature findings", result.Targets)
		}
	}
}

func TestValidate_InvocationSignatureRejectsMalformedBindingsAndDefaults(t *testing.T) {
	t.Parallel()

	cfg := invocationSignatureValidationConfig(func(cfg *interfaces.FactoryConfig) {
		cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{
			UnknownNamedArgumentPolicy: string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect),
			Parameters: []interfaces.InvocationParameterConfig{
				{
					Name:         "secret",
					ExternalName: "token",
					Sensitive:    true,
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 2},
					},
				},
				{
					Name:         "secret",
					ExternalName: "token",
					ValueMode:    string(factoryapi.FactoryInvocationParameterValueModeRepeated),
					DefaultValue: "one",
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindStdin)},
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
					},
				},
				{
					Name:         "extras",
					ExternalName: "extras",
					ValueMode:    string(factoryapi.FactoryInvocationParameterValueModeVariadic),
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 4},
					},
				},
			},
		}
	})

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureDuplicateParameterName)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureDuplicateNamedKey)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureSensitivePositional)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureInvalidDefaultShape)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureInvalidStdinRouting)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureInvalidPositionalOrdering)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureInvalidNamedRestShape)
}

func TestValidate_InvocationSignatureRejectsInvalidInterpolationReferences(t *testing.T) {
	t.Parallel()

	cfg := invocationSignatureValidationConfig(func(cfg *interfaces.FactoryConfig) {
		cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{
			Parameters: []interfaces.InvocationParameterConfig{
				{
					Name:         "items",
					ExternalName: "item",
					ValueMode:    string(factoryapi.FactoryInvocationParameterValueModeRepeated),
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
					},
				},
			},
			OutputContract: &interfaces.InvocationOutputContractConfig{
				Mode:          string(factoryapi.FactoryInvocationOutputContractModeFile),
				PathParameter: "missing-output",
			},
		}
		cfg.Workers[0].Model = "${missing}"
		cfg.Workstations[0].Body = "Use ${items}"
	})

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureUnknownOutputPathParameter)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureInvalidInterpolationReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureIncompatibleInterpolationReference)
}

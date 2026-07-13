// backendsizecheck:ignore-file consolidated orchestrator validation tests remain with validation_test until dedicated validation test seams split.
// pkgmaintcheck:ignore-file-lines consolidated orchestrator validation tests remain with validation_test until dedicated validation test seams split.
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

func TestValidate_LegacyPetriFactoryWithoutOrchestratorRemainsValid(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "legacy-petri",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name:   "task",
			States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}, {Name: "done", Type: interfaces.StateTypeTerminal}},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker", Type: interfaces.WorkerTypeModel}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute",
			WorkerTypeName: "worker",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}

	if got := interfaces.EffectiveOrchestratorKind(cfg); got != interfaces.OrchestratorKindPetri {
		t.Fatalf("EffectiveOrchestratorKind = %q, want PETRI", got)
	}
	if targets := factoryvalidation.OrchestratorTargets(cfg); len(targets) > 0 {
		t.Fatalf("orchestrator targets = %#v, want none for legacy Petri factory", targets)
	}
	if !factoryvalidation.IsPetriOrchestratorValidationScope(cfg) {
		t.Fatal("expected legacy factory to remain in Petri validation scope")
	}
}

func TestValidate_JavaScriptFactoryAcceptsSourceRefWithoutPetriGraph(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "dynamic-workflow",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef:  "factory/workflows/review.js",
				Entrypoint: "main",
				Metadata:   map[string]string{"team": "platform"},
			},
		},
	}

	result := factoryvalidation.Validate(cfg)
	if result.HasTargets() {
		t.Fatalf("validation targets = %#v, want none for valid JavaScript factory", result.Targets)
	}
}

func TestValidate_JavaScriptFactoryRejectsMissingSource(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-javascript",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind:       interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{},
		},
	}

	assertValidationCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeOrchestratorJavaScriptMissingSource)
}

func TestValidate_JavaScriptFactoryRejectsEmptyNamedAgentPreset(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-named-agent",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef: "factory/workflows/review.js",
				Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
					"reviewer": {Preset: "   "},
				},
			},
		},
	}

	result := factoryvalidation.Validate(cfg)
	assertValidationCode(t, result.Targets, factoryvalidation.CodeOrchestratorJavaScriptInvalidAgent)
	if !strings.Contains(result.Targets[0].Message, "reviewer") {
		t.Fatalf("diagnostic = %q, want agent id", result.Targets[0].Message)
	}
}

func TestValidate_JavaScriptFactoryRejectsPetriGraphFields(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-javascript",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef: "factory/workflows/review.js",
			},
		},
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}}}},
	}

	assertValidationCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeOrchestratorIncompatiblePetriField)
}

func TestValidate_UnsupportedOrchestratorKindRejected(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-kind",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: "STREAM",
		},
	}

	assertValidationCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeOrchestratorUnsupportedKind)
}

func assertValidationCode(t *testing.T, targets []factoryvalidation.Target, code string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("validation targets = %#v, want code %q", targets, code)
}

func workPropagationValidationBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:   "execute-story",
			Inputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs: []interfaces.IOConfig{
				{WorkTypeName: "story", StateName: "complete"},
			},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		}},
	}
}

func TestValidate_WorkPropagationSupportedModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []interfaces.WorkPropagationMode{
		interfaces.WorkPropagationModeOutputAsPayload,
		interfaces.WorkPropagationModePreserveInput,
	} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := workPropagationValidationBaseConfig()
			cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{Mode: mode}

			result := factoryvalidation.Validate(cfg)
			for _, target := range result.Targets {
				if target.Code == factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
					t.Fatalf("targets = %#v, want supported mode %q to stay valid", result.Targets, mode)
				}
			}
		})
	}
}

func TestValidate_WorkPropagationOmitted(t *testing.T) {
	t.Parallel()

	result := factoryvalidation.Validate(workPropagationValidationBaseConfig())
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			t.Fatalf("targets = %#v, want omitted workPropagation to stay valid", result.Targets)
		}
	}
}

func TestValidate_WorkPropagationUnsupportedMode(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode("MERGE_PAYLOAD"),
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)

	var matched bool
	for _, target := range result.Targets {
		if target.Code != factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			continue
		}
		matched = true
		if target.Subject.Type != factoryvalidation.SubjectTypeWorkstation || target.Subject.ID != "execute-story" {
			t.Fatalf("target subject = %#v, want workstation execute-story", target.Subject)
		}
		if target.Path != "factory.workstations[0](execute-story).workPropagation.mode" {
			t.Fatalf("target path = %q, want workstation workPropagation path", target.Path)
		}
		if !strings.Contains(target.Message, `unsupported workPropagation.mode "MERGE_PAYLOAD"`) {
			t.Fatalf("target message = %q, want unsupported mode detail", target.Message)
		}
	}
	if !matched {
		t.Fatal("expected unsupported work propagation target")
	}
}

func TestValidate_WorkPropagationEmptyMode(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}

func TestValidate_WorkPropagationUnsupportedModeUsesGeneratedEnumValues(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode(factoryapi.WorkPropagationMode("PRESERVE_OUTPUT")),
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}

package validation

import (
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestValidationResult_HasErrors_FalseWithOnlyWarningsAndHints(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityHint, Path: "b", Message: "hint", Rule: "r2"},
		},
	}
	if vr.HasErrors() {
		t.Fatal("HasErrors() should be false when only warnings and hints present")
	}
}

func TestValidationResult_HasErrors_TrueWithErrors(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityError, Path: "b", Message: "err", Rule: "r2"},
		},
	}
	if !vr.HasErrors() {
		t.Fatal("HasErrors() should be true when error findings present")
	}
}

func TestValidationResult_Errors_ReturnsOnlyErrors(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityError, Path: "b", Message: "err1", Rule: "r2"},
			{Severity: SeverityHint, Path: "c", Message: "hint", Rule: "r3"},
			{Severity: SeverityError, Path: "d", Message: "err2", Rule: "r4"},
		},
	}
	errs := vr.Errors()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].Path != "b" || errs[1].Path != "d" {
		t.Fatalf("unexpected error paths: %v", errs)
	}
}

func TestConfigValidator_ReportsAllErrors(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		InputTypes: []factorydefinitions.InputTypeConfig{
			{Name: "", Type: "default"},
		},
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{{
				Name: "init",
				Type: factorydefinitions.StateTypeInitial,
			}},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "ws1",
			Inputs: []factorydefinitions.IOConfig{{
				WorkTypeName: "task",
				StateName:    "init",
			}},
			Outputs: []factorydefinitions.IOConfig{{
				WorkTypeName: "task",
				StateName:    "nonexistent",
			}},
		}},
	}
	result := NewConfigValidator(nil).Validate(cfg)
	result.Findings = append(
		result.Findings,
		ruleCanonicalFactoryDefinitionValidation(cfg)...,
	)
	if !result.HasErrors() {
		t.Fatal("expected errors")
	}
	errs := result.Errors()
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors from independent rules, got %d: %v", len(errs), errs)
	}
	assertFindingExists(t, errs, "input-type-name")
	assertFindingExists(t, errs, CodeDanglingPlaceReference)
}

func TestRuleInputTypes_MissingName(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{InputTypes: []factorydefinitions.InputTypeConfig{{Name: "", Type: "default"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-name")
}

func TestRuleInputTypes_ReservedDefault(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{InputTypes: []factorydefinitions.InputTypeConfig{{Name: "default", Type: "default"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-reserved")
}

func TestRuleInputTypes_Duplicate(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{InputTypes: []factorydefinitions.InputTypeConfig{
		{Name: "foo", Type: "default"},
		{Name: "foo", Type: "default"},
	}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-duplicate")
}

func TestRuleInputTypes_MissingType(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{InputTypes: []factorydefinitions.InputTypeConfig{{Name: "foo", Type: ""}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-type")
}

func TestRuleInputTypes_UnknownType(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{InputTypes: []factorydefinitions.InputTypeConfig{{Name: "foo", Type: "bogus"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-type")
}

func TestRuleInputTypes_ValidConfig(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{InputTypes: []factorydefinitions.InputTypeConfig{{
		Name: "batch",
		Type: factorydefinitions.InputKindDefault,
	}}}
	findings := ruleInputTypes(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

type stubRequiredToolChecker map[string]RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool factorydefinitions.RequiredToolConfig) RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return RequiredToolCheckResult{}
}

func testBaseConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "w1"}},
	}
}

func assertErrString(message string) error {
	return &staticErr{message: message}
}

type staticErr struct {
	message string
}

func (e *staticErr) Error() string {
	return e.message
}

func assertFindingExists(t *testing.T, findings []Finding, rule string) {
	t.Helper()
	for _, f := range findings {
		if f.Rule == rule && f.Severity == SeverityError {
			return
		}
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}

func assertFindingMatch(t *testing.T, findings []Finding, rule string, pathSubstring string, messageSubstring string) {
	t.Helper()
	for _, f := range findings {
		if f.Rule != rule || f.Severity != SeverityError {
			continue
		}
		if !strings.Contains(f.Path, pathSubstring) {
			continue
		}
		if !strings.Contains(f.Message, messageSubstring) {
			t.Fatalf("finding message = %q, want substring %q", f.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}

func TestRuleModelInvokeWorkstations_AcceptsCompatibleModelInvokeWorkstation(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "tts-worker",
		Type: factorydefinitions.WorkerTypeModel,
		Operations: []factorydefinitions.ModelOperation{{
			Name: "TTS",
			Inputs: []factorydefinitions.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []factorydefinitions.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio},
			}},
		}},
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:      "speak",
		Type:      factorydefinitions.WorkstationTypeInvoke,
		Operation: "TTS",
		OperationBindings: []factorydefinitions.ModelOperationBinding{{
			Slot: "text",
			Selector: &factorydefinitions.ModelOperationBindingSelector{
				Label: "utterance",
			},
		}},
		WorkerTypeName: "tts-worker",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleModelInvokeWorkstations_AcceptsCompatibleModelInvokeWorkstationAcrossWorkerLocality(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		worker factorydefinitions.FactoryWorkerConfig
	}{
		{
			name: "local worker",
			worker: factorydefinitions.FactoryWorkerConfig{
				Name:          "tts-worker",
				Type:          factorydefinitions.WorkerTypeModel,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelProvider: workerexecution.RunnerIDCodex,
				ModelLocality: factorydefinitions.ModelLocalityLocal,
				Operations: []factorydefinitions.ModelOperation{{
					Name: "TTS",
					Inputs: []factorydefinitions.ModelOperationSlot{
						{Name: "text", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText}, Required: true},
						{Name: "voice", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeJSON}},
					},
					Outputs: []factorydefinitions.ModelOperationSlot{{Name: "audio", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio}}},
				}},
			},
		},
		{
			name: "cloud worker",
			worker: factorydefinitions.FactoryWorkerConfig{
				Name:          "tts-worker",
				Type:          factorydefinitions.WorkerTypeModel,
				Model:         "gpt-4o-mini-tts",
				ModelProvider: workerexecution.RunnerIDCodex,
				ModelLocality: factorydefinitions.ModelLocalityCloud,
				Operations: []factorydefinitions.ModelOperation{{
					Name: "TTS",
					Inputs: []factorydefinitions.ModelOperationSlot{
						{Name: "text", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText}, Required: true},
						{Name: "voice", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeJSON}},
					},
					Outputs: []factorydefinitions.ModelOperationSlot{{Name: "audio", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio}}},
				}},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workers = []factorydefinitions.FactoryWorkerConfig{tt.worker}
			cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
				Name:           "speak",
				Type:           factorydefinitions.WorkstationTypeInvoke,
				Operation:      "TTS",
				WorkerTypeName: "tts-worker",
				OperationBindings: []factorydefinitions.ModelOperationBinding{
					{Slot: "text", Selector: &factorydefinitions.ModelOperationBindingSelector{Label: "utterance"}},
					{Slot: "voice", Config: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"name":"alloy"}`)}}},
				},
				Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			}}

			findings := ruleModelInvokeWorkstations(cfg)
			if len(findings) != 0 {
				t.Fatalf("expected no findings, got %#v", findings)
			}
		})
	}
}

func TestRuleModelInvokeWorkstations_RejectsOperationOnNonModelInvokeType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "legacy",
		Type:           factorydefinitions.WorkstationTypeModel,
		Operation:      "TTS",
		WorkerTypeName: "w1",
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-type", "workstations[0](legacy).operation", "only supported on INFERENCE_RUN or legacy MODEL_INVOKE")
}

func TestRuleModelInvokeWorkstations_RejectsWorkerCompatibilityAndOperationMismatch(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{
		{
			Name: "scripted",
			Type: factorydefinitions.WorkerTypeScript,
		},
		{
			Name: "tts-worker",
			Type: factorydefinitions.WorkerTypeModel,
			Operations: []factorydefinitions.ModelOperation{{
				Name: "EMBED",
				Inputs: []factorydefinitions.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
				}},
				Outputs: []factorydefinitions.ModelOperationSlot{{
					Name:         "vector",
					ContentTypes: []string{factorydefinitions.ModelOperationContentTypeJSON},
				}},
			}},
		},
	}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{
			Name:           "bad-worker-type",
			Type:           factorydefinitions.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "scripted",
		},
		{
			Name:           "bad-operation",
			Type:           factorydefinitions.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "tts-worker",
		},
	}

	findings := append(ruleModelInvokeWorkstations(cfg), ruleWorkerWorkstationBehaviorCompatibility(cfg)...)
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[0](bad-worker-type).worker", `workstation "bad-worker-type"`)
	assertFindingMatch(t, findings, "workstation-model-invoke-operation-mismatch", "workstations[1](bad-operation).operation", `does not declare requested operation "TTS"`)
}

func TestRuleModelInvokeWorkstations_RejectsIncompleteContentContract(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "tts-worker",
		Type: factorydefinitions.WorkerTypeModel,
		Operations: []factorydefinitions.ModelOperation{{
			Name: "TTS",
			Inputs: []factorydefinitions.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
			}},
		}},
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "speak",
		Type:           factorydefinitions.WorkstationTypeInvoke,
		Operation:      "TTS",
		WorkerTypeName: "tts-worker",
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-content-contract", "workstations[0](speak).operation", "incompatible content contract")
}

func TestRuleModelInvokeWorkstations_RejectsDuplicateUnknownAndEmptyOperationBindings(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "tts-worker",
		Type: factorydefinitions.WorkerTypeModel,
		Operations: []factorydefinitions.ModelOperation{{
			Name: "TTS",
			Inputs: []factorydefinitions.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []factorydefinitions.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio},
			}},
		}},
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "speak",
		Type:           factorydefinitions.WorkstationTypeInvoke,
		Operation:      "TTS",
		WorkerTypeName: "tts-worker",
		OperationBindings: []factorydefinitions.ModelOperationBinding{
			{Slot: "text", Selector: &factorydefinitions.ModelOperationBindingSelector{Label: "utterance"}},
			{Slot: "text", Config: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "fallback"}}},
			{Slot: "voice", Selector: &factorydefinitions.ModelOperationBindingSelector{Role: "system"}},
			{Slot: "style"},
		},
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-binding-duplicate", "workstations[0](speak).operationBindings[1](text).slot", `duplicate operation binding for slot "text"`)
	assertFindingMatch(t, findings, "workstation-model-invoke-binding-unknown-slot", "workstations[0](speak).operationBindings[2](voice).slot", `unknown input slot "voice"`)
	assertFindingMatch(t, findings, "workstation-model-invoke-binding-empty", "workstations[0](speak).operationBindings[3](style)", "must declare a selector, config content, or default content")
}

func TestRuleWorkerModelOperations_RejectsDuplicateOperationAndSlotNames(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:          "tts-worker",
		Type:          factorydefinitions.WorkerTypeModel,
		ModelLocality: factorydefinitions.ModelLocalityLocal,
		Operations: []factorydefinitions.ModelOperation{{
			Name: "TTS",
			Inputs: []factorydefinitions.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText}},
				{Name: "text", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeJSON}},
			},
			Outputs: []factorydefinitions.ModelOperationSlot{{Name: "audio", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio}}},
		}, {
			Name:    "TTS",
			Outputs: []factorydefinitions.ModelOperationSlot{{Name: "audio", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio}}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-duplicate", "workers[0](tts-worker).operations[1](TTS).name", `duplicate operation name "TTS"`)
	assertFindingMatch(t, findings, "worker-model-operation-slot-duplicate", "workers[0](tts-worker).operations[0](TTS).inputs[1](text).name", `duplicate input slot name "text"`)
}

func TestRuleWorkerModelOperations_RejectsCapabilityDeclarationsOnScriptWorkers(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:          "scripted",
		Type:          factorydefinitions.WorkerTypeScript,
		ModelLocality: factorydefinitions.ModelLocalityCloud,
		Operations:    []factorydefinitions.ModelOperation{{Name: "TTS"}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-worker-type", "workers[0](scripted)", "require worker type INFERENCE_WORKER or legacy MODEL_WORKER")
}

func TestRuleWorkerModelOperations_RejectsCapabilityDeclarationsOnAgentWorkers(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "executor",
		Type: factorydefinitions.WorkerTypeAgent,
		Operations: []factorydefinitions.ModelOperation{{
			Name:    "TTS",
			Inputs:  []factorydefinitions.ModelOperationSlot{{Name: "text", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText}}},
			Outputs: []factorydefinitions.ModelOperationSlot{{Name: "audio", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio}}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-worker-type", "workers[0](executor)", "require worker type INFERENCE_WORKER or legacy MODEL_WORKER")
}

func TestRuleWorkerModelOperations_RejectsMissingSlotContentTypes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "tts-worker",
		Type: factorydefinitions.WorkerTypeModel,
		Operations: []factorydefinitions.ModelOperation{{
			Name:   "TTS",
			Inputs: []factorydefinitions.ModelOperationSlot{{Name: "text"}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-slot-content-types", "workers[0](tts-worker).operations[0](TTS).inputs[0](text).contentTypes", "at least one content type is required")
}

func TestRuleResourceUsage_NonexistentResource(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []factorydefinitions.ResourceConfig{{Name: "bogus", Capacity: 1}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceUsage_ZeroCapacity(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []factorydefinitions.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []factorydefinitions.ResourceConfig{{Name: "gpu", Capacity: 0}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
}

func TestRuleResourceUsage_ValidConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []factorydefinitions.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []factorydefinitions.ResourceConfig{{Name: "gpu", Capacity: 2}},
	}}
	findings := ruleResourceUsage(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleResourceUsage_ValidatesWorkerRequirements(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []factorydefinitions.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:      "worker-a",
		Resources: []factorydefinitions.ResourceConfig{{Name: "gpu", Capacity: 0}, {Name: "missing", Capacity: 1}},
	}}

	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceDefinitions_RequiresModelMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []factorydefinitions.ResourceConfig{{
		Name:     "omnivoice-cache",
		Type:     factorydefinitions.ResourceTypeModel,
		Capacity: 1,
	}}

	findings := ruleResourceDefinitions(cfg)
	assertFindingExists(t, findings, "resource-model-model")
	assertFindingExists(t, findings, "resource-model-backend")
	assertFindingExists(t, findings, "resource-model-load-policy")
}

func TestRuleResourceDefinitions_RequiresProviderQuotaMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []factorydefinitions.ResourceConfig{{
		Name:     "codex-tts-quota",
		Type:     factorydefinitions.ResourceTypeProviderQuota,
		Capacity: 2,
	}}

	findings := ruleResourceDefinitions(cfg)
	assertFindingExists(t, findings, "resource-provider-quota-provider")
	assertFindingExists(t, findings, "resource-provider-quota-model")
}

func TestRuleResourceDefinitions_AcceptsModelResourceMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []factorydefinitions.ResourceConfig{{
		Name:       "omnivoice-cache",
		Type:       factorydefinitions.ResourceTypeModel,
		Capacity:   1,
		Model:      "OMNIVOICE_Q4_K_M",
		Backend:    "LLAMACPP",
		LoadPolicy: "ON_DEMAND",
	}}

	if findings := ruleResourceDefinitions(cfg); len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleRequiredTools_MissingNameAndCommand(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		RequiredTools: []factorydefinitions.RequiredToolConfig{{}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-name")
	assertFindingExists(t, findings, "required-tool-command")
}

func TestConfigValidator_RequiredToolsReportsPresentAndMissingCommandsDeterministically(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		RequiredTools: []factorydefinitions.RequiredToolConfig{
			{Name: "Go toolchain", Command: "go"},
			{Name: "Missing helper", Command: "missing-tool"},
		},
	}

	validator := NewConfigValidator(stubRequiredToolChecker{
		"go":           {ResolvedPath: "/usr/bin/go"},
		"missing-tool": {Err: assertErrString(`required tool "Missing helper" command "missing-tool" was not found on PATH`)},
	})

	result := validator.Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected missing required tool to produce an error")
	}
	if len(result.Errors()) != 1 {
		t.Fatalf("expected one required-tool error, got %#v", result.Errors())
	}
	finding := result.Errors()[0]
	if finding.Rule != "required-tool-missing" {
		t.Fatalf("expected required-tool-missing rule, got %#v", finding)
	}
	if finding.Path != "resourceManifest.requiredTools[1].command" {
		t.Fatalf("expected path-specific missing-tool finding, got %#v", finding)
	}
	if !strings.Contains(finding.Message, `"missing-tool" was not found on PATH`) {
		t.Fatalf("expected PATH lookup guidance, got %#v", finding)
	}
}

func TestRuleRequiredTools_InvalidVersionProbeUsesVersionArgsPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		RequiredTools: []factorydefinitions.RequiredToolConfig{{
			Name:        "Python",
			Command:     "python",
			VersionArgs: []string{"--version"},
		}},
	}

	findings := ruleRequiredTools(stubRequiredToolChecker{
		"python": {
			FailureKind: RequiredToolFailureKindVersionProbe,
			Err:         assertErrString(`required tool "Python" command "python" failed version probe "--version": exit status 1`),
		},
	})(cfg)
	assertFindingExists(t, findings, "required-tool-version-probe")
	if findings[0].Path != "resourceManifest.requiredTools[0].versionArgs" {
		t.Fatalf("expected versionArgs path, got %#v", findings[0])
	}
}

func TestRuleRequiredTools_MissingCommandWithVersionArgsUsesCommandPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		RequiredTools: []factorydefinitions.RequiredToolConfig{{
			Name:        "Portable helper",
			Command:     "missing-helper",
			VersionArgs: []string{"--version"},
		}},
	}

	findings := ruleRequiredTools(stubRequiredToolChecker{
		"missing-helper": {
			FailureKind: RequiredToolFailureKindMissing,
			Err:         assertErrString(`required tool "Portable helper" command "missing-helper" was not found on PATH`),
		},
	})(cfg)
	assertFindingExists(t, findings, "required-tool-missing")
	if findings[0].Path != "resourceManifest.requiredTools[0].command" {
		t.Fatalf("expected command path for missing tool, got %#v", findings[0])
	}
}

func TestRuleRequiredTools_RejectsBlankVersionArgsEntries(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		RequiredTools: []factorydefinitions.RequiredToolConfig{{
			Name:        "Python",
			Command:     "python",
			VersionArgs: []string{"--version", ""},
		}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-version-args")
}

func TestRuleBundledFiles_RejectsUnsupportedTypeEncodingAndRoot(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		BundledFiles: []factorydefinitions.BundledFileConfig{{
			Type:       "BINARY",
			TargetPath: "factory/misc/helper.bin",
			Content: factorydefinitions.BundledFileContentConfig{
				Encoding: "base64",
				Inline:   "AA==",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-type")
	assertFindingExists(t, findings, "bundled-file-content-encoding")
}

func TestRuleBundledFiles_RejectsUnsafeTargetPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		BundledFiles: []factorydefinitions.BundledFileConfig{{
			Type:       "SCRIPT",
			TargetPath: "../scripts/setup-workspace.py",
			Content: factorydefinitions.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "print('portable')\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-path")
	if findings[0].Path != "resourceManifest.bundledFiles[0].targetPath" {
		t.Fatalf("expected targetPath-specific finding, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsAbsoluteTargetPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		BundledFiles: []factorydefinitions.BundledFileConfig{{
			Type:       factorydefinitions.BundledFileTypeScript,
			TargetPath: "/factory/scripts/setup-workspace.py",
			Content: factorydefinitions.BundledFileContentConfig{
				Encoding: factorydefinitions.BundledFileEncodingUTF8,
				Inline:   "print('portable')\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-path")
	if !strings.Contains(findings[0].Message, "not absolute") {
		t.Fatalf("expected absolute-path guidance, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsMissingInlineContent(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		BundledFiles: []factorydefinitions.BundledFileConfig{{
			Type:       factorydefinitions.BundledFileTypeRootHelper,
			TargetPath: "Makefile",
			Content: factorydefinitions.BundledFileContentConfig{
				Encoding: factorydefinitions.BundledFileEncodingUTF8,
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-content-inline")
	if findings[0].Path != "resourceManifest.bundledFiles[0].content.inline" {
		t.Fatalf("expected inline-specific finding, got %#v", findings[0])
	}
}

func TestRuleBundledFilesOnPathFailsClosedWithoutSourceInspection(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		BundledFiles: []factorydefinitions.BundledFileConfig{{
			Type:       factorydefinitions.BundledFileTypeScript,
			TargetPath: "factory/scripts/setup.js",
			Content: factorydefinitions.BundledFileContentConfig{
				Encoding: factorydefinitions.BundledFileEncodingUTF8,
			},
		}},
	}
	findings := ruleBundledFilesOnPath(
		t.TempDir(),
		cfg,
		func(string, factorydefinitions.BundledFileConfig) (string, bool) {
			return "setup.js", true
		},
		nil,
	)
	assertFindingExists(t, findings, "bundled-file-source-inspection")
}

func TestRuleBundledFiles_RejectsUnsupportedRootHelperTarget(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		BundledFiles: []factorydefinitions.BundledFileConfig{{
			Type:       factorydefinitions.BundledFileTypeRootHelper,
			TargetPath: "README.md",
			Content: factorydefinitions.BundledFileContentConfig{
				Encoding: factorydefinitions.BundledFileEncodingUTF8,
				Inline:   "outside allowlist\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root-helper")
}

func TestRuleWorkerWorkstationBehaviorCompatibility_AcceptsCompatiblePairings(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{
		{Name: "infer", Type: factorydefinitions.WorkerTypeInference, Operations: inferenceOperationFixture()},
		{Name: "legacy-infer", Type: factorydefinitions.WorkerTypeModel, Operations: inferenceOperationFixture()},
		{Name: "agent", Type: factorydefinitions.WorkerTypeAgent},
		{Name: "legacy-agent", Type: factorydefinitions.WorkerTypeModel},
		{Name: "script", Type: factorydefinitions.WorkerTypeScript},
		{Name: "poller", Type: factorydefinitions.WorkerTypePoller, Provider: factorydefinitions.HostedWorkerProviderLinear},
	}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{
			Name:           "infer-run",
			Type:           factorydefinitions.WorkstationTypeInference,
			Operation:      "TTS",
			WorkerTypeName: "infer",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "legacy-infer-run",
			Type:           factorydefinitions.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "legacy-infer",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "agent-run",
			Type:           factorydefinitions.WorkstationTypeAgent,
			WorkerTypeName: "agent",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "legacy-agent-run",
			Type:           factorydefinitions.WorkstationTypeModel,
			WorkerTypeName: "legacy-agent",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "script-run",
			Type:           factorydefinitions.WorkstationTypeScript,
			WorkerTypeName: "script",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "poller-run",
			Type:           factorydefinitions.WorkstationTypePoller,
			WorkerTypeName: "poller",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
	}

	findings := ruleWorkerWorkstationBehaviorCompatibility(cfg)
	for _, finding := range findings {
		if finding.Rule == "workstation-worker-behavior-compatibility" {
			t.Fatalf("unexpected compatibility finding: %+v", finding)
		}
	}
}

func TestRuleWorkerWorkstationBehaviorCompatibility_RejectsIncompatiblePairings(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{
		{Name: "infer", Type: factorydefinitions.WorkerTypeInference, Operations: inferenceOperationFixture()},
		{Name: "agent", Type: factorydefinitions.WorkerTypeAgent},
		{Name: "script", Type: factorydefinitions.WorkerTypeScript},
	}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{
			Name:           "agent-with-infer",
			Type:           factorydefinitions.WorkstationTypeAgent,
			WorkerTypeName: "infer",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "infer-with-agent",
			Type:           factorydefinitions.WorkstationTypeInference,
			Operation:      "TTS",
			WorkerTypeName: "agent",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "poller-with-agent",
			Type:           factorydefinitions.WorkstationTypePoller,
			WorkerTypeName: "agent",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
	}

	findings := ruleWorkerWorkstationBehaviorCompatibility(cfg)
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[0](agent-with-infer).worker", "agent-run")
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[1](infer-with-agent).worker", "inference-run")
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[2](poller-with-agent).worker", "poller-run")
}

func TestConfigValidator_LegacyModelWorkstationPairingRemainsValid(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:             "planner",
		Type:             factorydefinitions.WorkerTypeModel,
		ModelProvider:    "CLAUDE",
		ExecutorProvider: "SCRIPT_WRAP",
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "plan-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "planner",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	result := NewConfigValidator(nil).Validate(cfg)
	for _, finding := range result.Findings {
		if finding.Rule == "workstation-worker-behavior-compatibility" {
			t.Fatalf("legacy MODEL_WORKSTATION + MODEL_WORKER should remain valid, got %+v", finding)
		}
	}
}

func inferenceOperationFixture() []factorydefinitions.ModelOperation {
	return []factorydefinitions.ModelOperation{{
		Name: "TTS",
		Inputs: []factorydefinitions.ModelOperationSlot{{
			Name:         "text",
			ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
		}},
		Outputs: []factorydefinitions.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio},
		}},
	}}
}

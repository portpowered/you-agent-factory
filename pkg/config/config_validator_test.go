package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidationResult_HasErrors_FalseWithOnlyWarningsAndHints(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityHint, Path: "b", Message: "hint", Rule: "r2"},
		},
	}
	if vr.HasErrors() { t.Fatal("HasErrors() should be false when only warnings and hints present") }
}

func TestValidationResult_HasErrors_TrueWithErrors(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityError, Path: "b", Message: "err", Rule: "r2"},
		},
	}
	if !vr.HasErrors() { t.Fatal("HasErrors() should be true when error findings present") }
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
	cfg := &interfaces.FactoryConfig{
		InputTypes: []interfaces.InputTypeConfig{
			{Name: "", Type: "default"},
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{{
				Name: "init",
				Type: interfaces.StateTypeInitial,
			}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "ws1",
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "task",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "task",
				StateName:    "nonexistent",
			}},
		}},
	}
	result := NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected errors")
	}
	errs := result.Errors()
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors from independent rules, got %d: %v", len(errs), errs)
	}
	assertFindingExists(t, errs, "input-type-name")
	assertFindingExists(t, errs, factoryvalidation.CodeDanglingPlaceReference)
}

func TestRuleInputTypes_MissingName(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "", Type: "default"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-name")
}

func TestRuleInputTypes_ReservedDefault(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "default", Type: "default"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-reserved")
}

func TestRuleInputTypes_Duplicate(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{
		{Name: "foo", Type: "default"},
		{Name: "foo", Type: "default"},
	}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-duplicate")
}

func TestRuleInputTypes_MissingType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "foo", Type: ""}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-type")
}

func TestRuleInputTypes_UnknownType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "foo", Type: "bogus"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-type")
}

func TestRuleInputTypes_ValidConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{
		Name: "batch",
		Type: interfaces.InputKindDefault,
	}}}
	findings := ruleInputTypes(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

type stubRequiredToolChecker map[string]RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool interfaces.RequiredToolConfig) RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return RequiredToolCheckResult{}
}

func testBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w1"}},
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
			t.Fatalf("finding path = %q, want substring %q", f.Path, pathSubstring)
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
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "speak",
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{{
			Slot: "text",
			Selector: &interfaces.ModelOperationBindingSelector{
				Label: "utterance",
			},
		}},
		WorkerTypeName: "tts-worker",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
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
		worker interfaces.WorkerConfig
	}{
		{
			name: "local worker",
			worker: interfaces.WorkerConfig{
				Name:          "tts-worker",
				Type:          interfaces.WorkerTypeModel,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelProvider: interfaces.RunnerIDCodex,
				ModelLocality: interfaces.ModelLocalityLocal,
				Operations: []interfaces.ModelOperation{{
					Name: "TTS",
					Inputs: []interfaces.ModelOperationSlot{
						{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
						{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
					},
					Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
				}},
			},
		},
		{
			name: "cloud worker",
			worker: interfaces.WorkerConfig{
				Name:          "tts-worker",
				Type:          interfaces.WorkerTypeModel,
				Model:         "gpt-4o-mini-tts",
				ModelProvider: interfaces.RunnerIDCodex,
				ModelLocality: interfaces.ModelLocalityCloud,
				Operations: []interfaces.ModelOperation{{
					Name: "TTS",
					Inputs: []interfaces.ModelOperationSlot{
						{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
						{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
					},
					Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
				}},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workers = []interfaces.WorkerConfig{tt.worker}
			cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
				Name:           "speak",
				Type:           interfaces.WorkstationTypeInvoke,
				Operation:      "TTS",
				WorkerTypeName: "tts-worker",
				OperationBindings: []interfaces.ModelOperationBinding{
					{Slot: "text", Selector: &interfaces.ModelOperationBindingSelector{Label: "utterance"}},
					{Slot: "voice", Config: []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeJSON, JSON: []byte(`{"name":"alloy"}`)}}},
				},
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
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
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "legacy",
		Type:           interfaces.WorkstationTypeModel,
		Operation:      "TTS",
		WorkerTypeName: "w1",
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-type", "workstations[0](legacy).operation", "only supported on MODEL_INVOKE")
}

func TestRuleModelInvokeWorkstations_RejectsWorkerCompatibilityAndOperationMismatch(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{
		{
			Name: "scripted",
			Type: interfaces.WorkerTypeScript,
		},
		{
			Name: "tts-worker",
			Type: interfaces.WorkerTypeModel,
			Operations: []interfaces.ModelOperation{{
				Name: "EMBED",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "vector",
					ContentTypes: []string{interfaces.ModelOperationContentTypeJSON},
				}},
			}},
		},
	}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{
			Name:           "bad-worker-type",
			Type:           interfaces.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "scripted",
		},
		{
			Name:           "bad-operation",
			Type:           interfaces.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "tts-worker",
		},
	}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-worker-compatibility", "workstations[0](bad-worker-type).worker", `worker "scripted" is incompatible`)
	assertFindingMatch(t, findings, "workstation-model-invoke-operation-mismatch", "workstations[1](bad-operation).operation", `does not declare requested operation "TTS"`)
}

func TestRuleModelInvokeWorkstations_RejectsIncompleteContentContract(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
			}},
		}},
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInvoke,
		Operation:      "TTS",
		WorkerTypeName: "tts-worker",
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-content-contract", "workstations[0](speak).operation", "incompatible content contract")
}

func TestRuleModelInvokeWorkstations_RejectsDuplicateUnknownAndEmptyOperationBindings(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInvoke,
		Operation:      "TTS",
		WorkerTypeName: "tts-worker",
		OperationBindings: []interfaces.ModelOperationBinding{
			{Slot: "text", Selector: &interfaces.ModelOperationBindingSelector{Label: "utterance"}},
			{Slot: "text", Config: []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeText, Text: "fallback"}}},
			{Slot: "voice", Selector: &interfaces.ModelOperationBindingSelector{Role: "system"}},
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
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		ModelLocality: interfaces.ModelLocalityLocal,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}},
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
			},
			Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
		}, {
			Name:    "TTS",
			Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-duplicate", "workers[0](tts-worker).operations[1](TTS).name", `duplicate operation name "TTS"`)
	assertFindingMatch(t, findings, "worker-model-operation-slot-duplicate", "workers[0](tts-worker).operations[0](TTS).inputs[1](text).name", `duplicate input slot name "text"`)
}

func TestRuleWorkerModelOperations_RejectsCapabilityDeclarationsOnScriptWorkers(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:          "scripted",
		Type:          interfaces.WorkerTypeScript,
		ModelLocality: interfaces.ModelLocalityCloud,
		Operations:    []interfaces.ModelOperation{{Name: "TTS"}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-worker-type", "workers[0](scripted)", "require worker type MODEL_WORKER")
}

func TestRuleWorkerModelOperations_RejectsMissingSlotContentTypes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name:   "TTS",
			Inputs: []interfaces.ModelOperationSlot{{Name: "text"}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-slot-content-types", "workers[0](tts-worker).operations[0](TTS).inputs[0](text).contentTypes", "at least one content type is required")
}

func TestRuleResourceUsage_NonexistentResource(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "bogus", Capacity: 1}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceUsage_ZeroCapacity(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 0}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
}

func TestRuleResourceUsage_ValidConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 2}},
	}}
	findings := ruleResourceUsage(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleResourceUsage_ValidatesWorkerRequirements(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:      "worker-a",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 0}, {Name: "missing", Capacity: 1}},
	}}

	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceDefinitions_RequiresModelMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:     "omnivoice-cache",
		Type:     interfaces.ResourceTypeModel,
		Capacity: 1,
	}}

	findings := ruleResourceDefinitions(cfg)
	assertFindingExists(t, findings, "resource-model-model")
	assertFindingExists(t, findings, "resource-model-backend")
	assertFindingExists(t, findings, "resource-model-load-policy")
}

func TestRuleResourceDefinitions_RequiresProviderQuotaMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:     "codex-tts-quota",
		Type:     interfaces.ResourceTypeProviderQuota,
		Capacity: 2,
	}}

	findings := ruleResourceDefinitions(cfg)
	assertFindingExists(t, findings, "resource-provider-quota-provider")
	assertFindingExists(t, findings, "resource-provider-quota-model")
}

func TestRuleResourceDefinitions_AcceptsModelResourceMetadata(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{
		Name:       "omnivoice-cache",
		Type:       interfaces.ResourceTypeModel,
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-name")
	assertFindingExists(t, findings, "required-tool-command")
}

func TestConfigValidator_RequiredToolsReportsPresentAndMissingCommandsDeterministically(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{
			{Name: "Go toolchain", Command: "go"},
			{Name: "Missing helper", Command: "missing-tool"},
		},
	}

	validator := NewConfigValidator(WithRequiredToolChecker(stubRequiredToolChecker{
		"go":           {ResolvedPath: "/usr/bin/go"},
		"missing-tool": {Err: assertErrString(`required tool "Missing helper" command "missing-tool" was not found on PATH`)},
	}))
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "BINARY",
			TargetPath: "factory/misc/helper.bin",
			Content: interfaces.BundledFileContentConfig{
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "SCRIPT",
			TargetPath: "../scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeScript,
			TargetPath: "/factory/scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
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
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeRootHelper,
			TargetPath: "Makefile",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-content-inline")
	if findings[0].Path != "resourceManifest.bundledFiles[0].content.inline" {
		t.Fatalf("expected inline-specific finding, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_AcceptsSupportedDiskBackedScriptAndDocWithoutInline(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestValidatePortableResourceManifestOnPath_AcceptsSupportedDiskBackedBundledFilesWithoutInline(t *testing.T) {
	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(scripts): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(factoryDir, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll(docs): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "scripts", "setup-workspace.py"), []byte("print('portable')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "docs", "usage.md"), []byte("# Usage\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(doc): %v", err)
	}

	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	if err := validatePortableResourceManifestOnPath(factoryDir, cfg); err != nil {
		t.Fatalf("validatePortableResourceManifestOnPath: %v", err)
	}
}

func TestConfigValidator_ValidateAcceptsSupportedDiskBackedBundledFilesWithoutInline(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	result := NewConfigValidator().Validate(cfg)
	if result.HasErrors() {
		t.Fatalf("expected config validator to accept supported disk-backed bundled files without inline content, got %#v", result.Errors())
	}
}

func TestConfigValidator_BundledFilesAcceptCanonicalScriptAndDocTargets(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       "SCRIPT",
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "print('portable')\n",
				},
			},
			{
				Type:       "DOC",
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "# Usage\n",
				},
			},
			{
				Type:       interfaces.BundledFileTypeRootHelper,
				TargetPath: "Makefile",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
					Inline:   "test:\n\tgo test ./...\n",
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestRuleBundledFiles_RejectsTargetOutsideCanonicalRootForType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "DOC",
			TargetPath: "factory/scripts/usage.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "# Usage\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
}

func TestRuleBundledFiles_RejectsUnsupportedInputTargetShape(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeInput,
			TargetPath: "factory/inputs/task/default/nested/starter.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "starter work\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
	if !strings.Contains(findings[0].Message, "factory/inputs/<work-type>/<channel>/<file>") {
		t.Fatalf("expected INPUT shape guidance, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsUnsupportedRootHelperTarget(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeRootHelper,
			TargetPath: "README.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "outside allowlist\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root-helper")
}

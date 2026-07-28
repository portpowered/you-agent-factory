package validation

import (
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type Finding = factorydefinitions.TopologyFinding
type ValidationResult = factorydefinitions.TopologyValidationResult

// validationRule is a function that inspects a factory config and returns findings.
type validationRule func(cfg *factorydefinitions.FactoryConfig) []Finding

const (
	portableBundledScriptRoot = "factory/scripts/"
	portableBundledDocRoot    = "factory/docs/"
	portableBundledInputRoot  = "factory/inputs/"
)

type RequiredToolCheckResult = factorydefinitions.RequiredToolCheckResult
type RequiredToolFailureKind = factorydefinitions.RequiredToolFailureKind

const (
	RequiredToolFailureKindNone         = factorydefinitions.RequiredToolFailureKindNone
	RequiredToolFailureKindMissing      = factorydefinitions.RequiredToolFailureKindMissing
	RequiredToolFailureKindVersionProbe = factorydefinitions.RequiredToolFailureKindVersionProbe
)

type RequiredToolChecker = factorydefinitions.RequiredToolChecker

// ConfigValidator runs all registered validation rules against a factory config.
type ConfigValidator struct {
	requiredToolChecker RequiredToolChecker
	rules               []validationRule
}

// NewConfigValidator creates a Config validator with its external tool checker
// supplied directly. A nil checker performs representation-only validation.
func NewConfigValidator(requiredToolChecker RequiredToolChecker) *ConfigValidator {
	cv := &ConfigValidator{
		requiredToolChecker: requiredToolChecker,
	}
	cv.rules = []validationRule{
		ruleInputTypes,
		ruleFactoryGuards,
		ruleGuards,
		ruleWorkstationKind,
		ruleClassifierWorkstations,
		ruleCronWorkstations,
		rulePollerWorkstations,
		ruleHostedWorkers,
		ruleWorkerModelOperations,
		ruleAgentWorkerTools,
		ruleModelInvokeWorkstations,
		rulePerInputGuards,
		ruleResourceDefinitions,
		ruleResourceUsage,
		ruleRequiredTools(cv.requiredToolChecker),
		ruleBundledFiles,
	}
	return cv
}

// Validate runs all rules and returns the aggregated result.
func (cv *ConfigValidator) Validate(cfg *factorydefinitions.FactoryConfig) *ValidationResult {
	result := &ValidationResult{}
	for _, rule := range cv.rules {
		result.Findings = append(result.Findings, rule(cfg)...)
	}
	return result
}

// ValidateRequiredTools runs only the declarative required-tool validation
// rules. Load boundaries can use this narrower pass without re-running the full
// topology validator.
func ValidateRequiredTools(cfg *factorydefinitions.FactoryConfig, checker RequiredToolChecker) *ValidationResult {
	result := &ValidationResult{}
	result.Findings = append(result.Findings, ruleRequiredTools(checker)(cfg)...)
	return result
}

func validatePortableResourceManifest(cfg *factorydefinitions.FactoryConfig, checker RequiredToolChecker, bundledFileRule func(*factorydefinitions.FactoryConfig) []Finding) *ValidationResult {
	result := ValidateRequiredTools(cfg, checker)
	result.Findings = append(result.Findings, bundledFileRule(cfg)...)
	return result
}

func validatePortableResourceManifestOnPath(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
	resolveSource factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
	requiredToolChecker RequiredToolChecker,
) error {
	result := validatePortableResourceManifest(cfg, requiredToolChecker, func(cfg *factorydefinitions.FactoryConfig) []Finding {
		return ruleBundledFilesOnPath(factoryDir, cfg, resolveSource, inspectSource)
	})
	if !result.HasErrors() {
		return nil
	}
	return fmt.Errorf("%s", result.Error())
}

func validatePortableBundledFilesForExpandOnPath(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
	resolveSource factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
) error {
	result := validatePortableResourceManifest(cfg, nil, func(cfg *factorydefinitions.FactoryConfig) []Finding {
		if strings.TrimSpace(factoryDir) == "" {
			return ruleBundledFiles(cfg)
		}
		return ruleBundledFilesOnPath(factoryDir, cfg, resolveSource, inspectSource)
	})
	if !result.HasErrors() {
		return nil
	}
	return fmt.Errorf("%s", result.Error())
}

func ValidatePortableResourceManifestOnPath(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
	requiredToolChecker RequiredToolChecker,
) error {
	return validatePortableResourceManifestOnPath(factoryDir, cfg, nil, nil, requiredToolChecker)
}

func ValidatePortableResourceManifestOnPathWithSourceResolver(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
	resolveSource factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
	requiredToolChecker RequiredToolChecker,
) error {
	return validatePortableResourceManifestOnPath(
		factoryDir,
		cfg,
		resolveSource,
		inspectSource,
		requiredToolChecker,
	)
}

func ValidatePortableBundledFilesForExpandOnPath(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
) error {
	return validatePortableBundledFilesForExpandOnPath(factoryDir, cfg, nil, nil)
}

func ValidatePortableBundledFilesForExpandOnPathWithSourceResolver(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
	resolveSource factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
) error {
	return validatePortableBundledFilesForExpandOnPath(
		factoryDir,
		cfg,
		resolveSource,
		inspectSource,
	)
}

func BundledFileFindings(cfg *factorydefinitions.FactoryConfig) []Finding {
	return ruleBundledFiles(cfg)
}

// --- Rule: input type validation ---

func ruleResourceUsage(cfg *factorydefinitions.FactoryConfig) []Finding {
	var findings []Finding
	validResources := make(map[string]bool)
	for _, r := range cfg.Resources {
		validResources[r.Name] = true
	}

	for wi, worker := range cfg.Workers {
		for ri, ru := range worker.Resources {
			path := fmt.Sprintf("workers[%d](%s).resources[%d]", wi, worker.Name, ri)
			if !validResources[ru.Name] {
				findings = append(findings, Finding{
					Severity: SeverityError, Path: path,
					Message: fmt.Sprintf("references non-existent resource %q", ru.Name),
					Rule:    "resource-usage-ref",
				})
			}
			if ru.Capacity <= 0 {
				findings = append(findings, Finding{
					Severity: SeverityError, Path: path,
					Message: "capacity must be positive",
					Rule:    "resource-usage-capacity",
				})
			}
		}
	}

	for wi, ws := range cfg.Workstations {
		for ri, ru := range ws.Resources {
			path := fmt.Sprintf("workstations[%d](%s).resources[%d]", wi, ws.Name, ri)
			if !validResources[ru.Name] {
				findings = append(findings, Finding{
					Severity: SeverityError, Path: path,
					Message: fmt.Sprintf("references non-existent resource %q", ru.Name),
					Rule:    "resource-usage-ref",
				})
			}
			if ru.Capacity <= 0 {
				findings = append(findings, Finding{
					Severity: SeverityError, Path: path,
					Message: "capacity must be positive",
					Rule:    "resource-usage-capacity",
				})
			}
		}
	}
	return findings
}

func ruleResourceDefinitions(cfg *factorydefinitions.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Resources) == 0 {
		return nil
	}

	var findings []Finding
	for i, resource := range cfg.Resources {
		basePath := fmt.Sprintf("resources[%d](%s)", i, resource.Name)
		if strings.TrimSpace(resource.Name) == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".name",
				Message:  "missing required 'name' field",
				Rule:     "resource-name",
			})
		}
		if resource.Capacity <= 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".capacity",
				Message:  "capacity must be positive",
				Rule:     "resource-capacity",
			})
		}

		switch strings.TrimSpace(resource.Type) {
		case "", factorydefinitions.ResourceTypeInvocationSlot:
			continue
		case factorydefinitions.ResourceTypeModel:
			if strings.TrimSpace(resource.Model) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".model",
					Message:  "MODEL resources require a non-empty model identifier",
					Rule:     "resource-model-model",
				})
			}
			if strings.TrimSpace(resource.Backend) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".backend",
					Message:  "MODEL resources require a non-empty backend",
					Rule:     "resource-model-backend",
				})
			}
			if strings.TrimSpace(resource.LoadPolicy) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".loadPolicy",
					Message:  "MODEL resources require a non-empty loadPolicy",
					Rule:     "resource-model-load-policy",
				})
			}
		case factorydefinitions.ResourceTypeProviderQuota:
			if strings.TrimSpace(resource.Provider) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".provider",
					Message:  "PROVIDER_QUOTA resources require a non-empty provider identity",
					Rule:     "resource-provider-quota-provider",
				})
			}
			if strings.TrimSpace(resource.Model) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".model",
					Message:  "PROVIDER_QUOTA resources require a non-empty model identifier",
					Rule:     "resource-provider-quota-model",
				})
			}
		}
	}

	return findings
}

// --- Rule: portable required-tool validation ---

func ruleRequiredTools(checker RequiredToolChecker) validationRule {
	return func(cfg *factorydefinitions.FactoryConfig) []Finding {
		if cfg == nil || cfg.ResourceManifest == nil || len(cfg.ResourceManifest.RequiredTools) == 0 {
			return nil
		}

		var findings []Finding
		for i, tool := range cfg.ResourceManifest.RequiredTools {
			basePath := fmt.Sprintf("resourceManifest.requiredTools[%d]", i)
			if strings.TrimSpace(tool.Name) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".name",
					Message:  "missing required 'name' field",
					Rule:     "required-tool-name",
				})
			}
			if strings.TrimSpace(tool.Command) == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".command",
					Message:  "missing required 'command' field",
					Rule:     "required-tool-command",
				})
				continue
			}
			for argIndex, arg := range tool.VersionArgs {
				if strings.TrimSpace(arg) != "" {
					continue
				}
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     fmt.Sprintf("%s.versionArgs[%d]", basePath, argIndex),
					Message:  "versionArgs entries must be non-empty strings",
					Rule:     "required-tool-version-args",
				})
			}
			if checker == nil {
				continue
			}
			result := checker.Check(tool)
			if result.Err == nil {
				continue
			}
			rule := "required-tool-missing"
			path := basePath + ".command"
			if result.FailureKind == RequiredToolFailureKindVersionProbe {
				rule = "required-tool-version-probe"
				path = basePath + ".versionArgs"
			}
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     path,
				Message:  result.Err.Error(),
				Rule:     rule,
			})
		}
		return findings
	}
}

// --- Rule: portable bundled-file validation ---

func ruleBundledFiles(cfg *factorydefinitions.FactoryConfig) []Finding {
	return ruleBundledFilesWithContentValidator(cfg, func(basePath string, file factorydefinitions.BundledFileConfig) []Finding {
		return validateBundledFileContent(basePath, file)
	})
}

func ruleBundledFilesOnPath(
	factoryDir string,
	cfg *factorydefinitions.FactoryConfig,
	resolveSource factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
) []Finding {
	return ruleBundledFilesWithContentValidator(cfg, func(basePath string, file factorydefinitions.BundledFileConfig) []Finding {
		return validateBundledFileContentOnPath(
			factoryDir,
			basePath,
			file,
			resolveSource,
			inspectSource,
		)
	})
}

func ruleBundledFilesWithContentValidator(cfg *factorydefinitions.FactoryConfig, validateContent func(basePath string, file factorydefinitions.BundledFileConfig) []Finding) []Finding {
	if cfg == nil || cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) == 0 {
		return nil
	}

	var findings []Finding
	seenTargetPaths := make(map[string]int, len(cfg.ResourceManifest.BundledFiles))
	for i, file := range cfg.ResourceManifest.BundledFiles {
		basePath := fmt.Sprintf("resourceManifest.bundledFiles[%d]", i)
		if previousIndex, ok := seenTargetPaths[file.TargetPath]; ok {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".targetPath",
				Message: fmt.Sprintf(
					"targetPath %q collides with resourceManifest.bundledFiles[%d]",
					file.TargetPath,
					previousIndex,
				),
				Rule: "bundled-file-target-duplicate",
			})
		} else {
			seenTargetPaths[file.TargetPath] = i
		}
		findings = append(findings, validateBundledFileType(basePath, file)...)
		findings = append(findings, validateBundledFileTarget(basePath, file)...)
		findings = append(findings, validateContent(basePath, file)...)
	}

	return findings
}

func validateBundledFileType(basePath string, file factorydefinitions.BundledFileConfig) []Finding {
	err := factorydefinitions.ValidatePortableBundledFileType(file)
	if err == nil {
		return nil
	}
	return []Finding{{
		Severity: SeverityError,
		Path:     basePath + ".type",
		Message:  err.Error(),
		Rule:     "bundled-file-type",
	}}
}

func validateBundledFileTarget(basePath string, file factorydefinitions.BundledFileConfig) []Finding {
	err := factorydefinitions.ValidatePortableBundledFileTarget(file)
	if err == nil {
		return nil
	}
	rule := "bundled-file-target-path"
	var validationErr *factorydefinitions.PortableBundledFileValidationError
	if errors.As(err, &validationErr) {
		switch validationErr.Kind {
		case factorydefinitions.PortableBundledFileValidationTargetRoot:
			rule = "bundled-file-target-root"
		case factorydefinitions.PortableBundledFileValidationTargetRootHelper:
			rule = "bundled-file-target-root-helper"
		}
	}
	return []Finding{{
		Severity: SeverityError,
		Path:     basePath + ".targetPath",
		Message:  err.Error(),
		Rule:     rule,
	}}
}

func validateBundledFileContent(basePath string, file factorydefinitions.BundledFileConfig) []Finding {
	findings := validateBundledFileEncoding(basePath, file)
	if strings.TrimSpace(file.Content.Inline) == "" && !factorydefinitions.ShouldOmitSupportedPortableBundledInline(file) {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.inline",
			Message:  "missing required 'inline' field",
			Rule:     "bundled-file-content-inline",
		})
	}
	return findings
}

func validateBundledFileEncoding(basePath string, file factorydefinitions.BundledFileConfig) []Finding {
	var findings []Finding
	if strings.TrimSpace(file.Content.Encoding) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.encoding",
			Message:  "missing required 'encoding' field",
			Rule:     "bundled-file-content-encoding",
		})
	} else if file.Content.Encoding != factorydefinitions.BundledFileEncodingUTF8 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.encoding",
			Message:  fmt.Sprintf("encoding %q is unsupported; use %q", file.Content.Encoding, factorydefinitions.BundledFileEncodingUTF8),
			Rule:     "bundled-file-content-encoding",
		})
	}
	return findings
}

func validateBundledFileContentOnPath(
	factoryDir string,
	basePath string,
	file factorydefinitions.BundledFileConfig,
	resolveSource factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
) []Finding {
	if strings.TrimSpace(file.Content.Inline) != "" {
		return validateBundledFileContent(basePath, file)
	}
	if resolveSource == nil {
		return validateBundledFileContent(basePath, file)
	}
	if inspectSource == nil {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".content.inline",
			Message:  "portable bundled-file source inspection is required",
			Rule:     "bundled-file-source-inspection",
		}}
	}
	if sourcePath, ok := resolveSource(
		factoryDir,
		file,
	); ok {
		info, err := inspectSource.Stat(sourcePath)
		if err == nil && !info.IsDir() {
			if strings.TrimSpace(file.Content.Encoding) == "" {
				return nil
			}
			return validateBundledFileEncoding(basePath, file)
		}
	}
	return validateBundledFileContent(basePath, file)
}

func buildValidWorkstations(cfg *factorydefinitions.FactoryConfig) map[string]bool {
	ws := make(map[string]bool)
	for _, w := range cfg.Workstations {
		ws[w.Name] = true
	}
	return ws
}

func ruleWorkerModelOperations(cfg *factorydefinitions.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Workers) == 0 {
		return nil
	}

	var findings []Finding
	for workerIndex, worker := range cfg.Workers {
		basePath := fmt.Sprintf("workers[%d](%s)", workerIndex, worker.Name)
		if len(worker.Operations) == 0 && strings.TrimSpace(worker.ModelLocality) == "" {
			continue
		}
		if strings.TrimSpace(worker.Type) != "" && !factorydefinitions.IsInferenceWorkerType(worker.Type) {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath,
				Message:  "model capability declarations require worker type INFERENCE_WORKER or legacy MODEL_WORKER",
				Rule:     "worker-model-operation-worker-type",
			})
		}

		seenOperations := make(map[string]bool, len(worker.Operations))
		for operationIndex, operation := range worker.Operations {
			operationPath := fmt.Sprintf("%s.operations[%d](%s)", basePath, operationIndex, operation.Name)
			name := strings.TrimSpace(operation.Name)
			if name == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     operationPath + ".name",
					Message:  "missing required 'name' field",
					Rule:     "worker-model-operation-name",
				})
			} else if seenOperations[name] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     operationPath + ".name",
					Message:  fmt.Sprintf("duplicate operation name %q on the same worker", name),
					Rule:     "worker-model-operation-duplicate",
				})
			}
			seenOperations[name] = true

			findings = append(findings, validateModelOperationSlots(operation.Inputs, operationPath+".inputs", "input")...)
			findings = append(findings, validateModelOperationSlots(operation.Outputs, operationPath+".outputs", "output")...)
		}
	}
	return findings
}

func validateModelOperationSlots(slots []factorydefinitions.ModelOperationSlot, path string, direction string) []Finding {
	if len(slots) == 0 {
		return nil
	}

	var findings []Finding
	seenSlots := make(map[string]bool, len(slots))
	for slotIndex, slot := range slots {
		slotPath := fmt.Sprintf("%s[%d](%s)", path, slotIndex, slot.Name)
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     slotPath + ".name",
				Message:  "missing required 'name' field",
				Rule:     "worker-model-operation-slot-name",
			})
		} else if seenSlots[name] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     slotPath + ".name",
				Message:  fmt.Sprintf("duplicate %s slot name %q within one operation direction", direction, name),
				Rule:     "worker-model-operation-slot-duplicate",
			})
		}
		seenSlots[name] = true

		if len(slot.ContentTypes) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     slotPath + ".contentTypes",
				Message:  "at least one content type is required",
				Rule:     "worker-model-operation-slot-content-types",
			})
		}
	}
	return findings
}

func ruleModelInvokeWorkstations(cfg *factorydefinitions.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]factorydefinitions.FactoryWorkerConfig, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var findings []Finding
	for workstationIndex, workstation := range cfg.Workstations {
		findings = append(findings, validateModelInvokeWorkstation(workstation, workstationIndex, workersByName)...)
	}

	return findings
}

func validateModelInvokeWorkstation(workstation factorydefinitions.FactoryWorkstationConfig, workstationIndex int, workersByName map[string]factorydefinitions.FactoryWorkerConfig) []Finding {
	basePath := fmt.Sprintf("workstations[%d](%s)", workstationIndex, workstation.Name)
	operationName := strings.TrimSpace(workstation.Operation)
	if !factorydefinitions.IsInferenceRunWorkstationType(workstation.Type) {
		return validateNonInvokeOperationUsage(basePath, operationName)
	}

	findings := requiredInferenceRunWorkstationFindings(workstation, basePath, operationName)
	if strings.TrimSpace(workstation.WorkerTypeName) == "" {
		return findings
	}

	worker, ok := workersByName[workstation.WorkerTypeName]
	if !ok {
		return findings
	}
	workerFindings, operation, ok := validateModelInvokeWorker(workstation, worker, basePath, operationName)
	findings = append(findings, workerFindings...)
	if !ok {
		return findings
	}

	return append(findings, validateModelOperationBindings(workstation.OperationBindings, operation.Inputs, basePath+".operationBindings")...)
}

func validateNonInvokeOperationUsage(basePath string, operationName string) []Finding {
	if operationName == "" {
		return nil
	}
	return []Finding{{
		Severity: SeverityError,
		Path:     basePath + ".operation",
		Message:  "operation is only supported on INFERENCE_RUN or legacy MODEL_INVOKE workstations",
		Rule:     "workstation-model-invoke-type",
	}}
}

func requiredInferenceRunWorkstationFindings(workstation factorydefinitions.FactoryWorkstationConfig, basePath string, operationName string) []Finding {
	var findings []Finding
	if operationName == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  "inference-run workstation requires an uppercase operation name",
			Rule:     "workstation-model-invoke-operation",
		})
	}
	if strings.TrimSpace(workstation.WorkerTypeName) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".worker",
			Message:  "inference-run workstation requires a worker reference",
			Rule:     "workstation-model-invoke-worker",
		})
	}
	return findings
}

func validateModelInvokeWorker(workstation factorydefinitions.FactoryWorkstationConfig, worker factorydefinitions.FactoryWorkerConfig, basePath string, operationName string) ([]Finding, factorydefinitions.ModelOperation, bool) {
	if strings.TrimSpace(worker.Type) != "" && !factorydefinitions.IsInferenceWorkerType(worker.Type) {
		return nil, factorydefinitions.ModelOperation{}, false
	}
	if operationName == "" {
		return nil, factorydefinitions.ModelOperation{}, false
	}

	operation, found := findWorkerOperation(worker.Operations, operationName)
	if !found {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  fmt.Sprintf("worker %q does not declare requested operation %q", workstation.WorkerTypeName, operationName),
			Rule:     "workstation-model-invoke-operation-mismatch",
		}}, factorydefinitions.ModelOperation{}, false
	}
	if len(operation.Inputs) == 0 || len(operation.Outputs) == 0 {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  fmt.Sprintf("worker %q operation %q has an incompatible content contract; inference-run workstations require at least one input slot and one output slot", workstation.WorkerTypeName, operationName),
			Rule:     "workstation-model-invoke-content-contract",
		}}, factorydefinitions.ModelOperation{}, false
	}

	return nil, operation, true
}

func findWorkerOperation(operations []factorydefinitions.ModelOperation, name string) (factorydefinitions.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return factorydefinitions.ModelOperation{}, false
}

func validateModelOperationBindings(bindings []factorydefinitions.ModelOperationBinding, inputs []factorydefinitions.ModelOperationSlot, path string) []Finding {
	if len(bindings) == 0 {
		return nil
	}

	knownSlots := make(map[string]bool, len(inputs))
	for _, slot := range inputs {
		name := strings.TrimSpace(slot.Name)
		if name != "" {
			knownSlots[name] = true
		}
	}

	seen := make(map[string]bool, len(bindings))
	var findings []Finding
	for i, binding := range bindings {
		bindingPath := fmt.Sprintf("%s[%d](%s)", path, i, binding.Slot)
		slotName := strings.TrimSpace(binding.Slot)
		if slotName == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath + ".slot",
				Message:  "operation binding requires a slot name",
				Rule:     "workstation-model-invoke-binding-slot",
			})
			continue
		}
		if seen[slotName] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath + ".slot",
				Message:  fmt.Sprintf("duplicate operation binding for slot %q", slotName),
				Rule:     "workstation-model-invoke-binding-duplicate",
			})
			continue
		}
		seen[slotName] = true
		if !knownSlots[slotName] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath + ".slot",
				Message:  fmt.Sprintf("operation binding references unknown input slot %q", slotName),
				Rule:     "workstation-model-invoke-binding-unknown-slot",
			})
		}
		if selectorIsEmpty(binding.Selector) && len(binding.Config) == 0 && len(binding.DefaultContent) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     bindingPath,
				Message:  "operation binding must declare a selector, config content, or default content",
				Rule:     "workstation-model-invoke-binding-empty",
			})
		}
	}
	return findings
}

func selectorIsEmpty(selector *factorydefinitions.ModelOperationBindingSelector) bool {
	if selector == nil {
		return true
	}
	return strings.TrimSpace(selector.Slot) == "" &&
		strings.TrimSpace(selector.Label) == "" &&
		strings.TrimSpace(selector.Type) == "" &&
		strings.TrimSpace(selector.Role) == ""
}

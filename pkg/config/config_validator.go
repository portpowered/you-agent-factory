package config

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// Severity classifies the importance of a validation finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityHint    Severity = "hint"
)

// Finding represents a single validation issue discovered in a factory config.
type Finding struct {
	Severity Severity
	Path     string // e.g. "workstations[0].inputs[1]"
	Message  string // human-readable description
	Rule     string // identifier like "workstation-input-ref"
}

// ValidationResult aggregates all findings from a validation pass.
type ValidationResult struct {
	Findings []Finding
}

// HasErrors returns true if any finding has error severity.
func (vr *ValidationResult) HasErrors() bool {
	for _, f := range vr.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only error-severity findings.
func (vr *ValidationResult) Errors() []Finding {
	var errs []Finding
	for _, f := range vr.Findings {
		if f.Severity == SeverityError {
			errs = append(errs, f)
		}
	}
	return errs
}

// Error returns a formatted error string listing all error-severity findings.
func (vr *ValidationResult) Error() string {
	errs := vr.Errors()
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "validation failed: %d errors", len(errs))
	for _, f := range errs {
		fmt.Fprintf(&b, "\n- [%s] %s: %s", f.Rule, f.Path, f.Message)
	}
	return b.String()
}

// validationRule is a function that inspects a factory config and returns findings.
type validationRule func(cfg *interfaces.FactoryConfig) []Finding

const (
	portableBundledScriptRoot = "factory/scripts/"
	portableBundledDocRoot    = "factory/docs/"
)

// RequiredToolCheckResult captures the availability result for one declarative
// required tool entry.
type RequiredToolCheckResult struct {
	ResolvedPath string
	FailureKind  RequiredToolFailureKind
	Err          error
}

// RequiredToolFailureKind classifies the canonical source of a required-tool
// validation failure.
type RequiredToolFailureKind string

const (
	RequiredToolFailureKindNone         RequiredToolFailureKind = ""
	RequiredToolFailureKindMissing      RequiredToolFailureKind = "missing"
	RequiredToolFailureKindVersionProbe RequiredToolFailureKind = "version-probe"
)

// RequiredToolChecker validates one required external tool entry without
// performing any installation or embedding behavior.
type RequiredToolChecker interface {
	Check(tool interfaces.RequiredToolConfig) RequiredToolCheckResult
}

type requiredToolCheckerFunc func(tool interfaces.RequiredToolConfig) RequiredToolCheckResult

func (f requiredToolCheckerFunc) Check(tool interfaces.RequiredToolConfig) RequiredToolCheckResult {
	return f(tool)
}

// ConfigValidatorOption configures optional validation behavior.
type ConfigValidatorOption func(*ConfigValidator)

// ConfigValidator runs all registered validation rules against a factory config.
type ConfigValidator struct {
	requiredToolChecker              RequiredToolChecker
	requireDefaultHandlingWorkType   bool
	rules                            []validationRule
}

// NewConfigValidator creates a ConfigValidator with all built-in validation rules.
func NewConfigValidator(opts ...ConfigValidatorOption) *ConfigValidator {
	cv := &ConfigValidator{
		requiredToolChecker: requiredToolCheckerFunc(checkRequiredToolOnPath),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cv)
		}
	}
	cv.rules = []validationRule{
		ruleInputTypes,
		cv.ruleWorkTypeHandlingBehavior,
		rulePlaceReferences,
		ruleFactoryGuards,
		ruleGuards,
		ruleWorkstationKind,
		ruleClassifierWorkstations,
		ruleCronWorkstations,
		rulePollerWorkstations,
		ruleHostedWorkers,
		ruleWorkerModelOperations,
		ruleModelInvokeWorkstations,
		ruleWorkerReferences,
		rulePerInputGuards,
		ruleResourceDefinitions,
		ruleResourceUsage,
		ruleRequiredTools(cv.requiredToolChecker),
		ruleBundledFiles,
	}
	return cv
}

// WithRequiredToolChecker overrides the external required-tool availability
// checker, primarily for deterministic tests.
func WithRequiredToolChecker(checker RequiredToolChecker) ConfigValidatorOption {
	return func(cv *ConfigValidator) {
		if checker != nil {
			cv.requiredToolChecker = checker
		}
	}
}

// WithRequireDefaultHandlingWorkType enables validation that exactly one work type
// declares handlingBehavior DEFAULT. Use this for simplified you run --factory flows.
func WithRequireDefaultHandlingWorkType() ConfigValidatorOption {
	return func(cv *ConfigValidator) {
		cv.requireDefaultHandlingWorkType = true
	}
}

// Validate runs all rules and returns the aggregated result.
func (cv *ConfigValidator) Validate(cfg *interfaces.FactoryConfig) *ValidationResult {
	result := &ValidationResult{}
	for _, rule := range cv.rules {
		result.Findings = append(result.Findings, rule(cfg)...)
	}
	return result
}

// ValidateRequiredTools runs only the declarative required-tool validation
// rules. Load boundaries can use this narrower pass without re-running the full
// topology validator.
func ValidateRequiredTools(cfg *interfaces.FactoryConfig, checker RequiredToolChecker) *ValidationResult {
	result := &ValidationResult{}
	result.Findings = append(result.Findings, ruleRequiredTools(checker)(cfg)...)
	return result
}

func validatePortableResourceManifest(cfg *interfaces.FactoryConfig, checker RequiredToolChecker, bundledFileRule func(*interfaces.FactoryConfig) []Finding) *ValidationResult {
	result := ValidateRequiredTools(cfg, checker)
	result.Findings = append(result.Findings, bundledFileRule(cfg)...)
	return result
}

func validatePortableResourceManifestOnPath(factoryDir string, cfg *interfaces.FactoryConfig) error {
	result := validatePortableResourceManifest(cfg, requiredToolCheckerFunc(checkRequiredToolOnPath), func(cfg *interfaces.FactoryConfig) []Finding {
		return ruleBundledFilesOnPath(factoryDir, cfg)
	})
	if !result.HasErrors() {
		return nil
	}
	return fmt.Errorf("%s", result.Error())
}

func validatePortableBundledFilesForExpandOnPath(factoryDir string, cfg *interfaces.FactoryConfig) error {
	result := validatePortableResourceManifest(cfg, nil, func(cfg *interfaces.FactoryConfig) []Finding {
		if strings.TrimSpace(factoryDir) == "" {
			return ruleBundledFiles(cfg)
		}
		return ruleBundledFilesOnPath(factoryDir, cfg)
	})
	if !result.HasErrors() {
		return nil
	}
	return fmt.Errorf("%s", result.Error())
}

// --- Rule: work type handling behavior ---

func (cv *ConfigValidator) ruleWorkTypeHandlingBehavior(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil {
		return nil
	}

	var findings []Finding
	defaultHandlingCount := 0
	for workTypeIndex, workType := range cfg.WorkTypes {
		basePath := fmt.Sprintf("workTypes[%d](%s)", workTypeIndex, workType.Name)
		seenBehaviors := make(map[string]bool, len(workType.HandlingBehavior))
		workTypeDeclaresDefault := false
		for behaviorIndex, behavior := range workType.HandlingBehavior {
			behaviorPath := fmt.Sprintf("%s.handlingBehavior[%d]", basePath, behaviorIndex)
			canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(behavior)
			if canonical == "" {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     behaviorPath,
					Message:  fmt.Sprintf("unsupported handlingBehavior value %q", behavior),
					Rule:     "work-type-handling-behavior-value",
				})
				continue
			}
			if seenBehaviors[canonical] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     behaviorPath,
					Message:  fmt.Sprintf("duplicate handlingBehavior value %q on the same work type", canonical),
					Rule:     "work-type-handling-behavior-duplicate",
				})
				continue
			}
			seenBehaviors[canonical] = true
			if canonical == interfaces.WorkTypeHandlingBehaviorDefault {
				workTypeDeclaresDefault = true
			}
		}
		if workTypeDeclaresDefault {
			defaultHandlingCount++
		}
	}

	if defaultHandlingCount > 1 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     "workTypes",
			Message:  fmt.Sprintf("expected at most one work type with handlingBehavior DEFAULT, found %d", defaultHandlingCount),
			Rule:     "work-type-handling-behavior-unique-default",
		})
	}
	if cv.requireDefaultHandlingWorkType && defaultHandlingCount == 0 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     "workTypes",
			Message:  "expected exactly one work type with handlingBehavior DEFAULT for simplified prompt runs",
			Rule:     "work-type-handling-behavior-required-default",
		})
	}
	return findings
}

// --- Rule: input type validation ---

func ruleResourceUsage(cfg *interfaces.FactoryConfig) []Finding {
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

func ruleResourceDefinitions(cfg *interfaces.FactoryConfig) []Finding {
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
		case "", interfaces.ResourceTypeInvocationSlot:
			continue
		case interfaces.ResourceTypeModel:
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
		case interfaces.ResourceTypeProviderQuota:
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
	return func(cfg *interfaces.FactoryConfig) []Finding {
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

func ruleBundledFiles(cfg *interfaces.FactoryConfig) []Finding {
	return ruleBundledFilesWithContentValidator(cfg, func(basePath string, file interfaces.BundledFileConfig) []Finding {
		return validateBundledFileContent(basePath, file)
	})
}

func ruleBundledFilesOnPath(factoryDir string, cfg *interfaces.FactoryConfig) []Finding {
	return ruleBundledFilesWithContentValidator(cfg, func(basePath string, file interfaces.BundledFileConfig) []Finding {
		return validateBundledFileContentOnPath(factoryDir, basePath, file)
	})
}

func ruleBundledFilesWithContentValidator(cfg *interfaces.FactoryConfig, validateContent func(basePath string, file interfaces.BundledFileConfig) []Finding) []Finding {
	if cfg == nil || cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) == 0 {
		return nil
	}

	var findings []Finding
	for i, file := range cfg.ResourceManifest.BundledFiles {
		basePath := fmt.Sprintf("resourceManifest.bundledFiles[%d]", i)
		findings = append(findings, validateBundledFileType(basePath, file)...)
		findings = append(findings, validateBundledFileTarget(basePath, file)...)
		findings = append(findings, validateContent(basePath, file)...)
	}

	return findings
}

func validateBundledFileType(basePath string, file interfaces.BundledFileConfig) []Finding {
	if strings.TrimSpace(file.Type) == "" {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".type",
			Message:  "missing required 'type' field",
			Rule:     "bundled-file-type",
		}}
	}
	if isSupportedBundledFileType(file.Type) {
		return nil
	}
	return []Finding{{
		Severity: SeverityError,
		Path:     basePath + ".type",
		Message: fmt.Sprintf(
			"type %q must be one of %q, %q, %q, or %q",
			file.Type,
			interfaces.BundledFileTypeScript,
			interfaces.BundledFileTypeDoc,
			interfaces.BundledFileTypeInput,
			interfaces.BundledFileTypeRootHelper,
		),
		Rule: "bundled-file-type",
	}}
}

func validateBundledFileTarget(basePath string, file interfaces.BundledFileConfig) []Finding {
	targetPath := strings.TrimSpace(file.TargetPath)
	if targetPath == "" {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".targetPath",
			Message:  "missing required 'targetPath' field",
			Rule:     "bundled-file-target-path",
		}}
	}
	if err := validateBundledFileTargetPath(targetPath); err != nil {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".targetPath",
			Message:  err.Error(),
			Rule:     "bundled-file-target-path",
		}}
	}
	if file.Type == interfaces.BundledFileTypeRootHelper && !isSupportedPortableBundledRootHelperTarget(targetPath) {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".targetPath",
			Message:  fmt.Sprintf("targetPath %q must be one of the supported root helper files", targetPath),
			Rule:     "bundled-file-target-root-helper",
		}}
	}
	if expectedRoot := bundledFileRootForType(file.Type); expectedRoot != "" && !strings.HasPrefix(targetPath, expectedRoot) {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".targetPath",
			Message:  fmt.Sprintf("targetPath %q must stay under %q for %s bundled files", targetPath, expectedRoot, file.Type),
			Rule:     "bundled-file-target-root",
		}}
	}
	if file.Type == interfaces.BundledFileTypeInput && !isSupportedPortableBundledInputTarget(targetPath) {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".targetPath",
			Message:  fmt.Sprintf("targetPath %q must use factory/inputs/<work-type>/<channel>/<file> for INPUT bundled files", targetPath),
			Rule:     "bundled-file-target-root",
		}}
	}
	return nil
}

func validateBundledFileContent(basePath string, file interfaces.BundledFileConfig) []Finding {
	findings := validateBundledFileEncoding(basePath, file)
	if strings.TrimSpace(file.Content.Inline) == "" && !shouldOmitSupportedPortableBundledInline(file) {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.inline",
			Message:  "missing required 'inline' field",
			Rule:     "bundled-file-content-inline",
		})
	}
	return findings
}

func validateBundledFileEncoding(basePath string, file interfaces.BundledFileConfig) []Finding {
	var findings []Finding
	if strings.TrimSpace(file.Content.Encoding) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.encoding",
			Message:  "missing required 'encoding' field",
			Rule:     "bundled-file-content-encoding",
		})
	} else if file.Content.Encoding != interfaces.BundledFileEncodingUTF8 {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.encoding",
			Message:  fmt.Sprintf("encoding %q is unsupported; use %q", file.Content.Encoding, interfaces.BundledFileEncodingUTF8),
			Rule:     "bundled-file-content-encoding",
		})
	}
	return findings
}

func validateBundledFileContentOnPath(factoryDir, basePath string, file interfaces.BundledFileConfig) []Finding {
	if strings.TrimSpace(file.Content.Inline) != "" {
		return validateBundledFileContent(basePath, file)
	}
	if sourcePath, ok := supportedPortableBundledSourcePath(factoryDir, file); ok {
		info, err := os.Stat(sourcePath)
		if err == nil && !info.IsDir() {
			if strings.TrimSpace(file.Content.Encoding) == "" {
				return nil
			}
			return validateBundledFileEncoding(basePath, file)
		}
	}
	return validateBundledFileContent(basePath, file)
}

// --- Helpers ---

func checkRequiredToolOnPath(tool interfaces.RequiredToolConfig) RequiredToolCheckResult {
	command := strings.TrimSpace(tool.Command)
	if command == "" {
		return RequiredToolCheckResult{}
	}

	resolvedPath, err := exec.LookPath(command)
	if err != nil {
		return RequiredToolCheckResult{
			FailureKind: RequiredToolFailureKindMissing,
			Err:         fmt.Errorf("required tool %q command %q was not found on PATH", tool.Name, tool.Command),
		}
	}

	if len(tool.VersionArgs) == 0 {
		return RequiredToolCheckResult{ResolvedPath: resolvedPath}
	}

	cmd := exec.Command(resolvedPath, tool.VersionArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := fmt.Sprintf(
			"required tool %q command %q failed version probe %q: %v",
			tool.Name,
			tool.Command,
			strings.Join(tool.VersionArgs, " "),
			err,
		)
		if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
			message += fmt.Sprintf(" (%s)", trimmed)
		}
		return RequiredToolCheckResult{
			ResolvedPath: resolvedPath,
			FailureKind:  RequiredToolFailureKindVersionProbe,
			Err:          fmt.Errorf("%s", message),
		}
	}

	return RequiredToolCheckResult{ResolvedPath: resolvedPath}
}

func isSupportedBundledFileType(fileType string) bool {
	switch fileType {
	case interfaces.BundledFileTypeScript, interfaces.BundledFileTypeDoc, interfaces.BundledFileTypeInput, interfaces.BundledFileTypeRootHelper:
		return true
	default:
		return false
	}
}

func isSupportedPortableBundledFile(file interfaces.BundledFileConfig) bool {
	if !isSupportedBundledFileType(file.Type) {
		return false
	}
	targetPath := strings.TrimSpace(file.TargetPath)
	if targetPath == "" {
		return false
	}
	if file.Type == interfaces.BundledFileTypeRootHelper {
		return isSupportedPortableBundledRootHelperTarget(targetPath)
	}
	expectedRoot := bundledFileRootForType(file.Type)
	if expectedRoot == "" || !strings.HasPrefix(targetPath, expectedRoot) {
		return false
	}
	if file.Type == interfaces.BundledFileTypeInput {
		return isSupportedPortableBundledInputTarget(targetPath)
	}
	return true
}

func bundledFileRootForType(fileType string) string {
	switch fileType {
	case interfaces.BundledFileTypeScript:
		return portableBundledScriptRoot
	case interfaces.BundledFileTypeDoc:
		return portableBundledDocRoot
	case interfaces.BundledFileTypeInput:
		return portableBundledInputRoot
	default:
		return ""
	}
}

func isSupportedPortableBundledRootHelperTarget(targetPath string) bool {
	switch targetPath {
	case "Makefile":
		return true
	case "factory/portable-dependencies.json":
		return true
	default:
		return false
	}
}

func isSupportedPortableBundledInputTarget(targetPath string) bool {
	if !strings.HasPrefix(targetPath, portableBundledInputRoot) {
		return false
	}
	relativePath := strings.TrimPrefix(targetPath, portableBundledInputRoot)
	parts := strings.Split(relativePath, "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func validateBundledFileTargetPath(targetPath string) error {
	if filepath.IsAbs(targetPath) || path.IsAbs(targetPath) || filepath.VolumeName(targetPath) != "" {
		return fmt.Errorf("targetPath %q must be factory-relative, not absolute", targetPath)
	}
	if strings.Contains(targetPath, "\\") {
		return fmt.Errorf("targetPath %q must use forward slashes", targetPath)
	}
	cleaned := path.Clean(targetPath)
	if cleaned == "." {
		return fmt.Errorf("targetPath %q must point to a file inside the factory root", targetPath)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("targetPath %q cannot escape the factory root", targetPath)
	}
	if cleaned != targetPath {
		return fmt.Errorf("targetPath %q must already be canonical and must not contain '.' or '..' segments", targetPath)
	}
	if strings.HasSuffix(targetPath, "/") {
		return fmt.Errorf("targetPath %q must point to a file, not a directory", targetPath)
	}
	return nil
}

func buildValidPlaces(cfg *interfaces.FactoryConfig) map[string]bool {
	places := make(map[string]bool)
	for _, wt := range cfg.WorkTypes {
		for _, s := range wt.States {
			places[fmt.Sprintf("%s:%s", wt.Name, s.Name)] = true
		}
	}
	for _, r := range cfg.Resources {
		places[fmt.Sprintf("%s:available", r.Name)] = true
	}
	return places
}

func buildValidWorkstations(cfg *interfaces.FactoryConfig) map[string]bool {
	ws := make(map[string]bool)
	for _, w := range cfg.Workstations {
		ws[w.Name] = true
	}
	return ws
}

func ruleWorkerModelOperations(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Workers) == 0 {
		return nil
	}

	var findings []Finding
	for workerIndex, worker := range cfg.Workers {
		basePath := fmt.Sprintf("workers[%d](%s)", workerIndex, worker.Name)
		if len(worker.Operations) == 0 && strings.TrimSpace(worker.ModelLocality) == "" {
			continue
		}
		if strings.TrimSpace(worker.Type) != "" && worker.Type != interfaces.WorkerTypeModel {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath,
				Message:  "model capability declarations require worker type MODEL_WORKER",
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

func validateModelOperationSlots(slots []interfaces.ModelOperationSlot, path string, direction string) []Finding {
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

func ruleModelInvokeWorkstations(cfg *interfaces.FactoryConfig) []Finding {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	workersByName := make(map[string]interfaces.WorkerConfig, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		workersByName[worker.Name] = worker
	}

	var findings []Finding
	for workstationIndex, workstation := range cfg.Workstations {
		findings = append(findings, validateModelInvokeWorkstation(workstation, workstationIndex, workersByName)...)
	}

	return findings
}

func validateModelInvokeWorkstation(workstation interfaces.FactoryWorkstationConfig, workstationIndex int, workersByName map[string]interfaces.WorkerConfig) []Finding {
	basePath := fmt.Sprintf("workstations[%d](%s)", workstationIndex, workstation.Name)
	operationName := strings.TrimSpace(workstation.Operation)
	if strings.TrimSpace(workstation.Type) != interfaces.WorkstationTypeInvoke {
		return validateNonInvokeOperationUsage(basePath, operationName)
	}

	findings := requiredModelInvokeWorkstationFindings(workstation, basePath, operationName)
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
		Message:  "operation is only supported on MODEL_INVOKE workstations",
		Rule:     "workstation-model-invoke-type",
	}}
}

func requiredModelInvokeWorkstationFindings(workstation interfaces.FactoryWorkstationConfig, basePath string, operationName string) []Finding {
	var findings []Finding
	if operationName == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  "MODEL_INVOKE workstation requires an uppercase operation name",
			Rule:     "workstation-model-invoke-operation",
		})
	}
	if strings.TrimSpace(workstation.WorkerTypeName) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".worker",
			Message:  "MODEL_INVOKE workstation requires a worker reference",
			Rule:     "workstation-model-invoke-worker",
		})
	}
	return findings
}

func validateModelInvokeWorker(workstation interfaces.FactoryWorkstationConfig, worker interfaces.WorkerConfig, basePath string, operationName string) ([]Finding, interfaces.ModelOperation, bool) {
	if strings.TrimSpace(worker.Type) != "" && worker.Type != interfaces.WorkerTypeModel {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".worker",
			Message:  fmt.Sprintf("worker %q is incompatible with MODEL_INVOKE; declare type MODEL_WORKER and model operations", workstation.WorkerTypeName),
			Rule:     "workstation-model-invoke-worker-compatibility",
		}}, interfaces.ModelOperation{}, false
	}
	if operationName == "" {
		return nil, interfaces.ModelOperation{}, false
	}

	operation, found := findWorkerOperation(worker.Operations, operationName)
	if !found {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  fmt.Sprintf("worker %q does not declare requested operation %q", workstation.WorkerTypeName, operationName),
			Rule:     "workstation-model-invoke-operation-mismatch",
		}}, interfaces.ModelOperation{}, false
	}
	if len(operation.Inputs) == 0 || len(operation.Outputs) == 0 {
		return []Finding{{
			Severity: SeverityError,
			Path:     basePath + ".operation",
			Message:  fmt.Sprintf("worker %q operation %q has an incompatible content contract; MODEL_INVOKE requires at least one input slot and one output slot", workstation.WorkerTypeName, operationName),
			Rule:     "workstation-model-invoke-content-contract",
		}}, interfaces.ModelOperation{}, false
	}

	return nil, operation, true
}

func findWorkerOperation(operations []interfaces.ModelOperation, name string) (interfaces.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return interfaces.ModelOperation{}, false
}

func validateModelOperationBindings(bindings []interfaces.ModelOperationBinding, inputs []interfaces.ModelOperationSlot, path string) []Finding {
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

func selectorIsEmpty(selector *interfaces.ModelOperationBindingSelector) bool {
	if selector == nil {
		return true
	}
	return strings.TrimSpace(selector.Slot) == "" &&
		strings.TrimSpace(selector.Label) == "" &&
		strings.TrimSpace(selector.Type) == "" &&
		strings.TrimSpace(selector.Role) == ""
}

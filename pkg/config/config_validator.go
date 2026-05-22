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
	requiredToolChecker RequiredToolChecker
	rules               []validationRule
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
		rulePlaceReferences,
		ruleFactoryGuards,
		ruleGuards,
		ruleWorkstationKind,
		ruleCronWorkstations,
		ruleWorkerModelOperations,
		ruleWorkerReferences,
		rulePerInputGuards,
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

// --- Rule: input type validation ---

func ruleResourceUsage(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validResources := make(map[string]bool)
	for _, r := range cfg.Resources {
		validResources[r.Name] = true
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
			"type %q must be one of %q, %q, or %q",
			file.Type,
			interfaces.BundledFileTypeScript,
			interfaces.BundledFileTypeDoc,
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
	return expectedRoot != "" && strings.HasPrefix(targetPath, expectedRoot)
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

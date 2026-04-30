package config

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/timework"
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
		ruleGuards,
		ruleWorkstationKind,
		ruleCronWorkstations,
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

// ValidatePortableResourceManifest runs only portability-manifest validation
// rules. Load boundaries can use this narrower pass without re-running the full
// topology validator.
func ValidatePortableResourceManifest(cfg *interfaces.FactoryConfig, checker RequiredToolChecker) *ValidationResult {
	result := ValidateRequiredTools(cfg, checker)
	result.Findings = append(result.Findings, ruleBundledFiles(cfg)...)
	return result
}

func validatePortableResourceManifestOnPath(cfg *interfaces.FactoryConfig) error {
	result := ValidatePortableResourceManifest(cfg, requiredToolCheckerFunc(checkRequiredToolOnPath))
	if !result.HasErrors() {
		return nil
	}
	return fmt.Errorf("%s", result.Error())
}

func validatePortableResourceManifestForExpand(cfg *interfaces.FactoryConfig) error {
	result := ValidatePortableResourceManifest(cfg, nil)
	if !result.HasErrors() {
		return nil
	}
	return fmt.Errorf("%s", result.Error())
}

// --- Rule: input type validation ---

func ruleInputTypes(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	seen := make(map[string]bool)
	for i, it := range cfg.InputTypes {
		path := fmt.Sprintf("input_types[%d]", i)
		if it.Name == "" {
			findings = append(findings, Finding{
				Severity: SeverityError, Path: path,
				Message: "missing required 'name' field", Rule: "input-type-name",
			})
			continue
		}
		pathNamed := fmt.Sprintf("input_types[%d](%s)", i, it.Name)
		if it.Name == "default" {
			findings = append(findings, Finding{
				Severity: SeverityError, Path: path,
				Message: "'default' is an implicit input type and must not be declared", Rule: "input-type-reserved",
			})
		}
		if seen[it.Name] {
			findings = append(findings, Finding{
				Severity: SeverityError, Path: path,
				Message: fmt.Sprintf("duplicate input type name %q", it.Name), Rule: "input-type-duplicate",
			})
		}
		seen[it.Name] = true

		switch it.Type {
		case interfaces.InputKindDefault:
			// valid
		case "":
			findings = append(findings, Finding{
				Severity: SeverityError, Path: pathNamed,
				Message: "missing required 'type' field", Rule: "input-type-type",
			})
		default:
			findings = append(findings, Finding{
				Severity: SeverityError, Path: pathNamed,
				Message: fmt.Sprintf("unknown input type %q (supported: %q)", it.Type, interfaces.InputKindDefault),
				Rule:    "input-type-type",
			})
		}
	}
	return findings
}

// --- Rule: place reference validation ---

func rulePlaceReferences(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validPlaces := buildValidPlaces(cfg)

	for wi, ws := range cfg.Workstations {
		for ii, input := range ws.Inputs {
			if !validPlaces[mapToID(input)] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     fmt.Sprintf("workstations[%d](%s).inputs[%d]", wi, ws.Name, ii),
					Message:  fmt.Sprintf("references non-existent state %q of work type %q", input.StateName, input.WorkTypeName),
					Rule:     "workstation-input-ref",
				})
			}
		}
		for oi, output := range ws.Outputs {
			if !validPlaces[mapToID(output)] {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     fmt.Sprintf("workstations[%d](%s).outputs[%d]", wi, ws.Name, oi),
					Message:  fmt.Sprintf("references non-existent state %q of work type %q", output.StateName, output.WorkTypeName),
					Rule:     "workstation-output-ref",
				})
			}
		}
		if ws.OnRejection != nil && !validPlaces[mapToID(*ws.OnRejection)] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).on_rejection", wi, ws.Name),
				Message:  fmt.Sprintf("references non-existent state %q of work type %q", ws.OnRejection.StateName, ws.OnRejection.WorkTypeName),
				Rule:     "workstation-on-rejection-ref",
			})
		}
		if ws.OnFailure != nil && !validPlaces[mapToID(*ws.OnFailure)] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).on_failure", wi, ws.Name),
				Message:  fmt.Sprintf("references non-existent state %q of work type %q", ws.OnFailure.StateName, ws.OnFailure.WorkTypeName),
				Rule:     "workstation-on-failure-ref",
			})
		}
	}
	return findings
}

// --- Rule: guard validation ---

func ruleGuards(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validWorkstations := buildValidWorkstations(cfg)

	for wi, ws := range cfg.Workstations {
		for gi, g := range ws.Guards {
			path := fmt.Sprintf("workstations[%d](%s).guards[%d]", wi, ws.Name, gi)
			switch g.Type {
			case interfaces.GuardTypeVisitCount:
				if g.Workstation == "" {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("guard of type %q requires 'workstation' parameter", g.Type),
						Rule:    "guard-visit-count-workstation",
					})
				} else if !validWorkstations[g.Workstation] {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("references non-existent workstation %q", g.Workstation),
						Rule:    "guard-visit-count-workstation",
					})
				}
				if g.MaxVisits <= 0 {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("guard of type %q requires positive 'max_visits'", g.Type),
						Rule:    "guard-visit-count-max-visits",
					})
				}
			case interfaces.GuardTypeMatchesFields:
				if g.MatchConfig == nil || strings.TrimSpace(g.MatchConfig.InputKey) == "" {
					findings = append(findings, Finding{
						Severity: SeverityError, Path: path,
						Message: fmt.Sprintf("guard of type %q requires non-empty 'match_config.input_key'", g.Type),
						Rule:    "guard-matches-fields-input-key",
					})
				}
			default:
				findings = append(findings, Finding{
					Severity: SeverityError, Path: path,
					Message: fmt.Sprintf("unsupported workstation guard type %q (workstation guards support: visit_count, matches_fields; use per-input guards for child fan-in)", g.Type),
					Rule:    "guard-unknown-type",
				})
			}
		}
	}
	return findings
}

// --- Rule: workstation kind validation ---

func ruleWorkstationKind(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validKinds := map[interfaces.WorkstationKind]bool{
		interfaces.WorkstationKindStandard: true,
		interfaces.WorkstationKindRepeater: true,
		interfaces.WorkstationKindCron:     true,
	}
	for wi, ws := range cfg.Workstations {
		if ws.Kind == "" {
			continue
		}
		if !validKinds[ws.Kind] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).kind", wi, ws.Name),
				Message:  fmt.Sprintf("unknown kind %q (valid kinds: standard, repeater, cron)", ws.Kind),
				Rule:     "workstation-kind",
			})
		}
	}
	return findings
}

// --- Rule: cron workstation validation ---

// portos:func-length-exception owner=agent-factory reason=cron-validation-rule-table review=2026-07-18 removal=split-cron-field-validators-before-adding-more-cron-options
func ruleCronWorkstations(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding

	for wi, ws := range cfg.Workstations {
		basePath := fmt.Sprintf("workstations[%d](%s)", wi, ws.Name)

		if ws.Kind != interfaces.WorkstationKindCron {
			if ws.Cron != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".cron",
					Message:  "cron configuration is only valid when kind is \"cron\"",
					Rule:     "cron-type",
				})
			}
			continue
		}

		if ws.Cron == nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron",
				Message:  "cron workstation requires a 'cron' configuration object",
				Rule:     "cron-config",
			})
			continue
		}

		if ws.Cron.HasUnsupportedInterval() {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron.interval",
				Message:  "cron.interval is not supported; use cron.schedule",
				Rule:     "cron-interval",
			})
		}

		hasSchedule := strings.TrimSpace(ws.Cron.Schedule) != ""
		if !hasSchedule {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron.schedule",
				Message:  "cron workstation requires non-empty 'schedule'",
				Rule:     "cron-schedule",
			})
		} else if err := timework.ValidateCronSchedule(ws.Cron.Schedule); err != nil {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".cron.schedule",
				Message:  err.Error(),
				Rule:     "cron-schedule",
			})
		}
		if strings.TrimSpace(ws.Cron.Jitter) != "" {
			if _, err := timework.ParseCronJitter(ws.Cron); err != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".cron.jitter",
					Message:  fmt.Sprintf("jitter must be a non-negative duration, got %q", ws.Cron.Jitter),
					Rule:     "cron-jitter",
				})
			}
		}
		if strings.TrimSpace(ws.Cron.ExpiryWindow) != "" {
			if _, err := timework.ParseCronExpiryWindow(ws.Cron, 1); err != nil {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Path:     basePath + ".cron.expiry_window",
					Message:  fmt.Sprintf("expiry_window must be a positive duration, got %q", ws.Cron.ExpiryWindow),
					Rule:     "cron-expiry-window",
				})
			}
		}
		if len(ws.Outputs) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".outputs",
				Message:  "cron workstation requires at least one configured output",
				Rule:     "cron-output",
			})
		}
		if strings.TrimSpace(ws.WorkerTypeName) == "" {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     basePath + ".worker",
				Message:  "cron workstation requires a worker because cron dispatches through the normal worker path",
				Rule:     "cron-worker",
			})
		}
	}

	return findings
}

// --- Rule: worker reference validation ---

func ruleWorkerReferences(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validWorkers := make(map[string]bool)
	for _, w := range cfg.Workers {
		validWorkers[w.Name] = true
	}
	for wi, ws := range cfg.Workstations {
		if ws.WorkerTypeName != "" && !validWorkers[ws.WorkerTypeName] {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Path:     fmt.Sprintf("workstations[%d](%s).worker", wi, ws.Name),
				Message:  fmt.Sprintf("references non-existent worker %q", ws.WorkerTypeName),
				Rule:     "workstation-worker-ref",
			})
		}
	}
	return findings
}

// --- Rule: per-input guard validation ---

func rulePerInputGuards(cfg *interfaces.FactoryConfig) []Finding {
	var findings []Finding
	validWorkstations := buildValidWorkstations(cfg)

	for wi, ws := range cfg.Workstations {
		inputWorkTypes := perInputGuardWorkTypes(ws.Inputs)

		for ii, input := range ws.Inputs {
			if input.Guard == nil {
				continue
			}
			path := fmt.Sprintf("workstations[%d](%s).inputs[%d].guard", wi, ws.Name, ii)
			findings = append(findings, validatePerInputGuard(input, path, inputWorkTypes, validWorkstations)...)
		}
	}
	return findings
}

func perInputGuardWorkTypes(inputs []interfaces.IOConfig) map[string]bool {
	workTypes := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		workTypes[input.WorkTypeName] = true
	}
	return workTypes
}

func validatePerInputGuard(input interfaces.IOConfig, path string, inputWorkTypes, validWorkstations map[string]bool) []Finding {
	switch input.Guard.Type {
	case interfaces.GuardTypeAllChildrenComplete, interfaces.GuardTypeAnyChildFailed:
		return validateParentAwareInputGuard(input, path, inputWorkTypes, validWorkstations)
	case interfaces.GuardTypeSameName:
		return validateSameNameInputGuard(input, path, inputWorkTypes)
	default:
		return []Finding{{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("unsupported guard type %q (per-input guards support: all_children_complete, any_child_failed, same_name)", input.Guard.Type),
			Rule:     "per-input-guard-type",
		}}
	}
}

func validateParentAwareInputGuard(input interfaces.IOConfig, path string, inputWorkTypes, validWorkstations map[string]bool) []Finding {
	findings := validatePeerInputReference(
		path,
		"parent_input",
		input.Guard.Type,
		input.Guard.ParentInput,
		input.WorkTypeName,
		inputWorkTypes,
		"per-input-guard-parent-input",
	)
	if input.Guard.SpawnedBy != "" && !validWorkstations[input.Guard.SpawnedBy] {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("spawned_by references non-existent workstation %q", input.Guard.SpawnedBy),
			Rule:     "per-input-guard-spawned-by",
		})
	}
	return findings
}

func validateSameNameInputGuard(input interfaces.IOConfig, path string, inputWorkTypes map[string]bool) []Finding {
	return validatePeerInputReference(
		path,
		"match_input",
		input.Guard.Type,
		input.Guard.MatchInput,
		input.WorkTypeName,
		inputWorkTypes,
		"per-input-guard-match-input",
	)
}

func validatePeerInputReference(path, fieldName string, guardType interfaces.GuardType, reference, workTypeName string, inputWorkTypes map[string]bool, fieldRule string) []Finding {
	if reference == "" {
		return []Finding{{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("guard of type %q requires %q", guardType, fieldName),
			Rule:     fieldRule,
		}}
	}

	var findings []Finding
	if !inputWorkTypes[reference] {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("%s %q does not match any input work type", fieldName, reference),
			Rule:     fieldRule,
		})
	}
	if reference == workTypeName {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     path,
			Message:  fmt.Sprintf("%s %q cannot reference its own input", fieldName, reference),
			Rule:     "per-input-guard-self-ref",
		})
	}
	return findings
}

// --- Rule: resource usage validation ---

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
	if cfg == nil || cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) == 0 {
		return nil
	}

	var findings []Finding
	for i, file := range cfg.ResourceManifest.BundledFiles {
		basePath := fmt.Sprintf("resourceManifest.bundledFiles[%d]", i)
		findings = append(findings, validateBundledFileType(basePath, file)...)
		findings = append(findings, validateBundledFileTarget(basePath, file)...)
		findings = append(findings, validateBundledFileContent(basePath, file)...)
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
	if strings.TrimSpace(file.Content.Inline) == "" {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Path:     basePath + ".content.inline",
			Message:  "missing required 'inline' field",
			Rule:     "bundled-file-content-inline",
		})
	}
	return findings
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
	case interfaces.BundledFileTypeScript, interfaces.BundledFileTypeDoc, interfaces.BundledFileTypeRootHelper:
		return true
	default:
		return false
	}
}

func bundledFileRootForType(fileType string) string {
	switch fileType {
	case interfaces.BundledFileTypeScript:
		return portableBundledScriptRoot
	case interfaces.BundledFileTypeDoc:
		return portableBundledDocRoot
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

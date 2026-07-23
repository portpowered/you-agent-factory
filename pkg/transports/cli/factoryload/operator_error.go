// Package factoryload renders Factory Definition load failures for CLI users.
package factoryload

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const defaultCLIBinaryName = "you"

// OperatorError carries CLI diagnostics for a blocking Factory Definition
// validation failure at a resolved Factory path.
type OperatorError struct {
	FactoryPath string
	Err         error
}

func (e *OperatorError) Error() string {
	if e == nil {
		return ""
	}
	return FormatOperatorDiagnostic(e.FactoryPath, e.Err)
}

func (e *OperatorError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *OperatorError) Is(target error) bool {
	return e != nil && errors.Is(e.Err, target)
}

func AsOperatorError(err error) (*OperatorError, bool) {
	var operatorErr *OperatorError
	if !errors.As(err, &operatorErr) {
		return nil, false
	}
	return operatorErr, true
}

func ConfigValidateRecoveryCommand(factoryPath string) string {
	return ConfigValidateRecoveryCommandForCLI(defaultCLIBinaryName, factoryPath)
}

func ConfigValidateRecoveryCommandForCLI(cliName, factoryPath string) string {
	cliName = strings.TrimSpace(cliName)
	if cliName == "" {
		cliName = defaultCLIBinaryName
	}
	path := strings.TrimSpace(factoryPath)
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	if path == "" {
		return cliName + " factory config validate"
	}
	return cliName + " factory config validate " + quoteFactoryPath(path)
}

func FormatOperatorDiagnostic(factoryPath string, err error) string {
	var builder strings.Builder
	if context := blockingErrorContext(err); context != "" {
		builder.WriteString(context)
		builder.WriteString("\n")
	}
	builder.WriteString(factorydefinitions.ErrInvalidNamedFactory.Error())
	builder.WriteString(": factory topology contains invalid graph references")
	if findings := blockingFindings(err); len(findings) > 0 {
		builder.WriteString("\nBlocking findings:")
		for _, finding := range findings {
			builder.WriteString("\n")
			builder.WriteString(formatFinding(finding))
		}
	}
	builder.WriteString("\nRecovery:\n  ")
	builder.WriteString(ConfigValidateRecoveryCommand(factoryPath))
	return builder.String()
}

func blockingErrorContext(err error) string {
	loadErr, ok := factorydefinitions.AsBlockingFactoryLoadError(err)
	if !ok || err == loadErr {
		return ""
	}
	return strings.TrimSuffix(err.Error(), ": "+loadErr.Error())
}

type finding struct {
	rule    string
	path    string
	message string
}

func blockingFindings(err error) []finding {
	loadErr, ok := factorydefinitions.AsBlockingFactoryLoadError(err)
	if !ok {
		return nil
	}
	findings := make([]finding, 0, len(loadErr.Targets))
	for _, target := range loadErr.Targets {
		path := target.Path
		if path == "" {
			path = target.Subject.ID
		}
		findings = append(findings, finding{
			rule:    target.Code,
			path:    path,
			message: target.Message,
		})
	}
	return findings
}

func formatFinding(finding finding) string {
	rule := strings.TrimSpace(finding.rule)
	path := strings.TrimSpace(finding.path)
	message := strings.TrimSpace(finding.message)
	switch {
	case rule != "" && path != "":
		return fmt.Sprintf("- [%s] %s: %s", rule, path, message)
	case rule != "":
		return fmt.Sprintf("- [%s] %s", rule, message)
	case path != "":
		return fmt.Sprintf("- %s: %s", path, message)
	default:
		return fmt.Sprintf("- %s", message)
	}
}

func WrapOperatorError(factoryPath string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsOperatorError(err); ok {
		return err
	}
	if _, ok := factorydefinitions.AsBlockingFactoryLoadError(err); !ok {
		return err
	}
	return &OperatorError{
		FactoryPath: strings.TrimSpace(factoryPath),
		Err:         err,
	}
}

func MaybeFormatOperatorError(err error, factoryPath string) error {
	return WrapOperatorError(factoryPath, err)
}

func MaybeFormatOperatorErrorForNamedFactory(
	err error,
	candidates factorydefinitions.NamedFactoryCandidatePaths,
) error {
	if _, ok := AsOperatorError(err); ok {
		return err
	}
	if projectPath := strings.TrimSpace(candidates.Project); projectPath != "" {
		if wrapped := WrapOperatorError(projectPath, err); wrapped != err {
			return wrapped
		}
	}
	if globalPath := strings.TrimSpace(candidates.Global); globalPath != "" {
		return WrapOperatorError(globalPath, err)
	}
	return err
}

func quoteFactoryPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if !strings.ContainsAny(path, " \t\n\"'\\") {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

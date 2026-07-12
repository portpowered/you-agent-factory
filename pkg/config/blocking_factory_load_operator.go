package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const defaultCLIBinaryName = "you"

// BlockingFactoryLoadOperatorError carries operator-facing diagnostics for a
// blocking factory-load validation failure at a resolved factory path.
type BlockingFactoryLoadOperatorError struct {
	FactoryPath string
	Err         error
}

func (e *BlockingFactoryLoadOperatorError) Error() string {
	if e == nil {
		return ""
	}
	return FormatBlockingFactoryLoadOperatorDiagnostic(e.FactoryPath, BlockingFactoryLoadFindings(e.Err))
}

func (e *BlockingFactoryLoadOperatorError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *BlockingFactoryLoadOperatorError) Is(target error) bool {
	if e == nil {
		return false
	}
	return errors.Is(e.Err, target)
}

// AsBlockingFactoryLoadOperatorError returns operator diagnostics when err wraps
// a BlockingFactoryLoadOperatorError.
func AsBlockingFactoryLoadOperatorError(err error) (*BlockingFactoryLoadOperatorError, bool) {
	var operatorErr *BlockingFactoryLoadOperatorError
	if !errors.As(err, &operatorErr) {
		return nil, false
	}
	return operatorErr, true
}

// FactoryConfigValidateRecoveryCommand returns the single recovery command
// operators should run after a materialization or upgrade validation failure.
func FactoryConfigValidateRecoveryCommand(factoryPath string) string {
	return FactoryConfigValidateRecoveryCommandForCLI(defaultCLIBinaryName, factoryPath)
}

// FactoryConfigValidateRecoveryCommandForCLI returns the recovery command using
// the provided CLI binary name.
func FactoryConfigValidateRecoveryCommandForCLI(cliName, factoryPath string) string {
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
	return cliName + " factory config validate " + quoteFactoryPathForCLI(path)
}

// FormatBlockingFactoryLoadOperatorDiagnostic renders blocking findings and the
// single validate recovery command for operator-visible CLI output.
func FormatBlockingFactoryLoadOperatorDiagnostic(factoryPath string, findings []Finding) string {
	var builder strings.Builder
	builder.WriteString(ErrInvalidNamedFactory.Error())
	builder.WriteString(": factory topology contains invalid graph references")
	if len(findings) > 0 {
		builder.WriteString("\nBlocking findings:")
		for _, finding := range findings {
			builder.WriteString("\n")
			builder.WriteString(FormatBlockingFactoryLoadFinding(finding))
		}
	}
	builder.WriteString("\nRecovery:\n  ")
	builder.WriteString(FactoryConfigValidateRecoveryCommand(factoryPath))
	return builder.String()
}

// FormatBlockingFactoryLoadFinding renders one blocking factory-load finding.
func FormatBlockingFactoryLoadFinding(finding Finding) string {
	rule := strings.TrimSpace(finding.Rule)
	path := strings.TrimSpace(finding.Path)
	message := strings.TrimSpace(finding.Message)
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

// WrapBlockingFactoryLoadOperatorError formats err for operators when it wraps
// structured blocking factory-load validation findings.
func WrapBlockingFactoryLoadOperatorError(factoryPath string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsBlockingFactoryLoadOperatorError(err); ok {
		return err
	}
	if _, ok := AsBlockingFactoryLoadError(err); !ok {
		return err
	}
	return &BlockingFactoryLoadOperatorError{
		FactoryPath: strings.TrimSpace(factoryPath),
		Err:         err,
	}
}

// MaybeFormatBlockingFactoryLoadOperatorError wraps err with operator diagnostics
// when it carries structured blocking findings and is not already wrapped.
func MaybeFormatBlockingFactoryLoadOperatorError(err error, factoryPath string) error {
	return WrapBlockingFactoryLoadOperatorError(factoryPath, err)
}

// MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory applies operator
// diagnostics for named-factory resolution failures when the error chain is not
// already wrapped but carries structured blocking findings.
func MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(
	err error,
	projectRoot, globalRoot, name string,
) error {
	if _, ok := AsBlockingFactoryLoadOperatorError(err); ok {
		return err
	}
	if projectPath, projectErr := NamedFactoryDirPath(projectRoot, name); projectErr == nil {
		if wrapped := WrapBlockingFactoryLoadOperatorError(projectPath, err); wrapped != err {
			return wrapped
		}
	}
	if globalPath, globalErr := NamedFactoryDirPath(globalRoot, name); globalErr == nil {
		return WrapBlockingFactoryLoadOperatorError(globalPath, err)
	}
	return err
}

func quoteFactoryPathForCLI(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if !strings.ContainsAny(path, " \t\n\"'\\") {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// NamedFactoryDirPath returns the on-disk directory for a persisted named factory.
func NamedFactoryDirPath(rootDir, name string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	segment, err := NamedFactoryNameToLayoutSegment(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, segment), nil
}

package blockingload

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/factoryerrors"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

const defaultCLIBinaryName = "you"

// BlockingFactoryLoadError reports blocking factory-load validation failures with
// structured canonical targets preserved for callers.
type BlockingFactoryLoadError struct {
	Targets []factoryvalidation.Target
}

func (e *BlockingFactoryLoadError) Error() string {
	if e == nil {
		return factoryerrors.ErrInvalidNamedFactory.Error()
	}
	if len(e.Targets) == 0 {
		return fmt.Sprintf("%v: factory topology contains invalid graph references", factoryerrors.ErrInvalidNamedFactory)
	}
	return fmt.Sprintf(
		"%v: factory topology contains invalid graph references (%d blocking validation targets)",
		factoryerrors.ErrInvalidNamedFactory,
		len(e.Targets),
	)
}

func (e *BlockingFactoryLoadError) Is(target error) bool {
	return target == factoryerrors.ErrInvalidNamedFactory
}

// NewBlockingFactoryLoadError wraps non-empty blocking validation targets.
func NewBlockingFactoryLoadError(result factoryvalidation.Result) error {
	if len(result.Targets) == 0 {
		return nil
	}
	return &BlockingFactoryLoadError{
		Targets: append([]factoryvalidation.Target(nil), result.Targets...),
	}
}

// IsInvalidNamedFactory reports whether err wraps ErrInvalidNamedFactory.
func IsInvalidNamedFactory(err error) bool {
	return errors.Is(err, factoryerrors.ErrInvalidNamedFactory)
}

// AsBlockingFactoryLoadError returns structured blocking findings when err wraps
// a BlockingFactoryLoadError from materialization, upgrade, or factory load.
func AsBlockingFactoryLoadError(err error) (*BlockingFactoryLoadError, bool) {
	var loadErr *BlockingFactoryLoadError
	if !errors.As(err, &loadErr) {
		return nil, false
	}
	return loadErr, true
}

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
	return FormatBlockingFactoryLoadOperatorDiagnostic(e.FactoryPath, e.Err)
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
func FormatBlockingFactoryLoadOperatorDiagnostic(factoryPath string, err error) string {
	var builder strings.Builder
	builder.WriteString(factoryerrors.ErrInvalidNamedFactory.Error())
	builder.WriteString(": factory topology contains invalid graph references")
	if findings := blockingFactoryLoadFindings(err); len(findings) > 0 {
		builder.WriteString("\nBlocking findings:")
		for _, finding := range findings {
			builder.WriteString("\n")
			builder.WriteString(formatBlockingFactoryLoadFinding(finding))
		}
	}
	builder.WriteString("\nRecovery:\n  ")
	builder.WriteString(FactoryConfigValidateRecoveryCommand(factoryPath))
	return builder.String()
}

type blockingFinding struct {
	rule    string
	path    string
	message string
}

func blockingFactoryLoadFindings(err error) []blockingFinding {
	loadErr, ok := AsBlockingFactoryLoadError(err)
	if !ok {
		return nil
	}
	findings := make([]blockingFinding, 0, len(loadErr.Targets))
	for _, target := range loadErr.Targets {
		path := target.Path
		if path == "" {
			path = target.Subject.ID
		}
		findings = append(findings, blockingFinding{
			rule:    target.Code,
			path:    path,
			message: target.Message,
		})
	}
	return findings
}

func formatBlockingFactoryLoadFinding(finding blockingFinding) string {
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
	if projectPath, projectErr := namedFactoryDirPath(projectRoot, name); projectErr == nil {
		if wrapped := WrapBlockingFactoryLoadOperatorError(projectPath, err); wrapped != err {
			return wrapped
		}
	}
	if globalPath, globalErr := namedFactoryDirPath(globalRoot, name); globalErr == nil {
		return WrapBlockingFactoryLoadOperatorError(globalPath, err)
	}
	return err
}

func namedFactoryDirPath(rootDir, name string) (string, error) {
	return namedfactorypath.MapDir(strings.TrimSpace(rootDir), name)
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

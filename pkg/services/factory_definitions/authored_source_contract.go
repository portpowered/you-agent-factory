package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type AuthoredFactoryFormat string

const (
	AuthoredFactoryFormatJSON AuthoredFactoryFormat = "JSON"
	AuthoredFactoryFormatYAML AuthoredFactoryFormat = "YAML"
)

const SupportedAuthoredFactoryExtensions = ".json, .yaml, and .yml"
const SupportedAuthoredFactoryRootFiles = "factory.json, factory.yaml, and factory.yml"

// AuthoredFactorySource retains the selected source identity while carrying
// JSON-compatible bytes into representation mapping and validation.
type AuthoredFactorySource struct {
	Path   string
	Format AuthoredFactoryFormat
	Data   []byte
}

// AuthoredFactorySourceLoader resolves an authored Factory Definition path and
// returns its selected source identity and JSON-compatible representation.
type AuthoredFactorySourceLoader func(path string) (AuthoredFactorySource, error)

// ValidatedAuthoredFactoryDefinitionLoader selects, loads, and validates one
// authored Factory Definition. It is intentionally narrower than Service so a
// Runtime-facing caller receives only the detached definition facts it needs.
type ValidatedAuthoredFactoryDefinitionLoader interface {
	LoadValidatedAuthoredFactoryDefinition(
		context.Context,
		LoadValidatedAuthoredFactoryDefinitionRequest,
	) (LoadValidatedAuthoredFactoryDefinitionResult, error)
}

// LoadValidatedAuthoredFactoryDefinitionRequest carries only caller-selected
// authored source context. It does not contain filesystem, loader, validator,
// logger, Runtime, or Factory Session collaborators.
type LoadValidatedAuthoredFactoryDefinitionRequest struct {
	Directory        string
	SourcePath       string
	ExecutionBaseDir string
}

// AuthoredFactoryDefinitionIdentity records the selected authored source
// without retaining its payload bytes.
type AuthoredFactoryDefinitionIdentity struct {
	Path   string
	Format AuthoredFactoryFormat
}

// LoadValidatedAuthoredFactoryDefinitionResult contains detached effective
// definition facts. Callers cannot mutate the loader-owned source through this
// result.
type LoadValidatedAuthoredFactoryDefinitionResult struct {
	Source                  AuthoredFactoryDefinitionIdentity
	Definition              *FactoryConfig
	FactoryDir              string
	RuntimeBaseDir          string
	BundledFileReplacements []PortableBundledFileReplacement
	Validation              ValidationResult
}

// AuthoredFactoryDefinitionLoadFailureKind identifies the failing owned
// loading phase without exposing authored payload data.
type AuthoredFactoryDefinitionLoadFailureKind string

const (
	AuthoredFactoryDefinitionLoadFailureMissing    AuthoredFactoryDefinitionLoadFailureKind = "missing"
	AuthoredFactoryDefinitionLoadFailureMalformed  AuthoredFactoryDefinitionLoadFailureKind = "malformed"
	AuthoredFactoryDefinitionLoadFailureUnresolved AuthoredFactoryDefinitionLoadFailureKind = "unresolved"
	AuthoredFactoryDefinitionLoadFailureValidation AuthoredFactoryDefinitionLoadFailureKind = "validation"
	AuthoredFactoryDefinitionLoadFailureDependency AuthoredFactoryDefinitionLoadFailureKind = "dependency"
)

var (
	ErrAuthoredFactoryDefinitionMissing    = errors.New("authored factory definition is missing")
	ErrAuthoredFactoryDefinitionMalformed  = errors.New("authored factory definition is malformed")
	ErrAuthoredFactoryDefinitionUnresolved = errors.New("authored factory definition is unresolved")
	ErrAuthoredFactoryDefinitionValidation = errors.New("authored factory definition validation failed")
	ErrAuthoredFactoryDefinitionDependency = errors.New("authored factory definition dependency failed")
)

// AuthoredFactoryDefinitionLoadFailure is the typed, payload-safe failure
// returned by ValidatedAuthoredFactoryDefinitionLoader. Cause remains
// inspectable with errors.Is/errors.As for callers that own the dependency.
type AuthoredFactoryDefinitionLoadFailure struct {
	Kind       AuthoredFactoryDefinitionLoadFailureKind
	Source     AuthoredFactoryDefinitionIdentity
	Validation ValidationResult
	Cause      error
}

func (e *AuthoredFactoryDefinitionLoadFailure) Error() string {
	if e == nil {
		return "authored Factory Definition load failed"
	}
	message := "authored Factory Definition load failed"
	if e.Kind != "" {
		message += ": " + string(e.Kind)
	}
	if source := strings.TrimSpace(e.Source.Path); source != "" {
		message += fmt.Sprintf(" (%s)", source)
	}
	return message
}

func (e *AuthoredFactoryDefinitionLoadFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Cause != nil {
		return e.Cause
	}
	switch e.Kind {
	case AuthoredFactoryDefinitionLoadFailureMissing:
		return ErrAuthoredFactoryDefinitionMissing
	case AuthoredFactoryDefinitionLoadFailureMalformed:
		return ErrAuthoredFactoryDefinitionMalformed
	case AuthoredFactoryDefinitionLoadFailureUnresolved:
		return ErrAuthoredFactoryDefinitionUnresolved
	case AuthoredFactoryDefinitionLoadFailureValidation:
		return ErrAuthoredFactoryDefinitionValidation
	default:
		return ErrAuthoredFactoryDefinitionDependency
	}
}

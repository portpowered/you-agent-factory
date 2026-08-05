package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
)

type PackagedFactoryInstallOutcome string

const (
	PackagedFactoryInstallCreated  PackagedFactoryInstallOutcome = "created"
	PackagedFactoryInstallSkipped  PackagedFactoryInstallOutcome = "skipped"
	PackagedFactoryInstallReplaced PackagedFactoryInstallOutcome = "replaced"
)

// PackagedFactoryInstallParams carries one Definitions-owned packaged
// installation request after catalog selection has returned a detached
// definition.
type PackagedFactoryInstallParams struct {
	NamedFactoriesRoot string
	Definition         PackagedDefinition
	Format             PackagedFactoryFormat
	Replace            bool
}

type PackagedFactoryInstallResult struct {
	Name       string
	FactoryDir string
	Outcome    PackagedFactoryInstallOutcome
	Format     PackagedFactoryFormat
}

// PackagedFactoryInstaller owns validation and persistence of the packaged
// Factory catalog into one named-Factory root.
type PackagedFactoryInstaller interface {
	EnsurePackagedFactories(
		context.Context,
		string,
		[]PackagedDefinition,
	) ([]PackagedFactoryInstallResult, error)
}

// Packaging is the focused Factory Definitions capability for published
// package discovery and installation. It intentionally exposes only
// Definitions request, result, value, and typed-error contracts.
type Packaging interface {
	ListBuiltInPackagedFactories(
		context.Context,
		ListBuiltInPackagedFactoriesRequest,
	) (ListBuiltInPackagedFactoriesResult, error)
	ResolveBuiltInPackagedFactory(
		context.Context,
		ResolveBuiltInPackagedFactoryRequest,
	) (ResolveBuiltInPackagedFactoryResult, error)
	InstallPackagedFactory(
		context.Context,
		InstallPackagedFactoryRequest,
	) (InstallPackagedFactoryResult, error)
}

// PackagedFactoryCatalog is the direct, read-only Definitions port consumed by
// Packaging. It is retained separately from the compatibility callback bundle
// so new consumers never receive raw callbacks.
type PackagedFactoryCatalog interface {
	ListBuiltInPackagedFactories(
		context.Context,
		ListBuiltInPackagedFactoriesRequest,
	) (ListBuiltInPackagedFactoriesResult, error)
	ResolveBuiltInPackagedFactory(
		context.Context,
		ResolveBuiltInPackagedFactoryRequest,
	) (ResolveBuiltInPackagedFactoryResult, error)
}

// PackagedFactoryInstallation is the direct Definitions port used by
// Packaging after package selection. It is not a customer-facing operation;
// callers of Packaging provide only install intent.
type PackagedFactoryInstallation interface {
	InstallPackagedFactory(
		context.Context,
		PackagedFactoryInstallParams,
	) (PackagedFactoryInstallResult, error)
}

// PackagedFactoryErrorClassification makes package failures stable and
// inspectable without exposing source bytes, artifact locators, or filesystem
// implementation details.
type PackagedFactoryErrorClassification string

const (
	PackagedFactoryErrorMissing     PackagedFactoryErrorClassification = "missing"
	PackagedFactoryErrorMalformed   PackagedFactoryErrorClassification = "malformed"
	PackagedFactoryErrorUnsupported PackagedFactoryErrorClassification = "unsupported"
	PackagedFactoryErrorIntegrity   PackagedFactoryErrorClassification = "integrity"
)

var (
	ErrPackagedFactoryMissing           = errors.New("packaged factory is missing")
	ErrMalformedPackagedFactory         = errors.New("malformed packaged factory")
	ErrUnsupportedPackagedFactoryFormat = errors.New("unsupported packaged Factory format")
	ErrPackagedFactoryIntegrity         = errors.New("packaged factory integrity check failed")
)

// PackagedFactoryInputError carries safe package failure facts. Name and
// Format are caller-selected public values; Artifact is a Factory-relative
// target path and never a host path or package locator.
type PackagedFactoryInputError struct {
	Classification PackagedFactoryErrorClassification
	Name           string
	Format         PackagedFactoryFormat
	Artifact       string
	Cause          error
}

func (e *PackagedFactoryInputError) Error() string {
	if e == nil {
		return "invalid packaged factory input"
	}
	message := packagedFactoryErrorSentinel(e.Classification).Error()
	if e.Classification == PackagedFactoryErrorUnsupported && e.Format != "" {
		return fmt.Sprintf("%s %q", message, e.Format)
	}
	if e.Name != "" {
		message = fmt.Sprintf("%s %q", message, e.Name)
	}
	if e.Format != "" {
		message = fmt.Sprintf("%s (%s)", message, e.Format)
	}
	if e.Artifact != "" {
		message = fmt.Sprintf("%s artifact %q", message, e.Artifact)
	}
	return message
}

func (e *PackagedFactoryInputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *PackagedFactoryInputError) Is(target error) bool {
	if e == nil {
		return false
	}
	if target == packagedFactoryErrorSentinel(e.Classification) {
		return true
	}
	return e.Classification == PackagedFactoryErrorMissing &&
		target == ErrUnknownPackagedFactoryIdentity
}

// NewPackagedFactoryInputError produces one stable typed package failure.
// Cause remains available through standard Go error matching while Error keeps
// the diagnostic free of package bytes and host filesystem paths.
func NewPackagedFactoryInputError(
	classification PackagedFactoryErrorClassification,
	name string,
	format PackagedFactoryFormat,
	artifact string,
	cause error,
) error {
	return &PackagedFactoryInputError{
		Classification: classification,
		Name:           name,
		Format:         format,
		Artifact:       artifact,
		Cause:          cause,
	}
}

func packagedFactoryErrorSentinel(
	classification PackagedFactoryErrorClassification,
) error {
	switch classification {
	case PackagedFactoryErrorMissing:
		return ErrPackagedFactoryMissing
	case PackagedFactoryErrorMalformed:
		return ErrMalformedPackagedFactory
	case PackagedFactoryErrorUnsupported:
		return ErrUnsupportedPackagedFactoryFormat
	case PackagedFactoryErrorIntegrity:
		return ErrPackagedFactoryIntegrity
	default:
		return ErrMalformedPackagedFactory
	}
}

package resolvedinput

import "fmt"

// Source identifies the channel that supplied a resolved CLI input value.
type Source string

const (
	SourceCLIFlag                 Source = "cli-flag"
	SourcePositionalArgument      Source = "positional-argument"
	SourceEnvironment             Source = "environment"
	SourceOperatorConfig          Source = "operator-config"
	SourceStdin                   Source = "stdin"
	SourceManifestDefault         Source = "manifest-default"
	SourceFactorySignatureDefault Source = "factory-signature-default"
)

// State describes how the winning value entered the resolved snapshot.
type State struct {
	Provenance Source
	Changed    bool
	Default    bool
}

// ResolutionFailure classifies invalid schema or candidate data.
type ResolutionFailure string

const (
	ResolutionFailureInvalidDefinition ResolutionFailure = "invalid-definition"
	ResolutionFailureInvalidPrecedence ResolutionFailure = "invalid-precedence"
	ResolutionFailureUndeclaredInput   ResolutionFailure = "undeclared-input"
	ResolutionFailureUndeclaredSource  ResolutionFailure = "undeclared-source"
	ResolutionFailureDuplicateSource   ResolutionFailure = "duplicate-source"
	ResolutionFailureValueKind         ResolutionFailure = "value-kind"
)

// ResolutionError is a structured diagnostic for invalid resolver input.
type ResolutionError struct {
	Failure ResolutionFailure
	InputID string
	Source  Source
	Detail  string
}

func (e *ResolutionError) Error() string {
	context := "resolved CLI input"
	if e.InputID != "" {
		context += fmt.Sprintf(" %q", e.InputID)
	}
	if e.Source != "" {
		context += fmt.Sprintf(" source %q", e.Source)
	}
	return fmt.Sprintf("%s: %s (%s)", context, e.Detail, e.Failure)
}

func newResolutionError(failure ResolutionFailure, inputID string, source Source, detail string) error {
	return &ResolutionError{Failure: failure, InputID: inputID, Source: source, Detail: detail}
}

func (s Source) valid() bool {
	switch s {
	case SourceCLIFlag, SourcePositionalArgument, SourceEnvironment, SourceOperatorConfig,
		SourceStdin, SourceManifestDefault, SourceFactorySignatureDefault:
		return true
	default:
		return false
	}
}

func stateFor(source Source) State {
	isDefault := source == SourceManifestDefault || source == SourceFactorySignatureDefault
	return State{Provenance: source, Changed: !isDefault, Default: isDefault}
}

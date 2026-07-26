// Package completionprojection maps an already-resolved selected-Factory
// invocation schema into detached facts for shell-specific completion adapters.
package completionprojection

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	TargetFlags  = "flags"
	TargetValues = "values"

	CandidateKindFlag  = "flag"
	CandidateKindValue = "value"

	DirectiveKindFilesystemDelegation = "filesystem-delegation"

	documentedBooleanTrue  = "true"
	documentedBooleanFalse = "false"
)

// Context identifies the kind of completion requested by a future shell
// adapter. EnteredPrefix is carried only as input to matching policy and must
// never be copied into returned facts or errors.
type Context struct {
	Target             string
	ParameterBindingID string
	EnteredPrefix      string
}

// Candidate is one ordered, detached completion choice.
type Candidate struct {
	Kind               string
	ParameterBindingID string
	Value              string
	Description        string
}

// Directive describes work that a future shell adapter should perform.
type Directive struct {
	Kind               string
	ParameterBindingID string
}

// Projection contains all completion facts for one request.
type Projection struct {
	Candidates []Candidate
	Directives []Directive
}

// Project deterministically maps an effective selected-Factory schema to
// completion facts. It consumes detached values only and performs no selection,
// runtime, shell, filesystem, or process-global work.
func Project(
	ctx context.Context,
	schema climanifest.EffectiveInputSchema,
	completionContext Context,
) (Projection, error) {
	if err := cancellationError(ctx); err != nil {
		return Projection{}, err
	}
	if err := validateSchema(ctx, schema); err != nil {
		return Projection{}, err
	}
	if schema.FactoryInputMode != climanifest.EffectiveFactoryInputModeSignature {
		return Projection{}, nil
	}

	switch completionContext.Target {
	case TargetFlags:
		return projectFlags(ctx, schema)
	case TargetValues:
		return projectValues(ctx, schema, completionContext.ParameterBindingID)
	default:
		return Projection{}, nil
	}
}

func projectFlags(ctx context.Context, schema climanifest.EffectiveInputSchema) (Projection, error) {
	var candidates []Candidate
	for _, parameter := range schema.FactoryParameters {
		if err := cancellationError(ctx); err != nil {
			return Projection{}, err
		}
		if !hasNamedBinding(parameter.Bindings) {
			continue
		}
		candidates = append(candidates, flagCandidates(parameter)...)
	}
	return Projection{Candidates: candidates}, nil
}

func projectValues(
	ctx context.Context,
	schema climanifest.EffectiveInputSchema,
	bindingID string,
) (Projection, error) {
	for _, parameter := range schema.FactoryParameters {
		if err := cancellationError(ctx); err != nil {
			return Projection{}, err
		}
		if parameter.BindingID != bindingID {
			continue
		}
		return parameterValues(ctx, parameter)
	}
	return Projection{}, nil
}

func parameterValues(
	ctx context.Context,
	parameter climanifest.EffectiveFactoryParameter,
) (Projection, error) {
	if parameter.Sensitive {
		return Projection{}, nil
	}

	values := parameter.Choices
	if len(values) == 0 && parameter.TypeHint == work.InvocationParameterTypeHintBooleanString {
		values = []string{documentedBooleanTrue, documentedBooleanFalse}
	}

	candidates, err := valueCandidates(ctx, parameter, values)
	if err != nil {
		return Projection{}, err
	}
	projection := Projection{Candidates: candidates}
	if parameter.TypeHint == work.InvocationParameterTypeHintFilePath {
		projection.Directives = []Directive{{
			Kind:               DirectiveKindFilesystemDelegation,
			ParameterBindingID: parameter.BindingID,
		}}
	}
	return projection, nil
}

func hasNamedBinding(bindings []work.InvocationParameterBindingConfig) bool {
	for _, binding := range bindings {
		if binding.Kind == work.InvocationParameterBindingKindNamed {
			return true
		}
	}
	return false
}

func flagCandidates(parameter climanifest.EffectiveFactoryParameter) []Candidate {
	candidates := make([]Candidate, 0, len(parameter.Aliases)+1)
	candidates = append(candidates, Candidate{
		Kind:               CandidateKindFlag,
		ParameterBindingID: parameter.BindingID,
		Value:              "--" + parameter.PreferredExternalName,
		Description:        parameter.Description,
	})
	for _, alias := range parameter.Aliases {
		candidates = append(candidates, Candidate{
			Kind:               CandidateKindFlag,
			ParameterBindingID: parameter.BindingID,
			Value:              "--" + alias,
			Description:        parameter.Description,
		})
	}
	return candidates
}

func valueCandidates(
	ctx context.Context,
	parameter climanifest.EffectiveFactoryParameter,
	values []string,
) ([]Candidate, error) {
	if len(values) == 0 {
		return nil, nil
	}
	description := valueDescription(parameter)
	candidates := make([]Candidate, 0, len(values))
	for _, value := range values {
		if err := cancellationError(ctx); err != nil {
			return nil, err
		}
		candidates = append(candidates, Candidate{
			Kind:               CandidateKindValue,
			ParameterBindingID: parameter.BindingID,
			Value:              value,
			Description:        description,
		})
	}
	return candidates, nil
}

func valueDescription(parameter climanifest.EffectiveFactoryParameter) string {
	description := strings.TrimSpace(parameter.Description)
	if parameter.Sensitive {
		return description
	}

	defaultDescription := ""
	switch {
	case parameter.DefaultValue != nil:
		defaultDescription = "Default: " + *parameter.DefaultValue + "."
	case len(parameter.DefaultValues) > 0:
		defaultDescription = "Defaults: " + strings.Join(parameter.DefaultValues, ", ") + "."
	}
	if description == "" {
		return defaultDescription
	}
	if defaultDescription == "" {
		return description
	}
	return description + " " + defaultDescription
}

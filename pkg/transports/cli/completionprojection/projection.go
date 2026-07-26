// Package completionprojection maps an already-resolved selected-Factory
// invocation schema into detached facts for shell-specific completion adapters.
package completionprojection

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const (
	TargetFlags  = "flags"
	TargetValues = "values"

	CandidateKindFlag = "flag"
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
	_ context.Context,
	schema climanifest.EffectiveInputSchema,
	completionContext Context,
) (Projection, error) {
	if completionContext.Target != TargetFlags {
		return Projection{}, nil
	}

	var candidates []Candidate
	for _, parameter := range schema.FactoryParameters {
		if !hasNamedBinding(parameter.Bindings) {
			continue
		}
		candidates = append(candidates, flagCandidates(parameter)...)
	}
	return Projection{Candidates: candidates}, nil
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

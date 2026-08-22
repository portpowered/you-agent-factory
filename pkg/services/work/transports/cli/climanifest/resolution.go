package climanifest

import (
	"fmt"
	"slices"
)

const (
	SourceCLI                     = "cli"
	SourceStdin                   = "stdin"
	SourceEnvironment             = "environment"
	SourceOperatorConfig          = "operator-config"
	SourceManifestDefault         = "manifest-default"
	SourceFactorySignatureDefault = "factory-signature-default"
)

// CanonicalPrecedence returns the single source-resolution policy for canonical CLI
// inputs. A higher tier replaces every lower tier. Within the winning tier,
// scalar observations use the last value and repeated observations append in
// observation order; distinct bindings targeting the same input are rejected.
func CanonicalPrecedence() Precedence {
	return Precedence{
		Order: []string{
			SourceCLI,
			SourceStdin,
			SourceEnvironment,
			SourceOperatorConfig,
			SourceManifestDefault,
			SourceFactorySignatureDefault,
		},
		WithinTier:       WithinTierRule{Scalar: "last", Repeated: "append"},
		AcrossTiers:      "replace",
		MultipleBindings: "reject",
	}
}

// IsCanonical reports whether a policy exactly matches the canonical source
// order and tie behavior.
func (p Precedence) IsCanonical() bool {
	want := CanonicalPrecedence()
	return slices.Equal(p.Order, want.Order) && p.WithinTier == want.WithinTier &&
		p.AcrossTiers == want.AcrossTiers && p.MultipleBindings == want.MultipleBindings
}

// ResolutionCandidate is one observed value and its provenance.
type ResolutionCandidate struct {
	Source    string
	BindingID string
	Value     InputValue
}

// ResolutionResult is the winning value together with its source provenance.
type ResolutionResult struct {
	Source    string
	BindingID string
	Value     InputValue
}

// ResolveInputValue applies a validated precedence contract without performing
// IO or mutating the supplied candidates.
func ResolveInputValue(precedence Precedence, acceptedSources []string, repeated bool, candidates []ResolutionCandidate) (ResolutionResult, bool, error) {
	if !precedence.IsCanonical() {
		return ResolutionResult{}, false, fmt.Errorf("precedence does not match the canonical CLI source policy")
	}
	accepted := make(map[string]bool, len(acceptedSources))
	for _, source := range acceptedSources {
		accepted[source] = true
	}
	bySource := make(map[string][]ResolutionCandidate)
	for _, candidate := range candidates {
		if !accepted[candidate.Source] {
			return ResolutionResult{}, false, fmt.Errorf("source %q is not accepted by the input", candidate.Source)
		}
		bySource[candidate.Source] = append(bySource[candidate.Source], candidate)
	}
	for _, source := range precedence.Order {
		tier := bySource[source]
		if len(tier) == 0 {
			continue
		}
		bindingID := tier[0].BindingID
		for _, candidate := range tier[1:] {
			if candidate.BindingID != bindingID {
				return ResolutionResult{}, false, fmt.Errorf("source %q has multiple bindings for one input", source)
			}
		}
		result := ResolutionResult{Source: source, BindingID: bindingID, Value: tier[len(tier)-1].Value}
		if repeated {
			values := make([]string, 0)
			for _, candidate := range tier {
				if candidate.Value.StringArray == nil {
					return ResolutionResult{}, false, fmt.Errorf("repeated source %q supplied a non-repeated value", source)
				}
				values = append(values, (*candidate.Value.StringArray)...)
			}
			result.Value = InputValue{StringArray: &values}
		}
		return result, true, nil
	}
	return ResolutionResult{}, false, nil
}

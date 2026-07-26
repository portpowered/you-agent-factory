package completionprojection

import (
	"context"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryNameParameterBindingID identifies the existing selected-Factory
// parameter without registering it with a shell or command framework.
const FactoryNameParameterBindingID = "you.run.flag.named"

// ProjectFactoryNames maps an already-resolved effective Factory catalog to
// detached selected-Factory value candidates. Catalog discovery owns
// precedence, shadowing, diagnostics, and ordering; this projection only
// preserves its valid entries and derives presentation metadata.
func ProjectFactoryNames(
	ctx context.Context,
	catalog factorydefinitions.ListEffectiveFactoriesResult,
) (Projection, error) {
	if err := cancellationError(ctx); err != nil {
		return Projection{}, err
	}

	candidates := make([]Candidate, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if err := cancellationError(ctx); err != nil {
			return Projection{}, err
		}
		candidates = append(candidates, Candidate{
			Kind:               CandidateKindValue,
			ParameterBindingID: FactoryNameParameterBindingID,
			Value:              entry.Name,
			Description:        factoryNameDescription(entry.Definition),
		})
	}
	if err := cancellationError(ctx); err != nil {
		return Projection{}, err
	}
	return Projection{Candidates: candidates}, nil
}

func factoryNameDescription(definition *factorydefinitions.FactoryConfig) string {
	if definition == nil || definition.Description == nil {
		return ""
	}
	return strings.TrimSpace(definition.Description.Value)
}

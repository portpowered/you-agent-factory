package engine

import (
	"errors"
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func isNonFatalPetriMutationPersistenceError(err error) bool {
	var marker interface {
		NonFatalPetriMutationPersistenceError()
	}
	return errors.As(err, &marker)
}

// applyMutations applies a batch of mutations to a marking atomically.
// It returns an error if any mutation references a non-existent token or place.
func applyMutations(marking *petri.Marking, places map[string]*petri.Place, mutations []interfaces.MarkingMutation, now time.Time) error {
	for i, m := range mutations {
		switch m.Type {
		case interfaces.MutationMove:
			if err := applyMove(marking, places, m, now); err != nil {
				return fmt.Errorf("mutation %d (MOVE): %w", i, err)
			}
		case interfaces.MutationCreate:
			if err := applyCreate(marking, places, m, now); err != nil {
				return fmt.Errorf("mutation %d (CREATE): %w", i, err)
			}
		case interfaces.MutationConsume:
			if err := applyConsume(marking, m); err != nil {
				return fmt.Errorf("mutation %d (CONSUME): %w", i, err)
			}
		default:
			return fmt.Errorf("mutation %d: unknown type %q", i, m.Type)
		}
	}
	return nil
}

func applyMove(marking *petri.Marking, places map[string]*petri.Place, m interfaces.MarkingMutation, now time.Time) error {
	token, ok := marking.Tokens[m.TokenID]
	if !ok {
		return fmt.Errorf("token %q not found", m.TokenID)
	}
	if _, ok := places[m.FromPlace]; !ok {
		return fmt.Errorf("from-place %q not found", m.FromPlace)
	}
	if _, ok := places[m.ToPlace]; !ok {
		return fmt.Errorf("to-place %q not found", m.ToPlace)
	}

	marking.RemoveToken(m.TokenID)
	token.PlaceID = m.ToPlace
	token.EnteredAt = now

	// Apply optional failure records (used by cascading failure subsystem).
	if len(m.FailureRecords) > 0 {
		token.History.FailureLog = append(token.History.FailureLog, m.FailureRecords...)
		token.History.LastError = m.FailureRecords[len(m.FailureRecords)-1].Error
	}

	marking.AddToken(token)
	return nil
}

func applyCreate(marking *petri.Marking, places map[string]*petri.Place, m interfaces.MarkingMutation, now time.Time) error {
	if _, ok := places[m.ToPlace]; !ok {
		return fmt.Errorf("to-place %q not found", m.ToPlace)
	}
	if m.NewToken == nil {
		return fmt.Errorf("CREATE mutation missing NewToken")
	}

	token := factorytoken.FromWorker(*m.NewToken)
	token.PlaceID = m.ToPlace
	if token.EnteredAt.IsZero() {
		token.EnteredAt = now
	}
	marking.AddToken(&token)
	return nil
}

func applyConsume(marking *petri.Marking, m interfaces.MarkingMutation) error {
	if _, ok := marking.Tokens[m.TokenID]; !ok {
		return fmt.Errorf("token %q not found", m.TokenID)
	}

	marking.RemoveToken(m.TokenID)
	return nil
}

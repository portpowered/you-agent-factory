// Package sessionobservations defines the scoped durable observations consumed
// by Factory Runtime opening. It contains no reusable behavior or session state.
package sessionobservations

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Observations belongs to one already-opened durable session. Opening owners
// receive it per call; only that session's callbacks may retain it.
type Observations interface {
	RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	PublishWorkerProgress(workers.ProgressFragment)
}

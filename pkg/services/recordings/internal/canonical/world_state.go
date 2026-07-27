package canonical

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// ReconstructWorldState reduces canonical facts through the private projection
// capability and returns a detached world-state view.
func ReconstructWorldState(
	projection recordings.ProjectionService,
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if request.SelectedTick < 0 {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	if err := ValidateProjectionEvents(request.Scope, request.After, request.Events); err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	events := make([]factorydefinitions.FactoryEvent, len(request.Events))
	for index, event := range request.Events {
		events[index] = FactoryEventFromCanonical(event)
	}
	state, err := projection.ReconstructFactoryWorldState(events, request.SelectedTick)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	through := recordings.CanonicalEventCursor{}
	if request.After != nil {
		through = *request.After
	}
	if len(request.Events) > 0 {
		through = request.Events[len(request.Events)-1].Cursor
	}
	return recordings.ReconstructWorldStateResult{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         request.Scope,
			Through:       through,
			SelectedTick:  request.SelectedTick,
			Payload:       string(payload),
		},
	}, nil
}

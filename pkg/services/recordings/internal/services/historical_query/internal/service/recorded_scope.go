package service

import (
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

// inferRecordedScope selects from one already-read artifact. The normal decoder
// subsequently checks every event against this scope, schema, order and integrity.
func inferRecordedScope(payload []byte, identity recordings.HistoricalRecordingIdentity) (recordings.HistoricalRecordingIdentity, error) {
	var session string
	if replayimpl.IsReplayV2Artifact(payload) {
		stream, err := replayimpl.ParseReplayV2(payload)
		if err != nil {
			return identity, historicalQueryError(recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err)
		}
		if len(stream.Events) > 0 && stream.Events[0].Context.SessionID != nil {
			session = *stream.Events[0].Context.SessionID
		}
	} else {
		var header struct {
			SchemaVersion string `json:"schemaVersion"`
			Summary       struct {
				Scope recordings.CanonicalEventScope `json:"scope"`
			} `json:"summary"`
			Events []struct {
				Context factorydefinitions.FactoryEventContext `json:"context"`
			} `json:"events"`
		}
		if err := json.Unmarshal(payload, &header); err != nil {
			return identity, historicalQueryError(recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err)
		}
		if header.SchemaVersion == string(recordings.PortableArtifactSchemaV1) {
			session = header.Summary.Scope.FactorySessionID
		} else if len(header.Events) > 0 && header.Events[0].Context.SessionID != nil {
			session = *header.Events[0].Context.SessionID
		}
	}
	if session == "" {
		return identity, historicalQueryError(recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", nil)
	}
	identity.Scope.FactorySessionID = session
	return identity, nil
}

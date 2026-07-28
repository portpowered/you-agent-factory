package replay

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/replay"
)

const CurrentSchemaVersion = replayimpl.CurrentSchemaVersion

var (
	ArtifactFromEventStream         = replayimpl.ArtifactFromEventStream
	ArtifactFromEventStreamFile     = replayimpl.ArtifactFromEventStreamFile
	FactoryMetadataWarnings         = replayimpl.FactoryMetadataWarnings
	Load                            = replayimpl.Load
	MarshalArtifact                 = replayimpl.MarshalArtifact
	NewArtifactClock                = replayimpl.NewArtifactClock
	NewCompletionDeliveryPlan       = replayimpl.NewCompletionDeliveryPlan
	NewEventLogArtifact             = replayimpl.NewEventLogArtifact
	NewRecorder                     = replayimpl.NewRecorder
	NewSideEffects                  = replayimpl.NewSideEffects
	NewSubmissionHook               = replayimpl.NewSubmissionHook
	NewWorkStateChangeHook          = replayimpl.NewWorkStateChangeHook
	Save                            = replayimpl.Save
	SaveArtifactFromEventStreamFile = replayimpl.SaveArtifactFromEventStreamFile
	Validate                        = replayimpl.Validate
)

type (
	CompletionDeliveryPlan   = replayimpl.CompletionDeliveryPlan
	DivergenceError          = replayimpl.DivergenceError
	DivergenceReport         = replayimpl.DivergenceReport
	EventStreamArtifactResult = replayimpl.EventStreamArtifactResult
	InspectAdjacentFactoryPath = replayimpl.InspectAdjacentFactoryPath
	MetadataMismatchWarning  = replayimpl.MetadataMismatchWarning
	OpenEventStreamFile        = replayimpl.OpenEventStreamFile
	Recorder                   = replayimpl.Recorder
	SideEffects                = replayimpl.SideEffects
	SubmissionHook             = replayimpl.SubmissionHook
	WorkStateChangeHook        = replayimpl.WorkStateChangeHook
)

// Preserve factory_definitions import for vocabulary boundary tests that assert
// the shim still depends on the published Factory contract.
var _ = interfaces.FactoryEventTypeRunRequest

// Preserve platform replay import for storage boundary tests that assert the
// shim still depends on policy-free artifact filesystem mechanics.
var _ platformreplay.Storage

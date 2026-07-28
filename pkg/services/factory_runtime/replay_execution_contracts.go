package factory

import "github.com/portpowered/infinite-you/pkg/services/recordings"

// Runtime-owned replay execution handoff vocabulary published at the Factory
// Runtime root. Recordings replay-execution edges produce these neutral shapes;
// Runtime assembly consumes them through this root contract rather than nested
// Runtime implementation packages.
type (
	ReplayHook             = recordings.ReplayHook
	ReplaySnapshot         = recordings.ReplaySnapshot
	ReplayHookResult       = recordings.ReplayHookResult
	ReplayWorkToken        = recordings.ReplayWorkToken
	ReplayExecutionFactory = recordings.ReplayExecutionFactory
)

const (
	// ReplaySubmissionHookName identifies the Recordings submission replay hook
	// adapted into Runtime root SubmissionHook contracts.
	ReplaySubmissionHookName = "replay-artifact-submissions"
	// ReplayWorkStateChangeHookName identifies the Recordings work-state replay
	// hook adapted into Runtime root SubmissionHook contracts.
	ReplayWorkStateChangeHookName = "replay-artifact-work-state-changes"
)

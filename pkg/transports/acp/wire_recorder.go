package acp

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
)

// ErrWireRecordingDisabled represents an explicitly disabled recording role,
// rather than a failure to acquire an active transcript.
var ErrWireRecordingDisabled = errors.New("ACP wire recording is disabled")

// WireTranscript keeps the transport-facing name for the platform transcript
// contract.
type WireTranscript = wiretranscript.WireTranscript

// WireRecorder keeps the transport-facing name for the platform recorder
// contract.
type WireRecorder = wiretranscript.WireRecorder

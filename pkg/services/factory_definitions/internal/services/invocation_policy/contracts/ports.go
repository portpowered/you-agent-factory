package contracts

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// FileReader is the private byte-input seam used while resolving one
// invocation. Definitions receives copied bytes from its caller; it never
// reaches into a caller-owned filesystem.
type FileReader func(string) ([]byte, error)

// DecisionEnvelope is retained as a private parser value for the packaged
// decision-envelope implementation. It is not part of the Definitions root.
type DecisionEnvelope struct {
	Decision           string                 `json:"decision"`
	Feedback           string                 `json:"feedback"`
	Output             string                 `json:"output,omitempty"`
	RecordedOutputWork []work.FactoryWorkItem `json:"recorded_output_work,omitempty"`
}

// TTSInvocationWaitOutcome and TTSInvocationFailure are private packaged-TTS
// observation values. Their policy is owned by the TTS consumer, not by the
// published Definitions service.
type TTSInvocationWaitOutcome string

const (
	TTSInvocationWaitOutcomeLoading           TTSInvocationWaitOutcome = "loading"
	TTSInvocationWaitOutcomeModelNotReady     TTSInvocationWaitOutcome = "model_not_ready"
	TTSInvocationWaitOutcomeGenerationFailed  TTSInvocationWaitOutcome = "generation_failed"
	TTSInvocationWaitOutcomeUnresolvedFailure TTSInvocationWaitOutcome = "unresolved_failure"
)

type TTSInvocationFailure struct {
	Outcome      TTSInvocationWaitOutcome
	ErrorCode    string
	FailureClass string
	Message      string
}

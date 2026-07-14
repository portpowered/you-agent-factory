package responseevents

import shared "github.com/portpowered/infinite-you/pkg/interfaces/responseevents"

type ContentBlockKind = shared.ContentBlockKind
type ContentBlock = shared.ContentBlock
type SessionPayload = shared.SessionPayload
type RunPayload = shared.RunPayload
type TurnPayload = shared.TurnPayload
type MessagePayload = shared.MessagePayload
type MessageDeltaPayload = shared.MessageDeltaPayload
type ReasoningPayload = shared.ReasoningPayload
type ToolPayload = shared.ToolPayload
type ToolDeltaPayload = shared.ToolDeltaPayload
type FileChangePayload = shared.FileChangePayload
type PlanStep = shared.PlanStep
type PlanPayload = shared.PlanPayload
type ProgressPayload = shared.ProgressPayload
type UsagePayload = shared.UsagePayload
type ErrorPayload = shared.ErrorPayload
type StreamGapPayload = shared.StreamGapPayload

const (
	ContentBlockText             = shared.ContentBlockText
	ContentBlockReasoningSummary = shared.ContentBlockReasoningSummary
	ContentBlockToolRequest      = shared.ContentBlockToolRequest
	ContentBlockImageRef         = shared.ContentBlockImageRef
	ContentBlockResourceRef      = shared.ContentBlockResourceRef
	ContentBlockStructuredOutput = shared.ContentBlockStructuredOutput
)

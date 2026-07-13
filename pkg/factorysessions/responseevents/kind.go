package responseevents

// Kind identifies the semantic category of one FactoryResponseEvent.
type Kind string

const (
	KindSession    Kind = "SESSION"
	KindRun        Kind = "RUN"
	KindTurn       Kind = "TURN"
	KindMessage    Kind = "MESSAGE"
	KindReasoning  Kind = "REASONING"
	KindTool       Kind = "TOOL"
	KindFileChange Kind = "FILE_CHANGE"
	KindPlan       Kind = "PLAN"
	KindProgress   Kind = "PROGRESS"
	KindUsage      Kind = "USAGE"
	KindError      Kind = "ERROR"
	KindStreamGap  Kind = "STREAM_GAP"
)

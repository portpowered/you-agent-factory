package interfaces

// File directories

const (
	FactoryDir      = "factory"
	WorkflowsDir    = "workflows"
	WorkTypesDir    = "work-types"
	InputsDir       = "inputs"
	WorkersDir      = "workers"
	WorkstationsDir = "workstations"

	DefaultChannelName = "default"
	ArtifactsDirectory = "artifacts"
	StateDir           = "state"
	TransitionsDir     = "transitions"
	ArcsDir            = "arcs"
	GuardsDir          = "guards"
	ResourcesDir       = "resources"
	TokensDir          = "tokens"
	MarkingsDir        = "markings"

	FactoryAgentsFileName = "AGENTS.md"

	// relevant files.
	FactoryConfigFile         = "factory.json"
	CurrentFactoryPointerFile = ".current-factory"
	MetadataFile              = "metadata.json"
	MarkingFile               = "marking.json"
)

// Extensions

const (
	JsonExtension = ".json"
)

// Token fields
// These are fields that are used to color tokens as they go through the petri net.
const (
	WorkID     = "work_id"
	WorkTypeID = "work_type_id"
	TraceID    = "trace_id"
	ParentID   = "parent_id"
	Tags       = "tags"
	Relations  = "relations"
	Payload    = "payload"
)

// Internal time work contract.
const (
	SystemTimeWorkTypeID         = "__system_time"
	SystemTimePendingState       = "pending"
	SystemTimePendingPlaceID     = SystemTimeWorkTypeID + ":" + SystemTimePendingState
	SystemTimeExpiryTransitionID = SystemTimeWorkTypeID + ":expire"

	SystemTimeDashboardWorkTypeID         = "time"
	SystemTimeDashboardPendingPlaceID     = SystemTimeDashboardWorkTypeID + ":" + SystemTimePendingState
	SystemTimeDashboardExpiryTransitionID = SystemTimeDashboardWorkTypeID + ":expire"

	TimeWorkTagKeySource          = "agent_factory.source"
	TimeWorkTagKeyCronWorkstation = "agent_factory.cron.workstation"
	TimeWorkTagKeyNominalAt       = "agent_factory.time.nominal_at"
	TimeWorkTagKeyDueAt           = "agent_factory.time.due_at"
	TimeWorkTagKeyExpiresAt       = "agent_factory.time.expires_at"
	TimeWorkTagKeyJitter          = "agent_factory.time.jitter"

	TimeWorkSourceCron = "cron"
)

// IsSystemTimeWorkType reports whether a work type is the internal time-work
// type used for cron ticks.
func IsSystemTimeWorkType(workTypeID string) bool {
	return workTypeID == SystemTimeWorkTypeID
}

// IsSystemTimePlace reports whether a place belongs to the internal time-work
// state machine.
func IsSystemTimePlace(placeID string) bool {
	return placeID == SystemTimePendingPlaceID
}

// IsSystemTimeToken reports whether a token is an internal time-work token.
func IsSystemTimeToken(token *Token) bool {
	return token != nil && IsSystemTimeWorkType(token.Color.WorkTypeID)
}

// Executor configuration
// These are keys to store data for the messages between executors.
const (
	// These are well known tags that are used to store data for the messages between executors.
	RejectionFeedback = "_rejection_feedback"
)

// Resource constants

// Resource avialable states
const (
	ResourceStateAvailable = "available"
)

// WorkerType constants for worker AGENTS.md frontmatter.
const (
	WorkerTypeModel  = "MODEL_WORKER"
	WorkerTypeScript = "SCRIPT_WORKER"
)

// WorkstationType constants for workstation AGENTS.md frontmatter.
const (
	WorkstationTypeModel   = "MODEL_WORKSTATION"
	WorkstationTypeInvoke  = "MODEL_INVOKE"
	WorkstationTypeLogical = "LOGICAL_MOVE"
)

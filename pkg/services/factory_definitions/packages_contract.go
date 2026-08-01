package factorydefinitions

import (
	"io/fs"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

type PackagedDefinition = contracts.PackagedDefinition
type PackagedFactoryFormat = contracts.PackagedFactoryFormat

const (
	PackagedFactoryFormatJSON = contracts.PackagedFactoryFormatJSON
	PackagedFactoryFormatYAML = contracts.PackagedFactoryFormatYAML
	PackagedFactoryFormatYML  = contracts.PackagedFactoryFormatYML
)

const (
	PackagedDeepResearchFactoryName      = "@you/deep-research"
	PackagedFusionFactoryName            = "@you/fusion"
	PackagedGoalFactoryName              = "@you/goal"
	PackagedGoalWorkTypeName             = "goal"
	PackagedGoalExecuteWorkstationName   = "execute-goal"
	PackagedGoalPlanWorkstationName      = "plan-goal"
	PackagedGoalCheckWorkstationName     = "check-goal"
	PackagedGoalReviewWorkstationName    = "review-goal"
	PackagedReviewFactoryName            = "@you/review"
	PackagedReviewExecuteWorkstationName = "execute-review-work"
	PackagedReviewWorkstationName        = "review-review-work"
	PackagedTournamentFactoryName        = "@you/tournament"
	PackagedSpawnFactoryName             = "@you/spawn"
	PackagedLoopFactoryName              = "@you/loop"
	PackagedPlanExecuteFactoryName       = "@you/plan-execute"
	PackagedPlanParallelFactoryName      = "@you/plan-parallel"
	PackagedClassifyFactoryName          = "@you/classify"
	PackagedFullFlowFactoryName          = "@you/full-flow"
)

// PackagedFactoryAssetDefinition describes one authored packaged Factory and
// the assets available beneath its package-owned asset root.
type PackagedFactoryAssetDefinition = contracts.PackagedFactoryAssetDefinition

// PackagedFactoryAssetFileSystem is the exact filesystem effect used when
// assembling packaged Factory assets from an authored package directory.
type PackagedFactoryAssetFileSystem = fs.FS

// PackagedFactoryInstallOutcome is the detached result status returned by the
// Definitions distribution capability.
type PackagedFactoryInstallOutcome string

const (
	PackagedFactoryInstallCreated  PackagedFactoryInstallOutcome = "created"
	PackagedFactoryInstallSkipped  PackagedFactoryInstallOutcome = "skipped"
	PackagedFactoryInstallReplaced PackagedFactoryInstallOutcome = "replaced"
)

// PackagedFactoryInstallResult carries one concrete installation result.
type PackagedFactoryInstallResult struct {
	Name       string
	FactoryDir string
	Outcome    PackagedFactoryInstallOutcome
	Format     PackagedFactoryFormat
}

// PackagedFactoryInstallParams is the private distribution input needed to
// materialize one selected packaged Definition.
type PackagedFactoryInstallParams struct {
	NamedFactoriesRoot string
	Definition         PackagedDefinition
	Format             PackagedFactoryFormat
	Replace            bool
}

// QuorumLineageInput is the detached lineage identity consumed by packaged
// quorum relation policy.
type QuorumLineageInput struct {
	WorkID     string
	WorkTypeID string
}

// TTSInvocationWaitOutcome identifies the detached packaged-TTS wait result.
type TTSInvocationWaitOutcome string

const (
	TTSInvocationWaitOutcomeLoading           TTSInvocationWaitOutcome = "loading"
	TTSInvocationWaitOutcomeModelNotReady     TTSInvocationWaitOutcome = "model_not_ready"
	TTSInvocationWaitOutcomeGenerationFailed  TTSInvocationWaitOutcome = "generation_failed"
	TTSInvocationWaitOutcomeUnresolvedFailure TTSInvocationWaitOutcome = "unresolved_failure"
)

// TTSInvocationFailure carries detached packaged-TTS failure facts.
type TTSInvocationFailure struct {
	Outcome      TTSInvocationWaitOutcome
	ErrorCode    string
	FailureClass string
	Message      string
}

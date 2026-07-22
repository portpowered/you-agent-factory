package factorydefinitions

import "time"

// WorkstationExecutionPolicyService resolves execution policy authored on a
// Workstation definition. Consumers receive this contract from composition.
type WorkstationExecutionPolicyService interface {
	ExecutionTimeout(*FactoryWorkstationConfig) (time.Duration, error)
}

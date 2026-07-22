package factorydefinitions

// WorkPropagationPolicyService resolves the payload propagation policy
// authored on a Workstation definition.
type WorkPropagationPolicyService interface {
	Mode(*FactoryWorkstationConfig) WorkPropagationMode
}

// WorkPropagationPolicyFunc adapts an injected policy operation to the service
// role consumed by Factory Runtime. The operation remains owned and selected
// by Factory Definitions/Wire; consumers can supply a programmed root-contract
// result without importing the policy implementation.
type WorkPropagationPolicyFunc func(
	*FactoryWorkstationConfig,
) WorkPropagationMode

func (resolve WorkPropagationPolicyFunc) Mode(
	workstation *FactoryWorkstationConfig,
) WorkPropagationMode {
	return resolve(workstation)
}

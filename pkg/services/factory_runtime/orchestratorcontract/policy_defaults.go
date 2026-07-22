package orchestratorcontract

const (
	DefaultMaxAgents      = 16
	DefaultDeploymentCap  = 1000
	DefaultMaxDepth       = 1
	DefaultMaxRetries     = 0
	DefaultConcurrencyCap = 4
)

// DefaultEffectivePolicy returns the bounded read-only MVP policy defaults.
func DefaultEffectivePolicy() EffectivePolicy {
	return EffectivePolicy{
		Mode:                  ModeReadOnly,
		MaxAgents:             DefaultMaxAgents,
		Concurrency:           defaultConcurrencyForMaxAgents(DefaultMaxAgents),
		MaxDepth:              DefaultMaxDepth,
		MaxRetries:            DefaultMaxRetries,
		AllowNetwork:          false,
		AllowConnectors:       false,
		AllowDangerFullAccess: false,
		WritableRoots:         []string{},
		OutputAuditMode:       OutputAuditModeAuto,
	}
}

func defaultConcurrencyForMaxAgents(maxAgents int) int {
	if maxAgents < 1 {
		return 1
	}
	if maxAgents < DefaultConcurrencyCap {
		return maxAgents
	}
	return DefaultConcurrencyCap
}

func deploymentCap(request Request) int {
	if request.DeploymentCap > 0 {
		return request.DeploymentCap
	}
	return DefaultDeploymentCap
}

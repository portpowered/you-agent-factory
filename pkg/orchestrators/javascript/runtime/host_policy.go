package workflowruntime

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
)

func (g *runtimeGlobals) denyChildSlots(count int) error {
	if count <= 0 {
		return nil
	}
	current := g.records.childDispatchCountValue()
	if current+count > g.policy.MaxAgents {
		return fmt.Errorf(
			"policy denied: requested fanout %d exceeds maxAgents %d",
			count,
			g.policy.MaxAgents,
		)
	}
	return nil
}

func (g *runtimeGlobals) denyChildRequest(req ChildExecutionRequest) error {
	return workflowpolicy.ValidateChildRequest(g.policy, childPolicyRequest(req))
}

func (g *runtimeGlobals) denyArtifactSize(sizeBytes int64) error {
	if g.policy.MaxArtifactBytes == nil || *g.policy.MaxArtifactBytes <= 0 {
		return nil
	}
	if sizeBytes <= *g.policy.MaxArtifactBytes {
		return nil
	}
	return fmt.Errorf(
		"policy denied: artifact content size %d exceeds maxArtifactBytes %d",
		sizeBytes,
		*g.policy.MaxArtifactBytes,
	)
}

func childPolicyRequest(req ChildExecutionRequest) workflowpolicy.ChildRequest {
	return workflowpolicy.ChildRequest{
		Label:           req.Label,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		WritableRoots:   append([]string(nil), req.WritableRoots...),
		AllowNetwork:    req.AllowNetwork,
		Concurrency:     req.Concurrency,
	}
}

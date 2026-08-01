package lifecycle

import (
	"context"
	"fmt"
	"strings"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
)

// ResolveInvocationDefinition resolves authored invocation policy from the
// owner-local invocation_policy subservice and returns detached values only.
func (s *Service) ResolveInvocationDefinition(
	ctx context.Context,
	request factoryroot.ResolveInvocationDefinitionRequest,
) (factoryroot.ResolveInvocationDefinitionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryroot.ResolveInvocationDefinitionResult{}, err
	}
	if s == nil || s.invocationPolicyService == nil {
		return factoryroot.UnimplementedService{}.ResolveInvocationDefinition(ctx, request)
	}

	definition, err := factoryroot.CloneFactoryConfig(request.Definition.Factory)
	if err != nil {
		return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
			"%w: clone effective Factory: %v",
			factoryroot.ErrInvalidInvocationDefinition,
			err,
		)
	}
	if definition == nil {
		return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
			"%w: effective Factory configuration is required",
			factoryroot.ErrInvalidInvocationDefinition,
		)
	}

	policy := s.invocationPolicyService
	interpolation := policy.InvocationInterpolation()
	workTypePolicy := policy.InvocationWorkType()
	outputPolicy := policy.InvocationOutput()
	decisionPolicy := policy.DecisionEnvelope()
	executionPolicy := policy.WorkstationExecution()
	propagationPolicy := policy.WorkPropagation()
	if interpolation == nil || workTypePolicy == nil || outputPolicy == nil ||
		decisionPolicy == nil || executionPolicy == nil || propagationPolicy == nil {
		return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
			"%w: invocation policy is incomplete",
			factoryroot.ErrInvalidInvocationDefinition,
		)
	}

	args := request.Arguments
	readFile := resolvedInvocationFileReader(request.ResolvedFileInput)
	if err := interpolation.ValidateInvocationInterpolation(definition, &args, readFile); err != nil {
		return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
			"%w: resolve invocation arguments: %v",
			factoryroot.ErrInvalidInvocationDefinition,
			err,
		)
	}
	for index := range definition.Workers {
		worker, err := interpolation.InterpolateWorkerConfig(definition.Workers[index], &args, readFile)
		if err != nil {
			return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
				"%w: resolve worker %q: %v",
				factoryroot.ErrInvalidInvocationDefinition,
				definition.Workers[index].Name,
				err,
			)
		}
		definition.Workers[index] = worker
	}
	for index := range definition.Workstations {
		workstation, err := interpolation.InterpolateWorkstationConfig(definition.Workstations[index], &args, readFile)
		if err != nil {
			return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
				"%w: resolve workstation %q: %v",
				factoryroot.ErrInvalidInvocationDefinition,
				definition.Workstations[index].Name,
				err,
			)
		}
		definition.Workstations[index] = workstation
	}

	defaultWork, err := workTypePolicy.DefaultWorkType(definition)
	if err != nil {
		return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
			"%w: resolve default work type: %v",
			factoryroot.ErrInvalidInvocationDefinition,
			err,
		)
	}

	workstations := make(map[string]factoryroot.ResolvedWorkstationPolicy, len(definition.Workstations))
	for index := range definition.Workstations {
		workstation := &definition.Workstations[index]
		timeout, err := executionPolicy.ExecutionTimeout(workstation)
		if err != nil {
			return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
				"%w: resolve workstation %q execution timeout: %v",
				factoryroot.ErrInvalidInvocationDefinition,
				workstation.Name,
				err,
			)
		}

		name := strings.TrimSpace(workstation.Name)
		if name == "" {
			name = strings.TrimSpace(workstation.ID)
		}
		if name == "" {
			return factoryroot.ResolveInvocationDefinitionResult{}, fmt.Errorf(
				"%w: workstation identity is required",
				factoryroot.ErrInvalidInvocationDefinition,
			)
		}
		workstations[name] = factoryroot.ResolvedWorkstationPolicy{
			ExecutionTimeout: timeout,
			PropagationMode:  propagationPolicy.Mode(workstation),
			OutputMode:       invocationOutputMode(outputPolicy, workstation),
			DecisionMode:     decisionEnvelopeMode(decisionPolicy, workstation),
		}
	}

	kind := factoryroot.FactoryBehaviorKindStandard
	if strings.HasPrefix(strings.TrimSpace(definition.Name), "@you/") ||
		strings.HasPrefix(strings.TrimSpace(definition.Project), "builtin-") {
		kind = factoryroot.FactoryBehaviorKindPackaged
	}
	return factoryroot.ResolveInvocationDefinitionResult{
		Factory:      *definition,
		DefaultWork:  defaultWork,
		Workstations: workstations,
		FactoryKind:  kind,
	}, nil
}

func resolvedInvocationFileReader(inputs map[string][]byte) factoryeffects.FileReader {
	return func(path string) ([]byte, error) {
		contents, ok := inputs[path]
		if !ok {
			return nil, fmt.Errorf("resolved file input %q is unavailable", path)
		}
		return append([]byte(nil), contents...), nil
	}
}

func invocationOutputMode(
	policy factoryeffects.InvocationOutputShapingService,
	workstation *factoryroot.FactoryWorkstationConfig,
) factoryroot.InvocationOutputMode {
	switch {
	case policy.ShouldFormatTTSInvocationMetadata(workstation):
		return factoryroot.InvocationOutputModeTTS
	case policy.ShouldFormatInvocationSummary(workstation):
		return factoryroot.InvocationOutputModeSummary
	case policy.ShouldFormatInvocationResponse(workstation):
		return factoryroot.InvocationOutputModeResponse
	default:
		return factoryroot.InvocationOutputModeDefault
	}
}

func decisionEnvelopeMode(
	policy factoryeffects.DecisionEnvelopeService,
	workstation *factoryroot.FactoryWorkstationConfig,
) factoryroot.DecisionEnvelopeMode {
	if policy.UsesGoalRoutingDecisionEnvelope(workstation) {
		return factoryroot.DecisionEnvelopeModeGoalRouting
	}
	if policy.UsesDecisionEnvelopeOutcome(workstation) {
		return factoryroot.DecisionEnvelopeModeEnvelope
	}
	return factoryroot.DecisionEnvelopeModeNone
}

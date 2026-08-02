package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicydecisionenvelope "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/decisionenvelope"
	invocationpolicyinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation"
	invocationpolicyoutput "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationoutput"
	invocationpolicyworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype"
	invocationpolicyworkpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation"
	invocationpolicyworkstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the private, stateless invocation resolver. It owns no runtime,
// Work, worker, persistence, or filesystem effects.
type Service struct{}

var _ invocationpolicy.Service = (*Service)(nil)

func New() *Service {
	return &Service{}
}

func (s *Service) ResolveInvocationDefinition(
	ctx context.Context,
	request factorydefinitions.ResolveInvocationDefinitionRequest,
) (factorydefinitions.ResolveInvocationDefinitionResult, error) {
	if s == nil {
		return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("resolver is required")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return factorydefinitions.ResolveInvocationDefinitionResult{}, err
		}
	}
	if request.Definition.Factory == nil {
		return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("effective Factory is required")
	}

	resolved, err := factorydefinitions.CloneFactoryConfig(request.Definition.Factory)
	if err != nil {
		return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("clone effective Factory: %v", err)
	}
	if resolved == nil {
		return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("effective Factory is required")
	}

	fileInputs := cloneFileInputs(request.ResolvedFileInput)
	arguments := cloneInvocationArguments(request.Arguments)
	readFile := func(path string) ([]byte, error) {
		data, ok := fileInputs[path]
		if !ok {
			return nil, fmt.Errorf("file input %q is not supplied", path)
		}
		return append([]byte(nil), data...), nil
	}

	if err := invocationpolicyinterpolation.ValidateInvocationInterpolation(
		resolved,
		&arguments,
		invocationpolicyinterpolation.FileReader(readFile),
	); err != nil {
		return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("interpolate effective Factory: %v", err)
	}
	for index := range resolved.Workers {
		worker, err := invocationpolicyinterpolation.InterpolateWorkerConfig(
			resolved.Workers[index],
			&arguments,
			invocationpolicyinterpolation.FileReader(readFile),
		)
		if err != nil {
			return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("interpolate worker %q: %v", resolved.Workers[index].Name, err)
		}
		resolved.Workers[index] = worker
	}
	for index := range resolved.Workstations {
		workstation, err := invocationpolicyinterpolation.InterpolateWorkstationConfig(
			resolved.Workstations[index],
			&arguments,
			invocationpolicyinterpolation.FileReader(readFile),
		)
		if err != nil {
			return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("interpolate workstation %q: %v", resolved.Workstations[index].Name, err)
		}
		invocationpolicyworkstationexecution.NormalizeExecutionLimit(&workstation)
		resolved.Workstations[index] = workstation
	}

	defaultWorkType, err := invocationpolicyworktype.DefaultWorkType(resolved)
	if err != nil {
		return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("resolve default Work type: %v", err)
	}

	policies := make(map[string]factorydefinitions.ResolvedWorkstationPolicy, len(resolved.Workstations))
	for index := range resolved.Workstations {
		workstation := &resolved.Workstations[index]
		name := strings.TrimSpace(workstation.Name)
		if name == "" {
			return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("workstation name is required")
		}
		if _, exists := policies[name]; exists {
			return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("workstation %q is duplicated", name)
		}
		timeout, err := invocationpolicyworkstationexecution.ExecutionTimeout(workstation)
		if err != nil {
			return factorydefinitions.ResolveInvocationDefinitionResult{}, invalidInvocation("resolve workstation %q timeout: %v", name, err)
		}
		outputMode := factorydefinitions.InvocationOutputModeDefault
		switch {
		case invocationpolicyoutput.ShouldFormatInvocationSummary(workstation):
			outputMode = factorydefinitions.InvocationOutputModeSummary
		case invocationpolicyoutput.ShouldFormatInvocationResponse(workstation):
			outputMode = factorydefinitions.InvocationOutputModeResponse
		case invocationpolicyoutput.ShouldFormatTTSInvocationMetadata(workstation):
			outputMode = factorydefinitions.InvocationOutputModeTTS
		}
		decisionMode := factorydefinitions.DecisionEnvelopeModeNone
		if invocationpolicydecisionenvelope.UsesDecisionEnvelopeOutcome(workstation) {
			decisionMode = factorydefinitions.DecisionEnvelopeModeEnvelope
			if invocationpolicydecisionenvelope.UsesGoalRoutingDecisionEnvelope(workstation) {
				decisionMode = factorydefinitions.DecisionEnvelopeModeGoalRouting
			}
		}
		policies[name] = factorydefinitions.ResolvedWorkstationPolicy{
			ExecutionTimeout: timeout,
			PropagationMode:  invocationpolicyworkpropagation.Mode(workstation),
			OutputMode:       outputMode,
			DecisionMode:     decisionMode,
		}
	}

	return factorydefinitions.ResolveInvocationDefinitionResult{
		Factory:             *resolved,
		DefaultWorkType:     defaultWorkType,
		WorkstationPolicies: policies,
		FactoryKind:         factoryKind(resolved),
	}, nil
}

func invalidInvocation(format string, args ...any) error {
	return fmt.Errorf("%w: %s", factorydefinitions.ErrInvalidInvocationDefinition, fmt.Sprintf(format, args...))
}

func cloneFileInputs(inputs map[string][]byte) map[string][]byte {
	if len(inputs) == 0 {
		return nil
	}
	cloned := make(map[string][]byte, len(inputs))
	for name, data := range inputs {
		cloned[name] = append([]byte(nil), data...)
	}
	return cloned
}

func cloneInvocationArguments(arguments factorydefinitions.InvocationArguments) factorydefinitions.InvocationArguments {
	if len(arguments.Arguments) == 0 {
		return factorydefinitions.InvocationArguments{}
	}
	cloned := factorydefinitions.InvocationArguments{Arguments: make(map[string]factorydefinitions.InvocationArgument, len(arguments.Arguments))}
	for name, argument := range arguments.Arguments {
		clonedArgument := argument
		clonedArgument.Values = append([]string(nil), argument.Values...)
		clonedArgument.Sources = append([]work.InvocationArgumentSource(nil), argument.Sources...)
		cloned.Arguments[name] = clonedArgument
	}
	return cloned
}

func factoryKind(cfg *factorydefinitions.FactoryConfig) factorydefinitions.FactoryBehaviorKind {
	if cfg != nil && (strings.HasPrefix(strings.TrimSpace(cfg.Name), "@you/") || strings.HasPrefix(strings.TrimSpace(cfg.Project), "builtin-")) {
		return factorydefinitions.FactoryBehaviorKindPackaged
	}
	return factorydefinitions.FactoryBehaviorKindStandard
}

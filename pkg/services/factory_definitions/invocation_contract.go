package factorydefinitions

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// InvocationArguments is the transport-independent argument projection used
// when resolving an authored Factory for one invocation.
type InvocationArguments = work.InvocationArguments
type InvocationArgument = work.InvocationArgument

// FactoryBehaviorKind identifies whether a resolved definition is authored by
// the customer or supplied through the built-in packaged catalog.
type FactoryBehaviorKind string

const (
	FactoryBehaviorKindStandard FactoryBehaviorKind = "standard"
	FactoryBehaviorKindPackaged FactoryBehaviorKind = "packaged"
)

// InvocationOutputMode identifies the Definitions-owned output shaping hint
// selected from the resolved workstation policy.
type InvocationOutputMode string

const (
	InvocationOutputModeDefault  InvocationOutputMode = "default"
	InvocationOutputModeSummary  InvocationOutputMode = "summary"
	InvocationOutputModeResponse InvocationOutputMode = "response"
	InvocationOutputModeTTS      InvocationOutputMode = "tts"
)

// DecisionEnvelopeMode identifies the authored decision-envelope contract for
// a resolved workstation.
type DecisionEnvelopeMode string

const (
	DecisionEnvelopeModeNone        DecisionEnvelopeMode = "none"
	DecisionEnvelopeModeEnvelope    DecisionEnvelopeMode = "decision-envelope"
	DecisionEnvelopeModeGoalRouting DecisionEnvelopeMode = "goal-routing"
)

// ErrInvalidInvocationDefinition reports a definition that cannot be
// detached, interpolated, or projected into invocation policy.
var ErrInvalidInvocationDefinition = errors.New("invalid invocation definition")

// ResolveInvocationDefinitionRequest carries the already compiled effective
// definition and normalized invocation inputs. ResolvedFileInput is an
// explicit IO boundary: Definitions never reads the caller's filesystem.
type ResolveInvocationDefinitionRequest struct {
	Definition        EffectiveFactorySource
	Arguments         InvocationArguments
	ResolvedFileInput map[string][]byte
}

// ResolveInvocationDefinitionResult is a detached invocation-time projection
// of authored policy consumed by Workers, Runtime, and Sessions.
type ResolveInvocationDefinitionResult struct {
	Factory      FactoryConfig
	DefaultWork  string
	Workstations map[string]ResolvedWorkstationPolicy
	FactoryKind  FactoryBehaviorKind
}

// ResolvedWorkstationPolicy contains only authored policy facts needed by
// invocation execution. It excludes worker results and lifecycle handles.
type ResolvedWorkstationPolicy struct {
	ExecutionTimeout time.Duration
	PropagationMode  WorkPropagationMode
	OutputMode       InvocationOutputMode
	DecisionMode     DecisionEnvelopeMode
}

// InvocationDefinitionResolver resolves one effective Factory into detached
// invocation policy and applies argument interpolation before returning.
type InvocationDefinitionResolver func(
	context.Context,
	ResolveInvocationDefinitionRequest,
) (ResolveInvocationDefinitionResult, error)

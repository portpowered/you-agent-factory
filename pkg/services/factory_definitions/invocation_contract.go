package factorydefinitions

import (
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// InvocationArguments is the transport-independent argument projection used
// when resolving an authored Factory for one invocation. Definitions copies
// the values it needs before applying interpolation.
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
// selected from a workstation definition.
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

// ErrInvalidInvocationDefinition reports a definition that cannot be detached,
// interpolated, or projected into invocation facts.
var ErrInvalidInvocationDefinition = errors.New("invalid invocation definition")

// ResolveInvocationDefinitionRequest carries one already compiled effective
// definition and normalized invocation inputs. ResolvedFileInput is supplied
// by the caller as byte data; Definitions never reads the caller's filesystem.
type ResolveInvocationDefinitionRequest struct {
	Definition        EffectiveFactorySource
	Arguments         InvocationArguments
	ResolvedFileInput map[string][]byte
}

// ResolveInvocationDefinitionResult is a detached invocation-time projection
// consumed by Sessions, Runtime, Workers, and Work. It contains no runtime
// world state, Work result, relation, worker execution object, reader,
// callback, or persistence effect.
type ResolveInvocationDefinitionResult struct {
	Factory             FactoryConfig
	DefaultWorkType     string
	WorkstationPolicies map[string]ResolvedWorkstationPolicy
	FactoryKind         FactoryBehaviorKind
}

// ResolvedWorkstationPolicy contains only authored policy facts needed by
// invocation execution.
type ResolvedWorkstationPolicy struct {
	ExecutionTimeout time.Duration
	PropagationMode  WorkPropagationMode
	OutputMode       InvocationOutputMode
	DecisionMode     DecisionEnvelopeMode
}

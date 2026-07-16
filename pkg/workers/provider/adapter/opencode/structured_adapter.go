package opencode

import (
	"context"
	"errors"
	"os"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const providerSessionKind = "session_id"

// StructuredAdapter implements the provider-neutral adapter contract for one
// positively negotiated OpenCode installation. Capability negotiation remains
// separate so no customer request is needed to construct this adapter.
type StructuredAdapter struct {
	*NegotiatedAdapter
}

// NegotiatedAdapter executes exactly the protocol selected for one cached
// OpenCode installation decision.
type NegotiatedAdapter struct {
	decision          Decision
	resolver          *Resolver
	requireStructured bool
}

// NewStructuredAdapter binds structured execution to one negotiated decision.
func NewStructuredAdapter(decision Decision) (*StructuredAdapter, error) {
	if decision.Mode != ModeStructured {
		return nil, errors.New("opencode structured adapter requires a structured capability decision")
	}
	if strings.TrimSpace(decision.Installation.Executable) == "" {
		return nil, errors.New("opencode structured adapter requires a resolved executable")
	}
	negotiated, err := NewNegotiatedAdapter(decision, nil)
	if err != nil {
		return nil, err
	}
	return &StructuredAdapter{NegotiatedAdapter: negotiated}, nil
}

// NewNegotiatedAdapter binds execution to a cached structured or final-only
// decision. A resolver enables safe stale-capability downgrade.
func NewNegotiatedAdapter(decision Decision, resolver *Resolver) (*NegotiatedAdapter, error) {
	return newNegotiatedAdapter(decision, resolver, false)
}

// NewNegotiatedAdapterForRequest binds the selected protocol to one request's
// required-capability contract. In particular, a required structured stream
// may not silently downgrade to final-only execution.
func NewNegotiatedAdapterForRequest(
	decision Decision,
	resolver *Resolver,
	request workerexecution.ProviderInferenceRequest,
) (*NegotiatedAdapter, error) {
	return newNegotiatedAdapter(decision, resolver, requiresStructuredOutput(request))
}

func newNegotiatedAdapter(decision Decision, resolver *Resolver, requireStructured bool) (*NegotiatedAdapter, error) {
	if decision.Mode != ModeStructured && decision.Mode != ModeFinalOnly {
		return nil, errors.New("opencode adapter requires a negotiated capability decision")
	}
	if strings.TrimSpace(decision.Installation.Executable) == "" {
		return nil, errors.New("opencode adapter requires a resolved executable")
	}
	return &NegotiatedAdapter{decision: decision, resolver: resolver, requireStructured: requireStructured}, nil
}

func requiresStructuredOutput(request workerexecution.ProviderInferenceRequest) bool {
	for _, capability := range request.RequiredOptionalCapabilities {
		if capability == workerexecution.RunnerOptionalCapabilityStructuredOutput {
			return true
		}
	}
	return false
}

func (*NegotiatedAdapter) Identity() adapter.Identity {
	return adapter.Identity(modelprovider.OpenCode)
}

func (a *NegotiatedAdapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	request := input.Request
	args := []string{"run"}
	if a.decision.Mode == ModeStructured {
		args = append(args, "--format", "json")
	}
	if request.Model != "" {
		args = append(args, "--model", request.Model)
	}
	if request.OpenCodeAgent != "" {
		args = append(args, "--agent", request.OpenCodeAgent)
	}
	if request.SessionID != "" {
		args = append(args, "--session", request.SessionID)
	}
	if request.WorkingDirectory != "" {
		args = append(args, "--dir", request.WorkingDirectory)
	}
	if input.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, request.UserMessage)

	command := workerprocess.SubprocessRequestBase(request.Dispatch)
	command.Command = a.decision.Installation.Executable
	command.Args = args
	command.Env = structuredCommandEnv(request.EnvVars)
	command.WorkDir = request.WorkingDirectory
	command.InputTokens = append([]any(nil), request.InputTokens...)
	if request.WorkerType != "" {
		command.WorkerType = request.WorkerType
	}
	if request.WorkstationType != "" {
		command.WorkstationName = request.WorkstationType
	}
	if request.ProjectID != "" {
		command.ProjectID = request.ProjectID
	}
	return adapter.CommandBuildResult{Request: command}, nil
}

func structuredCommandEnv(overrides map[string]string) []string {
	automation := []workerprocess.CommandEnvEntry{
		{Name: "GIT_EDITOR", Value: "true"},
		{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
		{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
		{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
		{Name: "EDITOR", Value: "true"},
		{Name: "VISUAL", Value: "true"},
	}
	return workerprocess.MergeCommandEnv(os.Environ(), workerprocess.CommandEnvEntriesFromMap(overrides), automation)
}

func (a *NegotiatedAdapter) NewDecoder(_ context.Context, input adapter.DecoderContext) (adapter.Decoder, error) {
	if a.decision.Mode == ModeFinalOnly {
		return finalOnlyDecoder{}, nil
	}
	return newStructuredDecoder(input), nil
}

func (a *NegotiatedAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: a.decision.Capabilities()}, nil
}

var _ adapter.Adapter = (*StructuredAdapter)(nil)
var _ adapter.Adapter = (*NegotiatedAdapter)(nil)

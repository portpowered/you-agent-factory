// Invocation dependencies are projected from the active Factory Session.
package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/packagedtts"
	invocationruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/runtimeadapter"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// NewInvocationOwner constructs the canonical invocation owner from the
// session runtime's flat public callbacks.
func NewInvocationOwner(
	fs *SessionRuntime,
	inputFiles fileeffects.InvocationInputReader,
) (invocationservice.Service, error) {
	if fs == nil {
		return nil, fmt.Errorf("session runtime is required")
	}
	return invocationwire.New(invocationservice.Dependencies{
		FactoryConfig: func(sessionID string) (*interfaces.FactoryConfig, error) {
			runtimeConfig, err := runtimebinding.RuntimeConfigForSession(fs.sessionState, sessionID)
			if err != nil {
				return nil, err
			}
			return runtimeConfig.FactoryConfig(), nil
		},
		SubmitWork: fs.submitOwnedSessionInvocationWork,
		Observe: func(ctx context.Context, sessionID string, input sessioninvocation.SessionInvocationWaitInput) (sessioninvocation.SessionInvocationObservation, error) {
			return invocationruntime.Observe(ctx, fs.sessionState, sessionID, input, fs.worldStateProjector)
		},
		Telemetry: packagedtts.NewTelemetry(
			func(metric sessioninvocation.SessionInvocationMetric) {
				fs.recordInvocationMetric(metric.Name, metric.Labels)
			},
			func(record sessioninvocation.SessionInvocationLogRecord) {
				invocationruntime.WriteLogRecord(fs.logger, record)
			},
		),
		SpecialCase: packagedtts.NewSpecialCase(),
		ResolveDefinition: func(
			ctx context.Context,
			sessionID string,
			cfg *interfaces.FactoryConfig,
			args *interfaces.InvocationArguments,
			fileInputs map[string][]byte,
		) (interfaces.ResolveInvocationDefinitionResult, error) {
			if fs.definitions == nil {
				return interfaces.ResolveInvocationDefinitionResult{}, fmt.Errorf("Factory Definitions service is required")
			}
			runtimeConfig, err := runtimebinding.RuntimeConfigForSession(fs.sessionState, sessionID)
			if err != nil {
				return interfaces.ResolveInvocationDefinitionResult{}, err
			}
			if runtimeConfig == nil {
				return interfaces.ResolveInvocationDefinitionResult{}, fmt.Errorf("Factory Session runtime definition is unavailable")
			}
			definition := runtimeConfig.FactoryConfig()
			if definition == nil {
				definition = cfg
			}
			var invocationArgs interfaces.InvocationArguments
			if args != nil {
				invocationArgs = *args
			}
			return fs.definitions.ResolveInvocationDefinition(ctx, interfaces.ResolveInvocationDefinitionRequest{
				Definition: interfaces.EffectiveFactorySource{
					Factory:        definition,
					FactoryDir:     runtimeConfig.FactoryDir(),
					RuntimeBaseDir: runtimeConfig.RuntimeBaseDir(),
				},
				Arguments:         invocationArgs,
				ResolvedFileInput: fileInputs,
			})
		},
		InputFiles: inputFiles,
		Work:       work.NewInvocationPolicyService(),
	})
}

func (fs *SessionRuntime) submitOwnedSessionInvocationWork(ctx context.Context, sessionID string, request work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	return fs.SubmitWorkRequestForSession(ctx, sessionID, work.WorkRequestFromSubmitRequests([]work.SubmitRequest{request}))
}

func (fs *SessionRuntime) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.invocationMetricsRecorder == nil {
		return
	}
	fs.invocationMetricsRecorder.RecordInvocationMetric(factorysessions.InvocationMetric{Name: name, Labels: labels})
}

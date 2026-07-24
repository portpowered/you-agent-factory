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
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
)

// NewInvocationOwner constructs the canonical invocation owner from the
// session runtime's flat public callbacks.
func NewInvocationOwner(
	fs *SessionRuntime,
	interpolation interfaces.InvocationInterpolationService,
	invocationWorkTypes interfaces.InvocationWorkTypeService,
	ttsObservability interfaces.TTSObservabilityService,
	inputFiles fileeffects.InvocationInputReader,
) (invocationservice.Service, error) {
	if fs == nil {
		return nil, fmt.Errorf("session runtime is required")
	}
	return invocationwire.New(invocationservice.Dependencies{
		FactoryConfig: func(sessionID string) (*interfaces.FactoryConfig, error) {
			return invocationruntime.FactoryConfig(fs.sessionState, sessionID)
		},
		// SessionRuntime exposes the CTR-WORK admission method signature used as
		// the exact injected peer-root port for private invocation command.
		Work: fs,
		Observe: func(ctx context.Context, sessionID string, input sessioninvocation.SessionInvocationWaitInput) (sessioninvocation.SessionInvocationObservation, error) {
			return invocationruntime.Observe(ctx, fs.sessionState, sessionID, input, fs.worldStateProjector)
		},
		Telemetry: packagedtts.NewTelemetry(
			ttsObservability,
			func(metric sessioninvocation.SessionInvocationMetric) {
				fs.recordInvocationMetric(metric.Name, metric.Labels)
			},
			func(record sessioninvocation.SessionInvocationLogRecord) {
				invocationruntime.WriteLogRecord(fs.logger, record)
			},
		),
		SpecialCase:   packagedtts.NewSpecialCase(ttsObservability),
		Interpolation: interpolation,
		WorkTypes:     invocationWorkTypes,
		InputFiles:    inputFiles,
	})
}

func (fs *SessionRuntime) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.invocationMetricsRecorder == nil {
		return
	}
	fs.invocationMetricsRecorder.RecordInvocationMetric(factorysessions.InvocationMetric{Name: name, Labels: labels})
}

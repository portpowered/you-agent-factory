package runtimehost

import (
	"context"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/packages/tts"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// InvokeFactorySession transparently forwards session invocation to the
// canonical invocation owner.
func (fs *Host) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	return fs.sessionInvocationOwner().InvokeFactorySession(ctx, sessionID, request)
}

func (fs *Host) sessionInvocationOwner() invocations.SessionInvoker {
	if fs.sessionInvoker != nil {
		return fs.sessionInvoker
	}
	return invocations.NewSessionOwner(invocations.SessionOwnerDependencies{
		FactoryConfig: fs.sessionInvocationFactoryConfig,
		SubmitWork:    fs.submitOwnedSessionInvocationWork,
		Observe:       fs.observeSessionInvocation,
		Telemetry:     fs.sessionInvocationTelemetry(),
		SpecialCase:   hostSessionInvocationSpecialCase{},
	})
}

func (fs *Host) sessionInvocationTelemetry() invocations.SessionInvocationTelemetry {
	return invocations.NewSessionInvocationTelemetry(invocations.SessionInvocationTelemetryDependencies{
		RecordMetric: func(metric invocations.SessionInvocationMetric) {
			fs.recordInvocationMetric(metric.Name, metric.Labels)
		},
		RecordLog: fs.recordSessionInvocationLog,
		Packaged: &invocations.PackagedInvocationTelemetry{
			Active: tts.IsPackagedFactory, FactoryName: tts.PackagedFactoryName, Backend: tts.BackendRuntimeLabel(),
			AttemptsMetric: tts.MetricPackagedFactoryAttempts, SuccessMetric: tts.MetricPackagedFactorySuccess,
			FailureMetric: tts.MetricPackagedFactoryFailure, NotReadyMetric: tts.MetricPackagedFactoryNotReady,
			LoadingClass: tts.FailureClassLoading, SuccessClass: tts.FailureClassSuccess, NotReadyClass: tts.FailureClassModelNotReady,
		},
	})
}

func (fs *Host) recordSessionInvocationLog(record invocations.SessionInvocationLogRecord) {
	if fs == nil || fs.logger == nil {
		return
	}
	fields := make([]zap.Field, 0, len(record.Fields)+1)
	for key, value := range record.Fields {
		fields = append(fields, zap.Any(key, value))
	}
	if record.Error != nil {
		fields = append(fields, zap.Error(record.Error))
	}
	if record.Level == "warn" {
		fs.logger.Warn(record.Message, fields...)
		return
	}
	fs.logger.Info(record.Message, fields...)
}

func (fs *Host) sessionInvocationFactoryConfig(sessionID string) (*interfaces.FactoryConfig, error) {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	if err != nil || runtimeCfg == nil {
		return nil, err
	}
	return runtimeCfg.FactoryConfig(), nil
}

func (fs *Host) submitOwnedSessionInvocationWork(ctx context.Context, sessionID string, request interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
	return fs.SubmitWorkRequestForSession(ctx, sessionID, factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{request}))
}

func (fs *Host) observeSessionInvocation(ctx context.Context, sessionID string, input invocations.SessionInvocationWaitInput) (invocations.SessionInvocationObservation, error) {
	snapshot, err := fs.GetEngineStateSnapshotForSession(ctx, sessionID)
	if err != nil {
		return invocations.SessionInvocationObservation{}, err
	}
	worldState, err := fs.sessionInvocationWorldState(ctx, sessionID, snapshot.TickCount)
	if err != nil {
		return invocations.SessionInvocationObservation{}, err
	}
	return invocations.SessionInvocationObservation{
		WorldState: worldState, FactoryState: snapshot.FactoryState,
		ActiveWork:           snapshotHasActiveWork(snapshot),
		MissingPrimaryResult: classifyInvocationMissingPrimaryResultFromSnapshot(sessionID, snapshot, input),
	}, nil
}

type hostSessionInvocationSpecialCase struct{}

func (hostSessionInvocationSpecialCase) Active(cfg *interfaces.FactoryConfig) bool {
	return tts.IsPackagedFactory(cfg)
}

func (hostSessionInvocationSpecialCase) TerminalFailure(worldState interfaces.FactoryWorldState, requestID string) *invocations.SessionInvocationSpecialFailure {
	_, failure := tts.ClassifyInvocationWait(worldState, requestID, false)
	if failure == nil {
		return nil
	}
	return &invocations.SessionInvocationSpecialFailure{ErrorCode: failure.ErrorCode, Message: failure.Message, FailureClass: failure.FailureClass}
}

func (fs *Host) sessionInvocationWorldState(
	ctx context.Context,
	sessionID string,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	activeFactory, err := fs.sessionFactory(sessionID)
	if err != nil {
		return interfaces.FactoryWorldState{}, err
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return interfaces.FactoryWorldState{}, err
	}
	return projections.ReconstructFactoryWorldState(events, selectedTick)
}

func (fs *Host) recordInvocationMetric(name string, labels map[string]string) {
	if fs == nil || fs.cfg == nil || fs.cfg.InvocationMetricsRecorder == nil {
		return
	}
	fs.cfg.InvocationMetricsRecorder.RecordInvocationMetric(InvocationMetric{Name: name, Labels: labels})
}

func classifyInvocationMissingPrimaryResultFromSnapshot(
	sessionID string,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	input invocations.SessionInvocationWaitInput,
) *invocations.PrimaryResultError {
	if snapshot == nil || strings.TrimSpace(input.RequestID) == "" {
		return nil
	}
	tokens := make([]*interfaces.Token, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		leftID, rightID := "", ""
		if tokens[i] != nil {
			leftID = tokens[i].Color.WorkID
		}
		if tokens[j] != nil {
			rightID = tokens[j].Color.WorkID
		}
		if leftID == rightID {
			return tokenPlaceID(tokens[i]) < tokenPlaceID(tokens[j])
		}
		return leftID < rightID
	})
	for _, wantState := range []string{"blocked", "needs-human"} {
		for _, token := range tokens {
			if token == nil || token.Color.DataType == interfaces.DataTypeResource {
				continue
			}
			if strings.TrimSpace(token.Color.RequestID) != strings.TrimSpace(input.RequestID) || tokenStateName(token.PlaceID) != wantState {
				continue
			}
			return invocations.ClassifyMissingPrimaryResultWorkItem(input.RequestID, input.InvocationReturn, interfaces.FactoryWorkItem{
				ID: token.Color.WorkID, WorkTypeID: token.Color.WorkTypeID,
				DisplayName: token.Color.Name, PlaceID: token.PlaceID,
			}, sessionID)
		}
	}
	return nil
}

func tokenStateName(placeID string) string {
	trimmed := strings.TrimSpace(placeID)
	if _, suffix, ok := strings.Cut(trimmed, ":"); ok {
		return suffix
	}
	return trimmed
}

func tokenPlaceID(token *interfaces.Token) string {
	if token == nil {
		return ""
	}
	return token.PlaceID
}

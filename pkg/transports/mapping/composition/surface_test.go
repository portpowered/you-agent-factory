package composition

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
)

type SessionAPI = apisurface.SessionAPI
type ModelAPI = apisurface.ModelAPI
type FactorySaveAPI = apisurface.FactorySaveAPI
type InvocationAPI = apisurface.InvocationAPI
type SessionAPISurface = apisurface.SessionAPISurface
type FactoryInvocationResult = apisurface.FactoryInvocationResult

type sessionCollaboratorFake struct {
	SessionAPI
	getCurrentFactory           func(context.Context) (factoryapi.Factory, error)
	submitWorkRequestForSession func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

func (f *sessionCollaboratorFake) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return f.getCurrentFactory(ctx)
}

func (f *sessionCollaboratorFake) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return f.submitWorkRequestForSession(ctx, sessionID, request)
}

type modelCollaboratorFake struct {
	ModelAPI
	listModels func(context.Context) (factoryapi.ListModelsResponse, error)
	getModel   func(context.Context, string) (factoryapi.ModelDetail, error)
}

func (f *modelCollaboratorFake) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	return f.listModels(ctx)
}

func (f *modelCollaboratorFake) GetModel(ctx context.Context, name string) (factoryapi.ModelDetail, error) {
	return f.getModel(ctx, name)
}

type factoryDefinitionCollaboratorFake struct {
	FactorySaveAPI
	getCurrentFactoryForSession func(context.Context, string) (factoryapi.Factory, error)
	saveFactoryForSession       func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
}

func (f *factoryDefinitionCollaboratorFake) GetCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
) (factoryapi.Factory, error) {
	return f.getCurrentFactoryForSession(ctx, sessionID)
}

func (f *factoryDefinitionCollaboratorFake) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return f.saveFactoryForSession(ctx, sessionID, mode, request)
}

type invocationCollaboratorFake struct {
	InvocationAPI
	invokeFactorySession func(context.Context, string, factoryapi.InvocationRequest) (FactoryInvocationResult, error)
}

func (f *invocationCollaboratorFake) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (FactoryInvocationResult, error) {
	return f.invokeFactorySession(ctx, sessionID, request)
}

type durableExecutionCollaboratorFake struct {
	DurableSessionAPI
	startAsync func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error)
	startSync  func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionSyncExecutionResponse, error)
}

func (f *durableExecutionCollaboratorFake) StartDurableFactorySessionAsync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	return f.startAsync(ctx, request)
}

func (f *durableExecutionCollaboratorFake) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	return f.startSync(ctx, request)
}

func TestNewSessionAPISurfaceRejectsMissingCollaborators(t *testing.T) {
	session, model, definition, invocation, durable := composedSurfaceFakes()
	tests := []struct {
		role      string
		construct func() (SessionAPISurface, error)
	}{
		{role: "session", construct: func() (SessionAPISurface, error) {
			return NewSessionAPISurface(nil, model, definition, invocation, durable)
		}},
		{role: "model", construct: func() (SessionAPISurface, error) {
			return NewSessionAPISurface(session, nil, definition, invocation, durable)
		}},
		{role: "factory-definition", construct: func() (SessionAPISurface, error) {
			return NewSessionAPISurface(session, model, nil, invocation, durable)
		}},
		{role: "invocation", construct: func() (SessionAPISurface, error) {
			return NewSessionAPISurface(session, model, definition, nil, durable)
		}},
		{role: "durable-execution", construct: func() (SessionAPISurface, error) {
			return NewSessionAPISurface(session, model, definition, invocation, nil)
		}},
	}

	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			surface, err := test.construct()
			if surface != nil {
				t.Fatalf("surface = %T, want nil", surface)
			}
			if err == nil || !strings.Contains(err.Error(), test.role+" collaborator is required") {
				t.Fatalf("error = %v, want missing %s collaborator", err, test.role)
			}
		})
	}

	var typedNilSession *sessionCollaboratorFake
	surface, err := NewSessionAPISurface(typedNilSession, model, definition, invocation, durable)
	if surface != nil || err == nil || !strings.Contains(err.Error(), "session collaborator is required") {
		t.Fatalf("typed-nil result = (%T, %v), want missing session error", surface, err)
	}
}

func TestComposedSessionAPISurfaceDelegatesSessionOperations(t *testing.T) {
	session, model, definition, invocation, durable := composedSurfaceFakes()
	ctxKey := struct{}{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey, "preserved"))
	cancel()
	wantFactory := factoryapi.Factory{Name: "current"}
	wantResult := work.WorkRequestSubmitResult{RequestID: "request-1", Accepted: true}
	wantErr := errors.New("session failure")
	currentCalls := 0
	submitCalls := 0
	session.getCurrentFactory = func(got context.Context) (factoryapi.Factory, error) {
		currentCalls++
		if got.Value(ctxKey) != "preserved" {
			t.Fatal("GetCurrentFactory did not receive the original context")
		}
		return wantFactory, nil
	}
	session.submitWorkRequestForSession = func(
		got context.Context,
		sessionID string,
		_ work.WorkRequest,
	) (work.WorkRequestSubmitResult, error) {
		submitCalls++
		if sessionID != "session-1" {
			t.Fatalf("session ID = %q, want session-1", sessionID)
		}
		if !errors.Is(got.Err(), context.Canceled) {
			t.Fatalf("context error = %v, want canceled", got.Err())
		}
		return wantResult, wantErr
	}

	surface := mustComposeSurface(t, session, model, definition, invocation, durable)
	gotFactory, err := surface.GetCurrentFactory(ctx)
	if err != nil || !reflect.DeepEqual(gotFactory, wantFactory) {
		t.Fatalf("GetCurrentFactory() = (%+v, %v), want (%+v, nil)", gotFactory, err, wantFactory)
	}
	gotResult, err := surface.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{})
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(gotResult, wantResult) {
		t.Fatalf("SubmitWorkRequestForSession() = (%+v, %v), want (%+v, %v)", gotResult, err, wantResult, wantErr)
	}
	if currentCalls != 1 || submitCalls != 1 {
		t.Fatalf("session call counts = current:%d submit:%d, want 1 each", currentCalls, submitCalls)
	}
}

func TestComposedSessionAPISurfaceDelegatesModelOperations(t *testing.T) {
	session, model, definition, invocation, durable := composedSurfaceFakes()
	wantList := factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "model-1"}}}
	wantDetail := factoryapi.ModelDetail{Name: "model-1"}
	wantErr := errors.New("model failure")
	listCalls := 0
	getCalls := 0
	model.listModels = func(context.Context) (factoryapi.ListModelsResponse, error) {
		listCalls++
		return wantList, nil
	}
	model.getModel = func(_ context.Context, name string) (factoryapi.ModelDetail, error) {
		getCalls++
		if name != "model-1" {
			t.Fatalf("model name = %q, want model-1", name)
		}
		return wantDetail, wantErr
	}

	surface := mustComposeSurface(t, session, model, definition, invocation, durable)
	gotList, err := surface.ListModels(context.Background())
	if err != nil || !reflect.DeepEqual(gotList, wantList) {
		t.Fatalf("ListModels() = (%+v, %v), want (%+v, nil)", gotList, err, wantList)
	}
	gotDetail, err := surface.GetModel(context.Background(), "model-1")
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(gotDetail, wantDetail) {
		t.Fatalf("GetModel() = (%+v, %v), want (%+v, %v)", gotDetail, err, wantDetail, wantErr)
	}
	if listCalls != 1 || getCalls != 1 {
		t.Fatalf("model call counts = list:%d get:%d, want 1 each", listCalls, getCalls)
	}
}

func TestComposedSessionAPISurfaceDelegatesFactoryDefinitionOperations(t *testing.T) {
	session, model, definition, invocation, durable := composedSurfaceFakes()
	wantFactory := factoryapi.Factory{Name: "editable"}
	wantErr := errors.New("save failure")
	readCalls := 0
	saveCalls := 0
	definition.getCurrentFactoryForSession = func(_ context.Context, sessionID string) (factoryapi.Factory, error) {
		readCalls++
		if sessionID != "session-2" {
			t.Fatalf("read session ID = %q, want session-2", sessionID)
		}
		return wantFactory, nil
	}
	definition.saveFactoryForSession = func(
		_ context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error) {
		saveCalls++
		if sessionID != "session-2" || mode != factoryapi.FactorySaveMode("REPLACE_CURRENT") || !reflect.DeepEqual(request, wantFactory) {
			t.Fatalf("save arguments = (%q, %q, %+v)", sessionID, mode, request)
		}
		return wantFactory, wantErr
	}

	surface := mustComposeSurface(t, session, model, definition, invocation, durable)
	gotFactory, err := surface.GetCurrentFactoryForSession(context.Background(), "session-2")
	if err != nil || !reflect.DeepEqual(gotFactory, wantFactory) {
		t.Fatalf("GetCurrentFactoryForSession() = (%+v, %v), want (%+v, nil)", gotFactory, err, wantFactory)
	}
	gotFactory, err = surface.SaveFactoryForSession(
		context.Background(),
		"session-2",
		factoryapi.FactorySaveMode("REPLACE_CURRENT"),
		wantFactory,
	)
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(gotFactory, wantFactory) {
		t.Fatalf("SaveFactoryForSession() = (%+v, %v), want (%+v, %v)", gotFactory, err, wantFactory, wantErr)
	}
	if readCalls != 1 || saveCalls != 1 {
		t.Fatalf("definition call counts = read:%d save:%d, want 1 each", readCalls, saveCalls)
	}
}

func TestComposedSessionAPISurfaceDelegatesInvocationOperations(t *testing.T) {
	session, model, definition, invocation, durable := composedSurfaceFakes()
	wantResult := FactoryInvocationResult{RequestID: "request-3", SessionID: "session-3"}
	wantErr := errors.New("invocation failure")
	calls := 0
	invocation.invokeFactorySession = func(
		_ context.Context,
		sessionID string,
		_ factoryapi.InvocationRequest,
	) (FactoryInvocationResult, error) {
		calls++
		if sessionID != "session-3" {
			t.Fatalf("session ID = %q, want session-3", sessionID)
		}
		if calls == 1 {
			return wantResult, nil
		}
		return wantResult, wantErr
	}

	surface := mustComposeSurface(t, session, model, definition, invocation, durable)
	for call, expectedErr := range []error{nil, wantErr} {
		got, err := surface.InvokeFactorySession(context.Background(), "session-3", factoryapi.InvocationRequest{})
		if !errors.Is(err, expectedErr) || !reflect.DeepEqual(got, wantResult) {
			t.Fatalf("call %d = (%+v, %v), want (%+v, %v)", call+1, got, err, wantResult, expectedErr)
		}
	}
	if calls != 2 {
		t.Fatalf("invocation calls = %d, want 2", calls)
	}
}

func TestComposedSessionAPISurfaceDelegatesDurableExecutionOperations(t *testing.T) {
	session, model, definition, invocation, durable := composedSurfaceFakes()
	wantAsync := factoryapi.FactorySessionExecutionResponse{SessionId: "durable-1"}
	wantSync := factoryapi.FactorySessionSyncExecutionResponse{SessionId: "durable-1"}
	wantErr := errors.New("durable execution failure")
	asyncCalls := 0
	syncCalls := 0
	durable.startAsync = func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		asyncCalls++
		return wantAsync, nil
	}
	durable.startSync = func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionSyncExecutionResponse, error) {
		syncCalls++
		return wantSync, wantErr
	}

	surface := mustComposeSurface(t, session, model, definition, invocation, durable)
	gotAsync, err := surface.StartDurableFactorySessionAsync(context.Background(), factoryapi.FactorySessionExecutionRequest{})
	if err != nil || !reflect.DeepEqual(gotAsync, wantAsync) {
		t.Fatalf("StartDurableFactorySessionAsync() = (%+v, %v), want (%+v, nil)", gotAsync, err, wantAsync)
	}
	gotSync, err := surface.StartDurableFactorySessionSync(context.Background(), factoryapi.FactorySessionExecutionRequest{})
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(gotSync, wantSync) {
		t.Fatalf("StartDurableFactorySessionSync() = (%+v, %v), want (%+v, %v)", gotSync, err, wantSync, wantErr)
	}
	if asyncCalls != 1 || syncCalls != 1 {
		t.Fatalf("durable call counts = async:%d sync:%d, want 1 each", asyncCalls, syncCalls)
	}
}

func composedSurfaceFakes() (
	*sessionCollaboratorFake,
	*modelCollaboratorFake,
	*factoryDefinitionCollaboratorFake,
	*invocationCollaboratorFake,
	*durableExecutionCollaboratorFake,
) {
	return &sessionCollaboratorFake{},
		&modelCollaboratorFake{},
		&factoryDefinitionCollaboratorFake{},
		&invocationCollaboratorFake{},
		&durableExecutionCollaboratorFake{}
}

func mustComposeSurface(
	t *testing.T,
	session SessionAPI,
	model ModelAPI,
	definition FactorySaveAPI,
	invocation InvocationAPI,
	durable DurableSessionAPI,
) SessionAPISurface {
	t.Helper()
	surface, err := NewSessionAPISurface(session, model, definition, invocation, durable)
	if err != nil {
		t.Fatalf("NewSessionAPISurface() error = %v", err)
	}
	return surface
}

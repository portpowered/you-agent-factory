package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type completionRegistration struct {
	register func(context.Context, roles.SessionOpeningFacts, factoryruntime.RuntimeRecord, factoryruntime.RuntimeInitialCompletion, factoryruntime.RuntimeReplacementBuilder, factoryruntime.RuntimeLifecycle, factoryruntime.RuntimeSidecars, factoryruntime.Clock, *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error)
	sessions map[string]*livesession.LiveSession
}

func (fake *completionRegistration) RegisterOpening(ctx context.Context, facts roles.SessionOpeningFacts, record factoryruntime.RuntimeRecord, completion factoryruntime.RuntimeInitialCompletion, replacement factoryruntime.RuntimeReplacementBuilder, lifecycle factoryruntime.RuntimeLifecycle, sidecars factoryruntime.RuntimeSidecars, clock factoryruntime.Clock, logger *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error) {
	return fake.register(ctx, facts, record, completion, replacement, lifecycle, sidecars, clock, logger)
}
func (fake *completionRegistration) Resolve(id string) *livesession.LiveSession {
	return fake.sessions[id]
}

func TestRuntimeOpeningCompletionRetainsSelectedRecordAcrossRegistryChange(t *testing.T) {
	t.Parallel()
	session := &completionSession{}
	selected := &runtimebinding.SessionState{Owner: session}
	newer := &runtimebinding.SessionState{Owner: &completionSession{}}
	registration := &completionRegistration{sessions: map[string]*livesession.LiveSession{}}
	registration.register = func(context.Context, roles.SessionOpeningFacts, factoryruntime.RuntimeRecord, factoryruntime.RuntimeInitialCompletion, factoryruntime.RuntimeReplacementBuilder, factoryruntime.RuntimeLifecycle, factoryruntime.RuntimeSidecars, factoryruntime.Clock, *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error) {
		record := &livesession.LiveSession{ID: "selected", Handle: selected}
		// A concurrent registration can replace the key before completion resumes.
		registration.sessions["selected"] = &livesession.LiveSession{ID: "selected", Handle: newer}
		return session, record, nil, nil, nil, nil
	}
	routing := completionRouting{bind: func(string, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway) error {
		return nil
	}, unbind: func(string) { t.Fatal("stale completion unbound the newer record") }}
	host := completionHost(func(roles.LifecycleRuntime, factorysessions.RuntimeHostRequest, *zap.Logger) (roles.ProcessRuntime, error) {
		return nil, nil
	})
	mockWorkers := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{ID: "selected-worker"}}}
	cleanup := &runtimeOpeningCleanup{}
	result, err := NewRuntimeOpeningCompletion(registration, routing, nil, host).Complete(t.Context(),
		RuntimeCompletionRequest{Facts: roles.SessionOpeningFacts{FactorySessionID: "selected"}, MockWorkers: mockWorkers},
		nil, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, nil, nil, openingCoordinatorClock{}, zap.NewNop(), cleanup)
	if err != nil || result.State != selected || result.Lifecycle != session || !reflect.DeepEqual(selected.MockWorkersConfig(), mockWorkers) || newer.MockWorkersConfig() != nil ||
		runtimebinding.SessionStateFrom(registration.Resolve("selected")) != newer {
		t.Fatalf("completion retargeted its acquired record: %+v, %v", result, err)
	}
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
}

type completionRouting struct {
	bind   func(string, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway) error
	unbind func(string)
}

func (fake completionRouting) Bind(id string, host factorysessions.DefinitionHost, activation factorysessions.DefinitionActivationGateway) error {
	return fake.bind(id, host, activation)
}
func (fake completionRouting) Unbind(id string) { fake.unbind(id) }

type completionHost func(roles.LifecycleRuntime, factorysessions.RuntimeHostRequest, *zap.Logger) (roles.ProcessRuntime, error)

func (fake completionHost) Bind(lifecycle roles.LifecycleRuntime, request factorysessions.RuntimeHostRequest, logger *zap.Logger) (roles.ProcessRuntime, error) {
	return fake(lifecycle, request, logger)
}

type completionWebhooks func(context.Context, webhooks.StartRequest) (webhooks.Subscription, error)

func (fake completionWebhooks) Start(ctx context.Context, request webhooks.StartRequest) (webhooks.Subscription, error) {
	return fake(ctx, request)
}

type completionSession struct{ roles.ApplicationRuntime }

type completionSelectedSession struct {
	completionSession
	factoryruntime.Service
	bind func(string, factoryruntime.RuntimeBinding) error
}

func (session *completionSelectedSession) BindRuntime(id string, binding factoryruntime.RuntimeBinding) error {
	return session.bind(id, binding)
}

func TestRuntimeOpeningCompletionRetainsSelectedRuntimeCapabilities(t *testing.T) {
	t.Parallel()
	for _, available := range []bool{false, true} {
		t.Run(fmt.Sprint(available), func(t *testing.T) {
			t.Parallel()
			cause := &factorysessions.DetachedRequestError{Message: "selected publication failed"}
			calls := 0
			binding := (factoryruntime.RuntimeBinding{}).New("selected", bindingLegacyRuntime{})
			selected := &completionSelectedSession{bind: func(id string, got factoryruntime.RuntimeBinding) error {
				calls++
				if id != "selected" || got != binding {
					t.Fatal("publication substituted selected identity")
				}
				return cause
			}}
			var session roles.ApplicationRuntime = &completionSession{}
			if available {
				session = selected
			}
			registration := &completionRegistration{sessions: map[string]*livesession.LiveSession{}}
			registration.register = func(context.Context, roles.SessionOpeningFacts, factoryruntime.RuntimeRecord, factoryruntime.RuntimeInitialCompletion, factoryruntime.RuntimeReplacementBuilder, factoryruntime.RuntimeLifecycle, factoryruntime.RuntimeSidecars, factoryruntime.Clock, *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error) {
				registration.sessions["selected"] = &livesession.LiveSession{ID: "selected", Handle: &runtimebinding.SessionState{Owner: session.(runtimebinding.SessionProjectionOwner)}}
				return session, registration.sessions["selected"], nil, nil, nil, nil
			}
			routing := completionRouting{bind: func(string, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway) error {
				return nil
			}, unbind: func(string) {}}
			host := completionHost(func(roles.LifecycleRuntime, factorysessions.RuntimeHostRequest, *zap.Logger) (roles.ProcessRuntime, error) {
				registration.sessions["selected"] = &livesession.LiveSession{ID: "selected", Handle: &runtimebinding.SessionState{Owner: &completionSession{}}}
				return nil, nil
			})
			cleanup := &runtimeOpeningCleanup{}
			result, err := NewRuntimeOpeningCompletion(registration, routing, nil, host).Complete(t.Context(),
				RuntimeCompletionRequest{Facts: roles.SessionOpeningFacts{FactorySessionID: "selected"}},
				nil, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, nil, nil, openingCoordinatorClock{}, zap.NewNop(), cleanup)
			if err != nil || calls != 0 {
				t.Fatalf("completion activated publication: %v calls=%d", err, calls)
			}
			if available {
				if result.RuntimeService != selected || result.BindRuntime == nil {
					t.Fatal("completion lost selected capabilities")
				}
				if err := result.BindRuntime("selected", binding); !errors.Is(err, cause) || calls != 1 {
					t.Fatalf("selected publication cause: %v calls=%d", err, calls)
				}
			} else if result.RuntimeService != nil || result.BindRuntime != nil {
				t.Fatal("completion invented optional capabilities")
			}
			if err := cleanup.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func (*completionSession) BuildSessionProjectionContext(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	return factorysessions.ProjectionContext{}, nil
}

type completionRuntime struct {
	inertHostedInstance
	bind func(models.RuntimeScopeRef) error
}

func (fake completionRuntime) BindModelsRuntimeScope(scope models.RuntimeScopeRef) error {
	return fake.bind(scope)
}

type completionLoaded struct {
	factorydefinitions.LoadedFactorySource
	config *factorydefinitions.FactoryConfig
}

func (source completionLoaded) FactoryConfig() *factorydefinitions.FactoryConfig {
	return source.config
}
func (completionLoaded) FactoryDir() string { return "selected-factory" }

func (completionLoaded) RuntimeBaseDir() string { return "selected-runtime" }

// This unit observes only completion: all four collaborators and acquired
// runtime handles are controlled, without constructing Wire or a session tree.
type completionSelectionFixture struct {
	operation      *RuntimeOpeningCompletion
	registration   *completionRegistration
	logger         *zap.Logger
	clock          factoryruntime.Clock
	process        roles.ProcessRuntime
	calls          *[]string
	definitionHost factorysessions.DefinitionHost
	activation     factorysessions.DefinitionActivationGateway
}

func newCompletionSelectionFixture(t *testing.T) completionSelectionFixture {
	t.Helper()
	calls := []string{}
	registration := &completionRegistration{sessions: map[string]*livesession.LiveSession{}}
	logger, clock := zap.NewNop(), openingCoordinatorClock{}
	definitionHost := &struct{ factorysessions.DefinitionHost }{}
	activation := &struct {
		factorysessions.DefinitionActivationGateway
	}{}
	process := &struct{ roles.ProcessRuntime }{}
	routing := completionRouting{
		bind: func(id string, gotHost factorysessions.DefinitionHost, gotActivation factorysessions.DefinitionActivationGateway) error {
			if gotHost != definitionHost || gotActivation != activation {
				t.Fatal("routing replaced returned handles")
			}
			calls = append(calls, id+":route")
			return nil
		},
		unbind: func(id string) { calls = append(calls, id+":unroute") },
	}
	hooks := completionWebhooks(func(_ context.Context, request webhooks.StartRequest) (webhooks.Subscription, error) {
		id := request.Scope.FactorySessionID
		if len(request.Definitions) != 1 || request.RuntimeSource == nil || request.DeadLetterPath == "" {
			t.Fatalf("webhook request: %#v", request)
		}
		calls = append(calls, id+":webhook")
		return func(ctx context.Context) error {
			if ctx.Err() != nil {
				t.Fatal("canceled webhook cleanup")
			}
			calls = append(calls, id+":unsubscribe")
			return nil
		}, nil
	})
	host := completionHost(func(lifecycle roles.LifecycleRuntime, request factorysessions.RuntimeHostRequest, gotLogger *zap.Logger) (roles.ProcessRuntime, error) {
		if gotLogger != logger {
			t.Fatal("host logger substituted")
		}
		wrapped, ok := lifecycle.(*successorBoardStartup)
		if !ok || wrapped.publish == nil {
			t.Fatal("publication lifecycle was lost")
		}
		if err := wrapped.publish(context.Background()); err != nil {
			t.Fatal(err)
		}
		if request.Host != "selected-"+request.WorkFile || !request.MockWorkers {
			t.Fatalf("host selection: %#v", request)
		}
		calls = append(calls, request.WorkFile+":host")
		return process, nil
	})
	operation := NewRuntimeOpeningCompletion(registration, routing, hooks, host)
	return completionSelectionFixture{operation: operation, registration: registration, logger: logger, clock: clock, process: process, calls: &calls, definitionHost: definitionHost, activation: activation}
}

func (fixture completionSelectionFixture) open(t *testing.T, id string) (RuntimeCompletionResult, *runtimeOpeningCleanup) {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	initial := &factoryruntime.RuntimeInitialOpening{
		Record:     &struct{ factoryruntime.RuntimeRecord }{},
		Completion: factoryruntime.RuntimeInitialCompletion{SessionID: id, MetricsSessionID: id + "-metrics", CanonicalSessionIDGenerated: true, ResumeSourceCanonicalSessionID: id + "-source"},
		ReplacementBuilder: &struct {
			factoryruntime.RuntimeReplacementBuilder
		}{},
		Lifecycle: &struct {
			factoryruntime.RuntimeLifecycle
		}{},
		Sidecars:   &struct{ factoryruntime.RuntimeSidecars }{},
		Activation: &factoryruntime.RuntimeActivation{Close: func(context.Context) error { t.Fatal("completion consumed activation cleanup"); return nil }},
	}
	session := &completionSession{}
	facts := roles.SessionOpeningFacts{FactorySessionID: id, RuntimeID: id + "-runtime", GenerationID: id + "-generation", ModelsScope: scope, BackendScopeID: id + "-backend"}
	fixture.registration.register = func(_ context.Context, got roles.SessionOpeningFacts, gotRecord factoryruntime.RuntimeRecord, gotCompletion factoryruntime.RuntimeInitialCompletion, gotReplacement factoryruntime.RuntimeReplacementBuilder, gotLifecycle factoryruntime.RuntimeLifecycle, gotSidecars factoryruntime.RuntimeSidecars, gotClock factoryruntime.Clock, gotLogger *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error) {
		if got != facts || gotRecord != initial.Record || gotCompletion != initial.Completion || gotReplacement != initial.ReplacementBuilder || gotLifecycle != initial.Lifecycle || gotSidecars != initial.Sidecars || gotClock != fixture.clock || gotLogger != fixture.logger {
			t.Fatal("registration selections substituted")
		}
		fixture.registration.sessions[id] = &livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Owner: session}}
		*fixture.calls = append(*fixture.calls, id+":register")
		return session, fixture.registration.sessions[id], fixture.definitionHost, fixture.activation, func(ctx context.Context) error {
			if ctx.Err() != nil {
				t.Fatal("canceled registration cleanup")
			}
			delete(fixture.registration.sessions, id)
			*fixture.calls = append(*fixture.calls, id+":release")
			return nil
		}, nil
	}
	mock := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{ID: id, WorkerName: id}}}
	request := RuntimeCompletionRequest{Facts: facts, ActivateWebhooks: true, MockWorkers: mock,
		LoadedFactory:       completionLoaded{config: &factorydefinitions.FactoryConfig{Webhooks: []factorydefinitions.FactoryWebhookConfig{{Enabled: true}}}},
		Host:                factorysessions.RuntimeHostRequest{Host: "selected-" + id, WorkFile: id, MockWorkers: true},
		PublishCurrentBoard: func(context.Context) error { *fixture.calls = append(*fixture.calls, id+":publish"); return nil },
	}
	cleanup := &runtimeOpeningCleanup{}

	runtime := completionRuntime{bind: func(got models.RuntimeScopeRef) error {
		if got != scope {
			t.Fatal("Models scope substituted")
		}
		*fixture.calls = append(*fixture.calls, id+":models")
		return nil
	}}
	result, err := fixture.operation.Complete(t.Context(), request, initial.Record, initial.Completion, initial.ReplacementBuilder, initial.Lifecycle, initial.Sidecars, runtime.RecordingLedger(), runtime.BindModelsRuntimeScope, fixture.clock, fixture.logger, cleanup)
	if err != nil || result.Lifecycle.(*successorBoardStartup).LifecycleRuntime != session || result.ProcessRuntime != fixture.process {
		t.Fatalf("completion: %#v %v", result, err)
	}

	bound := runtimebinding.SessionStateFrom(fixture.registration.Resolve(id))
	if !reflect.DeepEqual(bound.MockWorkersConfig(), mock) {
		t.Fatal("mock selection was lost")
	}
	return result, cleanup
}

func TestRuntimeOpeningCompletionSelectionsAndOrdering(t *testing.T) {
	t.Parallel()
	fixture := newCompletionSelectionFixture(t)
	if len(*fixture.calls) != 0 {
		t.Fatal("construction activated a collaborator")
	}
	selected := map[string]RuntimeCompletionResult{}
	cleanups := map[string]*runtimeOpeningCleanup{}
	for _, id := range []string{"first", "peer"} {
		selected[id], cleanups[id] = fixture.open(t, id)
	}
	if selected["first"].State.(*runtimebinding.SessionState).Owner == selected["peer"].State.(*runtimebinding.SessionState).Owner {
		t.Fatal("overlapping openings shared runtime")
	}
	want := []string{"first:webhook", "first:register", "first:models", "first:route", "first:publish", "first:host", "peer:webhook", "peer:register", "peer:models", "peer:route", "peer:publish", "peer:host"}
	if !reflect.DeepEqual(*fixture.calls, want) {
		t.Fatalf("sequence: %v", *fixture.calls)
	}
	if err := cleanups["first"].Close(); err != nil {
		t.Fatal(err)
	}
	if fixture.registration.Resolve("peer") == nil {
		t.Fatal("candidate cleanup removed peer")
	}
	if err := cleanups["first"].Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual((*fixture.calls)[len(want):], []string{"first:unroute", "first:release", "first:unsubscribe"}) {
		t.Fatalf("release sequence: %v", *fixture.calls)
	}
	if err := cleanups["peer"].Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeOpeningCompletionFailureRetainsTypedCauseAndRetryableCleanup(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"webhook", "registration", "models", "routing", "host"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			cause := &factorysessions.DetachedRequestError{Field: "controlled", Message: stage}
			releaseCause := errors.New("release needs retry")
			releaseCalls, hookClose, routeClose := 0, 0, 0
			registration := &completionRegistration{sessions: map[string]*livesession.LiveSession{}}
			session := &completionSession{}
			registration.register = func(context.Context, roles.SessionOpeningFacts, factoryruntime.RuntimeRecord, factoryruntime.RuntimeInitialCompletion, factoryruntime.RuntimeReplacementBuilder, factoryruntime.RuntimeLifecycle, factoryruntime.RuntimeSidecars, factoryruntime.Clock, *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error) {
				var err error
				if stage == "registration" {
					err = cause
				}
				return session, registration.sessions["candidate"], nil, nil, func(ctx context.Context) error {
					if ctx.Err() != nil || ctx.Value(completionOwnershipKey{}) != "selected" {
						t.Fatal("release lost uncancelled ownership")
					}
					releaseCalls++
					if releaseCalls == 1 {
						return releaseCause
					}
					return nil
				}, err
			}
			hooks := completionWebhooks(func(context.Context, webhooks.StartRequest) (webhooks.Subscription, error) {
				if stage == "webhook" {
					return nil, cause
				}
				return func(ctx context.Context) error {
					if ctx.Err() != nil {
						t.Fatal("webhook cleanup canceled")
					}
					hookClose++
					return nil
				}, nil
			})
			routing := completionRouting{bind: func(string, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway) error {
				if stage == "routing" {
					return cause
				}
				return nil
			}, unbind: func(string) { routeClose++ }}
			host := completionHost(func(roles.LifecycleRuntime, factorysessions.RuntimeHostRequest, *zap.Logger) (roles.ProcessRuntime, error) {
				if stage == "host" {
					return nil, cause
				}
				return nil, nil
			})
			runtime := completionRuntime{bind: func(models.RuntimeScopeRef) error {
				if stage == "models" {
					return cause
				}
				return nil
			}}
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), completionOwnershipKey{}, "selected"))
			cancel()
			cleanup := &runtimeOpeningCleanup{}
			request := RuntimeCompletionRequest{Facts: roles.SessionOpeningFacts{FactorySessionID: "candidate"}, ActivateWebhooks: true, LoadedFactory: completionLoaded{config: &factorydefinitions.FactoryConfig{Webhooks: []factorydefinitions.FactoryWebhookConfig{{Enabled: true}}}}}
			result, err := NewRuntimeOpeningCompletion(registration, routing, hooks, host).Complete(ctx, request, nil, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, runtime.RecordingLedger(), runtime.BindModelsRuntimeScope, openingCoordinatorClock{}, zap.NewNop(), cleanup)
			assertCompletionFailureCleanup(t, stage, cause, releaseCause, result, err, cleanup, &releaseCalls, &hookClose, &routeClose)
		})
	}
}

func TestRuntimeOpeningCompletionStaleAndRepeatedReleasePreserveNewerAndPeer(t *testing.T) {
	t.Parallel()
	registration := &completionRegistration{sessions: map[string]*livesession.LiveSession{}}
	old, newer, peer := &completionSession{}, &completionSession{}, &completionSession{}
	registration.sessions["candidate"] = &livesession.LiveSession{Handle: &runtimebinding.SessionState{Owner: old}}
	registration.sessions["peer"] = &livesession.LiveSession{Handle: &runtimebinding.SessionState{Owner: peer}}
	registration.register = func(context.Context, roles.SessionOpeningFacts, factoryruntime.RuntimeRecord, factoryruntime.RuntimeInitialCompletion, factoryruntime.RuntimeReplacementBuilder, factoryruntime.RuntimeLifecycle, factoryruntime.RuntimeSidecars, factoryruntime.Clock, *zap.Logger) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway, func(context.Context) error, error) {
		return old, registration.sessions["candidate"], nil, nil, nil, nil
	}
	unbound := []string{}
	routing := completionRouting{bind: func(string, factorysessions.DefinitionHost, factorysessions.DefinitionActivationGateway) error {
		return nil
	}, unbind: func(id string) { unbound = append(unbound, id) }}
	host := completionHost(func(roles.LifecycleRuntime, factorysessions.RuntimeHostRequest, *zap.Logger) (roles.ProcessRuntime, error) {
		return nil, nil
	})
	cleanup := &completionCapturedCleanup{}
	_, err := NewRuntimeOpeningCompletion(registration, routing, nil, host).Complete(t.Context(), RuntimeCompletionRequest{Facts: roles.SessionOpeningFacts{FactorySessionID: "candidate"}}, nil, factoryruntime.RuntimeInitialCompletion{}, nil, nil, nil, nil, func(models.RuntimeScopeRef) error { return nil }, openingCoordinatorClock{}, zap.NewNop(), cleanup)
	if err != nil {
		t.Fatal(err)
	}
	registration.sessions["candidate"] = &livesession.LiveSession{Handle: &runtimebinding.SessionState{Owner: newer}}
	for range 2 {
		for _, action := range cleanup.actions {
			if err := action(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(unbound) != 0 || runtimebinding.SessionStateFrom(registration.Resolve("candidate")).Owner != newer || runtimebinding.SessionStateFrom(registration.Resolve("peer")).Owner != peer {
		t.Fatal("stale release retargeted a live registration")
	}
	delete(registration.sessions, "candidate")
	for _, action := range cleanup.actions {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(unbound, []string{"candidate"}) {
		t.Fatalf("absent owner unbind: %v", unbound)
	}
}

type completionCapturedCleanup struct{ actions []func() error }

func (cleanup *completionCapturedCleanup) Add(action func() error) {
	cleanup.actions = append(cleanup.actions, action)
}

type completionOwnershipKey struct{}

func assertCompletionFailureCleanup(t *testing.T, stage string, cause, errorRelease error, result RuntimeCompletionResult, err error, cleanup *runtimeOpeningCleanup, releaseCalls, hookClose, routeClose *int) {
	t.Helper()
	var typed *factorysessions.DetachedRequestError
	if !errors.Is(err, cause) || !errors.As(err, &typed) || !reflect.ValueOf(result).IsZero() {
		t.Fatalf("completion error: %#v %v", result, err)
	}
	closeErr := cleanup.Close()
	if stage != "webhook" && (!errors.Is(closeErr, errorRelease) || *releaseCalls != 1 || *hookClose != 1) {
		t.Fatalf("first cleanup: %v releases=%d hooks=%d", closeErr, *releaseCalls, *hookClose)
	}
	if !errors.Is(errors.Join(err, closeErr), cause) {
		t.Fatal("joining cleanup lost primary cause")
	}
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
	if stage != "webhook" && (*releaseCalls != 2 || *hookClose != 1) {
		t.Fatal("cleanup retried successful release or lost failed release")
	}
	if stage == "host" && *routeClose != 1 {
		t.Fatal("host failure did not unbind owned route")
	}
}

func TestRuntimeOpeningModelsBindingRetainsAcquiredCapability(t *testing.T) {
	t.Parallel()
	scope, err := (models.RuntimeScopeRef{}).Parse("selected")
	if err != nil {
		t.Fatal(err)
	}
	cause := &factorysessions.DetachedRequestError{Message: "selected Models binding failed"}
	calls := 0
	runtime := completionRuntime{bind: func(got models.RuntimeScopeRef) error {
		calls++
		if got != scope {
			t.Fatal("Models binding substituted the selected scope")
		}
		return cause
	}}
	bind := openedModelsScopeBinding(runtime)
	runtime.bind = func(models.RuntimeScopeRef) error { t.Fatal("binding retargeted a later handle"); return nil }
	if bind == nil || calls != 0 {
		t.Fatal("capability selection activated Models binding")
	}
	if err := bind(scope); !errors.Is(err, cause) || calls != 1 {
		t.Fatalf("selected binding: %v calls=%d", err, calls)
	}
	if openedModelsScopeBinding(inertHostedInstance{}) != nil {
		t.Fatal("absent optional capability became a binding")
	}
}

func TestRuntimeOpeningBindingRejectsAbsentSelectedState(t *testing.T) {
	t.Parallel()
	state := openingState(nil)
	if state != nil {
		t.Fatal("absent selected record became a nonnil mutation handle")
	}
	err := (&RuntimeOpeningBinding{}).Bind(t.Context(), RuntimeOpeningBindingRequest{}, state,
		nil, nil, nil, nil, nil, nil, nil)
	if err == nil || err.Error() != "construct runtime scope: completed session state is unavailable" {
		t.Fatalf("absent-state binding = %v", err)
	}
}

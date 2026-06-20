package run

import (
	"bytes"
	"context"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestResolveFactoryInvocationRequest_NamedGoalInputSourcesMatchSharedResolver(t *testing.T) {
	planSprint := "Plan the sprint"
	stdinText := "Ship the feature from stdin"

	tests := []struct {
		name       string
		cfg        RunConfig
		wantSource invocations.InputSourceLabel
		wantText   string
	}{
		{
			name: "positional text",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: &planSprint,
				StdinIsTTY:               func() bool { return true },
			},
			wantSource: invocations.InputSourcePositionalText,
			wantText:   planSprint,
		},
		{
			name: "explicit stdin text",
			cfg: RunConfig{
				Dir:                 "/tmp/builtin-goal",
				NamedFactoryName:    goal.PackagedFactoryName,
				InvocationStdinText: &stdinText,
				StdinIsTTY:          func() bool { return true },
			},
			wantSource: invocations.InputSourceStdinText,
			wantText:   stdinText,
		},
		{
			name: "piped non-tty stdin",
			cfg: RunConfig{
				Dir:              "/tmp/builtin-goal",
				NamedFactoryName: goal.PackagedFactoryName,
				Stdin:            strings.NewReader("Ship from pipe\n"),
				StdinIsTTY:       func() bool { return false },
			},
			wantSource: invocations.InputSourceStdinText,
			wantText:   "Ship from pipe\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request, invocationMode, err := resolveFactoryInvocationRequest(tc.cfg)
			if err != nil {
				t.Fatalf("resolveFactoryInvocationRequest: %v", err)
			}
			if !invocationMode {
				t.Fatal("expected invocation mode for named goal input source")
			}
			assertInvocationRequestMatchesSharedResolver(t, request, tc.wantSource, tc.wantText)
		})
	}
}

func TestRun_NamedGoalPositionalInvocationWritesPrimaryResult(t *testing.T) {
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var output bytes.Buffer

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				assertInvocationRequestMatchesSharedResolver(t, &request, invocations.InputSourcePositionalText, text)
				return apisurface.FactoryInvocationResult{
					RequestID: "request-goal-positional",
					TraceID:   "trace-goal-positional",
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{{
						Type: interfaces.WorkContentPartTypeText,
						Text: "goal completed",
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "goal completed" {
		t.Fatalf("stdout = %q, want primary result text", got)
	}
}

func TestRun_NamedGoalExplicitStdinInvocationWritesPrimaryResult(t *testing.T) {
	preserveRunGlobals(t)

	stdinText := "Ship the feature from explicit stdin"
	var output bytes.Buffer

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				assertInvocationRequestMatchesSharedResolver(t, &request, invocations.InputSourceStdinText, stdinText)
				return apisurface.FactoryInvocationResult{
					RequestID: "request-goal-stdin",
					TraceID:   "trace-goal-stdin",
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{{
						Type: interfaces.WorkContentPartTypeText,
						Text: "goal stdin completed",
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                 "/tmp/builtin-goal",
		NamedFactoryName:    goal.PackagedFactoryName,
		InvocationStdinText: &stdinText,
		StdinIsTTY:          func() bool { return true },
		Output:              &output,
		Port:                7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "goal stdin completed" {
		t.Fatalf("stdout = %q, want primary result text", got)
	}
}

func TestRun_NamedGoalPipedStdinInvocationWritesPrimaryResult(t *testing.T) {
	preserveRunGlobals(t)

	stdinText := "Ship from pipe\n"
	var output bytes.Buffer

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				assertInvocationRequestMatchesSharedResolver(t, &request, invocations.InputSourceStdinText, stdinText)
				return apisurface.FactoryInvocationResult{
					RequestID: "request-goal-pipe",
					TraceID:   "trace-goal-pipe",
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{{
						Type: interfaces.WorkContentPartTypeText,
						Text: "goal pipe completed",
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:              "/tmp/builtin-goal",
		NamedFactoryName: goal.PackagedFactoryName,
		Stdin:            strings.NewReader(stdinText),
		StdinIsTTY:       func() bool { return false },
		Output:           &output,
		Port:             7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "goal pipe completed" {
		t.Fatalf("stdout = %q, want primary result text", got)
	}
}

func assertInvocationRequestMatchesSharedResolver(
	t *testing.T,
	request *factoryapi.InvocationRequest,
	source invocations.InputSourceLabel,
	text string,
) {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.SourceKind != factoryapi.InvocationInputSourceKindText {
		t.Fatalf("sourceKind = %q, want text", request.SourceKind)
	}

	sources := invocations.TextInputSources{}
	switch source {
	case invocations.InputSourcePositionalText:
		sources.PositionalText = &text
	case invocations.InputSourceStdinText:
		sources.StdinText = &text
	default:
		t.Fatalf("unsupported source label %q", source)
	}

	resolved, err := invocations.ResolveTextInput(sources)
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}
	want := invocationRequestFromResolvedInput(resolved)
	if got := extractInvocationText(t, request); got != extractInvocationText(t, want) {
		t.Fatalf("invocation text = %q, want %q", got, extractInvocationText(t, want))
	}
	if request.SourceKind != want.SourceKind {
		t.Fatalf("sourceKind = %q, want %q", request.SourceKind, want.SourceKind)
	}
}

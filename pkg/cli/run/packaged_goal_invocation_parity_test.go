package run

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/service"
)

const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"

func TestNamedGoalCLIAndAPIInvocationRequestsMatchForSameLogicalText(t *testing.T) {
	apiRequest, err := invocationRequestFromLogicalAPIText(namedGoalParityText)
	if err != nil {
		t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
	}

	stdinText := namedGoalParityText
	tests := []struct {
		name string
		cfg  RunConfig
	}{
		{
			name: "positional cli",
			cfg: RunConfig{
				Dir:                      "/tmp/builtin-goal",
				NamedFactoryName:         goal.PackagedFactoryName,
				InvocationPositionalText: stringPtr(namedGoalParityText),
				StdinIsTTY:               func() bool { return true },
			},
		},
		{
			name: "explicit stdin cli",
			cfg: RunConfig{
				Dir:                 "/tmp/builtin-goal",
				NamedFactoryName:    goal.PackagedFactoryName,
				InvocationStdinText: stringPtr(namedGoalParityText),
				StdinIsTTY:          func() bool { return true },
			},
		},
		{
			name: "piped stdin cli",
			cfg: RunConfig{
				Dir:              "/tmp/builtin-goal",
				NamedFactoryName: goal.PackagedFactoryName,
				Stdin:            strings.NewReader(stdinText),
				StdinIsTTY:       func() bool { return false },
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cliRequest, invocationMode, err := resolveFactoryInvocationRequest(tc.cfg)
			if err != nil {
				t.Fatalf("resolveFactoryInvocationRequest: %v", err)
			}
			if !invocationMode {
				t.Fatal("expected invocation mode for named goal parity input source")
			}
			assertEquivalentInvocationRequests(t, cliRequest, apiRequest)
		})
	}
}

func TestRun_NamedGoalInvocationSuccessParityAcrossCLIAndAPIEnvelope(t *testing.T) {
	preserveRunGlobals(t)

	sharedResult := apisurface.FactoryInvocationResult{
		RequestID: "request-goal-parity-success",
		TraceID:   "trace-goal-parity-success",
		Status:    factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "goal parity completed",
		}},
	}

	var textOutput bytes.Buffer
	var jsonOutput bytes.Buffer
	invoke := func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		if sessionID != defaultFactorySessionID {
			t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
		}
		apiRequest, err := invocationRequestFromLogicalAPIText(namedGoalParityText)
		if err != nil {
			t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
		}
		assertEquivalentInvocationRequests(t, &request, apiRequest)
		return sharedResult, nil
	}

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: invoke,
		}, nil
	}

	baseCfg := RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: stringPtr(namedGoalParityText),
		StdinIsTTY:               func() bool { return true },
		Port:                     7437,
	}

	if err := Run(context.Background(), withRunOutput(baseCfg, &textOutput)); err != nil {
		t.Fatalf("Run text output: %v", err)
	}
	if got := textOutput.String(); got != "goal parity completed" {
		t.Fatalf("stdout = %q, want primary result text", got)
	}

	jsonCfg := baseCfg
	jsonCfg.JSONOutput = true
	if err := Run(context.Background(), withRunOutput(jsonCfg, &jsonOutput)); err != nil {
		t.Fatalf("Run json output: %v", err)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace(jsonOutput.Bytes()), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\n%s", err, jsonOutput.String())
	}
	assertInvocationResponseMatchesFactoryResult(t, cliResponse, sharedResult)
}

func TestNamedGoalInvocationSourceConflictParityAcrossCLIAndAPIContract(t *testing.T) {
	text := "Plan from args"
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

	cliCfg := RunConfig{
		Dir:                      "/tmp/builtin-goal",
		NamedFactoryName:         goal.PackagedFactoryName,
		InvocationPositionalText: &text,
		Stdin:                    strings.NewReader("Plan from stdin\n"),
		StdinIsTTY:               func() bool { return false },
	}
	_, _, cliErr := resolveFactoryInvocationRequest(cliCfg)
	assertStableSourceConflictError(t, cliErr)
	assertStableInvocationSourceConflictMessage(t, cliErr.Error(), conflictMessage)

	apiErr := &invocations.InputError{
		Code:    invocations.InputErrorCodeSourceConflict,
		Message: conflictMessage,
	}
	assertStableInvocationSourceConflictMessage(t, apiErr.Error(), conflictMessage)
}

func assertStableInvocationSourceConflictMessage(t *testing.T, got string, wantMessage string) {
	t.Helper()

	for _, fragment := range []string{
		string(invocations.InputSourcePositionalText),
		string(invocations.InputSourceStdinText),
		wantMessage,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("error = %q, want fragment %q", got, fragment)
		}
	}
}

func invocationRequestFromLogicalAPIText(text string) (*factoryapi.InvocationRequest, error) {
	resolved, err := invocations.ResolveAPITextInputContent([]interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: text,
	}})
	if err != nil {
		return nil, err
	}
	return invocationRequestFromResolvedInput(resolved), nil
}

func assertEquivalentInvocationRequests(
	t *testing.T,
	cliRequest *factoryapi.InvocationRequest,
	apiRequest *factoryapi.InvocationRequest,
) {
	t.Helper()

	if cliRequest == nil || apiRequest == nil {
		t.Fatal("invocation request = nil")
	}
	if cliRequest.SourceKind != apiRequest.SourceKind {
		t.Fatalf("sourceKind = %q, want %q", cliRequest.SourceKind, apiRequest.SourceKind)
	}
	if got := extractInvocationText(t, cliRequest); got != extractInvocationText(t, apiRequest) {
		t.Fatalf("invocation text = %q, want %q", got, extractInvocationText(t, apiRequest))
	}
}

func assertInvocationResponseMatchesFactoryResult(
	t *testing.T,
	response factoryapi.InvocationResponse,
	result apisurface.FactoryInvocationResult,
) {
	t.Helper()

	if response.RequestId != result.RequestID {
		t.Fatalf("requestId = %q, want %q", response.RequestId, result.RequestID)
	}
	if response.TraceId != result.TraceID {
		t.Fatalf("traceId = %q, want %q", response.TraceId, result.TraceID)
	}
	if response.Status != result.Status {
		t.Fatalf("status = %q, want %q", response.Status, result.Status)
	}
	assertGeneratedWorkContentPartsFromResponse(t, response.PrimaryResult, result.PrimaryResult)
}

func assertGeneratedWorkContentPartsFromResponse(
	t *testing.T,
	content *factoryapi.WorkContent,
	want []interfaces.WorkContentPart,
) {
	t.Helper()

	if content == nil {
		t.Fatal("primary result content = nil")
	}
	if len(*content) != len(want) {
		t.Fatalf("primary result parts = %d, want %d", len(*content), len(want))
	}
	for i, part := range want {
		gotPart, err := (*content)[i].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("AsWorkTextContentPart[%d]: %v", i, err)
		}
		if gotPart.Text != part.Text {
			t.Fatalf("primary result[%d].text = %q, want %q", i, gotPart.Text, part.Text)
		}
	}
}

func withRunOutput(cfg RunConfig, output *bytes.Buffer) RunConfig {
	cfg.Output = output
	return cfg
}

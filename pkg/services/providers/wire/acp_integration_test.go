package wire_test

import (
	"context"
	"errors"
	"os"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

const providersACPModeEnvironment = "YOU_TEST_PROVIDERS_ACP_MODE"

type integrationExecutableLocator struct{}

func (integrationExecutableLocator) LookPath(file string) (string, error) { return file, nil }

func TestProvidersACPAgentProcess(t *testing.T) {
	mode := os.Getenv(providersACPModeEnvironment)
	if mode != "block" && mode != "isolate" && mode != "fail" && mode != "success" {
		return
	}
	agent := &providersIntegrationAgent{mode: mode, sessionID: os.Getenv("YOU_TEST_ACP_SESSION_ID")}
	connection := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.connection = connection
	<-connection.Done()
}

type providersIntegrationAgent struct {
	connection *acpsdk.AgentSideConnection
	mode       string
	sessionID  string
}

func (*providersIntegrationAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{ProtocolVersion: acpsdk.ProtocolVersionNumber, AgentCapabilities: acpsdk.AgentCapabilities{}}, nil
}

func (a *providersIntegrationAgent) NewSession(context.Context, acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	sessionID := a.sessionID
	if sessionID == "" {
		sessionID = "providers-integration-session"
	}
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(sessionID)}, nil
}

func (a *providersIntegrationAgent) Prompt(ctx context.Context, request acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	if a.mode == "block" {
		if signal := os.Getenv("YOU_TEST_ACP_PROMPT_SIGNAL"); signal != "" {
			_ = os.WriteFile(signal, []byte("prompt-started"), 0o600)
		}
		<-ctx.Done()
		return acpsdk.PromptResponse{}, context.Cause(ctx)
	}
	if a.mode == "fail" {
		return acpsdk.PromptResponse{}, errors.New("functional ACP prompt failure")
	}
	if err := a.connection.SessionUpdate(ctx, acpsdk.SessionNotification{
		SessionId: request.SessionId,
		Update:    acpsdk.UpdateAgentMessageText("fresh ACP COMPLETE"),
	}); err != nil {
		return acpsdk.PromptResponse{}, err
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*providersIntegrationAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}
func (*providersIntegrationAgent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, nil
}
func (*providersIntegrationAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}
func (*providersIntegrationAgent) CloseSession(context.Context, acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	return acpsdk.CloseSessionResponse{}, nil
}
func (*providersIntegrationAgent) ListSessions(context.Context, acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, nil
}
func (*providersIntegrationAgent) ResumeSession(context.Context, acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, nil
}
func (*providersIntegrationAgent) SetSessionConfigOption(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, nil
}
func (*providersIntegrationAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

var _ acpsdk.Agent = (*providersIntegrationAgent)(nil)

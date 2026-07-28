package http

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

var (
	// ErrInvalidExecuteRequest reports malformed or incomplete execute HTTP inputs
	// at the Providers execute adapter edge.
	ErrInvalidExecuteRequest = errors.New("invalid provider execution request")
)

// SessionRefResponse is the adapter-owned HTTP shape for one detached provider
// session reference.
type SessionRefResponse struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

// ExecuteProgressResponse is the adapter-owned HTTP shape for one in-flight
// execute progress fact.
type ExecuteProgressResponse struct {
	Phase    string            `json:"phase"`
	Detail   string            `json:"detail,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ExecuteDiagnosticsResponse is the adapter-owned HTTP shape for execute
// diagnostics on success or failure.
type ExecuteDiagnosticsResponse struct {
	DurationMillis int64                     `json:"durationMillis,omitempty"`
	Progress       []ExecuteProgressResponse `json:"progress,omitempty"`
	Metadata       map[string]string         `json:"metadata,omitempty"`
}

// ExecuteResponse is the adapter-owned HTTP success shape for one provider
// execute attempt.
type ExecuteResponse struct {
	Content     string                      `json:"content"`
	SessionRef  *SessionRefResponse         `json:"sessionRef,omitempty"`
	Diagnostics *ExecuteDiagnosticsResponse `json:"diagnostics,omitempty"`
}

// ExecuteRequestBody is the adapter-owned HTTP request body for one provider
// execute attempt.
type ExecuteRequestBody struct {
	AttemptID          string             `json:"attemptId"`
	WorkerType         string             `json:"workerType,omitempty"`
	WorkstationName    string             `json:"workstationName,omitempty"`
	Model              string             `json:"model,omitempty"`
	SkipPermissions    bool               `json:"skipPermissions,omitempty"`
	SystemPrompt       string             `json:"systemPrompt,omitempty"`
	UserMessage        string             `json:"userMessage,omitempty"`
	InputTokens        []any              `json:"inputTokens,omitempty"`
	OutputSchema       string             `json:"outputSchema,omitempty"`
	ResumeSession      *SessionRefResponse `json:"resumeSession,omitempty"`
	WorkingDirectory   string             `json:"workingDirectory,omitempty"`
	Worktree           string             `json:"worktree,omitempty"`
	EnvVars            map[string]string  `json:"envVars,omitempty"`
	ProcessEnvironment []string           `json:"processEnvironment,omitempty"`
}

// ExecuteInput carries decoded HTTP inputs for one execute operation owned by
// this adapter.
type ExecuteInput struct {
	ProviderID string
	Body       io.Reader
}

// ExecuteRequestFromHTTP maps one execute HTTP request into the accepted
// Providers root request vocabulary.
func ExecuteRequestFromHTTP(input ExecuteInput) (providers.ExecuteRequest, error) {
	providerID := strings.TrimSpace(input.ProviderID)
	request := providers.ExecuteRequest{Provider: providers.ID(providerID)}
	if err := request.Provider.Validate(); err != nil {
		return providers.ExecuteRequest{}, err
	}

	body, err := decodeExecuteRequestBody(input.Body)
	if err != nil {
		return providers.ExecuteRequest{}, err
	}

	request.AttemptID = strings.TrimSpace(body.AttemptID)
	if request.AttemptID == "" {
		return providers.ExecuteRequest{}, ErrInvalidExecuteRequest
	}
	request.WorkerType = strings.TrimSpace(body.WorkerType)
	request.WorkstationName = strings.TrimSpace(body.WorkstationName)
	request.Model = strings.TrimSpace(body.Model)
	request.SkipPermissions = body.SkipPermissions
	request.SystemPrompt = body.SystemPrompt
	request.UserMessage = body.UserMessage
	request.InputTokens = body.InputTokens
	request.OutputSchema = strings.TrimSpace(body.OutputSchema)
	request.WorkingDirectory = strings.TrimSpace(body.WorkingDirectory)
	request.Worktree = strings.TrimSpace(body.Worktree)
	request.EnvVars = body.EnvVars
	request.ProcessEnvironment = body.ProcessEnvironment
	if body.ResumeSession != nil {
		resume, err := sessionRefFromHTTP(*body.ResumeSession)
		if err != nil {
			return providers.ExecuteRequest{}, err
		}
		request.ResumeSession = &resume
	}
	if err := request.Validate(); err != nil {
		return providers.ExecuteRequest{}, err
	}
	return request, nil
}

// ExecuteResponseToHTTP encodes one fake-root execute result into the
// adapter-owned HTTP success response shape.
func ExecuteResponseToHTTP(result providers.ExecuteResult) ExecuteResponse {
	response := ExecuteResponse{Content: result.Content}
	if result.SessionRef != nil {
		session := sessionRefToHTTP(*result.SessionRef)
		response.SessionRef = &session
	}
	if result.Diagnostics != nil {
		diagnostics := executeDiagnosticsToHTTP(*result.Diagnostics)
		response.Diagnostics = &diagnostics
	}
	return response
}

func decodeExecuteRequestBody(body io.Reader) (ExecuteRequestBody, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = io.ReadAll(body)
		if err != nil {
			return ExecuteRequestBody{}, err
		}
	}
	if len(payload) == 0 {
		return ExecuteRequestBody{}, ErrInvalidExecuteRequest
	}
	var request ExecuteRequestBody
	if err := json.Unmarshal(payload, &request); err != nil {
		return ExecuteRequestBody{}, ErrInvalidExecuteRequest
	}
	return request, nil
}

func sessionRefFromHTTP(input SessionRefResponse) (providers.SessionRef, error) {
	ref := providers.SessionRef{
		Provider: providers.ID(strings.TrimSpace(input.Provider)),
		Kind:     strings.TrimSpace(input.Kind),
		ID:       strings.TrimSpace(input.ID),
	}
	if err := ref.Validate(); err != nil {
		return providers.SessionRef{}, err
	}
	return ref, nil
}

func sessionRefToHTTP(ref providers.SessionRef) SessionRefResponse {
	return SessionRefResponse{
		Provider: ref.Provider.String(),
		Kind:     ref.Kind,
		ID:       ref.ID,
	}
}

func executeDiagnosticsToHTTP(diagnostics providers.ExecuteDiagnostics) ExecuteDiagnosticsResponse {
	response := ExecuteDiagnosticsResponse{
		DurationMillis: diagnostics.DurationMillis,
		Metadata:       diagnostics.Metadata,
	}
	if len(diagnostics.Progress) > 0 {
		response.Progress = make([]ExecuteProgressResponse, 0, len(diagnostics.Progress))
		for _, progress := range diagnostics.Progress {
			response.Progress = append(response.Progress, ExecuteProgressResponse{
				Phase:    progress.Phase,
				Detail:   progress.Detail,
				Metadata: progress.Metadata,
			})
		}
	}
	return response
}

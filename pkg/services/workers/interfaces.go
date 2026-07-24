// Package workers defines worker dispatcher contracts and compatibility helpers
// for script and model-based workers.
package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

const maxProviderIdentityLength = 128

// ProviderIdentity is the provider-neutral identity accepted at authored and
// runtime boundaries.
type ProviderIdentity string

// Validate checks that the identity uses its canonical authored form.
func (identity ProviderIdentity) Validate() error {
	if identity == "" {
		return fmt.Errorf("must not be empty")
	}
	if len(identity) > maxProviderIdentityLength {
		return fmt.Errorf("must not exceed %d bytes", maxProviderIdentityLength)
	}
	if !canonicalProviderIdentifier(string(identity)) {
		return fmt.Errorf("must start with a lowercase letter and contain only lowercase letters, digits, dots, or hyphens")
	}
	return nil
}

func canonicalProviderIdentifier(value string) bool {
	for index, character := range value {
		if character >= 'a' && character <= 'z' {
			continue
		}
		if index > 0 && (character >= '0' && character <= '9' || character == '.' || character == '-') {
			if character != '.' && character != '-' || index+1 < len(value) {
				continue
			}
		}
		return false
	}
	return !strings.Contains(value, "..") && !strings.Contains(value, "--") &&
		!strings.Contains(value, ".-") && !strings.Contains(value, "-.")
}

// HostedPollerClock, HostedPollerHTTPDoer, and HostedPollerSecretResolver are
// the Workers-owned external-effect contracts used to construct hosted worker
// pollers. Cross-service consumers name these root contracts instead of
// importing Workers implementation packages.
type HostedPollerClock interface {
	After(time.Duration) <-chan time.Time
}

type HostedPollerHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// HostedRuntimePaths is the minimum runtime view needed to resolve a hosted
// worker credential. Factory Definition runtime lookups satisfy this contract
// without becoming part of the Workers public dependency surface.
type HostedRuntimePaths interface {
	FactoryDir() string
	RuntimeBaseDir() string
}

type HostedPollerSecretResolver func(
	context.Context,
	HostedRuntimePaths,
	string,
) (string, error)

// FactoryDocsLoader loads the documentation bundled beneath one Factory root.
// Wire selects the production filesystem implementation; tests may replace the
// exact operation at the process edge.
type FactoryDocsLoader func(factoryDirectory string) (map[string]string, error)

// ResolveExecutableSymlinks resolves the installed provider executable before
// Workers fingerprints it for capability negotiation.
type ResolveExecutableSymlinks func(path string) (string, error)

// OperatingSystem is the process operating-system fact used by provider
// command construction. It is observed once by Wire, never by Worker logic.
type OperatingSystem string

// WorktreeFileSystem is the exact host-filesystem effect required to locate,
// validate, and reserve Factory-local Git worktree checkouts. Wire selects the
// production adapter; Worker code never falls back to process-global IO.
type WorktreeFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	Lstat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
}

// AgentToolFileSystem is the exact host-filesystem effect used by bounded
// Worker agent tools. Wire selects its implementation and agent-run execution
// fails closed when the effect is absent.
type AgentToolFileSystem interface {
	Abs(string) (string, error)
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]fs.DirEntry, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

// WorktreeGitCommander is the exact external process effect used by Worktree
// preparation. Keeping Git execution at this edge lets tests replace it
// without constructing an alternate Worker runtime.
type WorktreeGitCommander interface {
	Run(context.Context, string, ...string) (stdout string, stderr string, exitCode int, err error)
}

// FactoryWorktreePreparation describes a checkout selected for one Worker.
type FactoryWorktreePreparation struct {
	CheckoutPath string
	Reused       bool
}

// FactoryWorktreePreparer is the narrow Worker-owned role consumed by a
// workstation executor after Wire constructs it from exact external effects.
type FactoryWorktreePreparer interface {
	Prepare(context.Context, string, string) (FactoryWorktreePreparation, error)
}

// InvocationInput is one canonical, single-attempt Worker invocation.
// Retry policy belongs to the calling service.
type InvocationInput struct {
	Request ProviderInferenceRequest
	Attempt int
}

// InvocationResult is the canonical result returned by one Worker invocation.
type InvocationResult struct {
	Response        InferenceResponse
	Attempt         int
	ProviderSession *ProviderSessionMetadata
	FailureMetadata *WorkFailureMetadata
	FailureDecision *WorkFailureDecision
	FailureDetail   *FailureDetail
	Diagnostics     *SafeWorkDiagnostics
}

// InvocationExecutor is the public Worker service boundary for one invocation
// attempt. Provider selection and normalization remain Worker implementation
// details.
type InvocationExecutor interface {
	Execute(context.Context, InvocationInput) (InvocationResult, error)
}

// WorkerExecutor executes one dispatched Worker step.
type WorkerExecutor interface {
	Execute(context.Context, work.WorkDispatch) (WorkResult, error)
}

// WorkstationRequestExecutor handles execution after workstation resolution.
type WorkstationRequestExecutor interface {
	Execute(context.Context, WorkstationExecutionRequest) (WorkResult, error)
}

// Runner executes one normalized Worker runner request.
type Runner interface {
	Execute(context.Context, RunnerExecutionRequest) (RunnerExecutionResult, error)
}

func cloneInputTokens(rawTokens []any) []Token {
	if len(rawTokens) == 0 {
		return nil
	}

	out := make([]Token, 0, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := decodeToken(raw)
		if !ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func clonePetriInputTokens(inputTokens []Token) []any {
	if len(inputTokens) == 0 {
		return nil
	}

	out := make([]any, 0, len(inputTokens))
	for _, token := range inputTokens {
		out = append(out, token)
	}
	return out
}

func decodeToken(raw any) (Token, bool) {
	if token, ok := raw.(Token); ok {
		return token, true
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return Token{}, false
	}
	var token Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return Token{}, false
	}
	return token, true
}

// InputTokens converts typed petri tokens into the shared dispatch representation.
func InputTokens(tokens ...Token) []any {
	return clonePetriInputTokens(tokens)
}

// WorkDispatchInputTokens returns the token payload as typed petri tokens.
func WorkDispatchInputTokens(dispatch work.WorkDispatch) []Token {
	return cloneInputTokens(dispatch.InputTokens)
}

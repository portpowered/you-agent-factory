package runtimeapifixture

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type runtimeAPICommandProvider struct {
	testutil.NativeProvider
	runner platformprocess.CommandRunner
}

func newRuntimeAPICommandProvider(runner platformprocess.CommandRunner) providers.Service {
	provider := &runtimeAPICommandProvider{runner: runner}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider *runtimeAPICommandProvider) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		command = request.Provider.CanonicalSessionProvider()
	}
	commandRequest := platformprocess.CommandRequest{
		Command:                  command,
		Args:                     append([]string(nil), request.Args...),
		Stdin:                    []byte(request.UserMessage),
		Env:                      append([]string(nil), request.ProcessEnvironment...),
		WorkDir:                  request.WorkingDirectory,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
	result, err := provider.runner.Run(ctx, commandRequest)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	if failure := runtimeAPICommandFailure(result); failure != nil {
		return providers.ExecuteResult{}, failure
	}
	return providers.ExecuteResult{Content: runtimeAPICommandContent(command, result.Stdout)}, nil
}

func runtimeAPICommandFailure(result platformprocess.CommandResult) error {
	if result.ExitCode == 0 && len(result.Stderr) == 0 {
		return nil
	}
	message := strings.TrimSpace(string(result.Stderr))
	lower := strings.ToLower(message)
	kind := providers.ExecuteFailureKindUnknown
	switch {
	case strings.Contains(lower, "rate_limit"), strings.Contains(lower, "429"), strings.Contains(lower, "thrott"):
		kind = providers.ExecuteFailureKindThrottled
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "401"):
		kind = providers.ExecuteFailureKindAuthentication
	case strings.Contains(lower, "invalid"):
		kind = providers.ExecuteFailureKindInvalidRequest
	}
	if message == "" {
		message = "provider command failed"
	}
	return providers.ExecuteFailure{Kind: kind, Message: message}
}

func runtimeAPICommandContent(command string, stdout []byte) string {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		typeName, _ := record["type"].(string)
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "codex":
			if typeName != "item.completed" {
				continue
			}
			item, _ := record["item"].(map[string]any)
			text, _ := item["text"].(string)
			if text != "" {
				return text
			}
		case "claude":
			if typeName != "result" {
				continue
			}
			text, _ := record["result"].(string)
			if text != "" {
				return text
			}
		}
	}
	return trimmed
}

var _ providers.Service = (*runtimeAPICommandProvider)(nil)

package providers

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

func codexStructuredStreamStdout(message string) []byte {
	record, err := json.Marshal(map[string]any{
		"type":  "turn.failed",
		"error": map[string]string{"message": message},
	})
	if err != nil {
		panic(err)
	}
	return append(record, '\n')
}

func codexProcessExitResult(stderr string, exitCode int) provider.CommandResult {
	return provider.CommandResult{ExitCode: exitCode, Stderr: []byte(stderr)}
}

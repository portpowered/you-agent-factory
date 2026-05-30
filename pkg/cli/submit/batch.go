// Package submit implements agent-factory submit command behavior.
package submit

import "fmt"

// BatchConfig holds parameters for the submit batch command.
type BatchConfig struct {
	FilePath  string
	FileFlag  string
	DryRun    bool
	Server    string
	SessionID string
	JSON      bool
	Verbose   bool
	Debug     bool
	Args      []string
}

// SubmitBatch upserts a canonical FACTORY_REQUEST_BATCH to a running factory.
// Input resolution, HTTP, and output formatting are implemented in later stories.
func SubmitBatch(cfg BatchConfig) error {
	return fmt.Errorf("you submit batch is not yet available")
}

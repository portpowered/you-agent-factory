package climanifestparity

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

const ExecutionBaselinePath = "contracts/testdata/baseline/cli-command-execution.json"

// ExecutionBaseline is approved execution metadata for CLI command parity checks.
type ExecutionBaseline struct {
	FormatVersion string                            `json:"formatVersion"`
	Commands      map[string]ExecutionBaselineCommand `json:"commands"`
}

// ExecutionBaselineCommand carries approved side-effect and constraint evidence.
type ExecutionBaselineCommand struct {
	SideEffectKinds []string              `json:"sideEffectKinds"`
	Constraints     climanifest.Constraints `json:"constraints"`
}

// LoadExecutionBaseline decodes the approved CLI execution baseline document.
func LoadExecutionBaseline(path string) (ExecutionBaseline, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ExecutionBaseline{}, fmt.Errorf("read CLI execution baseline %s: %w", path, err)
	}
	var baseline ExecutionBaseline
	if err := json.Unmarshal(raw, &baseline); err != nil {
		return ExecutionBaseline{}, fmt.Errorf("decode CLI execution baseline: %w", err)
	}
	if baseline.FormatVersion == "" {
		return ExecutionBaseline{}, fmt.Errorf("CLI execution baseline missing formatVersion")
	}
	if len(baseline.Commands) == 0 {
		return ExecutionBaseline{}, fmt.Errorf("CLI execution baseline missing commands")
	}
	return baseline, nil
}

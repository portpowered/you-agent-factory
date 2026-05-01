package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

const (
	metadataFactoryHash        = "factory_hash"
	metadataWorkersHash        = "workers_hash"
	metadataWorkstationsHash   = "workstations_hash"
	metadataRuntimeConfigHash  = "runtime_config_hash"
	metadataReplaySourceFormat = "source_format"
)

// MetadataMismatchWarning describes replay artifact metadata that differs from
// the current checkout's loadable runtime config.
type MetadataMismatchWarning struct {
	Key      string
	Artifact string
	Current  string
}

// EmbeddedRuntimeConfig is the canonical runtime lookup reconstructed from an
// artifact's embedded configuration. It intentionally avoids filesystem reads.
type EmbeddedRuntimeConfig struct {
	Factory          *interfaces.FactoryConfig
	FactoryDirPath   string
	WorkerConfigs    map[string]*interfaces.WorkerConfig
	Workstations     map[string]*interfaces.FactoryWorkstationConfig
	WorkersByID      map[string]*interfaces.WorkerConfig
	WorkstationsByID map[string]*interfaces.FactoryWorkstationConfig
}

var _ interfaces.RuntimeConfigLookup = (*EmbeddedRuntimeConfig)(nil)

// FactoryConfig returns the embedded canonical public factory configuration.
func (c *EmbeddedRuntimeConfig) FactoryConfig() *interfaces.FactoryConfig {
	if c == nil {
		return nil
	}
	return c.Factory
}

// FactoryDir returns the authored factory root embedded in the replay artifact.
func (c *EmbeddedRuntimeConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.FactoryDirPath
}

// RuntimeBaseDir returns the effective execution base for relative runtime
// paths during replay-backed execution. Replay artifacts do not carry a
// separate runtime-base override, so relative runtime paths fall back to the
// embedded factory root.
func (c *EmbeddedRuntimeConfig) RuntimeBaseDir() string {
	return c.FactoryDir()
}

// Worker returns the embedded worker definition for the configured worker name.
func (c *EmbeddedRuntimeConfig) Worker(name string) (*interfaces.WorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.WorkerConfigs[name]
	return def, ok
}

// Workstation returns the embedded workstation definition for the configured workstation name.
func (c *EmbeddedRuntimeConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	def, ok := c.Workstations[name]
	return def, ok
}

func sha256JSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "sha256:error"
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:]))
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

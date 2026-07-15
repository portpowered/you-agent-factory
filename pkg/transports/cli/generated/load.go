package generated

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

//go:embed representative_family.json
var representativeFamilyJSON []byte

//go:embed session_family.json
var sessionFamilyJSON []byte

//go:embed work_family.json
var workFamilyJSON []byte

//go:embed factory_config_init_family.json
var factoryConfigInitFamilyJSON []byte

//go:embed models_docs_family.json
var modelsDocsFamilyJSON []byte

//go:embed run_submit_family.json
var runSubmitFamilyJSON []byte

//go:embed mcp_family.json
var mcpFamilyJSON []byte

//go:embed workflow_compatibility_family.json
var workflowCompatibilityFamilyJSON []byte

// RepresentativeFamilyManifest returns generated §4.3 metadata for the
// representative root/session-show command family.
func RepresentativeFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(representativeFamilyJSON, "representative")
}

// SessionFamilyManifest returns generated metadata for the complete canonical
// Factory Session command family.
func SessionFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(sessionFamilyJSON, "session")
}

// WorkFamilyManifest returns generated §4.3 metadata for the work
// inspection/control command family.
func WorkFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(workFamilyJSON, "work")
}

// FactoryConfigInitFamilyManifest returns generated §4.3 metadata for the
// factory/config/init command family.
func FactoryConfigInitFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(factoryConfigInitFamilyJSON, "factory/config/init")
}

// ModelsDocsFamilyManifest returns generated §4.3 metadata for the models/docs
// command family.
func ModelsDocsFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(modelsDocsFamilyJSON, "models/docs")
}

// RunSubmitFamilyManifest returns generated metadata for the run/submit family.
func RunSubmitFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(runSubmitFamilyJSON, "run/submit")
}

func MCPFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(mcpFamilyJSON, "canonical MCP")
}

func WorkflowCompatibilityFamilyManifest() (climanifest.Manifest, error) {
	return parseFamilyManifest(workflowCompatibilityFamilyJSON, "workflow compatibility")
}

func parseFamilyManifest(payload []byte, familyLabel string) (climanifest.Manifest, error) {
	var manifest climanifest.Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return climanifest.Manifest{}, fmt.Errorf("decode generated %s metadata: %w", familyLabel, err)
	}
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("generated %s metadata missing rootPath", familyLabel)
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("generated %s metadata missing commands", familyLabel)
	}
	return manifest, nil
}

// CommandByID returns one generated representative-family command record.
func CommandByID(id string) (climanifest.Command, error) {
	manifest, err := RepresentativeFamilyManifest()
	if err != nil {
		return climanifest.Command{}, err
	}
	return manifest.CommandByID(id)
}

// WorkCommandByID returns one generated work-family command record.
func WorkCommandByID(id string) (climanifest.Command, error) {
	manifest, err := WorkFamilyManifest()
	if err != nil {
		return climanifest.Command{}, err
	}
	return manifest.CommandByID(id)
}

// FactoryConfigInitCommandByID returns one generated factory/config/init command record.
func FactoryConfigInitCommandByID(id string) (climanifest.Command, error) {
	manifest, err := FactoryConfigInitFamilyManifest()
	if err != nil {
		return climanifest.Command{}, err
	}
	return manifest.CommandByID(id)
}

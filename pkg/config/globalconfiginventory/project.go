package globalconfiginventory

import (
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
)

// TopologyBaselineRelativePath is the committed global config topology inventory fixture.
const TopologyBaselineRelativePath = "pkg/config/globalconfiginventory/testdata/baseline/global-config-topology.json"

// ProjectTopologyInventory builds the deterministic global config topology inventory
// from current operatorconfig and systemconfig ownership boundaries.
func ProjectTopologyInventory() Inventory {
	return Inventory{
		FormatVersion: FormatVersion,
		SharedConfigFile: SharedConfigFile{
			RelativePath: ".you-agent-factory/config.json",
			ResolvedBy:   "defaultpaths.OperatorConfigPath(homeDir)",
		},
		SharedFileSplit: SharedFileSplit{
			Summary: "systemconfig owns backendScopeID load/generate/persist; operatorconfig owns defaults and workerPresets parse/resolve and tolerates sibling backendScopeID without claiming ownership",
			Owners: []FileOwner{
				{
					Package: ownerSystemconfig,
					Owns: []string{
						"backendScopeID",
					},
				},
				{
					Package: ownerOperatorconfig,
					Owns: []string{
						"defaults",
						"defaults.workerModelProvider",
						"defaults.workerModel",
						"workerPresets",
						"workerPresets[].id",
						"workerPresets[].modelProvider",
						"workerPresets[].model",
						"workerPresets[].reasoningEffort",
					},
					Tolerates:  []string{"backendScopeID"},
					DoesNotOwn: []string{"backendScopeID"},
				},
			},
		},
		PrecedenceChain:    operatorconfig.PrecedenceChain,
		UnknownFieldPolicy: topologyUnknownFieldPolicies(),
		Fields:             topologyFieldRecords(),
	}
}

func topologyUnknownFieldPolicies() []UnknownFieldPolicy {
	return []UnknownFieldPolicy{
		{
			Package: ownerOperatorconfig,
			Policy:  "ParseFileConfig uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, unknown nested keys, and trailing JSON values",
		},
		{
			Package: ownerSystemconfig,
			Policy:  "loadBackendScopeID unmarshals only backendScopeID and ignores other top-level keys on read; persistBackendScopeID rewrites the file through a raw-message map that preserves unrelated sibling keys",
		},
	}
}

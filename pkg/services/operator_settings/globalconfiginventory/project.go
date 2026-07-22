package globalconfiginventory

import (
	operator_settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// TopologyBaselineRelativePath is the committed global config topology inventory fixture.
const TopologyBaselineRelativePath = "pkg/services/operator_settings/globalconfiginventory/testdata/baseline/global-config-topology.json"

// ProjectTopologyInventory builds the deterministic global config topology inventory
// from the current Operator Settings service boundary.
func ProjectTopologyInventory() Inventory {
	return Inventory{
		FormatVersion: FormatVersion,
		SharedConfigFile: SharedConfigFile{
			RelativePath: ".you-agent-factory/config.json",
			ResolvedBy:   "operator_settings.DefaultConfigPath(homeDir)",
		},
		SharedFileSplit: SharedFileSplit{
			Summary: "operator_settings owns backendScopeID identity, defaults, and workerPresets behavior; its focused loaders share one service-owned config file",
			Owners: []FileOwner{
				{
					Package: ownerOperatorSettings,
					Owns: []string{
						"backendScopeID",
						"defaults",
						"defaults.workerModelProvider",
						"defaults.workerModel",
						"workerPresets",
						"workerPresets[].id",
						"workerPresets[].modelProvider",
						"workerPresets[].model",
						"workerPresets[].reasoningEffort",
					},
				},
			},
		},
		PrecedenceChain:    operator_settings.PrecedenceChain,
		UnknownFieldPolicy: topologyUnknownFieldPolicies(),
		Fields:             topologyFieldRecords(),
	}
}

func topologyUnknownFieldPolicies() []UnknownFieldPolicy {
	return []UnknownFieldPolicy{
		{
			Package: ownerOperatorSettings,
			Policy:  "settings parsing uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, unknown nested keys, and trailing JSON values",
		},
		{
			Package: ownerOperatorSettings,
			Policy:  "identity loading reads only backendScopeID and ignores other top-level keys; identity persistence rewrites through a raw-message map that preserves unrelated sibling keys",
		},
	}
}

package mockworkers

// TopologyBaselineRelativePath is the committed mock-worker topology inventory fixture.
const TopologyBaselineRelativePath = "pkg/services/workers/internal/interface/testdata/baseline/mock-workers-topology.json"

// ProjectTopologyInventory builds the deterministic mock-worker topology inventory
// from current ParseMockWorkersConfig / LoadMockWorkersConfig / Validate boundaries.
func ProjectTopologyInventory() Inventory {
	return Inventory{
		FormatVersion: FormatVersion,
		LoaderEntrypoints: []string{
			"ParseMockWorkersConfig",
			"LoadMockWorkersConfig",
			"MockWorkersConfig.Validate",
			"MockWorkerConfig.Validate",
		},
		UnknownFieldPolicy:   "ParseMockWorkersConfig ignores unknown object fields and reports sorted unique JSON paths; known-field validation and exactly one JSON document remain strict",
		EntrySelectionPolicy: "first matching mockWorkers[] entry wins; there is no response-sequence or multi-hit authoring surface beyond that first-match selection",
		RunTypeUnion:         topologyRunTypeUnion(),
		UnmatchedDispatchPolicies: []UnmatchedDispatchPolicy{
			{
				Value:           string(MockWorkerUnmatchedDispatchPolicyAccept),
				OmittedBehavior: true,
				Summary:         "default when unmatchedDispatchPolicy is omitted; unmatched dispatches return the synthetic accepted mock result",
			},
			{
				Value:   string(MockWorkerUnmatchedDispatchPolicyAccept),
				Summary: "explicit accept policy; same default-accept behavior as omission",
			},
			{
				Value:   string(MockWorkerUnmatchedDispatchPolicyPassthrough),
				Summary: "unmatched dispatches execute through the normal worker runner/provider path",
			},
		},
		ValidationBoundaries:    topologyValidationBoundaries(),
		NotAcceptedCapabilities: topologyNotAcceptedCapabilities(),
		Fields:                  topologyFieldRecords(),
	}
}

func topologyRunTypeUnion() RunTypeUnion {
	return RunTypeUnion{
		Summary: "runType is required on every mockWorkers[] entry and selects which nested config shape Validate enforces",
		Values: []RunTypeRecord{
			{
				Value:              string(MockWorkerRunTypeAccept),
				NestedConfigPolicy: "no nested config required; scriptConfig, rejectConfig, and usage are optional",
			},
			{
				Value:              string(MockWorkerRunTypeScript),
				NestedConfig:       "scriptConfig",
				NestedConfigPolicy: "scriptConfig is required; scriptConfig.command is required non-empty",
			},
			{
				Value:              string(MockWorkerRunTypeReject),
				NestedConfig:       "rejectConfig",
				NestedConfigPolicy: "rejectConfig is optional; rejectConfig.exitCode when set must be between 1 and 255; usage is optional",
			},
		},
	}
}

func topologyValidationBoundaries() []ValidationBoundary {
	return []ValidationBoundary{
		{
			Condition:    "trailing JSON after the root object",
			Owner:        ownerDecode,
			ErrorPattern: "unexpected trailing JSON",
		},
		{
			Condition:    "unknown unmatchedDispatchPolicy value",
			Owner:        ownerValidate,
			ErrorPattern: `unmatchedDispatchPolicy must be one of "accept" or "passthrough"`,
		},
		{
			Condition:    "unknown runType value",
			Owner:        ownerValidate,
			ErrorPattern: `runType must be one of "accept", "script", or "reject"`,
		},
		{
			Condition:    "runType script without scriptConfig",
			Owner:        ownerValidate,
			ErrorPattern: `scriptConfig is required when runType is "script"`,
		},
		{
			Condition:    "runType script with scriptConfig missing command",
			Owner:        ownerValidate,
			ErrorPattern: `scriptConfig.command is required when runType is "script"`,
		},
		{
			Condition:    "rejectConfig.exitCode outside 1-255 when set",
			Owner:        ownerValidate,
			ErrorPattern: "rejectConfig.exitCode must be between 1 and 255",
		},
		{
			Condition:    "usage provider or model is blank",
			Owner:        ownerValidate,
			ErrorPattern: "usage: provider is required",
		},
		{
			Condition:    "usage token count is negative",
			Owner:        ownerValidate,
			ErrorPattern: "usage: inputTokens must be non-negative",
		},
		{
			Condition:    "usage cached input exceeds input",
			Owner:        ownerValidate,
			ErrorPattern: "usage: cachedInputTokens must not exceed inputTokens",
		},
		{
			Condition:    "usage reasoning output exceeds output",
			Owner:        ownerValidate,
			ErrorPattern: "usage: reasoningOutputTokens must not exceed outputTokens",
		},
	}
}

func topologyNotAcceptedCapabilities() []NotAcceptedCapability {
	return []NotAcceptedCapability{
		{
			Category: "media",
			Reason:   "no media payload or media-response authoring fields are accepted by ParseMockWorkersConfig",
		},
		{
			Category: "dispatch delay or timing fields",
			Reason:   "no arbitrary delay or sleep is accepted; gateConfig only synchronizes a matched dispatch with explicit arrival and release files and a bounded timeout",
		},
		{
			Category: "artifact payloads",
			Reason:   "no artifact upload/download or artifact payload fields are accepted on mockWorkers entries",
		},
		{
			Category: "response sequences",
			Reason:   "no multi-step response sequence authoring exists beyond first-match mockWorkers[] entry selection",
		},
	}
}

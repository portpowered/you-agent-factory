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
		UnknownFieldPolicy:   "ParseMockWorkersConfig uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, unknown nested keys, and trailing JSON values",
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
				NestedConfigPolicy: "no nested config required; scriptConfig and rejectConfig are ignored when present",
			},
			{
				Value:              string(MockWorkerRunTypeScript),
				NestedConfig:       "scriptConfig",
				NestedConfigPolicy: "scriptConfig is required; scriptConfig.command is required non-empty",
			},
			{
				Value:              string(MockWorkerRunTypeReject),
				NestedConfig:       "rejectConfig",
				NestedConfigPolicy: "rejectConfig is optional; rejectConfig.exitCode when set must be between 1 and 255",
			},
		},
	}
}

func topologyValidationBoundaries() []ValidationBoundary {
	return []ValidationBoundary{
		{
			Condition:    "unknown top-level or nested JSON field",
			Owner:        ownerDecode,
			ErrorPattern: "unknown field",
		},
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
			Reason:   "no per-entry delay, sleep, or dispatch-timing fields are accepted; scriptConfig.timeout only bounds local script execution",
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

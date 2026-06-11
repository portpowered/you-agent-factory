package interfaces

type publicFactoryEnumNormalizerCase struct {
	name       string
	alias      string
	unknown    string
	want       string
	permissive func(string) string
	strict     func(string) string
}

func publicFactoryEnumNormalizerWorkerCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "worker type inference",
			alias:      "INFERENCE_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       WorkerTypeInference,
			permissive: PermissivePublicFactoryWorkerType,
			strict:     StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker type legacy model alias",
			alias:      "MODEL_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       WorkerTypeInference,
			permissive: PermissivePublicFactoryWorkerType,
			strict:     StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker type poller",
			alias:      "POLLER_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       WorkerTypePoller,
			permissive: PermissivePublicFactoryWorkerType,
			strict:     StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker type legacy hosted alias",
			alias:      "HOSTED_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       WorkerTypePoller,
			permissive: PermissivePublicFactoryWorkerType,
			strict:     StrictPublicFactoryWorkerType,
		},
	}
}

func publicFactoryEnumNormalizerProviderCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "worker model provider",
			alias:      "CODEX",
			unknown:    "mystery-provider",
			want:       "CODEX",
			permissive: PermissivePublicFactoryWorkerModelProvider,
			strict:     StrictPublicFactoryWorkerModelProvider,
		},
		{
			name:       "worker provider",
			alias:      "SCRIPT_WRAP",
			unknown:    "custom-executor",
			want:       "SCRIPT_WRAP",
			permissive: PermissivePublicFactoryWorkerProvider,
			strict:     StrictPublicFactoryWorkerProvider,
		},
		{
			name:       "hosted worker provider",
			alias:      "LINEAR",
			unknown:    "custom-hosted",
			want:       HostedWorkerProviderLinear,
			permissive: PermissivePublicFactoryHostedWorkerProvider,
			strict:     StrictPublicFactoryHostedWorkerProvider,
		},
		{
			name:       "worker model locality",
			alias:      "LOCAL",
			unknown:    "edge",
			want:       ModelLocalityLocal,
			permissive: PermissivePublicFactoryWorkerModelLocality,
			strict:     StrictPublicFactoryWorkerModelLocality,
		},
		{
			name:       "worker operation content type",
			alias:      "AUDIO",
			unknown:    "sound",
			want:       ModelOperationContentTypeAudio,
			permissive: PermissivePublicFactoryWorkerModelOperationContentType,
			strict:     StrictPublicFactoryWorkerModelOperationContentType,
		},
		{
			name:       "resource type",
			alias:      "MODEL",
			unknown:    "custom-resource",
			want:       ResourceTypeModel,
			permissive: PermissivePublicFactoryResourceType,
			strict:     StrictPublicFactoryResourceType,
		},
	}
}

func publicFactoryEnumNormalizerWorkstationCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "workstation type inference",
			alias:      "INFERENCE_RUN",
			unknown:    "CUSTOM_WORKSTATION",
			want:       WorkstationTypeInference,
			permissive: PermissivePublicFactoryWorkstationType,
			strict:     StrictPublicFactoryWorkstationType,
		},
		{
			name:       "workstation type legacy invoke alias",
			alias:      "MODEL_INVOKE",
			unknown:    "CUSTOM_WORKSTATION",
			want:       WorkstationTypeInference,
			permissive: PermissivePublicFactoryWorkstationType,
			strict:     StrictPublicFactoryWorkstationType,
		},
		{
			name:       "workstation type legacy model workstation alias",
			alias:      "MODEL_WORKSTATION",
			unknown:    "CUSTOM_WORKSTATION",
			want:       WorkstationTypeAgent,
			permissive: PermissivePublicFactoryWorkstationType,
			strict:     StrictPublicFactoryWorkstationType,
		},
	}
}

func publicFactoryEnumNormalizerMiscCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "runner id",
			alias:      "cursor-cli",
			unknown:    "custom-runner",
			want:       RunnerIDCursorCLI,
			permissive: PermissivePublicFactoryRunnerID,
			strict:     StrictPublicFactoryRunnerID,
		},
		{
			name:       "runner selection source",
			alias:      "factory",
			unknown:    "custom-source",
			want:       string(RunnerSelectionSourceFactory),
			permissive: PermissivePublicFactoryRunnerSelectionSource,
			strict:     StrictPublicFactoryRunnerSelectionSource,
		},
		{
			name:       "work type handling behavior",
			alias:      WorkTypeHandlingBehaviorDefault,
			unknown:    "PROMPT",
			want:       WorkTypeHandlingBehaviorDefault,
			permissive: PermissivePublicWorkTypeHandlingBehavior,
			strict:     StrictPublicWorkTypeHandlingBehavior,
		},
	}
}

func publicFactoryEnumNormalizerCases() []publicFactoryEnumNormalizerCase {
	cases := publicFactoryEnumNormalizerWorkerCases()
	cases = append(cases, publicFactoryEnumNormalizerProviderCases()...)
	cases = append(cases, publicFactoryEnumNormalizerWorkstationCases()...)
	cases = append(cases, publicFactoryEnumNormalizerMiscCases()...)
	return cases
}

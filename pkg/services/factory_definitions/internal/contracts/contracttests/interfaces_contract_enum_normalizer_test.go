package contracttests

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
			want:       interfaces.WorkerTypeInference,
			permissive: interfaces.PermissivePublicFactoryWorkerType,
			strict:     interfaces.StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker type legacy model alias",
			alias:      "MODEL_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       interfaces.WorkerTypeInference,
			permissive: interfaces.PermissivePublicFactoryWorkerType,
			strict:     interfaces.StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker type poller",
			alias:      "POLLER_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       interfaces.WorkerTypePoller,
			permissive: interfaces.PermissivePublicFactoryWorkerType,
			strict:     interfaces.StrictPublicFactoryWorkerType,
		},
		{
			name:       "worker type legacy hosted alias",
			alias:      "HOSTED_WORKER",
			unknown:    "CUSTOM_WORKER",
			want:       interfaces.WorkerTypePoller,
			permissive: interfaces.PermissivePublicFactoryWorkerType,
			strict:     interfaces.StrictPublicFactoryWorkerType,
		},
	}
}

func publicFactoryEnumNormalizerProviderCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "worker model provider",
			alias:      "CODEX",
			unknown:    "CUSTOM-PROVIDER",
			want:       "CODEX",
			permissive: interfaces.PermissivePublicFactoryWorkerModelProvider,
			strict:     interfaces.StrictPublicFactoryWorkerModelProvider,
		},
		{
			name:       "worker provider",
			alias:      "SCRIPT_WRAP",
			unknown:    "custom-executor",
			want:       "SCRIPT_WRAP",
			permissive: interfaces.PermissivePublicFactoryWorkerProvider,
			strict:     interfaces.StrictPublicFactoryWorkerProvider,
		},
		{
			name:       "hosted worker provider",
			alias:      "LINEAR",
			unknown:    "custom-hosted",
			want:       interfaces.HostedWorkerProviderLinear,
			permissive: interfaces.PermissivePublicFactoryHostedWorkerProvider,
			strict:     interfaces.StrictPublicFactoryHostedWorkerProvider,
		},
		{
			name:       "worker model locality",
			alias:      "LOCAL",
			unknown:    "edge",
			want:       workerconfig.ModelLocalityLocal,
			permissive: interfaces.PermissivePublicFactoryWorkerModelLocality,
			strict:     interfaces.StrictPublicFactoryWorkerModelLocality,
		},
		{
			name:       "worker operation content type",
			alias:      "AUDIO",
			unknown:    "sound",
			want:       workerconfig.ModelOperationContentTypeAudio,
			permissive: interfaces.PermissivePublicFactoryWorkerModelOperationContentType,
			strict:     interfaces.StrictPublicFactoryWorkerModelOperationContentType,
		},
		{
			name:       "resource type",
			alias:      "MODEL",
			unknown:    "custom-resource",
			want:       factoryresource.TypeModel,
			permissive: interfaces.PermissivePublicFactoryResourceType,
			strict:     interfaces.StrictPublicFactoryResourceType,
		},
	}
}

func TestStrictPublicFactoryWorkerModelProviderAcceptsCanonicalExtensionIdentity(t *testing.T) {
	t.Parallel()

	const identity = "customer.provider-v2"
	if got := interfaces.StrictPublicFactoryWorkerModelProvider(identity); got != identity {
		t.Fatalf("StrictPublicFactoryWorkerModelProvider(%q) = %q, want preserved identity", identity, got)
	}
	for _, malformed := range []string{
		"", " customer.provider", "Customer.provider", "customer_provider",
		"customer..provider", "customer-", "a" + strings.Repeat("b", 128),
	} {
		if got := interfaces.StrictPublicFactoryWorkerModelProvider(malformed); got != "" {
			t.Errorf("StrictPublicFactoryWorkerModelProvider(%q) = %q, want rejection", malformed, got)
		}
	}
}

func publicFactoryEnumNormalizerWorkstationCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "workstation type inference",
			alias:      "INFERENCE_RUN",
			unknown:    "CUSTOM_WORKSTATION",
			want:       interfaces.WorkstationTypeInference,
			permissive: interfaces.PermissivePublicFactoryWorkstationType,
			strict:     interfaces.StrictPublicFactoryWorkstationType,
		},
		{
			name:       "workstation type legacy invoke alias",
			alias:      "MODEL_INVOKE",
			unknown:    "CUSTOM_WORKSTATION",
			want:       interfaces.WorkstationTypeInference,
			permissive: interfaces.PermissivePublicFactoryWorkstationType,
			strict:     interfaces.StrictPublicFactoryWorkstationType,
		},
		{
			name:       "workstation type legacy model workstation alias",
			alias:      "MODEL_WORKSTATION",
			unknown:    "CUSTOM_WORKSTATION",
			want:       interfaces.WorkstationTypeAgent,
			permissive: interfaces.PermissivePublicFactoryWorkstationType,
			strict:     interfaces.StrictPublicFactoryWorkstationType,
		},
	}
}

func publicFactoryEnumNormalizerMiscCases() []publicFactoryEnumNormalizerCase {
	return []publicFactoryEnumNormalizerCase{
		{
			name:       "runner id",
			alias:      "cursor-cli",
			unknown:    "custom-runner",
			want:       workerexecution.RunnerIDCursorCLI,
			permissive: interfaces.PermissivePublicFactoryRunnerID,
			strict:     interfaces.StrictPublicFactoryRunnerID,
		},
		{
			name:       "runner selection source",
			alias:      "factory",
			unknown:    "custom-source",
			want:       string(workerexecution.RunnerSelectionSourceFactory),
			permissive: interfaces.PermissivePublicFactoryRunnerSelectionSource,
			strict:     interfaces.StrictPublicFactoryRunnerSelectionSource,
		},
		{
			name:       "work type handling behavior",
			alias:      interfaces.WorkTypeHandlingBehaviorDefault,
			unknown:    "PROMPT",
			want:       interfaces.WorkTypeHandlingBehaviorDefault,
			permissive: interfaces.PermissivePublicWorkTypeHandlingBehavior,
			strict:     interfaces.StrictPublicWorkTypeHandlingBehavior,
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

func TestPublicFactoryEnumNormalizers(t *testing.T) {
	for _, tt := range publicFactoryEnumNormalizerCases() {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.permissive("  " + tt.alias + "  "); got != tt.want {
				t.Fatalf("permissive(%q) = %q, want %q", tt.alias, got, tt.want)
			}
			if got := tt.strict("  " + tt.alias + "  "); got != tt.want {
				t.Fatalf("strict(%q) = %q, want %q", tt.alias, got, tt.want)
			}
			if got := tt.permissive("  " + tt.unknown + "  "); got != tt.unknown {
				t.Fatalf("permissive(%q) = %q, want trimmed unknown %q", tt.unknown, got, tt.unknown)
			}
			if got := tt.strict("  " + tt.unknown + "  "); got != "" {
				t.Fatalf("strict(%q) = %q, want rejection", tt.unknown, got)
			}
		})
	}
}

package providers

import (
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

func providerErrorCorpusEntryLabel(entry provider.ProviderErrorCorpusEntry) string {
	if strings.TrimSpace(entry.UpstreamSourceCase) == "" {
		return entry.Name
	}
	return entry.Name + " [" + entry.UpstreamSourceCase + "]"
}

func TestInvocationErrorCompatibility_SupportedCorpusEntriesPreserveStableWorkFailureTypes(t *testing.T) {
	corpus, err := provider.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("LoadProviderErrorCorpus() error = %v", err)
	}

	for _, entry := range corpus.Entries() {
		if !entry.Supported {
			continue
		}
		if entry.Provider == modelprovider.ProviderCodex ||
			entry.Provider == modelprovider.ProviderClaude ||
			entry.Provider == modelprovider.ProviderGemini ||
			entry.Provider == modelprovider.ProviderKiro ||
			entry.Provider == modelprovider.ProviderPi {
			continue
		}
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			providerErr := provider.NormalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			if providerErr.Type != entry.ExpectedType {
				t.Fatalf("ProviderError.Type = %q, want stable %q", providerErr.Type, entry.ExpectedType)
			}

			metadata := provider.WorkFailureMetadataFromError(providerErr)
			if metadata == nil || metadata.Type != entry.ExpectedType {
				t.Fatalf("WorkFailureMetadata.Type = %#v, want %q", metadata, entry.ExpectedType)
			}
			if metadata.Family != entry.ExpectedFamily {
				t.Fatalf("WorkFailureMetadata.Family = %q, want %q", metadata.Family, entry.ExpectedFamily)
			}

			detail := provider.SafeProviderFailureDetail(providerErr)
			if detail == nil || detail.Reason != entry.ExpectedType {
				t.Fatalf("FailureDetail.Reason = %#v, want %q", detail, entry.ExpectedType)
			}
		})
	}
}

func TestInvocationErrorCompatibility_CodexUnknownExitFallbackKeepsStableTypeWithAuditedMessage(t *testing.T) {
	t.Skip("codex exit-failure normalization moved to providers execution adapters")
}

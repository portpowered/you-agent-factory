package agentrun

import (
	"encoding/json"
	"sort"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"
)

// publishAgentFinalMessage delivers the authoritative final assistant turn to
// the request-scoped progress stream. The detached runner uses this helper
// without constructing a Workstation executor or retaining runtime state.
func publishAgentFinalMessage(
	publisher workerexecution.ProgressPublisher,
	dispatchID string,
	correlation workerexecution.ExecutionCorrelation,
	content string,
	identity ...*workerexecution.Draft,
) {
	if publisher == nil || strings.TrimSpace(content) == "" {
		return
	}
	payload, err := json.Marshal(workerexecution.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workerexecution.ContentBlock{{
			Kind: workerexecution.ContentBlockText,
			Text: strings.TrimSpace(content),
		}},
	})
	if err != nil {
		return
	}
	draft := workerexecution.Draft{
		Kind:       workerexecution.KindMessage,
		Phase:      workerexecution.PhaseCompleted,
		DispatchID: strings.TrimSpace(dispatchID),
		ItemID:     strings.TrimSpace(dispatchID) + "-final-message",
		Provenance: workerexecution.Provenance{
			Provider:        "agent-run",
			NativeEventType: "agent_final_response",
			Delivery:        workerexecution.DeliverySynthesized,
			Representation:  workerexecution.RepresentationSnapshot,
			Fidelity:        workerexecution.FidelityFinalOnly,
		},
		Payload: payload,
	}
	if len(identity) > 0 && identity[0] != nil {
		message := identity[0]
		draft.RunID, draft.TurnID, draft.ItemID = message.RunID, message.TurnID, message.ItemID
		draft.DispatchID = message.DispatchID
		draft.ParentItemID = message.ParentItemID
		draft.DeclaredSecretJSONPointers = append([]string(nil), message.DeclaredSecretJSONPointers...)
		draft.Provenance.Fidelity = workerexecution.FidelityNormalized
	}
	fragment := workerexecution.CanonicalDraftFragment(dispatchID, draft)
	fragment.Correlation = correlation
	publisher(fragment)
}

func capturedFinalContent(content string, request workerexecution.RunnerExecutionRequest) string {
	var secrets []string
	for key, value := range request.EnvVars {
		if diagnostics.ClassifyCommandEnvKey(key) == diagnostics.CommandEnvClassificationRedacted {
			secrets = append(secrets, value)
		}
	}
	for _, entry := range request.ProcessEnvironment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && diagnostics.ClassifyCommandEnvKey(key) == diagnostics.CommandEnvClassificationRedacted {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	var replacements []string
	for _, secret := range secrets {
		if secret != "" {
			replacements = append(replacements, secret, diagnostics.RedactedCommandEnvValue)
		}
	}
	return strings.NewReplacer(replacements...).Replace(content)
}

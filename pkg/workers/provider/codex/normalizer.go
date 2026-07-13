package codex

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

type commandOutputNormalizer struct {
	req             provider.CommandRequest
	publisher       provider.InferenceProgressPublisher
	decoder         *Decoder
	pendingTerminal *responseevents.Draft
}

const progressMessageLimit = 1024

// NewCommandOutputNormalizer selects exact Codex JSONL decoding only for
// commands that explicitly negotiated exec --json.
func NewCommandOutputNormalizer(req provider.CommandRequest, publisher provider.InferenceProgressPublisher) provider.CommandOutputNormalizer {
	if !hasArg(req.Args, "--json") || !isCodexCommand(req.Command) {
		return nil
	}
	return &commandOutputNormalizer{req: req, publisher: publisher, decoder: NewDecoder(adapter.DecoderContext{DispatchID: req.DispatchID})}
}

func (n *commandOutputNormalizer) Observe(stream string, chunk []byte) bool {
	if stream == workerprocess.OutputStreamStderr {
		if message := strings.TrimSpace(string(chunk)); message != "" {
			n.publisher(provider.ProgressFragment(n.req.DispatchID, nil, boundedText(message, progressMessageLimit)))
		}
		return true
	}
	if stream != workerprocess.OutputStreamStdout {
		return false
	}
	n.publishDecoded(n.decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk}))
	return true
}

func (n *commandOutputNormalizer) Flush(ctx context.Context, result provider.CommandResult, commandErr error) {
	n.publishDecoded(n.decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted}))
	if n.pendingTerminal == nil {
		return
	}
	if !terminalOutcomeOverridesNativeFailure(ctx, result, commandErr) {
		n.publishDraft(*n.pendingTerminal)
	}
	n.pendingTerminal = nil
}

func (n *commandOutputNormalizer) publishDecoded(decoded adapter.DecodeResult, err error) {
	if err != nil {
		n.publisher(provider.ProgressFragment(n.req.DispatchID, nil, "Codex JSONL decoding failed."))
	}
	for _, diagnostic := range decoded.Diagnostics {
		fragment := provider.ProgressFragment(n.req.DispatchID, nil, diagnostic.Message)
		fragment.ExternalEventType = diagnostic.Code
		n.publisher(fragment)
	}
	for index := range decoded.Drafts {
		draft := decoded.Drafts[index]
		if draft.Kind == responseevents.KindError && draft.Phase == responseevents.PhaseFailed {
			n.pendingTerminal = &draft
			continue
		}
		n.publishDraft(draft)
	}
}

func (n *commandOutputNormalizer) publishDraft(draft responseevents.Draft) {
	fragment := canonicalFragment(draft)
	fragment.CanonicalDraft = &draft
	n.publisher(fragment)
}

func terminalOutcomeOverridesNativeFailure(ctx context.Context, result provider.CommandResult, commandErr error) bool {
	return errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) || result.ExitCode == 124
}

func canonicalFragment(draft responseevents.Draft) provider.InferenceProgressFragment {
	session := providerSession(draft.ProviderSessionRef)
	fragment := provider.ProgressFragment(draft.DispatchID, session, string(draft.Kind))
	fragment.ExternalEventType = draft.Provenance.NativeEventType
	if draft.Kind == responseevents.KindMessage && draft.Phase == responseevents.PhaseCompleted {
		var payload responseevents.MessagePayload
		if json.Unmarshal(draft.Payload, &payload) == nil && len(payload.ContentBlocks) > 0 {
			fragment = provider.ResponseFragment(draft.DispatchID, session, payload.ContentBlocks[0].Text)
			fragment.ExternalEventType = draft.Provenance.NativeEventType
		}
	}
	return fragment
}

func providerSession(id string) *interfaces.ProviderSessionMetadata {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: ProviderSessionKindSessionID, ID: id}
}

func hasArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func isCodexCommand(command string) bool {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(command), `\`, "/"))
	extension := filepath.Ext(base)
	return strings.EqualFold(strings.TrimSuffix(base, extension), "codex")
}

func boundedText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

var _ provider.CommandOutputNormalizer = (*commandOutputNormalizer)(nil)

package service

import (
	"context"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// piStartupBanner is the exact text pi-acp 0.0.34 reports under
// `_meta.piAcp.startupInfo` and then replays as an agent_message_chunk outside
// the prompt turn. It is environment banner text, never model output.
const piStartupBanner = "Pi 0.81.0 - workspace /repo\nmodel: openai/gpt-5\n"

// startupMeta is the session metadata shape openSession hands back for Pi.
func startupMeta(startup string) map[string]any {
	return map[string]any{piAcpMetaKey: map[string]any{piAcpStartupInfoField: startup}}
}

// messageChunk builds the one SessionUpdate pi-acp uses both to replay the
// startup banner and to stream actual prompt output.
func messageChunk(sessionID, text string) acpsdk.SessionNotification {
	return acpsdk.SessionNotification{
		SessionId: acpsdk.SessionId(sessionID),
		Update: acpsdk.SessionUpdate{AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
			Content: acpsdk.TextBlock(text),
		}},
	}
}

// startupTurnHarness drives one turn the way daemon.execute does: reset the
// client, exchange notifications with the session/new response, then finish.
type startupTurnHarness struct {
	client   *client
	observed []providers.ExecuteProgress
}

func newStartupTurnHarness(t *testing.T) *startupTurnHarness {
	t.Helper()
	harness := &startupTurnHarness{client: &client{}}
	harness.client.reset(func(fact providers.ExecuteProgress) {
		harness.observed = append(harness.observed, fact)
	})
	return harness
}

func (h *startupTurnHarness) notify(t *testing.T, sessionID, text string) {
	t.Helper()
	if err := h.client.SessionUpdate(context.Background(), messageChunk(sessionID, text)); err != nil {
		t.Fatalf("SessionUpdate(%q) error = %v", text, err)
	}
}

// progress joins delivery and returns the turn's normalized facts, which is
// exactly the sequence the live observer saw.
func (h *startupTurnHarness) progress() []providers.ExecuteProgress {
	return h.client.completeProgress()
}

func (h *startupTurnHarness) observedKinds() []string {
	kinds := make([]string, 0, len(h.observed))
	for _, fact := range h.observed {
		kinds = append(kinds, fact.Metadata["native_type"]+"/"+fact.Phase+"/"+fact.Detail)
	}
	return kinds
}

// TestPiStartupInfoSuppressesNotificationBeforeMetadata covers the order that
// actually produces the false success: the banner notification lands while
// session/new is still in flight, so it is already accumulated when the
// metadata arrives. A failed Pi model request in that state must leave no
// result content at all instead of reporting the banner as an answer.
func TestPiStartupInfoSuppressesNotificationBeforeMetadata(t *testing.T) {
	t.Parallel()

	harness := newStartupTurnHarness(t)
	harness.notify(t, "s1", piStartupBanner)
	if got := harness.client.content(); got != piStartupBanner {
		t.Fatalf("pre-metadata content = %q, want the accumulated banner %q", got, piStartupBanner)
	}
	harness.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))

	if got := harness.client.content(); got != "" {
		t.Fatalf("content after suppression = %q, want empty so a failed request is not a success", got)
	}
	// A duplicate delivery of the same banner must stay suppressed.
	harness.notify(t, "s1", piStartupBanner)
	if got := harness.client.content(); got != "" {
		t.Fatalf("content after replayed banner = %q, want empty", got)
	}
	// Source-native progress is deliberately untouched: the notification was
	// real, and suppressing the result text must not erase the trace.
	if facts := harness.progress(); len(facts) == 0 {
		t.Fatal("suppression erased the turn's progress facts")
	}
}

// TestPiStartupInfoSuppressesNotificationAfterMetadata covers the other order,
// where the banner notification arrives only after the metadata is known, so
// it must be dropped as it is handled rather than withdrawn afterwards.
func TestPiStartupInfoSuppressesNotificationAfterMetadata(t *testing.T) {
	t.Parallel()

	harness := newStartupTurnHarness(t)
	harness.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
	harness.notify(t, "s1", piStartupBanner)
	harness.notify(t, "s1", piStartupBanner)

	if got := harness.client.content(); got != "" {
		t.Fatalf("content = %q, want empty for a banner-only failed turn", got)
	}
	if facts := harness.progress(); len(facts) == 0 {
		t.Fatal("suppression erased the turn's progress facts")
	}
}

// TestPiStartupInfoRetainsRealPromptOutput is the other half of the contract:
// only the exact banner is withheld. Real prompt output, an unrelated chunk
// that merely resembles the banner, and reasoning progress all survive.
func TestPiStartupInfoRetainsRealPromptOutput(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		order   func(*startupTurnHarness)
		want    string
		wantOut string
	}{
		{
			name: "banner before metadata then answer",
			order: func(h *startupTurnHarness) {
				h.notify(t, "s1", piStartupBanner)
				h.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
				h.notify(t, "s1", "The subagent finished.\n")
			},
			want: "The subagent finished.\n",
		},
		{
			name: "answer before banner and metadata",
			order: func(h *startupTurnHarness) {
				h.notify(t, "s1", "The subagent finished.\n")
				h.notify(t, "s1", piStartupBanner)
				h.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
			},
			want: "The subagent finished.\n",
		},
		{
			name: "banner after metadata then answer",
			order: func(h *startupTurnHarness) {
				h.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
				h.notify(t, "s1", piStartupBanner)
				h.notify(t, "s1", "The subagent finished.\n")
			},
			want: "The subagent finished.\n",
		},
		{
			name: "answer streamed before the late banner",
			order: func(h *startupTurnHarness) {
				h.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
				h.notify(t, "s1", "The subagent finished.\n")
				h.notify(t, "s1", piStartupBanner)
			},
			want: "The subagent finished.\n",
		},
		{
			name: "output only similar to the banner is kept",
			order: func(h *startupTurnHarness) {
				h.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
				h.notify(t, "s1", "Pi 0.81.0 - workspace /other\n")
			},
			want: "Pi 0.81.0 - workspace /other\n",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			harness := newStartupTurnHarness(t)
			testCase.order(harness)
			if got := harness.client.content(); got != testCase.want {
				t.Fatalf("content = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestPiStartupInfoDoesNotSuppressOtherProviderOutput proves the filter stays
// inert when the session metadata carries no Pi banner, so every other ACP
// provider keeps accumulating agent_message_chunk text unchanged.
func TestPiStartupInfoDoesNotSuppressOtherProviderOutput(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		meta map[string]any
	}{
		{name: "no metadata"},
		{name: "unrelated metadata", meta: map[string]any{"other": map[string]any{"startupInfo": piStartupBanner}}},
		{name: "banner field is not text", meta: map[string]any{piAcpMetaKey: map[string]any{piAcpStartupInfoField: 42}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			harness := newStartupTurnHarness(t)
			startup := piStartupInfo(testCase.meta)
			harness.client.suppressStartupChunk(startup)
			harness.notify(t, "s1", piStartupBanner)
			if got := harness.client.content(); got != piStartupBanner {
				t.Fatalf("content = %q, want the banner kept when no startup info was published", got)
			}
		})
	}
}

// TestPiStartupInfoResetPerTurn proves the recorded banner does not leak into
// the next turn, which would silently drop that turn's identical first chunk.
func TestPiStartupInfoResetPerTurn(t *testing.T) {
	t.Parallel()

	harness := newStartupTurnHarness(t)
	harness.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
	harness.progress()

	harness.client.reset(nil)
	harness.notify(t, "s1", piStartupBanner)
	if got := harness.client.content(); got != piStartupBanner {
		t.Fatalf("content in the following turn = %q, want the banner kept after reset", got)
	}
}

// TestPiStartupInfoPreservesProgressOrdering proves suppression only touches
// the accumulated result text: the observer still receives the banner's own
// progress fact ahead of the real answer's, exactly as the notification order
// delivered them.
func TestPiStartupInfoPreservesProgressOrdering(t *testing.T) {
	t.Parallel()

	harness := newStartupTurnHarness(t)
	harness.notify(t, "s1", piStartupBanner)
	harness.client.suppressStartupChunk(piStartupInfo(startupMeta(piStartupBanner)))
	harness.notify(t, "s1", "The subagent finished.\n")
	facts := harness.progress()

	if !reflect.DeepEqual(facts, harness.observed) {
		t.Fatalf("progress sequence and observed facts diverged:\nreturned = %#v\nobserved = %#v", facts, harness.observed)
	}
	var messages []string
	for _, fact := range facts {
		if fact.Metadata["kind"] == "message" && fact.Phase == "delta" {
			messages = append(messages, fact.Detail)
		}
	}
	want := []string{piStartupBanner, "The subagent finished.\n"}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("message deltas = %#v, want %#v", messages, want)
	}
	if got := harness.observedKinds(); len(got) == 0 {
		t.Fatalf("no observed progress kinds: %#v", got)
	}
}

// TestPiStartupInfoReadsSessionMeta pins the metadata lookup, including the
// in-process map spelling and the absent-value cases.
func TestPiStartupInfoReadsSessionMeta(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		meta map[string]any
		want string
	}{
		{name: "decoded JSON object", meta: startupMeta(piStartupBanner), want: piStartupBanner},
		{name: "in-process map", meta: map[string]any{piAcpMetaKey: map[string]string{piAcpStartupInfoField: piStartupBanner}}, want: piStartupBanner},
		{name: "no metadata", want: ""},
		{name: "empty object", meta: map[string]any{piAcpMetaKey: map[string]any{}}, want: ""},
		{name: "field absent", meta: map[string]any{piAcpMetaKey: map[string]any{"version": "0.0.34"}}, want: ""},
		{name: "vendor key is not an object", meta: map[string]any{piAcpMetaKey: piStartupBanner}, want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := piStartupInfo(testCase.meta); got != testCase.want {
				t.Fatalf("piStartupInfo(%#v) = %q, want %q", testCase.meta, got, testCase.want)
			}
		})
	}
}

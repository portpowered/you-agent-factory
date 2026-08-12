package workersessions

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderSessionObservationPublisher serializes the required Worker
// Sessions association ahead of the existing downstream progress publisher.
// Factory Runtime constructs it before Workers execution is assembled and
// binds the session-owned Service once its pool is available. A reference that
// cannot be committed is deliberately not forwarded, so no response output
// can present an unassociated, malformed, or foreign Provider Session as a
// resumable Worker Session.
type ProviderSessionObservationPublisher struct {
	mu       sync.RWMutex
	observer Service
	next     workers.ProgressPublisher

	// records serializes Worker record publication per Worker Session. Every
	// observation a Worker emits is committed to that Worker's own topic, and
	// PublishRecord requires non-decreasing SourceSequence within one source,
	// so sequence assignment and the commit that consumes it must happen under
	// one lock per session. A counter shared across sessions would interleave
	// two Workers' sequences and reject valid records.
	records   sync.Mutex
	sequences map[string]uint64
	logger    logging.Logger
}

// NewProviderSessionObservationPublisher creates an unbound progress bridge.
// Fragments without an exact typed reference remain non-resumable and continue
// to the supplied publisher, while reference-bearing fragments wait for Bind
// and a successful Worker Sessions association before they are forwarded.
func NewProviderSessionObservationPublisher(next workers.ProgressPublisher) *ProviderSessionObservationPublisher {
	return &ProviderSessionObservationPublisher{next: next}
}

// Bind attaches the one Worker Sessions service that owns the runtime's
// supervision registry. Binding is intentionally a construction-time action;
// replacing an existing observer would risk routing a live dispatch to a
// different Factory Runtime session, so only the first non-nil observer wins.
func (p *ProviderSessionObservationPublisher) Bind(observer Service) {
	if p == nil || observer == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.observer == nil {
		p.observer = observer
	}
}

// Publish commits a detached exact Provider Session observation before it
// forwards the original Workers progress fragment. This method intentionally
// has the same no-return signature as workers.ProgressPublisher: rejection is
// recorded by Worker Sessions' typed operation and safely suppresses only the
// reference-bearing output rather than fabricating fallback provider work.
func (p *ProviderSessionObservationPublisher) Publish(fragment workers.ProgressFragment) {
	if p == nil {
		return
	}
	observer, next := p.dependencies()
	if !providerFragmentAgrees(fragment) || !p.associateProviderSession(observer, fragment) {
		return
	}
	if fragment.Kind == workers.ProviderSessionObservedFragmentKind {
		return
	}
	if !p.publishWorkerObservation(observer, fragment) {
		return
	}
	if next != nil {
		next(fragment)
	}
}

func (p *ProviderSessionObservationPublisher) dependencies() (Service, workers.ProgressPublisher) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.observer, p.next
}

func providerFragmentAgrees(fragment workers.ProgressFragment) bool {
	metadata := workers.CloneProviderSessionMetadata(fragment.ProviderSessionRef)
	reference := workers.CloneProviderSessionReference(fragment.ProviderSessionReference)
	if reference != nil && metadata != nil &&
		(!sameProviderIdentity(reference.Provider.String(), metadata.Provider) ||
			reference.Kind != metadata.Kind || reference.ID != metadata.ID) {
		return false
	}
	provider := strings.TrimSpace(fragment.Provider)
	if provider != "" && reference != nil && strings.TrimSpace(reference.Provider.String()) != "" &&
		!sameProviderIdentity(provider, reference.Provider.String()) {
		return false
	}
	return provider == "" || metadata == nil || strings.TrimSpace(metadata.Provider) == "" ||
		sameProviderIdentity(provider, metadata.Provider)
}

func sameProviderIdentity(left, right string) bool {
	return strings.EqualFold(
		workers.CanonicalProviderSessionProvider(left),
		workers.CanonicalProviderSessionProvider(right),
	)
}

func (p *ProviderSessionObservationPublisher) associateProviderSession(
	observer Service,
	fragment workers.ProgressFragment,
) bool {
	reference := workers.CloneProviderSessionReference(fragment.ProviderSessionReference)
	if reference == nil {
		return true
	}
	if observer == nil {
		return false
	}
	_, err := observer.ObserveProviderSession(context.Background(), ProviderSessionObservationRequest{
		DispatchID: fragment.DispatchID,
		Reference:  reference.Clone(),
	})
	return err == nil
}

func (p *ProviderSessionObservationPublisher) publishWorkerObservation(
	observer Service,
	fragment workers.ProgressFragment,
) bool {
	if draft, ok := canonicalDraftFromFragment(fragment); ok {
		return p.publishCanonicalWorkerRecord(observer, fragment, draft)
	}
	if !isWorkerAuthoredFragment(fragment) {
		return true
	}
	return p.publishWorkerRecord(observer, fragment)
}

// isWorkerAuthoredFragment reports whether a fragment carries output the
// Worker itself produced, as opposed to the dispatch lifecycle signal the
// owning Factory Session consumes.
func isWorkerAuthoredFragment(fragment workers.ProgressFragment) bool {
	switch fragment.Kind {
	case workers.ProgressFragmentKind, workers.ResponseFragmentKind:
		return true
	default:
		return false
	}
}

func canonicalDraftFromFragment(fragment workers.ProgressFragment) (workers.Draft, bool) {
	var draft workers.Draft
	switch value := fragment.CanonicalDraft.(type) {
	case workers.Draft:
		draft = workers.CloneDraft(value)
	case *workers.Draft:
		if value == nil {
			return workers.Draft{}, false
		}
		draft = workers.CloneDraft(*value)
	default:
		return workers.Draft{}, false
	}
	if draft.Kind == "" {
		return workers.Draft{}, false
	}
	if draft.DispatchID == "" {
		draft.DispatchID = strings.TrimSpace(fragment.DispatchID)
	}
	return draft, true
}

func providerIdentityForFragment(fragment workers.ProgressFragment, draft *workers.Draft) string {
	if draft != nil {
		if provider := strings.TrimSpace(draft.Provenance.Provider); provider != "" {
			if !isSyntheticWorkerProvider(provider) {
				return provider
			}
		}
	}
	if provider := strings.TrimSpace(fragment.Provider); provider != "" {
		return provider
	}
	if reference := fragment.ProviderSessionReference; reference != nil {
		if provider := strings.TrimSpace(reference.Provider.String()); provider != "" {
			return provider
		}
	}
	if metadata := fragment.ProviderSessionRef; metadata != nil {
		return strings.TrimSpace(metadata.Provider)
	}
	return ""
}

func providerIdentityAgrees(fragment workers.ProgressFragment, draft workers.Draft) bool {
	provider := strings.TrimSpace(draft.Provenance.Provider)
	if provider == "" || isSyntheticWorkerProvider(provider) {
		return true
	}
	if explicit := strings.TrimSpace(fragment.Provider); explicit != "" &&
		!sameProviderIdentity(provider, explicit) {
		return false
	}
	if reference := fragment.ProviderSessionReference; reference != nil &&
		strings.TrimSpace(reference.Provider.String()) != "" &&
		!sameProviderIdentity(provider, reference.Provider.String()) {
		return false
	}
	if metadata := fragment.ProviderSessionRef; metadata != nil && strings.TrimSpace(metadata.Provider) != "" &&
		!sameProviderIdentity(provider, metadata.Provider) {
		return false
	}
	return true
}

func isSyntheticWorkerProvider(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), "agent-run")
}

func ensureProviderBinding(
	observer Service,
	req ProviderBindingRequest,
) (ProviderBindingResult, error) {
	return observer.EnsureProviderBinding(context.Background(), req)
}

func resolveWorkerSessionID(observer Service, dispatchID string) (string, error) {
	return observer.WorkerSessionIDForDispatch(context.Background(), dispatchID)
}

func suppressProviderOutput(err error) bool {
	return errors.Is(err, ErrProviderBindingConflict) ||
		errors.Is(err, ErrProviderBindingAttemptMismatch)
}

func (p *ProviderSessionObservationPublisher) publishCanonicalWorkerRecord(
	observer Service,
	fragment workers.ProgressFragment,
	draft workers.Draft,
) bool {
	sessionID := strings.TrimSpace(fragment.DispatchID)
	if observer == nil || sessionID == "" || draft.DispatchID == "" || !providerIdentityAgrees(fragment, draft) {
		return false
	}
	if provider := providerIdentityForFragment(fragment, &draft); provider != "" {
		binding, err := ensureProviderBinding(observer, ProviderBindingRequest{
			DispatchID: sessionID,
			Provider:   provider,
		})
		if err != nil {
			p.reportRejectedRecord(sessionID, draft, err)
			return !suppressProviderOutput(err)
		}
		if binding.WorkerSessionID != "" {
			sessionID = binding.WorkerSessionID
		}
	} else if resolved, err := resolveWorkerSessionID(observer, sessionID); err != nil {
		p.reportRejectedRecord(sessionID, draft, err)
		return !suppressProviderOutput(err)
	} else {
		sessionID = resolved
	}
	if err := p.publishWorkerDraft(observer, sessionID, draft); err != nil {
		return !errors.Is(err, ErrProviderBindingConflict)
	}
	return true
}

// publishWorkerRecord commits one Worker-authored observation to that Worker
// Session's topic.
//
// The fragment names the current Workers dispatch. The bound Worker Sessions
// service resolves that attempt to the stable Worker Session topic before the
// record is appended.
//
// A rejected record loses that record and nothing else: Publish has a
// no-return signature by design, and one malformed or late observation must
// never fail the dispatch that produced it. Rejection is reported rather than
// swallowed, because a silently dropped Worker observation is precisely the
// failure this routing exists to remove.
func (p *ProviderSessionObservationPublisher) publishWorkerRecord(
	observer Service,
	fragment workers.ProgressFragment,
) bool {
	sessionID := strings.TrimSpace(fragment.DispatchID)
	if observer == nil || sessionID == "" {
		return true
	}
	draft, ok := draftFromProgressFragment(fragment)
	if !ok {
		return true
	}
	if provider := providerIdentityForFragment(fragment, &draft); provider != "" {
		binding, err := ensureProviderBinding(observer, ProviderBindingRequest{
			DispatchID: sessionID,
			Provider:   provider,
		})
		if err != nil {
			p.reportRejectedRecord(sessionID, draft, err)
			return !suppressProviderOutput(err)
		}
		if binding.WorkerSessionID != "" {
			sessionID = binding.WorkerSessionID
		}
	} else if resolved, err := resolveWorkerSessionID(observer, sessionID); err != nil {
		p.reportRejectedRecord(sessionID, draft, err)
		return !errors.Is(err, ErrProviderBindingAttemptMismatch)
	} else {
		sessionID = resolved
	}
	if err := p.publishWorkerDraft(observer, sessionID, draft); err != nil {
		return !errors.Is(err, ErrProviderBindingConflict)
	}
	return true
}

func (p *ProviderSessionObservationPublisher) publishWorkerDraft(
	observer Service,
	sessionID string,
	draft workers.Draft,
) error {
	if observer == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	p.records.Lock()
	defer p.records.Unlock()
	if p.sequences == nil {
		p.sequences = map[string]uint64{}
	}
	p.sequences[sessionID]++
	sequence := p.sequences[sessionID]

	_, err := observer.PublishRecord(context.Background(), PublishRecordRequest{
		SessionID:      sessionID,
		Draft:          draft,
		SourceType:     WorkerObservationSourceType,
		SourceID:       events.SourceID(sessionID),
		SourceSequence: events.SourceSequence(sequence),
		SourceEventID:  events.SourceEventID(sessionID + "/" + strconv.FormatUint(sequence, 10)),
		SchemaID:       WorkerObservationSchemaID,
	})
	if err != nil {
		p.reportRejectedRecord(sessionID, draft, err)
		return err
	}
	return nil
}

// WorkerObservationSourceType and WorkerObservationSchemaID identify Worker
// observations on a Worker Session topic. They are stable: the Events
// idempotency identity built from them is what makes a retried publication
// resolve to its original record rather than committing a duplicate.
const (
	WorkerObservationSourceType events.SourceType = "worker_observation"
	WorkerObservationSchemaID   events.SchemaID   = "workers.draft.v1"
)

func (p *ProviderSessionObservationPublisher) reportRejectedRecord(
	sessionID string,
	draft workers.Draft,
	err error,
) {
	if p == nil || p.logger == nil {
		return
	}
	// A closed publication window is the expected race, not a defect: the
	// terminal record can commit while a final observation is still in
	// flight. It is still reported, so a Worker whose output stops early is
	// diagnosable rather than mysterious.
	p.logger.Warn(
		"worker session dropped a worker observation",
		"worker_session_id", sessionID,
		"kind", string(draft.Kind),
		"phase", string(draft.Phase),
		"outcome", rejectedRecordOutcome(err),
	)
}

func rejectedRecordOutcome(err error) string {
	switch {
	case errors.Is(err, ErrPublicationNotOpen):
		return "publication_not_open"
	case errors.Is(err, ErrOutOfOrderPublication):
		return "out_of_order"
	case errors.Is(err, ErrSessionNotFound):
		return "session_not_found"
	default:
		return "rejected"
	}
}

// WithLogger attaches the bounded operator-visible reporting surface used when
// a Worker observation cannot be committed. A nil logger leaves reporting off
// rather than failing construction, matching this type's existing
// never-fail-the-dispatch contract.
func (p *ProviderSessionObservationPublisher) WithLogger(logger logging.Logger) *ProviderSessionObservationPublisher {
	if p == nil {
		return p
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger = logger
	return p
}

// PublishOutcome distinguishes a newly committed Worker record from a
// duplicate resolved to its originally accepted Events identity. Both
// outcomes report the same AggregateSequence: a caller cannot distinguish
// them by position, only by Outcome.
type PublishOutcome int

const (
	PublishOutcomeUnspecified PublishOutcome = iota
	// PublishOutcomeAccepted reports that the record was newly committed and
	// assigned a new Events aggregate position.
	PublishOutcomeAccepted
	// PublishOutcomeDuplicate reports that an equivalent
	// (SourceType, SourceID, SourceSequence, SourceEventID) tuple was already
	// committed to this Worker Session's topic; the returned
	// AggregateSequence is the original position, not a new one.
	PublishOutcomeDuplicate
)

// PublishRecordRequest asks Service to append one validated, detached
// source-native workers.Draft record onto Topic(SessionID). SourceType,
// SourceID, SourceSequence, and SourceEventID form the complete explicit
// Events idempotency identity (events.AppendIdentity): repeating that exact
// tuple on the same Worker Session resolves to the original committed
// record instead of a second one. PublishRecord does not translate Draft
// into an Events-owned or ACP-owned kind union: Draft's existing Kind,
// Phase, Payload, and Provenance are preserved verbatim on the committed
// record.
type PublishRecordRequest struct {
	SessionID      string
	Draft          workers.Draft
	SourceType     events.SourceType
	SourceID       events.SourceID
	SourceSequence events.SourceSequence
	SourceEventID  events.SourceEventID
	SchemaID       events.SchemaID
}

// Validate reports whether req is well-formed enough to attempt publication:
// SessionID is a well-formed stable identity, every Events identity field
// (including SchemaID) is well-formed, and Draft satisfies the existing
// Workers draft validation rules (workers.ValidateDraft). Validate is pure
// and does not mutate req or call Events.
func (req PublishRecordRequest) Validate() error {
	if !validSessionID(req.SessionID) {
		return ErrInvalidSessionID
	}
	identity := events.AppendIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	if err := req.SchemaID.Validate(); err != nil {
		return err
	}
	return workers.ValidateDraft(req.Draft)
}

// PublishRecordResult is the detached outcome of one PublishRecord call.
type PublishRecordResult struct {
	SessionID string
	// AggregateSequence is the committed record's position within
	// Topic(SessionID), in commit order.
	AggregateSequence events.AggregateSequence
	Outcome           PublishOutcome
}

// ProviderBindingRequest carries the provider identity learned at the
// Workers dispatch boundary. Provider is an explicit provider identity from a
// canonical draft or trusted provider metadata; Worker Sessions never derives
// it from a Provider Session's opaque ID.
type ProviderBindingRequest struct {
	DispatchID string
	Provider   string
}

// Validate reports whether the dispatch and provider identities are present.
// The registry resolves the dispatch to its owning Worker Session.
func (req ProviderBindingRequest) Validate() error {
	if strings.TrimSpace(req.DispatchID) == "" || strings.TrimSpace(req.Provider) == "" {
		return ErrInvalidProviderBinding
	}
	return nil
}

// ProviderBindingOutcome distinguishes a newly emitted binding record from
// a provider identity already established by the opening or an earlier
// binding call.
type ProviderBindingOutcome string

const (
	ProviderBindingOutcomeAccepted  ProviderBindingOutcome = "ACCEPTED"
	ProviderBindingOutcomeDuplicate ProviderBindingOutcome = "DUPLICATE"
)

// ProviderBindingResult is detached evidence of the provider identity known
// for one Worker Session dispatch.
type ProviderBindingResult struct {
	WorkerSessionID string
	DispatchID      string
	Provider        string
	Outcome         ProviderBindingOutcome
}

// draftFromProgressFragment converts one Workers progress observation into the
// source-native workers.Draft that gets committed to the Worker Session topic.
//
// A Worker is a tool call, and everything a Worker produces is content inside
// that tool call, so every observation a Worker emits belongs on its own
// topic rather than on the owning Factory Session's response stream. This is
// the one place that translation happens.
//
// Two provider vocabularies arrive here and both must be handled, because a
// Factory can dispatch either kind of Worker:
//
//   - ACP execution carries a fact kind in Metadata["kind"] with a bare phase
//     in Type ("delta", "updated", ...).
//   - The native claude/codex adapters carry a dotted "noun.phase" in Type
//     with no Metadata["kind"] at all.
//
// It reports ok=false for an observation that carries no committable record,
// which is not an error: a provider emits plenty of transient detail that has
// no declared Worker vocabulary, and inventing a record for it would be worse
// than omitting it.
//
// Every returned Draft is a legal Kind/Phase pair carrying the payload shape
// that pair requires. That is load-bearing rather than incidental: a draft
// that violates it is rejected by workers.ValidateDraft inside PublishRecord
// and the observation is lost, which is exactly the failure mode this routing
// exists to remove.
func draftFromProgressFragment(fragment workers.ProgressFragment) (workers.Draft, bool) {
	kind, phase, ok := progressFactVocabulary(fragment)
	if !ok {
		return workers.Draft{}, false
	}

	payload, ok := progressDraftPayload(kind, phase, fragment)
	if !ok {
		return workers.Draft{}, false
	}
	// Every payload above is a plain struct, so encoding cannot fail. Handling
	// an impossible error here would add a branch no test can reach; a nil
	// encoding would instead be rejected by the validation below, which already
	// requires a non-empty, well-formed payload. Same reasoning as
	// serializedKnownChildUpdateBytes in the ACP child projector.
	encoded, _ := json.Marshal(payload)

	draft := workers.Draft{
		Kind:         kind,
		Phase:        phase,
		Provenance:   progressDraftProvenance(fragment),
		Payload:      encoded,
		DispatchID:   strings.TrimSpace(fragment.DispatchID),
		ItemID:       strings.TrimSpace(fragment.Metadata["item_id"]),
		ParentItemID: progressDraftParentItemID(kind, fragment.Metadata),
	}
	// The Kind/Phase pair policy and the per-pair payload rules are owned by
	// workers. Asking the owner is what keeps this converter from drifting
	// against them; a local copy of the table would be one more thing to keep
	// in sync, and a disagreement would surface as a silently lost observation
	// rather than a failing test.
	if err := workers.ValidateDraft(draft); err != nil {
		return workers.Draft{}, false
	}
	return draft, true
}

func progressDraftProvenance(fragment workers.ProgressFragment) workers.Provenance {
	provider := providerIdentityForFragment(fragment, nil)
	if provider == "" {
		return workers.Provenance{}
	}
	nativeType := strings.TrimSpace(fragment.Type)
	if nativeType == "" {
		nativeType = strings.TrimSpace(fragment.Metadata["native_type"])
	}
	delivery := workers.DeliveryNativeStream
	fidelity := workers.FidelityNormalized
	representation := workers.RepresentationNotification
	if isFinalOnlyProviderMessage(provider, nativeType) {
		delivery = workers.DeliveryNativeFinal
		fidelity = workers.FidelityFinalOnly
		representation = workers.RepresentationSnapshot
	} else if isFinalOnlyProviderLifecycle(provider, nativeType) {
		delivery = workers.DeliverySynthesized
		fidelity = workers.FidelityLifecycleOnly
	}
	return workers.Provenance{
		Delivery:        delivery,
		Fidelity:        fidelity,
		NativeEventType: nativeType,
		Provider:        provider,
		Representation:  representation,
	}
}

func isFinalOnlyProviderMessage(provider, nativeType string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), workers.RunnerIDAntigravity) &&
		strings.EqualFold(strings.TrimSpace(nativeType), "message.completed")
}

func isFinalOnlyProviderLifecycle(provider, nativeType string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), workers.RunnerIDAntigravity) &&
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(nativeType)), "run.")
}

// progressFactVocabulary resolves the fact's Kind and Phase from whichever of
// the two provider vocabularies it uses.
func progressFactVocabulary(fragment workers.ProgressFragment) (workers.Kind, workers.Phase, bool) {
	rawKind := strings.ToLower(strings.TrimSpace(fragment.Metadata["kind"]))
	rawPhase := strings.ToLower(strings.TrimSpace(fragment.Type))

	// The native adapters put "noun.phase" in Type. Splitting it here keeps
	// both vocabularies converging on one Kind/Phase resolution below rather
	// than growing two parallel converters that drift.
	if rawKind == "" {
		noun, dotted, found := strings.Cut(rawPhase, ".")
		if !found {
			return "", "", false
		}
		rawKind, rawPhase = noun, dotted
	}

	switch rawKind {
	case "message":
		phase, ok := progressPhase(rawPhase, workers.KindMessage)
		return workers.KindMessage, phase, ok
	case "reasoning":
		phase, ok := progressPhase(rawPhase, workers.KindReasoning)
		return workers.KindReasoning, phase, ok
	case "tool":
		// A tool call's transition is carried by its ACP status, not by the
		// fact's phase: a client sends tool_call once and then any number of
		// tool_call_updates whose only distinguishing field is Status.
		if status := strings.TrimSpace(fragment.Metadata["status"]); status != "" {
			rawPhase = toolStatusPhase(status)
		}
		phase, ok := progressPhase(rawPhase, workers.KindTool)
		return workers.KindTool, phase, ok
	case "run", "turn":
		phase, ok := progressPhase(rawPhase, workers.KindRun)
		return workers.KindRun, phase, ok
	case "file_change":
		return workers.KindFileChange, workers.PhaseUpdated, true
	case "plan":
		return workers.KindPlan, workers.PhaseUpdated, true
	case "usage":
		return workers.KindUsage, workers.PhaseUpdated, true
	case "error":
		return workers.KindError, workers.PhaseUpdated, true
	default:
		// Including "session": a provider's own session metadata is not this
		// Worker Session's lifecycle, and committing it as a SESSION record
		// would collide with the opening and terminal records worker_sessions
		// owns. It survives as labelled progress instead.
		return workers.KindProgress, workers.PhaseUpdated, true
	}
}

// progressPhase maps a provider-native phase word onto a declared Worker
// phase. Whether the resulting Kind/Phase pair is legal is decided by
// workers.ValidateDraft at the end of the conversion, so this only has to
// recognize the word.
func progressPhase(raw string, _ workers.Kind) (workers.Phase, bool) {
	var phase workers.Phase
	switch raw {
	case "started", "start":
		phase = workers.PhaseStarted
	case "delta":
		phase = workers.PhaseDelta
	case "completed", "complete":
		phase = workers.PhaseCompleted
	case "failed":
		phase = workers.PhaseFailed
	case "canceled", "cancelled":
		phase = workers.PhaseCanceled
	case "updated":
		// No content kind declares UPDATED. A provider that reports an
		// ongoing change means DELTA in this vocabulary.
		phase = workers.PhaseDelta
	default:
		return "", false
	}
	return phase, true
}

// toolStatusPhase maps the ACP tool-call status vocabulary onto a phase word.
func toolStatusPhase(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "started"
	case "in_progress":
		return "delta"
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	case "cancelled", "canceled":
		return "canceled"
	default:
		return "delta"
	}
}

// progressDraftPayload builds the payload shape the resolved Kind/Phase pair
// requires, reporting false when the fact does not carry enough to satisfy it.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func progressDraftPayload(
	kind workers.Kind,
	phase workers.Phase,
	fragment workers.ProgressFragment,
) (any, bool) {
	metadata := fragment.Metadata
	detail := fragment.Payload

	switch kind {
	case workers.KindMessage:
		if phase == workers.PhaseDelta {
			if detail == "" {
				return nil, false
			}
			return workers.MessageDeltaPayload{
				ContentBlockIndex: 0, ContentBlockKind: workers.ContentBlockText, TextDelta: detail,
			}, true
		}
		return workers.MessagePayload{
			Role:          "assistant",
			ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: detail}},
			Partial:       strings.EqualFold(strings.TrimSpace(metadata["partial"]), "true"),
		}, true
	case workers.KindReasoning:
		if phase == workers.PhaseDelta {
			return workers.ReasoningPayload{SummaryDelta: detail}, true
		}
		return workers.ReasoningPayload{Summary: detail}, true
	case workers.KindTool:
		return toolDraftPayload(phase, detail, metadata)
	case workers.KindRun:
		return workers.RunPayload{Status: strings.ToLower(string(phase))}, true
	case workers.KindFileChange:
		path := strings.TrimSpace(metadata["path"])
		operation := strings.TrimSpace(metadata["operation"])
		if path == "" || operation == "" {
			return nil, false
		}
		return workers.FileChangePayload{Path: path, Operation: operation, Summary: detail}, true
	case workers.KindPlan:
		return planDraftPayload(detail, metadata), true
	case workers.KindUsage:
		total, err := strconv.ParseInt(strings.TrimSpace(metadata["used_tokens"]), 10, 64)
		if err != nil {
			return nil, false
		}
		return workers.UsagePayload{TotalTokens: total}, true
	case workers.KindError:
		code := strings.TrimSpace(metadata["error_code"])
		if code == "" {
			code = "provider_failure"
		}
		message := detail
		if message == "" {
			message = "provider execution failed"
		}
		return workers.ErrorPayload{Code: code, Message: message}, true
	default:
		label := strings.TrimSpace(metadata["native_type"])
		if label == "" {
			label = strings.TrimSpace(fragment.Type)
		}
		if label == "" {
			return nil, false
		}
		return workers.ProgressPayload{Label: label, Message: detail}, true
	}
}

func toolDraftPayload(phase workers.Phase, detail string, metadata map[string]string) (any, bool) {
	toolCallID := strings.TrimSpace(metadata["item_id"])
	if toolCallID == "" {
		return nil, false
	}
	if phase == workers.PhaseDelta {
		// ToolDeltaPayload requires an output increment. A status-only update
		// carries nothing the opening record did not already say.
		output := detail
		if output == "" {
			output = strings.TrimSpace(metadata["raw_output"])
		}
		if output == "" {
			return nil, false
		}
		return workers.ToolDeltaPayload{ToolCallID: toolCallID, OutputDelta: output}, true
	}

	name := strings.TrimSpace(detail)
	if name == "" {
		name = strings.TrimSpace(metadata["tool_name"])
	}
	if name == "" {
		name = "tool"
	}
	payload := workers.ToolPayload{
		ToolCallID: toolCallID, ToolName: name, Status: strings.TrimSpace(metadata["status"]),
	}
	if raw := json.RawMessage(metadata["raw_input"]); json.Valid(raw) {
		payload.ArgumentsSummary = raw
	}
	if raw := json.RawMessage(metadata["raw_output"]); json.Valid(raw) {
		payload.ResultSummary = raw
	}
	return payload, true
}

// planDraftPayload decodes the structured entries an ACP plan update carries,
// falling back to the rendered summary when a provider supplied no structure.
func planDraftPayload(detail string, metadata map[string]string) workers.PlanPayload {
	var entries []struct {
		Content  string `json:"content"`
		Priority string `json:"priority"`
		Status   string `json:"status"`
	}
	if raw := strings.TrimSpace(metadata["entries"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &entries); err == nil {
			steps := make([]workers.PlanStep, 0, len(entries))
			for index, entry := range entries {
				content := strings.TrimSpace(entry.Content)
				if content == "" {
					continue
				}
				steps = append(steps, workers.PlanStep{
					ID:          "step-" + strconv.Itoa(index+1),
					Description: content,
					Status:      entry.Status,
				})
			}
			if len(steps) > 0 {
				return workers.PlanPayload{Steps: steps}
			}
		}
	}
	return workers.PlanPayload{Summary: detail}
}

// progressDraftParentItemID resolves the lineage a fact declares. Only a file
// change has one: ACP carries a diff as content inside the tool call that
// produced it, and that ownership must survive into the committed record.
func progressDraftParentItemID(kind workers.Kind, metadata map[string]string) string {
	if kind != workers.KindFileChange {
		return ""
	}
	return strings.TrimSpace(metadata["tool_call_id"])
}

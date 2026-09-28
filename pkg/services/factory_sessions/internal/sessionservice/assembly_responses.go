package service

import (
	"context"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// SubscribeResponses reads the live session's retained response stream from
// its canonical registry entry. Durable responses use the durable owner.
func (a *Assembly) SubscribeResponses(ctx context.Context, request factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error) {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.SessionResponseSubscriptionResult{}, err
	}
	if request.AfterSequence < 0 {
		return factorysessions.SessionResponseSubscriptionResult{}, canonicalRequestError("afterSequence", "sequence must not be negative")
	}
	if err := validateCanonicalResponseKinds(request.Kinds); err != nil {
		return factorysessions.SessionResponseSubscriptionResult{}, err
	}
	normalized := factorysessions.ResponseEventSubscriptionRequest{
		SessionID: strings.TrimSpace(request.SessionID), AfterSequence: request.AfterSequence,
		DispatchID: strings.TrimSpace(request.DispatchID),
		Kinds:      append([]factorysessions.ResponseEventKind(nil), request.Kinds...),
	}
	if session := a.Resolve(normalized.SessionID); session != nil {
		cursor, err := subscribeLiveResponses(ctx, a.responseStreams, session, normalized)
		if err != nil {
			return factorysessions.SessionResponseSubscriptionResult{}, err
		}
		return factorysessions.SessionResponseSubscriptionResult{Cursor: cursor}, nil
	}
	return a.Service.SubscribeResponses(ctx, request)
}

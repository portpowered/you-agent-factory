package run

import (
	"context"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

type sessionResponseStreamAttachable interface {
	SubscribeSessionResponseStream(
		sessionID string,
		dispatchID string,
		afterSequence int64,
	) (*factorysessions.SessionResponseStreamSubscription, error)
	SessionResponseStreamDispatchIDs(sessionID string) ([]string, error)
}

type responseStreamEventSink interface {
	onStreamSegment(factorysessions.SessionResponseStreamReadResult)
}

type discardResponseStreamSink struct{}

func (discardResponseStreamSink) onStreamSegment(factorysessions.SessionResponseStreamReadResult) {}

type responseStreamAttachment struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func startResponseStreamAttachment(
	ctx context.Context,
	attachable sessionResponseStreamAttachable,
	sessionID string,
	sink responseStreamEventSink,
) *responseStreamAttachment {
	if attachable == nil || sink == nil {
		return nil
	}
	attachCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runResponseStreamAttachment(attachCtx, attachable, sessionID, sink)
	}()
	return &responseStreamAttachment{
		cancel: cancel,
		done:   done,
	}
}

func (a *responseStreamAttachment) stop() {
	if a == nil {
		return
	}
	a.cancel()
	<-a.done
}

func runResponseStreamAttachment(
	ctx context.Context,
	attachable sessionResponseStreamAttachable,
	sessionID string,
	sink responseStreamEventSink,
) {
	subscribed := make(map[string]*factorysessions.SessionResponseStreamSubscription)
	var subscribedMu sync.Mutex
	defer func() {
		subscribedMu.Lock()
		defer subscribedMu.Unlock()
		for _, subscription := range subscribed {
			subscription.Detach()
		}
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		dispatchIDs, _ := attachable.SessionResponseStreamDispatchIDs(sessionID)
		for _, dispatchID := range dispatchIDs {
			subscribedMu.Lock()
			_, alreadySubscribed := subscribed[dispatchID]
			subscribedMu.Unlock()
			if alreadySubscribed {
				continue
			}

			subscription, err := attachable.SubscribeSessionResponseStream(sessionID, dispatchID, 0)
			if err != nil {
				continue
			}

			subscribedMu.Lock()
			subscribed[dispatchID] = subscription
			subscribedMu.Unlock()
			go consumeResponseStreamSubscription(ctx, subscription, sink)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func consumeResponseStreamSubscription(
	ctx context.Context,
	subscription *factorysessions.SessionResponseStreamSubscription,
	sink responseStreamEventSink,
) {
	defer subscription.Detach()
	for {
		result, err := subscription.Next(ctx)
		if err != nil {
			return
		}
		sink.onStreamSegment(result)
	}
}

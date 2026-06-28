package run

import (
	"context"
	"errors"
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

type responseStreamAttachment struct {
	cancel    context.CancelFunc
	done      chan struct{}
	consumers sync.WaitGroup
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
	attachment := &responseStreamAttachment{
		cancel: cancel,
		done:   done,
	}
	go func() {
		defer close(done)
		runResponseStreamAttachment(attachCtx, attachable, sessionID, sink, &attachment.consumers)
	}()
	return attachment
}

func (a *responseStreamAttachment) stop() {
	if a == nil {
		return
	}
	a.cancel()
	<-a.done
	a.consumers.Wait()
}

func runResponseStreamAttachment(
	ctx context.Context,
	attachable sessionResponseStreamAttachable,
	sessionID string,
	sink responseStreamEventSink,
	consumers *sync.WaitGroup,
) {
	subscribed := make(map[string]*factorysessions.SessionResponseStreamSubscription)
	var subscribedMu sync.Mutex

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
			consumers.Add(1)
			go func() {
				defer consumers.Done()
				consumeResponseStreamSubscription(ctx, subscription, sink)
			}()
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
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				drainResponseStreamSubscription(subscription, sink)
			}
			return
		}
		sink.onStreamSegment(result)
	}
}

func drainResponseStreamSubscription(
	subscription *factorysessions.SessionResponseStreamSubscription,
	sink responseStreamEventSink,
) {
	drainCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	for {
		result, err := subscription.Next(drainCtx)
		if err != nil {
			return
		}
		if len(result.Events) == 0 && !result.BehindRetainedWindow && result.Compaction == nil {
			return
		}
		sink.onStreamSegment(result)
	}
}

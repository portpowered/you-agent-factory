//go:build windows && managed_process_integration

package localai

import (
	"context"

	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

type asrProtocolCorrelation interface {
	RecordRequestSemanticSHA256(string) error
	ObserveEndpoint(context.Context, string) error
	AwaitResponseRelease(context.Context, string, string) error
}

func newASRProtocolCorrelation(ctx context.Context) asrProtocolCorrelation {
	controller := modelseffects.ASRLiveCorrelationFromContext(ctx)
	if controller == nil {
		return nil
	}
	return controller
}

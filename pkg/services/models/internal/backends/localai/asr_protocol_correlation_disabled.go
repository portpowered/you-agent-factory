//go:build !windows || !managed_process_integration

package localai

import "context"

type asrProtocolCorrelation interface {
	RecordRequestSemanticSHA256(string) error
	ObserveEndpoint(context.Context, string) error
	AwaitResponseRelease(context.Context, string, string) error
}

func newASRProtocolCorrelation(context.Context) asrProtocolCorrelation { return nil }

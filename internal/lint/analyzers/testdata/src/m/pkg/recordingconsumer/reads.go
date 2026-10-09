package recordingconsumer

import history "m/pkg/services/recordings"

type Alias = history.Request
type Forward interface {
	QueryHistoricalRecording(Alias) (history.Snapshot, error)
}
type Unrelated interface {
	QueryHistoricalRecording(int) (string, error)
}

func Forbidden(r history.Reader) {
	_, _ = r.QueryHistoricalRecording(history.Request{}) // want "recording-read:.*Forbidden#QueryHistoricalRecording::count=1"
}
func Forwarded(r Forward) {
	f := r.QueryHistoricalRecording // want "recording-read:.*Forwarded#QueryHistoricalRecording::count=1"
	_, _ = f(Alias{})
}
func Expression() {
	_ = history.Reader.QueryHistoricalRecording // want "recording-read:.*Expression#QueryHistoricalRecording::count=1"
}
func Codec() {
	_ = history.DecodeArtifact(nil) // want "recording-read:.*Codec#DecodeArtifact::count=1"
}
func Lookalike(r Unrelated) { _, _ = r.QueryHistoricalRecording(1) }

func TestNamedCaller(r history.Reader) {
	_, _ = r.QueryHistoricalRecording(history.Request{}) // want "recording-read:.*TestNamedCaller"
}

package recordingdebt // want "stale baseline entry.*Removed" "stale baseline entry.*Duplicate"

import "m/pkg/services/recordings"

func Listed(r recordings.Reader) { _, _ = r.QueryHistoricalRecording(recordings.Request{}) }
func Duplicate(r recordings.Reader) {
	_, _ = r.QueryHistoricalRecording(recordings.Request{}) // want "recording-read:.*Duplicate#QueryHistoricalRecording::count=2"
	_, _ = r.QueryHistoricalRecording(recordings.Request{})
}
func NewSite(r recordings.Reader) {
	_, _ = r.QueryHistoricalRecording(recordings.Request{}) // want "recording-read:.*NewSite#QueryHistoricalRecording::count=1"
}

package recordingconsumer

import "m/pkg/services/recordings"

func readerSubject(r recordings.Reader) { _, _ = r.QueryHistoricalRecording(recordings.Request{}) }

package recordings

type Request struct{}
type Snapshot struct{}
type Reader interface {
	QueryHistoricalRecording(Request) (Snapshot, error)
}

func DecodeArtifact([]byte) Snapshot { return Snapshot{} }

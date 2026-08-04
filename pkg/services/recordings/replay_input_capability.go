package recordings

// ReplayInputCapability is a narrow, Recordings-owned capability for
// classifying and loading one historical replay input selected by
// filesystem path into either a portable JavaScript Factory Session
// recording or a legacy embedded-Factory replay artifact. It composes
// Recordings' existing portable-recording decode/validate behavior and
// legacy replay-artifact loading behavior, so a caller that only needs to
// classify and load a replay input does not have to combine a raw file
// reader, an aliased decoder/validator, and a legacy loader itself.
type ReplayInputCapability interface {
	// LoadReplayInput reads the file at the selected path, classifies it as
	// a portable JavaScript Factory Session recording or a legacy
	// embedded-Factory replay artifact, and returns the decoded/validated
	// result for exactly one of those two families.
	LoadReplayInput(LoadReplayInputRequest) (LoadReplayInputResult, error)
}

// LoadReplayInputRequest selects one historical replay input by filesystem
// path.
type LoadReplayInputRequest struct {
	Path string
}

// LoadReplayInputResult contains exactly one of Portable or Legacy,
// depending on which replay input family the selected path contained.
type LoadReplayInputResult struct {
	Portable *PortableRecording
	Legacy   *ReplayArtifact
}

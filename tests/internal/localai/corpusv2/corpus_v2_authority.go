package corpusv2

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CorpusV2SampleExpectation pins a selected sample's identity and stream
// metadata independently of the bytes currently present in the external
// checkout.
type CorpusV2SampleExpectation struct {
	Study          string
	Band           string
	Attempt        string
	ClipBytes      int64
	ClipSHA256     string
	PromptBytes    int64
	PromptSHA256   string
	Codec          string
	Width          int
	Height         int
	FrameRate      string
	DurationMillis int64
	Frames         int64
}

func defaultCorpusV2SampleExpectations() []CorpusV2SampleExpectation {
	return []CorpusV2SampleExpectation{
		{Study: "selfie-jessie-duration-study", Band: "minimum", Attempt: "D007-a2001", ClipBytes: 393020, ClipSHA256: "0adf4be0b74415f8aad7c59aa8c2ecb83b4e23abaa03a1c42effe8fd8ea9b27e", PromptBytes: 1290, PromptSHA256: "e2c04f0ef9ae5a92208e73dd9b0ccb6f96ad00163d392c7df030fd19ca62bffe", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
		{Study: "selfie-jessie-duration-study", Band: "median", Attempt: "D010-a2001", ClipBytes: 455857, ClipSHA256: "196f96370be7bc0a5fb365ef86ad46244e2c46310f6b73d6c1ee9a67068d50c5", PromptBytes: 1399, PromptSHA256: "f0b8aaf1efaa389d8efc8e1902e5037a649446450218fb9f128d5dcf8109e4ef", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
		{Study: "selfie-jessie-duration-study", Band: "maximum", Attempt: "D040-a2001", ClipBytes: 939599, ClipSHA256: "58cd074f1c963ea9ff7322620a2d178b4b580934c2ee6d23dd24e64e11bf52a3", PromptBytes: 1481, PromptSHA256: "e8106ca828dedd0bf528687393c6c2abc31ccf16c7fa2cf54cc7a13f11ae6d02", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 10125, Frames: 243},
		{Study: "selfie-jessie-prompt-study", Band: "minimum", Attempt: "P004-a1007", ClipBytes: 395702, ClipSHA256: "eacdb3f7564deeea1da693b8b219082d9ee4ac70067d565ef8901242b4ec5140", PromptBytes: 1009, PromptSHA256: "8580aaf3c7ba60e44ebb59a3c35036828cc9cafd9ba20cf6ac98a2ad9f957cee", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
		{Study: "selfie-jessie-prompt-study", Band: "median", Attempt: "P090-a1008", ClipBytes: 451560, ClipSHA256: "0c90abe1cedb117772c0a4b47bf49b1a252bf78de5921082198583b5843e5233", PromptBytes: 2228, PromptSHA256: "a04b9875576c47b242fe41916145d42cbadc2b4aae612ddbd490872fd39101be", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
		{Study: "selfie-jessie-prompt-study", Band: "maximum", Attempt: "P099-a1003", ClipBytes: 483760, ClipSHA256: "f7567326e76bdf7cbdad5652b39cae1000d904be2a3d6274d371832e8883ca98", PromptBytes: 4410, PromptSHA256: "10a4937d251595c8744223a83f1f52f26bd38318911e2a0150acd73d5d0a713a", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
		{Study: "selfie-jessie-quality", Band: "minimum", Attempt: "Q08-a18", ClipBytes: 372001, ClipSHA256: "89e2df6b356cf26e305b860bab711b29d6ca6d3c897c17ffee2fcc832608e871", PromptBytes: 5972, PromptSHA256: "f6d35918cd69d1047ab9012a47ad7b5e2480005f2e0ee8191cb6e5481eded2f7", Codec: "avc1", Width: 480, Height: 864, FrameRate: "25/1", DurationMillis: 2640, Frames: 66},
		{Study: "selfie-jessie-quality", Band: "median", Attempt: "Q02-a1", ClipBytes: 470854, ClipSHA256: "3361b4b3bb818d7678569ab6786f32e0a2087719f506b32a6c6287cad5012790", PromptBytes: 3771, PromptSHA256: "bd55340b2a7b45bedf3a653357d31e4dc6f7efaf0666443caa8f495c0228cbed", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
		{Study: "selfie-jessie-quality", Band: "maximum", Attempt: "Q02-a18", ClipBytes: 599629, ClipSHA256: "eb06cf35dc6e0db810d2b63766cc82d41682394cf94d60415c87e228977b2fb7", PromptBytes: 5989, PromptSHA256: "a25019a0080cb560c1cbdb258aca9fe6313a00ab4b7fd09078e7b7c5cef67f38", Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: 5167, Frames: 124},
	}
}

// ValidateCorpusV2Manifest checks the parser's local-real observation against
// the selected corpus authority. It validates selected identities only; it
// never parses the index or selects representatives itself.
func ValidateCorpusV2Manifest(manifest CorpusV2Manifest, authority CorpusV2Authority) error {
	if err := validateCorpusV2ManifestIdentity(manifest, authority); err != nil {
		return err
	}
	if err := validateCorpusV2ManifestCounts(manifest, authority); err != nil {
		return err
	}
	return validateCorpusV2SampleExpectations(manifest, authority)
}

func validateCorpusV2ManifestIdentity(manifest CorpusV2Manifest, authority CorpusV2Authority) error {
	if manifest.SchemaVersion != CorpusV2SchemaVersion || !corpusV2SamePath(manifest.Repository, authority.RepositoryRoot) {
		return corpusV2Error(CorpusV2CodePathIdentity, "manifest.repository", authority.RepositoryRoot, manifest.Repository, nil)
	}
	if manifest.Commit != authority.Commit {
		return corpusV2Error(CorpusV2CodeSourceCommitMismatch, "manifest.commit", authority.Commit, manifest.Commit, nil)
	}
	if manifest.IndexPath != authority.IndexPath {
		return corpusV2Error(CorpusV2CodePathIdentity, "manifest.indexPath", authority.IndexPath, manifest.IndexPath, nil)
	}
	if !strings.EqualFold(manifest.IndexSHA256, authority.IndexSHA256) {
		return corpusV2Error(CorpusV2CodeIndexHashMismatch, "manifest.indexSha256", authority.IndexSHA256, manifest.IndexSHA256, nil)
	}
	if !manifest.ReadOnly || manifest.CopiedBytes != 0 || manifest.UploadedBytes != 0 || manifest.MissingSiblings != 0 {
		return corpusV2Error(CorpusV2CodeSourceMutation, "manifest.readOnly", "read-only with zero copy/upload/missing", fmt.Sprintf("readOnly=%t copied=%d uploaded=%d missing=%d", manifest.ReadOnly, manifest.CopiedBytes, manifest.UploadedBytes, manifest.MissingSiblings), nil)
	}
	return nil
}

func validateCorpusV2ManifestCounts(manifest CorpusV2Manifest, authority CorpusV2Authority) error {
	if authority.PairCount > 0 && len(manifest.Pairs) != authority.PairCount {
		return corpusV2Error(CorpusV2CodeCountMismatch, "manifest.pairs", fmt.Sprint(authority.PairCount), fmt.Sprint(len(manifest.Pairs)), nil)
	}
	if manifest.UniqueClips != len(manifest.Pairs) || manifest.UniquePrompts != len(manifest.Pairs) {
		return corpusV2Error(CorpusV2CodeCountMismatch, "manifest.uniquePairs", fmt.Sprint(len(manifest.Pairs)), fmt.Sprintf("clips=%d prompts=%d", manifest.UniqueClips, manifest.UniquePrompts), nil)
	}
	if err := validateCorpusV2UniquePairs(manifest.Pairs); err != nil {
		return err
	}
	wantSamples := len(authority.ExpectedSamples)
	if wantSamples == 0 {
		wantSamples = len(authority.RequiredStudies) * 3
	}
	if len(manifest.Samples) != wantSamples {
		return corpusV2Error(CorpusV2CodeCountMismatch, "manifest.samples", fmt.Sprint(wantSamples), fmt.Sprint(len(manifest.Samples)), nil)
	}
	return nil
}

func validateCorpusV2SampleExpectations(manifest CorpusV2Manifest, authority CorpusV2Authority) error {
	if len(authority.ExpectedSamples) == 0 {
		return nil
	}
	root, err := corpusV2AbsoluteRoot(authority.RepositoryRoot)
	if err != nil {
		return corpusV2Error(CorpusV2CodeAuthorityUnavailable, "repositoryRoot", "absolute corpus root", authority.RepositoryRoot, err)
	}
	for index, expected := range authority.ExpectedSamples {
		if err := validateCorpusV2SampleExpectation(manifest.Samples[index], expected, root, authority.Commit); err != nil {
			return err
		}
	}
	return nil
}

func validateCorpusV2SampleExpectation(sample CorpusV2Sample, expected CorpusV2SampleExpectation, root, commit string) error {
	field := fmt.Sprintf("samples.%s.%s", expected.Study, expected.Band)
	if sample.Study != expected.Study || sample.Band != expected.Band || sample.Attempt != expected.Attempt || sample.SourceCommit != commit {
		return corpusV2Error(CorpusV2CodeSelectionMismatch, field, expected.Attempt, sample.Attempt, nil)
	}
	clipPath := filepath.Join(root, "production", expected.Study, "attempts", expected.Attempt, "clip.mp4")
	promptPath := filepath.Join(root, "production", expected.Study, "attempts", expected.Attempt, "prompt.md")
	if !corpusV2SamePath(sample.Clip.Path, clipPath) || !corpusV2SamePath(sample.Prompt.Path, promptPath) {
		return corpusV2Error(CorpusV2CodePathIdentity, field+".path", clipPath, sample.Clip.Path, nil)
	}
	if !expectedFileIdentity(sample.Clip, expected.ClipBytes, expected.ClipSHA256) || !expectedFileIdentity(sample.Prompt, expected.PromptBytes, expected.PromptSHA256) {
		return corpusV2Error(CorpusV2CodeHashMismatch, field+".identity", "pinned clip and prompt identities", sample.Clip.Identity+";"+sample.Prompt.Identity, nil)
	}
	if !expectedStreamMetadata(sample.Stream, expected) {
		return corpusV2Error(CorpusV2CodeMetadataMismatch, field+".stream", "pinned stream metadata", fmt.Sprintf("%s %dx%d %s %dms frames=%d", sample.Stream.Codec, sample.Stream.Width, sample.Stream.Height, sample.Stream.FrameRate, sample.Stream.DurationMillis, sample.Stream.Frames), nil)
	}
	return nil
}

func expectedFileIdentity(identity CorpusV2FileIdentity, bytes int64, digest string) bool {
	wantIdentity := fmt.Sprintf("file:%d:%s", bytes, digest)
	return identity.Bytes == bytes && strings.EqualFold(identity.SHA256, digest) && identity.Identity == wantIdentity
}

func expectedStreamMetadata(stream CorpusV2StreamMetadata, expected CorpusV2SampleExpectation) bool {
	want := CorpusV2StreamMetadata{Codec: expected.Codec, Width: expected.Width, Height: expected.Height, FrameRate: expected.FrameRate, DurationMillis: expected.DurationMillis, Frames: expected.Frames}
	want.Identity = corpusV2IdentityForStream(want)
	return stream.Codec == want.Codec && stream.Width == want.Width && stream.Height == want.Height && stream.FrameRate == want.FrameRate && stream.DurationMillis == want.DurationMillis && stream.Frames == want.Frames && stream.Identity == want.Identity
}

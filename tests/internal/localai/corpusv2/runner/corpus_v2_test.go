package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCorpusInputSchemaContract(t *testing.T) {
	path := filepath.Join(corpusV2RepositoryRoot(t), "tests", "integration", "models", "omni_media_probe", "corpus-input.schema.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read corpus v2 schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatalf("decode corpus v2 schema: %v", err)
	}
	if schema["$id"] != "urn:you-agent-factory:tests:omni-video-corpus-input:v2" || schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema identity = %#v, want strict v2 object", schema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties are not an object")
	}
	if properties["schemaVersion"].(map[string]any)["const"] != CorpusV2SchemaVersion {
		t.Fatalf("schemaVersion const = %#v, want %q", properties["schemaVersion"], CorpusV2SchemaVersion)
	}
	corpus, ok := properties["corpus"].(map[string]any)
	if !ok {
		t.Fatal("corpus schema is not an object")
	}
	corpusProperties := corpus["properties"].(map[string]any)
	for field, expected := range map[string]any{"repository": CorpusV2Repository, "commit": CorpusV2Commit, "indexPath": CorpusV2IndexPath, "indexSha256": CorpusV2IndexSHA256, "mode": CorpusV2Mode} {
		got := corpusProperties[field].(map[string]any)["const"]
		if got != expected {
			t.Errorf("corpus.%s const = %#v, want %#v", field, got, expected)
		}
	}
	if !strings.Contains(string(body), `"ordering": {"const": "`+CorpusV2Ordering+`"}`) {
		t.Fatalf("schema does not pin ordering %q", CorpusV2Ordering)
	}
}

func TestCorpusParserRejectsMalformedAndDuplicate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rows := []corpusV2SyntheticRow{{Attempt: "a1"}, {Attempt: "a1"}}
	data := corpusV2SyntheticIndex(t, root, "study-a", rows)
	_, err := ParseCorpusV2Index(data, root)
	assertCorpusV2Code(t, err, CorpusV2CodeDuplicateClip)

	malformed := []byte("# wrong\n")
	_, err = ParseCorpusV2Index(malformed, root)
	assertCorpusV2Code(t, err, CorpusV2CodeMalformedIndex)

	escape := corpusV2SyntheticIndex(t, root, "study-a", []corpusV2SyntheticRow{{Attempt: "a1", Clip: filepath.Join(root, "..", "escape.mp4")}})
	_, err = ParseCorpusV2Index(escape, root)
	assertCorpusV2Code(t, err, CorpusV2CodePathEscape)
}

func TestCorpusSymlinkedParentsAreRejectedBeforeReading(t *testing.T) {
	t.Parallel()
	t.Run("media parent", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		outsideClip := filepath.Join(outside, "clip.mp4")
		if err := os.WriteFile(outsideClip, []byte("outside media bytes"), 0o600); err != nil {
			t.Fatalf("write external clip: %v", err)
		}
		attemptParent := filepath.Join(root, "production", "study-a", "attempts", "a1")
		if err := os.MkdirAll(filepath.Dir(attemptParent), 0o700); err != nil {
			t.Fatalf("create controlled corpus parent: %v", err)
		}
		if err := os.Symlink(outside, attemptParent); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("directory symlinks unavailable in this Windows environment: %v", err)
			}
			t.Fatalf("create media parent symlink: %v", err)
		}

		identity, data, err := corpusV2ReadIdentity(root, filepath.Join(attemptParent, "clip.mp4"))
		assertCorpusV2Code(t, err, CorpusV2CodePathEscape)
		if data != nil || identity.Identity != "" {
			t.Fatalf("escaped clip returned bytes or identity: bytes=%d identity=%q", len(data), identity.Identity)
		}
	})
	t.Run("index parent", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "video-output-index.md"), []byte("outside index bytes"), 0o600); err != nil {
			t.Fatalf("write external index: %v", err)
		}
		indexParent := filepath.Join(root, "docs", "selfiepostx")
		if err := os.MkdirAll(filepath.Dir(indexParent), 0o700); err != nil {
			t.Fatalf("create controlled index parent: %v", err)
		}
		if err := os.Symlink(outside, indexParent); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("directory symlinks unavailable in this Windows environment: %v", err)
			}
			t.Fatalf("create index parent symlink: %v", err)
		}

		_, err := corpusV2IndexAbsolutePath(CorpusV2Authority{RepositoryRoot: root, IndexPath: "docs/selfiepostx/video-output-index.md"})
		assertCorpusV2Code(t, err, CorpusV2CodePathEscape)
	})
}

func TestCorpusCaseOnlySymlinkAliasIsRejectedBeforeReading(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	indexedClip := filepath.Join(root, "clip.mp4")
	siblingClip := filepath.Join(root, "CLIP.mp4")
	if err := os.WriteFile(siblingClip, []byte("sibling media bytes"), 0o600); err != nil {
		t.Fatalf("write case-distinct sibling clip: %v", err)
	}
	if _, err := os.Lstat(indexedClip); err == nil {
		t.Skip("filesystem does not distinguish case-only sibling names")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe case-sensitive sibling name: %v", err)
	}
	if err := os.Symlink(filepath.Base(siblingClip), indexedClip); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("file symlinks unavailable in this Windows environment: %v", err)
		}
		t.Fatalf("create case-only clip symlink: %v", err)
	}

	identity, data, err := corpusV2ReadIdentity(root, indexedClip)
	assertCorpusV2Code(t, err, CorpusV2CodePathEscape)
	if data != nil || identity.Identity != "" || identity.Bytes != 0 {
		t.Fatalf("case-only symlink returned bytes or identity: bytes=%d identity=%q", len(data), identity.Identity)
	}
}

func TestCorpusSamePathFollowsFilesystemCaseSemantics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := filepath.Join(root, "Clip.mp4")
	caseVariant := filepath.Join(root, "CLIP.mp4")
	if err := os.WriteFile(first, []byte("media bytes"), 0o600); err != nil {
		t.Fatalf("write case probe file: %v", err)
	}
	_, err := os.Lstat(caseVariant)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe case-variant path: %v", err)
	}
	wantSamePath := err == nil
	if got := corpusV2SamePath(first, caseVariant); got != wantSamePath {
		t.Fatalf("case-variant path equality = %t, want %t for this filesystem", got, wantSamePath)
	}
}

func TestCorpusSelectionIsDeterministic(t *testing.T) {
	t.Parallel()
	authority := CorpusV2Authority{RequiredStudies: []string{"study-a"}}
	manifest := CorpusV2Manifest{Commit: "commit-a", Pairs: []CorpusV2Pair{
		corpusV2SyntheticPair("study-a", "a3", 3, 30),
		corpusV2SyntheticPair("study-a", "a1", 1, 10),
		corpusV2SyntheticPair("study-a", "a5", 5, 50),
		corpusV2SyntheticPair("study-a", "a2", 2, 20),
		corpusV2SyntheticPair("study-a", "a4", 4, 40),
	}}
	first, err := SelectCorpusV2Representatives(manifest, authority)
	if err != nil {
		t.Fatalf("first selection: %v", err)
	}
	second, err := SelectCorpusV2Representatives(manifest, authority)
	if err != nil {
		t.Fatalf("second selection: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("selection changed between reads: first=%#v second=%#v", first, second)
	}
	if len(first) != 3 || first[0].Attempt != "a1" || first[0].Band != "minimum" || first[1].Attempt != "a3" || first[1].Band != "median" || first[2].Attempt != "a5" || first[2].Band != "maximum" {
		t.Fatalf("selection = %#v, want a1/a3/a5 minimum/median/maximum", first)
	}
	for _, sample := range first {
		if sample.SourceCommit != manifest.Commit || sample.Clip.SHA256 == "" || sample.Prompt.SHA256 == "" || sample.Stream.Identity == "" {
			t.Fatalf("sample lacks immutable identities: %#v", sample)
		}
	}
}

func TestCorpusNegativeFailuresAreTypedAndPreHeavyweight(t *testing.T) {
	t.Parallel()
	t.Run("index mutation", func(t *testing.T) {
		root := t.TempDir()
		original := []byte("pinned index")
		digest := sha256.Sum256(original)
		authority := DefaultCorpusV2Authority()
		authority.RepositoryRoot = root
		authority.IndexSHA256 = hex.EncodeToString(digest[:])
		path := filepath.Join(root, "video-output-index.md")
		if err := os.WriteFile(path, []byte("mutated index"), 0o600); err != nil {
			t.Fatalf("write mutated index: %v", err)
		}
		data, _, err := corpusV2ReadIndexBytes(path, authority)
		if data != nil {
			t.Fatal("mutated index bytes were admitted")
		}
		assertCorpusV2Code(t, err, CorpusV2CodeIndexHashMismatch)
	})
	t.Run("missing sibling", func(t *testing.T) {
		root := t.TempDir()
		_, _, err := corpusV2ReadIdentity(root, filepath.Join(root, "prompt.md"))
		assertCorpusV2Code(t, err, CorpusV2CodeMissingSibling)
	})
	t.Run("unsupported stream metadata", func(t *testing.T) {
		first := corpusV2SyntheticPair("study-a", "a1", 1, 10)
		unsupported := corpusV2SyntheticPair("study-a", "a2", 2, 20)
		unsupported.Stream.Codec = "vp9"
		last := corpusV2SyntheticPair("study-a", "a3", 3, 30)
		manifest := CorpusV2Manifest{Commit: "commit-a", Pairs: []CorpusV2Pair{first, unsupported, last}}

		samples, err := SelectCorpusV2Representatives(manifest, CorpusV2Authority{RequiredStudies: []string{"study-a"}})
		assertCorpusV2Code(t, err, CorpusV2CodeInvalidMedia)
		if samples != nil {
			t.Fatalf("selected %d samples from unsupported stream metadata, want none", len(samples))
		}
	})
	t.Run("insufficient study", func(t *testing.T) {
		root := t.TempDir()
		index, err := ParseCorpusV2Index(corpusV2SyntheticIndex(t, root, "study-a", []corpusV2SyntheticRow{{Attempt: "a1"}, {Attempt: "a2"}}), root)
		if err != nil {
			t.Fatalf("parse short index: %v", err)
		}
		authority := CorpusV2Authority{RepositoryRoot: root, PairCount: 2, SectionCounts: map[string]int{"study-a": 2}, RequiredStudies: []string{"study-a"}}
		assertCorpusV2Code(t, ValidateCorpusV2Index(index, authority), CorpusV2CodeInsufficientRows)
	})
	t.Run("wrong attempt identity", func(t *testing.T) {
		root := t.TempDir()
		row := corpusV2SyntheticRow{Attempt: "a1", Clip: filepath.Join(root, "production", "study-a", "attempts", "a2", "clip.mp4")}
		_, err := ParseCorpusV2Index(corpusV2SyntheticIndex(t, root, "study-a", []corpusV2SyntheticRow{row}), root)
		assertCorpusV2Code(t, err, CorpusV2CodePathIdentity)
	})
}

func TestCorpusIndexConformance(t *testing.T) {
	authority, manifest := portableCorpusV2Fixture(t)
	indexTemplate, err := os.ReadFile(filepath.Join(authority.RepositoryRoot, authority.IndexPath))
	if err != nil {
		t.Fatalf("read portable corpus index: %v", err)
	}
	indexData := []byte(strings.ReplaceAll(string(indexTemplate), "{{ROOT}}", corpusV2Slash(authority.RepositoryRoot)))
	index, err := ParseCorpusV2Index(indexData, authority.RepositoryRoot)
	if err != nil {
		t.Fatalf("parse portable corpus index: %v", err)
	}
	if err := ValidateCorpusV2Index(index, authority); err != nil {
		t.Fatalf("validate portable corpus index counts: %v", err)
	}
	for _, pair := range index.Pairs {
		for _, path := range []string{pair.Clip.Path, pair.Prompt.Path} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("indexed sibling %s: %v", path, err)
			}
		}
	}
	if len(manifest.Pairs) != len(index.Pairs) || authority.IndexSHA256 != hashBytes(indexData) {
		t.Fatalf("portable manifest/index identities differ: manifest pairs=%d index pairs=%d", len(manifest.Pairs), len(index.Pairs))
	}
	t.Logf("portable index sha256=%s pairs=%d uniqueClips=%d uniquePrompts=%d copiedBytes=0 uploadedBytes=0", authority.IndexSHA256, len(index.Pairs), len(index.Pairs), len(index.Pairs))
}

func TestCorpusMetadataConformance(t *testing.T) {
	authority, manifest := portableCorpusV2Fixture(t)
	if !manifest.ReadOnly || manifest.CopiedBytes != 0 || manifest.UploadedBytes != 0 || manifest.MissingSiblings != 0 {
		t.Fatalf("manifest read-only accounting = %#v, want read-only and zero copy/upload/missing", manifest)
	}
	if len(manifest.Pairs) != authority.PairCount || manifest.UniqueClips != authority.PairCount || manifest.UniquePrompts != authority.PairCount || len(manifest.Samples) != len(authority.ExpectedSamples) {
		t.Fatalf("manifest counts = pairs=%d clips=%d prompts=%d samples=%d, want %d pairs and %d samples", len(manifest.Pairs), manifest.UniqueClips, manifest.UniquePrompts, len(manifest.Samples), authority.PairCount, len(authority.ExpectedSamples))
	}
	seen := make(map[string]struct{}, len(manifest.Samples))
	for _, sample := range manifest.Samples {
		if sample.SourceCommit != authority.Commit || sample.Clip.Path == "" || sample.Prompt.Path == "" || sample.Clip.Bytes <= 0 || sample.Prompt.Bytes <= 0 || sample.Clip.SHA256 == "" || sample.Prompt.SHA256 == "" {
			t.Fatalf("sample identity incomplete: %#v", sample)
		}
		if _, exists := seen[sample.Clip.Path]; exists {
			t.Fatalf("sample clip repeated: %s", sample.Clip.Path)
		}
		seen[sample.Clip.Path] = struct{}{}
		if err := validateCorpusV2Metadata(sample.Stream); err != nil {
			t.Fatalf("sample stream metadata: %v", err)
		}
		data, err := os.ReadFile(sample.Clip.Path)
		if err != nil {
			t.Fatalf("read synthetic clip for metadata conformance: %v", err)
		}
		parsed, err := parseCorpusV2MP4Metadata(data)
		if err != nil {
			t.Fatalf("parse synthetic clip metadata: %v", err)
		}
		parsed.Identity = corpusV2IdentityForStream(parsed)
		if parsed != sample.Stream {
			t.Fatalf("parsed synthetic metadata = %#v, manifest metadata = %#v", parsed, sample.Stream)
		}
		t.Logf("sample study=%s band=%s attempt=%s clipBytes=%d clipSha256=%s promptBytes=%d promptSha256=%s codec=%s dimensions=%dx%d frameRate=%s durationMillis=%d frames=%d sourceCommit=%s", sample.Study, sample.Band, sample.Attempt, sample.Clip.Bytes, sample.Clip.SHA256, sample.Prompt.Bytes, sample.Prompt.SHA256, sample.Stream.Codec, sample.Stream.Width, sample.Stream.Height, sample.Stream.FrameRate, sample.Stream.DurationMillis, sample.Stream.Frames, sample.SourceCommit)
	}
}

type corpusV2SyntheticRow struct {
	Attempt string
	Clip    string
}

func corpusV2SyntheticIndex(t *testing.T, root, study string, rows []corpusV2SyntheticRow) []byte {
	t.Helper()
	var builder strings.Builder
	fmt.Fprintln(&builder, "# Video and prompt output index")
	fmt.Fprintln(&builder, "Snapshot: test")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "This portable synthetic index contains repository-owned rows used only by component tests.")
	fmt.Fprintln(&builder)
	fmt.Fprintf(&builder, "## %s (%d)\n", study, len(rows))
	fmt.Fprintln(&builder, "| Attempt | Video path | Corresponding prompt path |")
	fmt.Fprintln(&builder, "| --- | --- | --- |")
	for _, row := range rows {
		clip := row.Clip
		if clip == "" {
			clip = filepath.Join(root, "production", study, "attempts", row.Attempt, "clip.mp4")
		}
		prompt := filepath.Join(filepath.Dir(clip), "prompt.md")
		fmt.Fprintf(&builder, "| %s | [%s](<%s>) | [%s](<%s>) |\n", row.Attempt, filepath.ToSlash(clip), filepath.ToSlash(clip), filepath.ToSlash(prompt), filepath.ToSlash(prompt))
	}
	return []byte(builder.String())
}

func corpusV2SyntheticPair(study, attempt string, duration, bytes int64) CorpusV2Pair {
	clipPath := "/tmp/" + study + "/" + attempt + "/clip.mp4"
	promptPath := "/tmp/" + study + "/" + attempt + "/prompt.md"
	stream := CorpusV2StreamMetadata{Identity: "stream:" + attempt, Codec: "avc1", Width: 480, Height: 864, FrameRate: "24/1", DurationMillis: duration * 1000, DurationSeconds: float64(duration), Frames: 120, durationNumerator: uint64(duration), durationDenominator: 1}
	return CorpusV2Pair{Study: study, Attempt: attempt, Clip: CorpusV2FileIdentity{Path: clipPath, Identity: fmt.Sprintf("file:%d:%s", bytes, attempt), Bytes: bytes, SHA256: strings.Repeat("a", 64)}, Prompt: CorpusV2FileIdentity{Path: promptPath, Identity: "prompt:" + attempt, Bytes: 10, SHA256: strings.Repeat("b", 64)}, Stream: stream}
}

func assertCorpusV2Code(t *testing.T, err error, want CorpusV2ValidationCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var validation *CorpusV2ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *CorpusV2ValidationError", err)
	}
	if validation.Code != want {
		t.Fatalf("validation code = %s, want %s (error=%v)", validation.Code, want, err)
	}
}

func corpusV2RepositoryRoot(t testing.TB) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate corpus v2 test root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", "..", ".."))
}

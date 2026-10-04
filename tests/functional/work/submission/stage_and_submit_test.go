package submission_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	platformcontentstaging "github.com/portpowered/infinite-you/pkg/platform/contentstaging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	stageAndSubmitWorkName  = "stage-and-submit-file-task"
	stageAndSubmitFileName  = "coverage.png"
	stageAndSubmitMediaType = "image/png"
)

// assertAPIStageAndSubmitFileCreatesExpectedWork proves the public HTTP
// stage-then-submit flow creates Work whose customer-visible content carries
// the staged file reference and metadata returned by POST /work/staged-files.
func assertAPIStageAndSubmitFileCreatesExpectedWork(
	t *testing.T,
	server *support.FunctionalAPIServer,
) {
	fileBytes := []byte("stage-and-submit-png-bytes")
	staged := stageSubmitWorkFile(
		t,
		server.URL(),
		"image",
		stageAndSubmitFileName,
		stageAndSubmitMediaType,
		fileBytes,
	)
	if strings.TrimSpace(staged.StagedFileRef) == "" {
		t.Fatalf("POST /work/staged-files stagedFileRef is empty, want backend-owned staged reference")
	}
	if strings.TrimSpace(string(staged.Url)) == "" {
		t.Fatalf("POST /work/staged-files url is empty, want backend-owned staged content URL")
	}
	if staged.FileName != stageAndSubmitFileName {
		t.Fatalf(
			"POST /work/staged-files fileName = %q, want %q",
			staged.FileName,
			stageAndSubmitFileName,
		)
	}
	if staged.MediaType != stageAndSubmitMediaType {
		t.Fatalf(
			"POST /work/staged-files mediaType = %q, want %q",
			staged.MediaType,
			stageAndSubmitMediaType,
		)
	}

	if got, err := os.ReadFile(stagedFilesystemPath(t, string(staged.Url))); err != nil || !bytes.Equal(got, fileBytes) {
		t.Fatalf("staged URL bytes = %q, %v", got, err)
	}
	imageItem := mustStageAndSubmitImageItem(
		t,
		staged.StagedFileRef,
		string(staged.Url),
		stageAndSubmitFileName,
		stageAndSubmitMediaType,
	)
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr(stageAndSubmitWorkName),
		WorkTypeName: batchInputsWorkType,
		Items:        &[]factoryapi.SubmitWorkItem{imageItem},
	})
	if submitted.TraceId == "" {
		t.Fatalf("POST /work traceId is empty, want customer-visible trace identity")
	}
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("POST /work workId is empty, want customer-visible work identity")
	}

	endpoint := support.DefaultSessionWorkURL(server.URL(), "/work/"+workID)
	got := support.GetJSON[factoryapi.Work](t, endpoint)
	if got.Name != stageAndSubmitWorkName {
		t.Fatalf("GET /work/%s name = %q, want %q", workID, got.Name, stageAndSubmitWorkName)
	}
	if support.StringPointerValue(got.WorkTypeName) != batchInputsWorkType {
		t.Fatalf(
			"GET /work/%s workTypeName = %q, want %q",
			workID,
			support.StringPointerValue(got.WorkTypeName),
			batchInputsWorkType,
		)
	}
	assertStageAndSubmitImageWorkContent(t, got, staged)
}

func stageSubmitWorkFile(
	t *testing.T,
	baseURL string,
	itemType string,
	fileName string,
	mediaType string,
	content []byte,
) factoryapi.StageSubmitWorkFileResponse {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"itemType":      itemType,
		"fileName":      fileName,
		"mediaType":     mediaType,
		"contentBase64": base64.StdEncoding.EncodeToString(content),
	})
	if err != nil {
		t.Fatalf("marshal stage submit-work request: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(baseURL, "/work/staged-files")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 201: %s", endpoint, response.StatusCode, payload)
	}
	var staged factoryapi.StageSubmitWorkFileResponse
	if err := json.NewDecoder(response.Body).Decode(&staged); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return staged
}

func mustStageAndSubmitImageItem(
	t *testing.T,
	stagedFileRef string,
	contentURL string,
	fileName string,
	mediaType string,
) factoryapi.SubmitWorkItem {
	t.Helper()

	var item factoryapi.SubmitWorkItem
	if err := item.FromSubmitWorkImageItem(factoryapi.SubmitWorkImageItem{
		Type:          factoryapi.SubmitWorkItemTypeImage,
		StagedFileRef: stagedFileRef,
		Url:           factoryapi.SubmitWorkContentURLProperty(contentURL),
		FileName:      fileName,
		MediaType:     mediaType,
	}); err != nil {
		t.Fatalf("encode submit-work image item: %v", err)
	}
	return item
}

func assertStageAndSubmitImageWorkContent(
	t *testing.T,
	work factoryapi.Work,
	staged factoryapi.StageSubmitWorkFileResponse,
) {
	t.Helper()

	if work.Content == nil || len(*work.Content) != 1 {
		t.Fatalf("GET /work content = %#v, want one staged image content part", work.Content)
	}
	imagePart, err := (*work.Content)[0].AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("decode projected image content: %v", err)
	}
	if imagePart.Type != factoryapi.WorkContentPartTypeImage {
		t.Fatalf("projected content type = %q, want %q", imagePart.Type, factoryapi.WorkContentPartTypeImage)
	}
	if string(imagePart.Url) != string(staged.Url) {
		t.Fatalf(
			"projected image url = %q, want staged response url %q",
			imagePart.Url,
			staged.Url,
		)
	}
	contentType := support.StringPointerValue(imagePart.ContentType)
	if contentType != stageAndSubmitMediaType {
		t.Fatalf(
			"projected image contentType = %q, want staged mediaType %q",
			contentType,
			stageAndSubmitMediaType,
		)
	}
	if imagePart.Metadata == nil {
		t.Fatalf("projected image metadata is nil, want staged file markers")
	}
	if (*imagePart.Metadata)["fileName"] != stageAndSubmitFileName {
		t.Fatalf(
			"projected image metadata fileName = %q, want %q",
			(*imagePart.Metadata)["fileName"],
			stageAndSubmitFileName,
		)
	}
	if (*imagePart.Metadata)["submissionItemType"] != "image" {
		t.Fatalf(
			"projected image metadata submissionItemType = %q, want %q",
			(*imagePart.Metadata)["submissionItemType"],
			"image",
		)
	}
}

// The staged-file API and Work API must preserve the same media identity for
// audio and documents as for images in the stage-then-submit flow.
func assertAPIStageAndSubmitMediaPreservesTypes(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	for _, scenario := range []struct {
		itemType, fileName, mediaType, contentType string
	}{
		{"audio", "speech.wav", "audio/wav", "AUDIO"},
		{"document", "notes.pdf", "application/pdf", "BINARY"},
	} {
		t.Run(scenario.itemType, func(t *testing.T) {
			staged := stageSubmitWorkFile(t, server.URL(), scenario.itemType, scenario.fileName, scenario.mediaType, []byte("staged media fixture"))
			payload, err := json.Marshal(map[string]string{
				"type": scenario.itemType, "stagedFileRef": staged.StagedFileRef,
				"url": string(staged.Url), "fileName": scenario.fileName, "mediaType": scenario.mediaType,
			})
			if err != nil {
				t.Fatal(err)
			}
			var item factoryapi.SubmitWorkItem
			if err := json.Unmarshal(payload, &item); err != nil {
				t.Fatal(err)
			}
			submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
				Name: stringPtr("stage-and-submit-" + scenario.itemType), WorkTypeName: batchInputsWorkType,
				Items: &[]factoryapi.SubmitWorkItem{item},
			})
			workID := support.StringPointerValue(submitted.WorkId)
			got := support.GetJSON[factoryapi.Work](t, support.DefaultSessionWorkURL(server.URL(), "/work/"+workID))
			if got.Content == nil || len(*got.Content) != 1 {
				t.Fatalf("Work content = %#v, want one %s part", got.Content, scenario.contentType)
			}
			partJSON, err := json.Marshal((*got.Content)[0])
			if err != nil {
				t.Fatal(err)
			}
			var part struct {
				Type, URL, ContentType string
				Metadata               map[string]any
			}
			if err := json.Unmarshal(partJSON, &part); err != nil {
				t.Fatal(err)
			}
			if part.Type != scenario.contentType || part.URL != string(staged.Url) || part.ContentType != scenario.mediaType ||
				part.Metadata["fileName"] != scenario.fileName || part.Metadata["submissionItemType"] != scenario.itemType {
				t.Fatalf("staged %s Work content = %s, want type, URL, media type and file identity preserved", scenario.itemType, partJSON)
			}
		})
	}
}

type stagingProcessWall struct{ nanos atomic.Int64 }

func (source *stagingProcessWall) Now() time.Time { return time.Unix(0, source.nanos.Load()).UTC() }

// These cells own the HTTP staging contract. Each immutable clock selection
// gets one root process; successive expiry observations reuse its session.
func TestSelectedProcessClockControlsStagedSubmissionExpiry(t *testing.T) {
	t.Parallel()
	for _, override := range []bool{false, true} {
		t.Run(strconv.FormatBool(override), func(t *testing.T) {
			t.Parallel()
			base := time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
			selected, specialized := &stagingProcessWall{}, &stagingProcessWall{}
			selected.nanos.Store(base.UnixNano())
			specialized.nanos.Store(base.Add(24 * time.Hour).UnixNano())
			effective := selected
			edges := serviceedges.Edges{Clock: selected, ProviderCommandRunner: submissionInputPreservingProviderRunner(),
				WorkContentStagingFileSystem: stagingRootFiles{root: t.TempDir()}}
			if override {
				edges.WorkContentStagingClock = specialized
				effective = specialized
			}
			dir := support.ScaffoldFactory(t, submissionInputPreservingFactoryConfig())
			configureSubmissionCodexWorkers(t, dir, "worker-a")
			server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir, Edges: edges})
			t.Cleanup(func() { server.Stop(t) })
			assertAPIStageAndSubmitFileCreatesExpectedWork(t, server)
			issuedAt := effective.Now()
			before := stageSubmitWorkFile(t, server.URL(), "image", stageAndSubmitFileName, stageAndSubmitMediaType, []byte("before expiry"))
			expired := stageSubmitWorkFile(t, server.URL(), "image", stageAndSubmitFileName, stageAndSubmitMediaType, []byte("exact expiry"))
			if override {
				selected.nanos.Store(base.Add(48 * time.Hour).UnixNano())
			}
			effective.nanos.Store(issuedAt.Add(time.Hour - time.Nanosecond).UnixNano())
			item := mustStageAndSubmitImageItem(t, before.StagedFileRef, string(before.Url), before.FileName, before.MediaType)
			support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{WorkTypeName: batchInputsWorkType, Items: &[]factoryapi.SubmitWorkItem{item}})
			prior := support.ListDefaultSessionWork(t, server.URL())
			effective.nanos.Store(issuedAt.Add(time.Hour).UnixNano())
			assertExpiredStagedSubmissionRejected(t, server, expired)
			if _, err := os.Stat(stagedFilesystemPath(t, string(expired.Url))); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expired file cleanup = %v", err)
			}
			after := support.ListDefaultSessionWork(t, server.URL())
			if len(after.Results) != len(prior.Results) {
				t.Fatalf("expired submission admitted Work: before=%d after=%d", len(prior.Results), len(after.Results))
			}
		})
	}
}

func assertExpiredStagedSubmissionRejected(t *testing.T, server *support.FunctionalAPIServer, staged factoryapi.StageSubmitWorkFileResponse) {
	t.Helper()
	item := mustStageAndSubmitImageItem(t, staged.StagedFileRef, string(staged.Url), staged.FileName, staged.MediaType)
	body, err := json.Marshal(factoryapi.SubmitWorkRequest{WorkTypeName: batchInputsWorkType, Items: &[]factoryapi.SubmitWorkItem{item}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(payload), "stagedFileRef has expired") {
		t.Fatalf("expiry response = %d %s", response.StatusCode, payload)
	}
}

func stagedFilesystemPath(t *testing.T, contentURL string) string {
	t.Helper()
	parsed, err := url.Parse(contentURL)
	if err != nil || parsed.Scheme != "file" {
		t.Fatalf("content URL = %q, %v", contentURL, err)
	}
	path := parsed.Path
	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
	}
	return filepath.FromSlash(path)
}

type stagingRootFiles struct {
	platformcontentstaging.FileSystem
	root string
}

func (files stagingRootFiles) MkdirTemp(_ string, pattern string) (string, error) {
	return os.MkdirTemp(files.root, pattern)
}

type failingStagingFiles struct{ stagingRootFiles }

func (files failingStagingFiles) WriteFile(string, []byte, fs.FileMode) error {
	return errors.New("controlled private staging failure")
}

func TestStagingWriteFailureRemovesPartialContentThroughPublicAPI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := support.ScaffoldFactory(t, submissionInputPreservingFactoryConfig())
	configureSubmissionCodexWorkers(t, dir, "worker-a")
	cfg := submissionServerConfig(dir, submissionInputPreservingProviderRunner())
	cfg.Edges.WorkContentStagingFileSystem = failingStagingFiles{stagingRootFiles: stagingRootFiles{root: root}}
	server := support.StartFunctionalAPIServer(t, cfg)
	t.Cleanup(func() { server.Stop(t) })
	payload := `{"itemType":"image","fileName":"failure.png","mediaType":"image/png","contentBase64":"aW1hZ2U="}`
	response, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work/staged-files"), "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusInternalServerError || failure.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("staging failure = %d %#v", response.StatusCode, failure)
	}
	if strings.Contains(failure.Message, "controlled private") {
		t.Fatalf("private effect detail exposed: %#v", failure)
	}
	remaining, err := os.ReadDir(root)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("partial staged content remains = %v, %v", remaining, err)
	}
}

package submission_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	stageAndSubmitWorkName  = "stage-and-submit-file-task"
	stageAndSubmitFileName  = "coverage.png"
	stageAndSubmitMediaType = "image/png"
)

// TestAPIStageAndSubmitFileCreatesExpectedWork proves the public HTTP stage-then-
// submit flow creates Work whose customer-visible content carries the staged file
// reference and metadata returned by POST /work/staged-files.
func TestAPIStageAndSubmitFileCreatesExpectedWork(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchInputsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

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

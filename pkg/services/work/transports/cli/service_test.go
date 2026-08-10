package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type testHTTPClock struct{}

func (testHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func testHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, testHTTPClock{})
	if err != nil {
		t.Fatalf("build test HTTP protocol: %v", err)
	}
	return protocol
}

type testListRequestPreparation struct{}

func (testListRequestPreparation) PrepareListRequest(
	_ context.Context,
	options workdomain.ListOptions,
) (workdomain.PreparedListRequest, error) {
	return workdomain.PreparedListRequest{Options: options}, nil
}

func constructedWorkCLIService(
	t *testing.T,
	prepare workdomain.ListRequestPreparation,
	visualize workdomain.VisualizationOperation,
) workcli.Service {
	t.Helper()
	service := workcli.New(workcli.Config{ListPrepare: prepare, Visualize: visualize})
	if service == nil {
		t.Fatal("New(cfg) = nil, want Work CLI service")
	}
	return service
}

func TestConstructedService_ListRequiresListPreparation(t *testing.T) {
	t.Parallel()

	service := workcli.New(workcli.Config{})
	var out bytes.Buffer
	err := service.List(workcli.ListConfig{
		Context: context.Background(),
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
	})
	if err == nil || err.Error() != "Work list request preparation is required" {
		t.Fatalf("error = %v, want Work list request preparation is required", err)
	}
}

func TestConstructedService_RequiresCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	prepare := workdomain.NewListRequestPreparation()
	service := constructedWorkCLIService(t, prepare, nil)

	tests := map[string]func() error{
		"list": func() error {
			return service.List(workcli.ListConfig{Context: context.Background(), HTTP: testHTTPProtocol(t)})
		},
		"move": func() error {
			return service.Move(workcli.MoveConfig{Context: context.Background(), HTTP: testHTTPProtocol(t)})
		},
		"show": func() error {
			return service.Show(workcli.ShowConfig{Context: context.Background(), HTTP: testHTTPProtocol(t)})
		},
	}
	for name, run := range tests {
		name, run := name, run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("error = %v, want output writer is required", err)
			}
		})
	}
}

func TestConstructedService_ShowRequiresCallerContext(t *testing.T) {
	t.Parallel()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	err := service.Show(workcli.ShowConfig{Output: &bytes.Buffer{}, HTTP: testHTTPProtocol(t)})
	if err == nil || err.Error() != "context is required" {
		t.Fatalf("error = %v, want context is required", err)
	}
}

func TestConstructedService_VisualizeRequiresOperationAndOutput(t *testing.T) {
	t.Parallel()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	if err := service.Visualize(workcli.VisualizeConfig{Output: &bytes.Buffer{}}); err == nil ||
		!strings.Contains(err.Error(), "operation is required") {
		t.Fatalf("missing operation error = %v", err)
	}
	if err := service.Visualize(workcli.VisualizeConfig{}); err == nil ||
		!strings.Contains(err.Error(), "output is required") {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestConstructedService_ListHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prepare := workdomain.NewListRequestPreparation()
	service := constructedWorkCLIService(t, prepare, nil)
	var out bytes.Buffer
	err := service.List(workcli.ListConfig{
		Context: ctx,
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestConstructedService_ListSuccessPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, testListRequestPreparation{}, nil)
	var out bytes.Buffer
	err := service.List(workcli.ListConfig{
		Context: context.Background(),
		Server:  srv.URL,
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := out.String(); got != "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\nwork-1\tReview PRD\tstory\treview\tPROCESSING\t\tnone\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestConstructedService_VisualizeSuccessPath(t *testing.T) {
	t.Parallel()

	operation := func(request workdomain.VisualizationRequest) (string, error) {
		if request.BatchFile != "batch.json" || request.Format != "mermaid" {
			t.Fatalf("request = %#v", request)
		}
		return "flowchart TD\n", nil
	}
	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), operation)
	var out bytes.Buffer
	err := service.Visualize(workcli.VisualizeConfig{
		BatchFile: "batch.json",
		Format:    "mermaid",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Visualize: %v", err)
	}
	if out.String() != "flowchart TD\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestConstructedService_ListRendersArtifactStatesAndPreservesJSONShape(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name: "Review PRD", WorkId: stringPtr("work-1"), WorkTypeName: stringPtr("story"),
				ExpectedArtifacts: &[]factoryapi.WorkExpectedArtifact{
					{Name: "report", Pattern: "reports/review.json", Verification: factoryapi.WorkExpectedArtifactVerificationPending},
					{Name: "manifest", Pattern: "reports/manifest.json", NonEmpty: true, Verification: factoryapi.WorkExpectedArtifactVerificationSatisfied},
					{Name: "empty", Pattern: "reports/empty.json", Verification: factoryapi.WorkExpectedArtifactVerificationFailed, Reason: expectedArtifactReasonPtr(factoryapi.ExpectedArtifactVerificationReasonEmpty)},
				},
			}},
		})
	}))
	defer server.Close()

	service := constructedWorkCLIService(t, testListRequestPreparation{}, nil)
	var human bytes.Buffer
	if err := service.List(workcli.ListConfig{Context: context.Background(), Server: server.URL, Output: &human, HTTP: testHTTPProtocol(t)}); err != nil {
		t.Fatalf("human List: %v", err)
	}
	if !strings.Contains(human.String(), "Artifacts: report=reports/review.json [PENDING]; manifest=reports/manifest.json [SATISFIED]; empty=reports/empty.json [FAILED: EMPTY]") {
		t.Fatalf("human output = %q, want concise artifact states", human.String())
	}
	if strings.Contains(human.String(), server.URL) || strings.Contains(human.String(), "\\") {
		t.Fatalf("human output exposed a host path: %q", human.String())
	}

	var encoded bytes.Buffer
	if err := service.List(workcli.ListConfig{Context: context.Background(), Server: server.URL, JSON: true, Output: &encoded, HTTP: testHTTPProtocol(t)}); err != nil {
		t.Fatalf("JSON List: %v", err)
	}
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(encoded.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].ExpectedArtifacts == nil ||
		len(*response.Results[0].ExpectedArtifacts) != 3 {
		t.Fatalf("JSON response = %#v, want API-shaped artifact projection", response)
	}
}

func expectedArtifactReasonPtr(reason factoryapi.ExpectedArtifactVerificationReason) *factoryapi.ExpectedArtifactVerificationReason {
	return &reason
}

func stringPtr(value string) *string {
	return &value
}

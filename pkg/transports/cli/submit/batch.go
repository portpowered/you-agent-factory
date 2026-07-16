package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
)

const batchErrorBodyPreviewLimit = 200

// BatchConfig holds parameters for the submit batch command.
type BatchConfig struct {
	FilePath    string
	FileFlag    string
	DryRun      bool
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Args        []string
	Stdin       io.Reader
	StdinIsTTY  func() bool
	Output      io.Writer
	Diagnostics io.Writer
}

// SubmitBatch upserts a canonical FACTORY_REQUEST_BATCH to a running factory.
func SubmitBatch(cfg BatchConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		return err
	}

	req, err := requests.ParseCanonicalWorkRequestJSON(resolved.data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", resolved.label, err)
	}
	if err := validateBatchRequest(req); err != nil {
		return err
	}

	batchSource := resolved.source
	endpointPath := batchSubmitEndpointPath(cfg.SessionID, req.RequestID)
	if cfg.DryRun {
		return printBatchDryRunSummary(cfg, req, batchSource, endpointPath)
	}

	return upsertBatchHTTP(cfg, req, resolved.data, batchSource)
}

func validateBatchRequest(req work.WorkRequest) error {
	if req.Type != work.WorkRequestTypeFactoryRequestBatch {
		return fmt.Errorf("batch type must be %q", work.WorkRequestTypeFactoryRequestBatch)
	}
	if strings.TrimSpace(req.RequestID) == "" {
		return fmt.Errorf("batch requestId is required")
	}
	if len(req.Works) == 0 {
		return fmt.Errorf("batch works must contain at least one item")
	}
	return nil
}

func printBatchDryRunSummary(cfg BatchConfig, req work.WorkRequest, batchSource, endpointPath string) error {
	names := make([]string, 0, len(req.Works))
	for _, work := range req.Works {
		names = append(names, work.Name)
	}
	relationCount := len(req.Relations)

	if cfg.JSON {
		return printBatchDryRunJSON(cfg.Output, cfg.SessionID, endpointPath, batchSource, req)
	}

	if _, err := fmt.Fprintf(cfg.Output, "requestId: %s\n", req.RequestID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.Output, "work count: %d\n", len(req.Works)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.Output, "works: %s\n", strings.Join(names, ", ")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.Output, "relationCount: %d\n", relationCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.Output, "batchSource: %s\n", batchSource); err != nil {
		return err
	}
	_, err := fmt.Fprintln(cfg.Output, "dry-run: no request sent")
	return err
}

func upsertBatchHTTP(cfg BatchConfig, req work.WorkRequest, body []byte, batchSource string) error {
	requestID := req.RequestID
	endpointPath := batchSubmitEndpointPath(cfg.SessionID, requestID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return err
	}

	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"submit batch request endpointPath=%s endpoint=%s server=%s session=%s batchSource=%s requestBytes=%d requestId=%q workCount=%d",
		endpointPath,
		endpointURL,
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		batchSource,
		len(body),
		requestID,
		len(req.Works),
	)

	started := time.Now()
	client := &http.Client{Timeout: submitRequestTimeout}
	var result factoryapi.UpsertWorkRequestResponse
	resp, err := clihttp.PutJSONCreated(
		context.Background(),
		client,
		endpointURL,
		bytes.NewReader(body),
		&result,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpointPath,
			LogLabel:     "submit batch",
		},
	)
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", endpointURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		return batchSubmissionHTTPError(resp.StatusCode, respBody)
	}

	responseBytes := 0
	if encoded, marshalErr := json.Marshal(result); marshalErr == nil {
		responseBytes = len(encoded)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s status=%d durationMillis=%d responseBytes=%d requestId=%s traceId=%s workCount=%d", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), responseBytes, result.RequestId, result.TraceId, len(result.Works))

	if cfg.JSON {
		return printBatchSuccessJSON(cfg.Output, cfg.SessionID, endpointPath, batchSource, req, result)
	}

	return printBatchSuccessHuman(cfg.Output, req, result)
}

func batchSubmissionHTTPError(statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		return fmt.Errorf("batch submission failed (%d): %s", statusCode, errResp.Message)
	}
	preview := strings.TrimSpace(string(body))
	if preview == "" {
		return fmt.Errorf("batch submission failed (%d)", statusCode)
	}
	if len(preview) > batchErrorBodyPreviewLimit {
		preview = preview[:batchErrorBodyPreviewLimit] + "..."
	}
	return fmt.Errorf("batch submission failed (%d): %s", statusCode, preview)
}

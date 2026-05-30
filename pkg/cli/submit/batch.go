package submit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BatchConfig holds parameters for the submit batch command.
type BatchConfig struct {
	FilePath  string
	FileFlag  string
	DryRun    bool
	Server    string
	SessionID string
	JSON      bool
	Verbose   bool
	Debug     bool
	Args      []string
	Output    io.Writer
	Diagnostics io.Writer
}

// SubmitBatch upserts a canonical FACTORY_REQUEST_BATCH to a running factory.
func SubmitBatch(cfg BatchConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	path, err := resolveBatchFileInput(cfg)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	req, err := requests.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if err := validateBatchRequest(req); err != nil {
		return err
	}

	batchSource := batchSourceFile
	if cfg.DryRun {
		return printBatchDryRunSummary(cfg, req, batchSource)
	}

	return upsertBatchHTTP(cfg, req, data, batchSource)
}

func validateBatchRequest(req interfaces.WorkRequest) error {
	if req.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		return fmt.Errorf("batch type must be %q", interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if strings.TrimSpace(req.RequestID) == "" {
		return fmt.Errorf("batch requestId is required")
	}
	if len(req.Works) == 0 {
		return fmt.Errorf("batch works must contain at least one item")
	}
	return nil
}

func printBatchDryRunSummary(cfg BatchConfig, req interfaces.WorkRequest, batchSource string) error {
	names := make([]string, 0, len(req.Works))
	for _, work := range req.Works {
		names = append(names, work.Name)
	}
	relationCount := len(req.Relations)

	if cfg.JSON {
		summary := map[string]any{
			"dryRun":        true,
			"requestId":     req.RequestID,
			"workCount":     len(req.Works),
			"relationCount": relationCount,
			"batchSource":   batchSource,
			"workNames":     names,
		}
		if traceID := batchTraceIDFromRequest(req); traceID != "" {
			summary["traceId"] = traceID
		}
		return json.NewEncoder(cfg.Output).Encode(summary)
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

func batchTraceIDFromRequest(req interfaces.WorkRequest) string {
	if req.CurrentChainingTraceID != "" {
		return req.CurrentChainingTraceID
	}
	for _, work := range req.Works {
		if work.TraceID != "" {
			return work.TraceID
		}
	}
	return ""
}

func upsertBatchHTTP(cfg BatchConfig, req interfaces.WorkRequest, body []byte, batchSource string) error {
	requestID := req.RequestID
	endpointPath := sessionpath.ScopedPath("/work-requests/"+url.PathEscape(requestID), cfg.SessionID)
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
	httpReq, err := http.NewRequest(http.MethodPut, endpointURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s error=unreachable durationMillis=%d", endpointPath, time.Since(started).Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpointURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), len(respBody))
		var errResp factoryapi.ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("batch submission failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("batch submission failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result factoryapi.UpsertWorkRequestResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s status=%d durationMillis=%d responseBytes=%d requestId=%s traceId=%s workCount=%d", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), len(respBody), result.RequestId, result.TraceId, len(result.Works))

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}

	_, err = fmt.Fprintf(cfg.Output, "Upserted batch %s (trace %s, %d works)\n", result.RequestId, result.TraceId, len(result.Works))
	return err
}

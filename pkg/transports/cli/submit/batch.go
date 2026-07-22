package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const batchErrorBodyPreviewLimit = 200

// BatchConfig holds parameters for the submit batch command.
type BatchConfig struct {
	Context     context.Context
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
	FileSystem  BatchInputFileSystem
	HTTP        clihttp.Protocol
}

func NewSubmitBatch(
	transport clihttp.Protocol,
	prepare work.FactoryRequestBatchPreparation,
) func(BatchConfig) error {
	return func(cfg BatchConfig) error { cfg.HTTP = transport; return SubmitBatch(prepare, cfg) }
}

// BatchInputFileSystem is the exact local-file effect required by batch input
// acquisition. Wire supplies the policy-free Platform adapter; CLI retains
// only source selection and error presentation.
type BatchInputFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
}

// SubmitBatch upserts a canonical FACTORY_REQUEST_BATCH to a running factory.
func SubmitBatch(prepare work.FactoryRequestBatchPreparation, cfg BatchConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if prepare == nil {
		return fmt.Errorf("Factory Request Batch preparation is required")
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		return err
	}

	prepared, err := prepare.PrepareFactoryRequestBatch(cfg.Context, resolved.data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", resolved.label, err)
	}
	req := prepared.Request

	batchSource := resolved.source
	endpointPath := batchSubmitEndpointPath(cfg.SessionID, req.RequestID)
	if cfg.DryRun {
		return printBatchDryRunSummary(cfg, req, batchSource, endpointPath)
	}

	return upsertBatchHTTP(cfg, req, prepared.CanonicalJSON, batchSource)
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
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
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

	var result factoryapi.UpsertWorkRequestResponse
	response, err := cfg.HTTP.PutJSONCreated(
		cfg.Context,
		endpointURL,
		bytes.NewReader(body),
		&result,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s error=unreachable durationMillis=%d", endpointPath, response.Duration.Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpointURL, err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s status=%d durationMillis=%d", endpointPath, resp.StatusCode, response.Duration.Milliseconds())
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
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit batch response endpointPath=%s status=%d durationMillis=%d responseBytes=%d requestId=%s traceId=%s workCount=%d", endpointPath, resp.StatusCode, response.Duration.Milliseconds(), responseBytes, result.RequestId, result.TraceId, len(result.Works))

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

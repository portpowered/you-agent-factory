package work

import (
	"encoding/json"
	"errors"
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
	"github.com/portpowered/infinite-you/pkg/factory/state"
)

const showRequestTimeout = 10 * time.Second

var ErrWorkNotFound = errors.New("work not found")

// ShowConfig holds parameters for the work show command.
type ShowConfig struct {
	Server      string
	SessionID   string
	WorkID      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Show requests one work item from a running factory via HTTP.
func Show(cfg ShowConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	workID := strings.TrimSpace(cfg.WorkID)
	if workID == "" {
		return fmt.Errorf("work id is required")
	}

	endpoint, err := showEndpoint(cfg, workID)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work show request endpointPath=%s endpoint=%s server=%s session=%s workId=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		workID,
	)

	client := &http.Client{Timeout: showRequestTimeout}
	started := time.Now()
	resp, err := client.Get(endpoint.String())
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, time.Since(started).Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "work show response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds())
		return showRequestError(resp, workID)
	}

	var token factoryapi.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	work := workFromTokenResponse(token)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work show response endpointPath=%s status=%d durationMillis=%d workId=%s",
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		stringValue(work.WorkId),
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(work)
	}
	return renderShowResult(cfg.Output, work)
}

func showEndpoint(cfg ShowConfig, workID string) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/work/"+url.PathEscape(workID), cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse work show endpoint: %w", err)
	}
	return *endpoint, nil
}

func showRequestError(resp *http.Response, workID string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("get work failed (%d)", resp.StatusCode)
	}
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		if resp.StatusCode == http.StatusNotFound && errResp.Code == factoryapi.NOTFOUND {
			return fmt.Errorf("%w: %s", ErrWorkNotFound, errResp.Message)
		}
		return fmt.Errorf("get work failed (%d): %s", resp.StatusCode, errResp.Message)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: work %q not found", ErrWorkNotFound, workID)
	}
	return fmt.Errorf("get work failed (%d)", resp.StatusCode)
}

func workFromTokenResponse(token factoryapi.TokenResponse) factoryapi.Work {
	workTypeID, stateName := state.SplitPlaceID(token.PlaceId)
	if token.WorkType != "" {
		workTypeID = token.WorkType
	}
	name := stringValue(token.Name)
	if name == "" {
		name = firstNonEmptyString(token.WorkId, token.Id)
	}
	work := factoryapi.Work{
		Name:                     name,
		WorkId:                   stringPtrIfNotEmpty(token.WorkId),
		WorkTypeName:             stringPtrIfNotEmpty(token.WorkType),
		ChainingTraceDepth:       token.ChainingTraceDepth,
		CurrentChainingTraceId:   token.CurrentChainingTraceId,
		PreviousChainingTraceIds: token.PreviousChainingTraceIds,
		Content:                  token.Content,
		Tags:                     token.Tags,
	}
	if token.TraceId != "" {
		work.TraceId = &token.TraceId
	}
	if stateName != "" {
		work.State = &factoryapi.WorkState{
			Name: stateName,
			Type: factoryapi.WorkStateType(state.CategoryForState(nil, workTypeID, stateName)),
		}
	}
	return work
}

func renderShowResult(output io.Writer, work factoryapi.Work) error {
	stateName, stateType := workStateColumns(work.State)
	lines := []struct {
		label string
		value string
	}{
		{"WORK ID", stringValue(work.WorkId)},
		{"NAME", work.Name},
		{"WORK TYPE", stringValue(work.WorkTypeName)},
		{"STATE NAME", stateName},
		{"STATE TYPE", stateType},
		{"TRACE", primaryTraceID(work)},
		{"RELATIONS", formatWorkRelations(work.Relations)},
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", line.label, line.value); err != nil {
			return err
		}
	}
	return nil
}

func primaryTraceID(work factoryapi.Work) string {
	if work.CurrentChainingTraceId != nil && *work.CurrentChainingTraceId != "" {
		return *work.CurrentChainingTraceId
	}
	return stringValue(work.TraceId)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

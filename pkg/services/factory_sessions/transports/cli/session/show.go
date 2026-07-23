// Package session implements factory-session lifecycle command behavior.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ShowConfig holds parameters for the session show command.
type ShowConfig struct {
	Context     context.Context
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func NewShow(transport clihttp.Protocol) func(ShowConfig) error {
	return func(cfg ShowConfig) error { cfg.HTTP = transport; return Show(cfg) }
}

// Show requests one live factory session projection from a running host via HTTP.
func Show(cfg ShowConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	if isDurableExecutionSessionID(cfg.SessionID) {
		return showDurableSession(cfg)
	}

	endpoint, err := showEndpoint(cfg)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session show request endpointPath=%s endpoint=%s server=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
	)

	var result factoryapi.FactorySession
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&result,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session show response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session show response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("factory session %q not found: %s", resolvedSessionID(cfg.SessionID), errResp.Message)
		}
		return fmt.Errorf("factory session %q not found", resolvedSessionID(cfg.SessionID))
	}
	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session show response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("get factory session failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("get factory session failed (%d)", resp.StatusCode)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session show response endpointPath=%s status=%d durationMillis=%d sessionId=%s orchestratorKind=%s",
		endpoint.Path,
		resp.StatusCode,
		response.Duration.Milliseconds(),
		result.Id,
		result.Runtime.OrchestratorKind,
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	partialResult, liveResult, err := fetchSessionResultProjections(cfg)
	if err != nil {
		return err
	}
	return renderShowResult(cfg.Output, result, partialResult, liveResult)
}

func showEndpoint(cfg ShowConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session show endpoint: %w", err)
	}
	return *endpoint, nil
}

func fetchSessionResultProjections(
	cfg ShowConfig,
) (*factoryapi.FactorySessionPartialResult, *factoryapi.FactorySessionLiveResult, error) {
	partialPath := sessionpath.ScopedPath("/partial-result", cfg.SessionID)
	partialEndpoint, err := cliserver.RequestURL(cfg.Server, partialPath)
	if err != nil {
		return nil, nil, err
	}
	resultPath := sessionpath.ScopedPath("/result", cfg.SessionID)
	resultEndpoint, err := cliserver.RequestURL(cfg.Server, resultPath)
	if err != nil {
		return nil, nil, err
	}
	var partialResult *factoryapi.FactorySessionPartialResult
	var liveResult *factoryapi.FactorySessionLiveResult
	var decodedPartial factoryapi.FactorySessionPartialResult
	if response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		partialEndpoint,
		&decodedPartial,
	); err == nil {
		resp := response.HTTP
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			partialResult = &decodedPartial
		}
	}
	var decodedResult factoryapi.FactorySessionLiveResult
	if response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		resultEndpoint,
		&decodedResult,
	); err == nil {
		resp := response.HTTP
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			liveResult = &decodedResult
		}
	}
	return partialResult, liveResult, nil
}

func renderShowResult(
	output io.Writer,
	session factoryapi.FactorySession,
	partialResult *factoryapi.FactorySessionPartialResult,
	liveResult *factoryapi.FactorySessionLiveResult,
) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "Factory session", value: session.Id},
		{label: "Project", value: session.Project},
		{label: "Folder path", value: session.FolderPath},
		{label: "Factory dir", value: session.FactoryDir},
		{label: "Default session", value: defaultMarker(session.IsDefault)},
		{label: "Target kind", value: string(session.Target.Kind)},
		{label: "Target name", value: targetName(session.Target.Name)},
		{label: "Orchestrator kind", value: string(session.Runtime.OrchestratorKind)},
		{label: "Runtime status", value: string(session.Runtime.Status)},
		{label: "Factory state", value: session.Runtime.Progress.FactoryState},
		{label: "Total tokens", value: fmt.Sprintf("%d", session.Runtime.Progress.TotalTokens)},
		{label: "In-flight dispatches", value: fmt.Sprintf("%d", session.Runtime.Progress.InFlightCount)},
	}
	if session.Runtime.Dialect != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Workflow dialect", value: strings.TrimSpace(*session.Runtime.Dialect)})
	}
	if session.Runtime.SourceRef != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Workflow source ref", value: strings.TrimSpace(*session.Runtime.SourceRef)})
	}
	if session.Runtime.PolicyHash != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Policy hash", value: strings.TrimSpace(*session.Runtime.PolicyHash)})
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if err := writeSessionLifecycleFields(output, session.Runtime.Lifecycle); err != nil {
		return err
	}
	if err := writeSessionStopSummary(output, session.Runtime.StopSummary); err != nil {
		return err
	}
	if err := writeSessionArtifactLines(output, session.Runtime.Artifacts); err != nil {
		return err
	}
	if err := writeSessionResultLines(output, partialResult, liveResult); err != nil {
		return err
	}

	switch session.Runtime.OrchestratorKind {
	case factoryapi.JAVASCRIPT:
		return renderJavaScriptSessionProjection(output, session.Runtime.Javascript)
	default:
		return renderPetriSessionProjection(output, session.Runtime.Petri)
	}
}

func writeSessionLifecycleFields(
	output io.Writer,
	lifecycle factoryapi.FactorySessionLifecycle,
) error {
	if _, err := fmt.Fprintf(output, "Session started:\t%s\n", lifecycle.StartedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Session updated:\t%s\n", lifecycle.UpdatedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if lifecycle.FinishedAt != nil {
		if _, err := fmt.Fprintf(output, "Session finished:\t%s\n", lifecycle.FinishedAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

func writeSessionStopSummary(output io.Writer, summary *factoryapi.FactoryStopSummary) error {
	if summary == nil {
		return nil
	}
	fields := sessionStopSummaryFields(summary)
	if summary.SessionLifecycleStatus != nil {
		fields = append(fields, "lifecycle="+string(*summary.SessionLifecycleStatus))
	}
	if _, err := fmt.Fprintf(output, "Stop summary:\t%s\n", strings.Join(fields, " ")); err != nil {
		return err
	}
	return writeStopSummaryDetailLines(output, summary)
}

func sessionStopSummaryFields(summary *factoryapi.FactoryStopSummary) []string {
	fields := []string{
		fmt.Sprintf("kind=%s", summary.StopKind),
		fmt.Sprintf("session=%s", summary.SessionId),
	}
	if workField := sessionStopSummaryWorkField(summary); workField != "" {
		fields = append(fields, "work="+workField)
	}
	if stateField := trimmedString(summary.WorkState); stateField != "" {
		fields = append(fields, "state="+stateField)
	}
	return fields
}

func sessionStopSummaryWorkField(summary *factoryapi.FactoryStopSummary) string {
	workName := trimmedString(summary.WorkName)
	workID := trimmedString(summary.WorkId)
	if workName == "" {
		return workID
	}
	if workID == "" {
		return workName
	}
	return fmt.Sprintf("%s [%s]", workName, workID)
}

func writeStopSummaryDetailLines(output io.Writer, summary *factoryapi.FactoryStopSummary) error {
	if err := writeStopDispatchLine(output, summary.LatestDispatch); err != nil {
		return err
	}
	if err := writeOptionalStopSummaryLine(output, "Stop result", summary.LatestResultSummary); err != nil {
		return err
	}
	if err := writeOptionalStopSummaryLine(output, "Recovery surface", summary.SuggestedRecoverySurface); err != nil {
		return err
	}
	return writeOptionalStopSummaryLine(output, "Recovery action", summary.SuggestedRecoveryAction)
}

func writeStopDispatchLine(output io.Writer, dispatch *factoryapi.FactoryStopDispatchSummary) error {
	if dispatch == nil {
		return nil
	}
	dispatchFields := []string{
		dispatch.DispatchId,
		fmt.Sprintf("status=%s", dispatch.Status),
		fmt.Sprintf("kind=%s", dispatch.DispatchKind),
	}
	if workstation := trimmedString(dispatch.WorkstationName); workstation != "" {
		dispatchFields = append(dispatchFields, "workstation="+workstation)
	}
	_, err := fmt.Fprintf(output, "Stop dispatch:\t%s\n", strings.Join(dispatchFields, " "))
	return err
}

func writeOptionalStopSummaryLine(output io.Writer, label string, value *string) error {
	text := trimmedString(value)
	if text == "" {
		return nil
	}
	_, err := fmt.Fprintf(output, "%s:\t%s\n", label, text)
	return err
}

func trimmedString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func writeSessionArtifactLines(
	output io.Writer,
	artifacts *[]factoryapi.FactoryArtifact,
) error {
	if artifacts == nil {
		return nil
	}
	for _, artifact := range *artifacts {
		label := artifact.Id
		if artifact.Label != nil && strings.TrimSpace(*artifact.Label) != "" {
			label = fmt.Sprintf("%s (%s)", artifact.Id, strings.TrimSpace(*artifact.Label))
		}
		if _, err := fmt.Fprintf(
			output,
			"Artifact ref:\t%s kind=%s visibility=%s\n",
			label,
			artifact.Kind,
			artifact.Visibility,
		); err != nil {
			return err
		}
	}
	return nil
}

func writeSessionResultLines(
	output io.Writer,
	partialResult *factoryapi.FactorySessionPartialResult,
	liveResult *factoryapi.FactorySessionLiveResult,
) error {
	if partialResult != nil && partialResult.PartialResultArtifactRef != nil {
		if _, err := fmt.Fprintf(
			output,
			"Partial result ref:\t%s (%s)\n",
			partialResult.PartialResultArtifactRef.Id,
			partialResult.PartialResultArtifactRef.Kind,
		); err != nil {
			return err
		}
	}
	if liveResult != nil && liveResult.ResultArtifactRef != nil {
		if _, err := fmt.Fprintf(
			output,
			"Final result ref:\t%s (%s)\n",
			liveResult.ResultArtifactRef.Id,
			liveResult.ResultArtifactRef.Kind,
		); err != nil {
			return err
		}
	}
	return nil
}

func renderPetriSessionProjection(output io.Writer, petri *factoryapi.FactorySessionPetriProjection) error {
	if petri == nil {
		_, err := fmt.Fprintln(output, "Petri projection:\tnone")
		return err
	}
	if _, err := fmt.Fprintf(output, "Petri marking tokens:\t%d\n", len(petri.Marking)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Enabled transitions:\t%d\n", len(petri.EnabledTransitions)); err != nil {
		return err
	}
	for _, transition := range petri.EnabledTransitions {
		if _, err := fmt.Fprintf(
			output,
			"Enabled transition:\t%s (%s)\n",
			transition.TransitionId,
			transition.WorkerType,
		); err != nil {
			return err
		}
	}
	return nil
}

func renderJavaScriptSessionProjection(output io.Writer, javascript *factoryapi.FactorySessionJavaScriptProjection) error {
	if javascript == nil {
		_, err := fmt.Fprintln(output, "Dynamic workflow projection:\tnone")
		return err
	}
	if _, err := fmt.Fprintln(output, "Dynamic workflow:\tJavaScript factory session"); err != nil {
		return err
	}
	if err := writeJavaScriptPhaseFields(output, javascript); err != nil {
		return err
	}
	return writeJavaScriptCheckpointRefs(output, javascript.Checkpoints)
}

func writeJavaScriptPhaseFields(output io.Writer, javascript *factoryapi.FactorySessionJavaScriptProjection) error {
	if javascript.Phase != nil {
		if _, err := fmt.Fprintf(output, "Phase:\t%s\n", strings.TrimSpace(*javascript.Phase)); err != nil {
			return err
		}
	}
	if len(javascript.Phases) > 0 {
		if _, err := fmt.Fprintf(output, "Phases:\t%s\n", strings.Join(javascript.Phases, ", ")); err != nil {
			return err
		}
	}
	if javascript.ArgsDigest != nil {
		if _, err := fmt.Fprintf(output, "Args digest:\t%s\n", strings.TrimSpace(*javascript.ArgsDigest)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "Script status:\t%s\n", javascript.ScriptStatus); err != nil {
		return err
	}
	_, err := fmt.Fprintf(
		output,
		"Child dispatches:\tqueued=%d running=%d completed=%d\n",
		javascript.ChildDispatchCounts.Queued,
		javascript.ChildDispatchCounts.Running,
		javascript.ChildDispatchCounts.Completed,
	)
	return err
}

func writeJavaScriptCheckpointRefs(output io.Writer, checkpoints *[]factoryapi.FactorySessionJavaScriptCheckpointRef) error {
	if checkpoints == nil {
		return nil
	}
	for _, checkpoint := range *checkpoints {
		label := checkpoint.Id
		if checkpoint.Label != nil && strings.TrimSpace(*checkpoint.Label) != "" {
			label = fmt.Sprintf("%s (%s)", checkpoint.Id, strings.TrimSpace(*checkpoint.Label))
		}
		if checkpoint.Summary != nil && strings.TrimSpace(*checkpoint.Summary) != "" {
			label = fmt.Sprintf("%s — %s", label, strings.TrimSpace(*checkpoint.Summary))
		}
		if _, err := fmt.Fprintf(output, "Checkpoint ref:\t%s\n", label); err != nil {
			return err
		}
	}
	return nil
}

func resolvedSessionID(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return sessionpath.DefaultFactorySessionID
	}
	return strings.TrimSpace(sessionID)
}

func isDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

func showDurableSession(cfg ShowConfig) error {
	endpoint, err := durableShowEndpoint(cfg)
	if err != nil {
		return err
	}

	var durable factoryapi.FactorySessionDurableReadModel
	response, err := cfg.HTTP.GetJSON(
		cfg.Context,
		endpoint.String(),
		&durable,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session show durable response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session show durable response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("factory session %q not found: %s", resolvedSessionID(cfg.SessionID), errResp.Message)
		}
		return fmt.Errorf("factory session %q not found", resolvedSessionID(cfg.SessionID))
	}
	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session show durable response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("get factory session failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("get factory session failed (%d)", resp.StatusCode)
	}

	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(durable)
	}
	return renderDurableShowResult(cfg.Output, durable)
}

func durableShowEndpoint(cfg ShowConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse durable session show endpoint: %w", err)
	}
	return *endpoint, nil
}

func renderDurableShowResult(output io.Writer, session factoryapi.FactorySessionDurableReadModel) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "Factory session", value: session.SessionId},
		{label: "Lifecycle status", value: string(session.Status)},
		{label: "Orchestrator kind", value: string(session.OrchestratorKind)},
		{label: "Source", value: formatDurableSource(session)},
		{label: "Duration", value: formatDurableDuration(session.Lifecycle)},
	}
	if session.Dialect != nil {
		rows = append(rows, struct {
			label string
			value string
		}{label: "Workflow dialect", value: strings.TrimSpace(*session.Dialect)})
	}
	rows = append(rows,
		struct{ label, value string }{label: "Current phase", value: formatOptionalString(session.Phase)},
		struct{ label, value string }{label: "Dispatch counts", value: formatDurableProgressSummary(session.Progress)},
		struct{ label, value string }{label: "Latest checkpoint", value: formatDurableCheckpoint(session.LatestCheckpoint)},
		struct{ label, value string }{label: "Effective policy", value: formatDurablePolicy(session)},
		struct{ label, value string }{label: "Budget", value: formatDurableBudget(session.Budgets)},
		struct{ label, value string }{label: "Usage", value: formatDurableUsage(session.Usage)},
		struct{ label, value string }{label: "Artifacts", value: formatDurableArtifacts(session.ArtifactRefs)},
		struct{ label, value string }{label: "Result availability", value: formatDurableResult(session)},
	)
	if session.ResultSummary != nil {
		rows = append(rows, struct{ label, value string }{label: "Result status", value: string(session.ResultSummary.ResultStatus)})
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if err := writeDurableLifecycleFields(output, session.Lifecycle); err != nil {
		return err
	}
	return writeDurablePhaseSummaries(output, session.PhaseSummaries)
}

func formatDurableProgressSummary(progress *factoryapi.FactorySessionDurableProgressCounts) string {
	if progress == nil {
		return "unavailable"
	}
	parts := make([]string, 0, 4)
	if progress.TotalDispatches != nil {
		parts = append(parts, fmt.Sprintf("total=%d", *progress.TotalDispatches))
	}
	if progress.CompletedDispatches != nil {
		parts = append(parts, fmt.Sprintf("completed=%d", *progress.CompletedDispatches))
	}
	if progress.InFlightDispatches != nil {
		parts = append(parts, fmt.Sprintf("in flight=%d", *progress.InFlightDispatches))
	}
	if progress.FailedDispatches != nil && *progress.FailedDispatches > 0 {
		parts = append(parts, fmt.Sprintf("failed=%d", *progress.FailedDispatches))
	}
	if len(parts) == 0 {
		return "unavailable"
	}
	return strings.Join(parts, ", ")
}

func formatOptionalString(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "unavailable"
	}
	return strings.TrimSpace(*value)
}

func formatDurableSource(session factoryapi.FactorySessionDurableReadModel) string {
	parts := []string{string(session.ResolvedSource.Kind)}
	if session.ResolvedSource.SourceRef != nil {
		parts = append(parts, "ref="+strings.TrimSpace(*session.ResolvedSource.SourceRef))
	}
	if session.SourceHash != nil {
		parts = append(parts, "hash="+strings.TrimSpace(*session.SourceHash))
	}
	return strings.Join(parts, " ")
}

func formatDurableDuration(lifecycle *factoryapi.FactorySessionDurableLifecycleTimestamps) string {
	if lifecycle == nil || lifecycle.StartedAt == nil || lifecycle.FinishedAt == nil {
		return "unavailable"
	}
	return lifecycle.FinishedAt.Sub(*lifecycle.StartedAt).Round(time.Millisecond).String()
}

func formatDurableCheckpoint(checkpoint *factoryapi.FactorySessionCheckpointRef) string {
	if checkpoint == nil {
		return "none"
	}
	parts := []string{checkpoint.Id}
	if checkpoint.Label != nil && strings.TrimSpace(*checkpoint.Label) != "" {
		parts = append(parts, "label="+strings.TrimSpace(*checkpoint.Label))
	}
	if checkpoint.Phase != nil && strings.TrimSpace(*checkpoint.Phase) != "" {
		parts = append(parts, "phase="+strings.TrimSpace(*checkpoint.Phase))
	}
	return strings.Join(parts, " ")
}

func formatDurablePolicy(session factoryapi.FactorySessionDurableReadModel) string {
	if session.EffectivePolicy == nil && session.EffectivePolicyHash == nil {
		return "unavailable"
	}
	if session.EffectivePolicyHash != nil {
		return "hash=" + strings.TrimSpace(*session.EffectivePolicyHash)
	}
	if session.EffectivePolicy.PolicyHash != nil {
		return "hash=" + strings.TrimSpace(*session.EffectivePolicy.PolicyHash)
	}
	return "available"
}

func formatDurableBudget(budgets *factoryapi.FactorySessionBudgets) string {
	if budgets == nil || budgets.MaxAgents == nil {
		return "unavailable"
	}
	return fmt.Sprintf("max agents=%d", *budgets.MaxAgents)
}

func formatDurableUsage(usage *factoryapi.FactorySessionUsage) string {
	if usage == nil {
		return "unavailable"
	}
	if len(usage.Resources) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(usage.Resources))
	for _, resource := range usage.Resources {
		parts = append(parts, fmt.Sprintf("%s=%d/%d", resource.Name, resource.Total-resource.Available, resource.Total))
	}
	return strings.Join(parts, ", ")
}

func formatDurableArtifacts(artifacts *[]factoryapi.FactoryArtifactRef) string {
	if artifacts == nil || len(*artifacts) == 0 {
		return "none"
	}
	ids := make([]string, 0, len(*artifacts))
	for _, artifact := range *artifacts {
		ids = append(ids, fmt.Sprintf("%s (%s)", artifact.Id, artifact.Kind))
	}
	return strings.Join(ids, ", ")
}

func formatDurableResult(session factoryapi.FactorySessionDurableReadModel) string {
	if session.ResultSummary == nil {
		if session.PartialResultAvailable != nil && *session.PartialResultAvailable {
			return "PARTIAL"
		}
		return "unavailable"
	}
	value := string(session.ResultSummary.ResultStatus)
	if session.ResultSummary.Summary != nil && strings.TrimSpace(*session.ResultSummary.Summary) != "" {
		value += " — " + strings.TrimSpace(*session.ResultSummary.Summary)
	}
	return value
}

func writeDurablePhaseSummaries(output io.Writer, summaries *[]factoryapi.FactorySessionDurablePhaseSummary) error {
	if summaries == nil || len(*summaries) == 0 {
		_, err := fmt.Fprintln(output, "Phase summaries:\tnone")
		return err
	}
	if _, err := fmt.Fprintln(output, "Phase summaries:"); err != nil {
		return err
	}
	for _, summary := range *summaries {
		counts := []string{}
		if summary.DispatchCount != nil {
			counts = append(counts, fmt.Sprintf("total=%d", *summary.DispatchCount))
		}
		if summary.CompletedDispatchCount != nil {
			counts = append(counts, fmt.Sprintf("completed=%d", *summary.CompletedDispatchCount))
		}
		if summary.FailedDispatchCount != nil {
			counts = append(counts, fmt.Sprintf("failed=%d", *summary.FailedDispatchCount))
		}
		if len(counts) == 0 {
			counts = append(counts, "counts unavailable")
		}
		if _, err := fmt.Fprintf(output, "- %s %s\n", summary.Phase, strings.Join(counts, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func writeDurableLifecycleFields(
	output io.Writer,
	lifecycle *factoryapi.FactorySessionDurableLifecycleTimestamps,
) error {
	if lifecycle == nil {
		return nil
	}
	fields := []struct {
		label string
		value *time.Time
	}{
		{label: "Queued at", value: lifecycle.QueuedAt},
		{label: "Started at", value: lifecycle.StartedAt},
		{label: "Paused at", value: lifecycle.PausedAt},
		{label: "Interrupted at", value: lifecycle.InterruptedAt},
		{label: "Resumed at", value: lifecycle.ResumedAt},
		{label: "Finished at", value: lifecycle.FinishedAt},
	}
	for _, field := range fields {
		if field.value == nil {
			continue
		}
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", field.label, field.value.Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

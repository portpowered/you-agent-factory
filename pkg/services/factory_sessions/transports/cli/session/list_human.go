package session

import (
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func renderListResult(output io.Writer, result factoryapi.ListFactorySessionsResponse) error {
	hasLive := len(result.Sessions) > 0
	hasDurable := result.DurableSessions != nil && len(*result.DurableSessions) > 0
	hasRecorded := result.RecordedSessions != nil && len(*result.RecordedSessions) > 0
	if !hasLive && !hasDurable && !hasRecorded {
		return renderListEmptyState(output, result.Scope)
	}
	if hasLive {
		if err := renderLiveSessionTable(output, result.Sessions); err != nil {
			return err
		}
	}
	if hasDurable {
		if hasLive {
			if _, err := fmt.Fprintln(output); err != nil {
				return err
			}
		}
		if err := renderDurableSessionTable(output, *result.DurableSessions); err != nil {
			return err
		}
	}
	if hasRecorded {
		if hasLive || hasDurable {
			if _, err := fmt.Fprintln(output); err != nil {
				return err
			}
		}
		if err := renderRecordedSessionTable(output, *result.RecordedSessions); err != nil {
			return err
		}
	}
	return nil
}

func renderListEmptyState(output io.Writer, scope *factoryapi.FactorySessionListScope) error {
	message := "No live factory sessions were found."
	if scope != nil {
		switch *scope {
		case factoryapi.FactorySessionListScopePersisted:
			message = "No persisted Factory Sessions were found."
		case factoryapi.FactorySessionListScopeHistory:
			message = "No recorded Factory Session history was found."
		case factoryapi.FactorySessionListScopeAll:
			message = "No live factory sessions, persisted Factory Sessions, or recorded history was found."
		}
	}
	_, err := fmt.Fprintln(output, message)
	return err
}

func renderRecordedSessionTable(output io.Writer, sessions []factoryapi.FactorySessionRecordedSummary) error {
	if _, err := fmt.Fprintln(output, "Factory Sessions (recorded history):"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "SESSION ID\tSOURCE\tARTIFACT REFERENCE\tFORMAT"); err != nil {
		return err
	}
	for _, session := range sessions {
		if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\n", session.SessionId, session.Source, session.ArtifactReference, session.Format); err != nil {
			return err
		}
	}
	return nil
}

func renderLiveSessionTable(output io.Writer, sessions []factoryapi.FactorySessionSummary) error {
	if _, err := fmt.Fprintln(output, "SESSION ID\tPROJECT\tFOLDER PATH\tFACTORY DIR\tDEFAULT\tORCHESTRATOR KIND\tTARGET KIND\tTARGET NAME"); err != nil {
		return err
	}
	for _, session := range sessions {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			session.Id,
			session.Project,
			session.FolderPath,
			session.FactoryDir,
			defaultMarker(session.IsDefault),
			orchestratorKindLabel(session),
			session.Target.Kind,
			targetName(session.Target.Name),
		); err != nil {
			return err
		}
	}
	return nil
}

func renderDurableSessionTable(output io.Writer, sessions []factoryapi.FactorySessionDurableSummary) error {
	if _, err := fmt.Fprintln(output, "Factory Sessions (durable):"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(
		output,
		"SESSION ID\tSTATUS\tORCHESTRATOR\tSOURCE KIND\tSOURCE REF\tRESULT STATUS\tPHASE\tPROGRESS\tACTIONS",
	); err != nil {
		return err
	}
	for _, session := range sessions {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			session.SessionId,
			session.Status,
			session.OrchestratorKind,
			session.ResolvedSource.Kind,
			resolvedSourceRef(session.ResolvedSource),
			durableResultStatus(session.ResultSummary),
			durablePhase(session.Phase),
			durableProgressSummary(session.Progress),
			durableActionSummary(session.Actions),
		); err != nil {
			return err
		}
	}
	return nil
}

func resolvedSourceRef(source factoryapi.FactorySessionResolvedSourceIdentity) string {
	if source.SourceRef == nil {
		return ""
	}
	return strings.TrimSpace(*source.SourceRef)
}

func durableResultStatus(summary *factoryapi.FactorySessionDurableResultSummary) string {
	if summary == nil {
		return ""
	}
	return string(summary.ResultStatus)
}

func durablePhase(phase *string) string {
	if phase == nil {
		return ""
	}
	return strings.TrimSpace(*phase)
}

func durableProgressSummary(progress *factoryapi.FactorySessionDurableProgressCounts) string {
	if progress == nil {
		return ""
	}
	var parts []string
	if progress.CompletedDispatches != nil {
		parts = append(parts, fmt.Sprintf("completed=%d", *progress.CompletedDispatches))
	}
	if progress.InFlightDispatches != nil {
		parts = append(parts, fmt.Sprintf("inFlight=%d", *progress.InFlightDispatches))
	}
	parts = appendProgressPart(parts, "queued", progress.QueuedDispatches)
	parts = appendProgressPart(parts, "running", progress.RunningDispatches)
	parts = appendProgressPart(parts, "canceled", progress.CanceledDispatches)
	parts = appendProgressPart(parts, "timedOut", progress.TimedOutDispatches)
	parts = appendProgressPart(parts, "skipped", progress.SkippedDispatches)
	parts = appendProgressPart(parts, "interrupted", progress.InterruptedDispatches)
	if progress.FailedDispatches != nil {
		parts = append(parts, fmt.Sprintf("failed=%d", *progress.FailedDispatches))
	}
	if progress.TotalDispatches != nil {
		parts = append(parts, fmt.Sprintf("total=%d", *progress.TotalDispatches))
	}
	if progress.PhaseCount != nil {
		parts = append(parts, fmt.Sprintf("phases=%d", *progress.PhaseCount))
	}
	return strings.Join(parts, " ")
}

func appendProgressPart(parts []string, label string, value *int) []string {
	if value == nil {
		return parts
	}
	return append(parts, fmt.Sprintf("%s=%d", label, *value))
}

func durableActionSummary(actions *factoryapi.FactorySessionDurableActionAvailability) string {
	if actions == nil {
		return ""
	}
	var enabled []string
	if boolPtrTrue(actions.CanPause) {
		enabled = append(enabled, "pause")
	}
	if boolPtrTrue(actions.CanResume) {
		enabled = append(enabled, "resume")
	}
	if boolPtrTrue(actions.CanCancel) {
		enabled = append(enabled, "cancel")
	}
	if boolPtrTrue(actions.CanTerminate) {
		enabled = append(enabled, "terminate")
	}
	if boolPtrTrue(actions.CanApprove) {
		enabled = append(enabled, "approve")
	}
	if boolPtrTrue(actions.CanRetryDispatch) {
		enabled = append(enabled, "retry-dispatch")
	}
	if len(enabled) == 0 {
		return "none"
	}
	return strings.Join(enabled, ",")
}

func boolPtrTrue(value *bool) bool {
	return value != nil && *value
}

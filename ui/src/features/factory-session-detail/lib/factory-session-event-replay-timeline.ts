import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import { formatDateTime } from "../../../i18n/formatters";
import type { FactorySessionDetailMessages } from "../messages/factory-session-detail";

export type FactorySessionEventReplayTone =
  | "danger"
  | "info"
  | "neutral"
  | "success"
  | "warning";

export interface FactorySessionEventReplayTimelineItem {
  detail?: string;
  id: string;
  orderLabel: string;
  referenceSummary?: string;
  timeLabel: string;
  title: string;
  tone: FactorySessionEventReplayTone;
  typeLabel: string;
}

export function buildFactorySessionEventReplayTimeline(
  events: FactoryEvent[],
  messages: FactorySessionDetailMessages,
  locale?: string,
): FactorySessionEventReplayTimelineItem[] {
  return [...events]
    .sort(compareFactoryReplayEvents)
    .map((event) => buildTimelineItem(event, messages, locale));
}

function buildTimelineItem(
  event: FactoryEvent,
  messages: FactorySessionDetailMessages,
  locale?: string,
): FactorySessionEventReplayTimelineItem {
  const eventDetails = describeEvent(event, messages);
  const referenceSummary = buildReferenceSummary(event, messages);

  return {
    detail: eventDetails.detail,
    id: event.id,
    orderLabel: messages.eventReplaySequenceTickLabel(
      event.context.sessionSequence ?? event.context.sequence,
      event.context.tick,
    ),
    referenceSummary,
    timeLabel: formatDateTime(event.context.eventTime, locale, {
      timeZone: "UTC",
    }),
    title: eventDetails.title,
    tone: eventDetails.tone,
    typeLabel: humanizeEventText(event.type),
  };
}

function describeEvent(
  event: FactoryEvent,
  messages: FactorySessionDetailMessages,
): Pick<FactorySessionEventReplayTimelineItem, "detail" | "title" | "tone"> {
  const payload = asRecord(event.payload);

  switch (event.type) {
    case FACTORY_EVENT_TYPES.sessionStarted:
      return {
        detail: summarizeText(payload.sourceRef),
        title: messages.eventReplaySessionStartedTitle,
        tone: "info",
      };
    case FACTORY_EVENT_TYPES.orchestratorPhaseChanged:
    case "JAVASCRIPT_PHASE_CHANGE":
      return {
        detail:
          summarizeText(payload.progressSummary) ??
          summarizeText(event.context.phaseName) ??
          summarizeText(payload.phase),
        title: messages.eventReplayPhaseChangedTitle,
        tone: "info",
      };
    case FACTORY_EVENT_TYPES.dispatchQueued:
      return {
        detail: [
          summarizeText(payload.label),
          typeof payload.queuePosition === "number"
            ? messages.eventReplayQueuePositionLabel(payload.queuePosition)
            : undefined,
        ]
          .filter(Boolean)
          .join(" · "),
        title: messages.eventReplayDispatchQueuedTitle,
        tone: "info",
      };
    case "DISPATCH_RECONCILED": {
      const status = summarizeText(payload.reconciledStatus);
      return {
        detail:
          summarizeText(asRecord(payload.failureDetail).message) ??
          (status
            ? messages.eventReplayDispatchStatusDetail(status)
            : undefined) ??
          summarizeText(asRecord(payload.resultArtifactRef).label),
        title: messages.eventReplayDispatchReconciledTitle,
        tone: mapStatusTone(status),
      };
    }
    case "DISPATCH_INTERRUPTED":
      return {
        detail: [
          summarizeText(payload.reason),
          payload.retryPlanned === true
            ? messages.eventReplayRetryPlannedLabel
            : undefined,
        ]
          .filter(Boolean)
          .join(" · "),
        title: messages.eventReplayDispatchInterruptedTitle,
        tone: payload.retryPlanned === true ? "warning" : "danger",
      };
    case FACTORY_EVENT_TYPES.sessionResultUpdated: {
      const resultStatus = summarizeText(payload.resultStatus);
      return {
        detail: resultStatus
          ? messages.eventReplayResultStatusDetail(resultStatus)
          : undefined,
        title: messages.eventReplaySessionResultUpdatedTitle,
        tone: mapStatusTone(resultStatus),
      };
    }
    case FACTORY_EVENT_TYPES.sessionCompleted: {
      const finalStatus = summarizeText(payload.finalStatus);
      return {
        detail:
          summarizeText(asRecord(payload.failureDetail).message) ??
          (finalStatus
            ? messages.eventReplayLifecycleStatusDetail(finalStatus)
            : undefined),
        title: messages.eventReplaySessionCompletedTitle,
        tone: mapStatusTone(finalStatus),
      };
    }
    case "ORCHESTRATOR_CHECKPOINT_WRITTEN":
    case "JAVASCRIPT_CHECKPOINT_REF": {
      const warningCount = Array.isArray(payload.warnings)
        ? payload.warnings.length
        : 0;
      return {
        detail:
          summarizeText(payload.label) ??
          (warningCount > 0
            ? messages.eventReplayWarningCountLabel(warningCount)
            : undefined),
        title: messages.eventReplayCheckpointRecordedTitle,
        tone: warningCount > 0 ? "warning" : "info",
      };
    }
    default:
      return {
        detail: undefined,
        title: humanizeEventText(event.type),
        tone: "neutral",
      };
  }
}

function buildReferenceSummary(
  event: FactoryEvent,
  messages: FactorySessionDetailMessages,
): string | undefined {
  const payload = asRecord(event.payload);
  const references: string[] = [];
  const phase = summarizeText(event.context.phaseName ?? event.context.phaseId);
  if (phase) {
    references.push(messages.eventReplayPhaseLabel(phase));
  }
  if (event.context.dispatchId) {
    references.push(
      messages.eventReplayDispatchLabel(event.context.dispatchId),
    );
  }
  if (event.context.checkpointId) {
    references.push(
      messages.eventReplayCheckpointLabel(event.context.checkpointId),
    );
  }

  const artifactIDs = extractArtifactIDs(payload);
  if (artifactIDs.length === 1) {
    references.push(messages.eventReplayArtifactLabel(artifactIDs[0]));
  } else if (artifactIDs.length > 1) {
    references.push(messages.eventReplayArtifactCountLabel(artifactIDs.length));
  }

  if (event.context.workIds && event.context.workIds.length > 0) {
    references.push(
      messages.eventReplayWorkLabel(event.context.workIds.length),
    );
  }

  return references.length > 0
    ? references.join(" · ")
    : messages.eventReplayNoContext;
}

function extractArtifactIDs(payload: Record<string, unknown>): string[] {
  const ids: string[] = [];
  if (Array.isArray(payload.artifactIds)) {
    for (const artifactID of payload.artifactIds) {
      if (typeof artifactID === "string" && artifactID.trim() !== "") {
        ids.push(artifactID);
      }
    }
  }

  const artifactRef = asRecord(payload.artifactRef);
  if (typeof artifactRef.id === "string" && artifactRef.id.trim() !== "") {
    ids.push(artifactRef.id);
  }

  const resultArtifactRef = asRecord(payload.resultArtifactRef);
  if (
    typeof resultArtifactRef.id === "string" &&
    resultArtifactRef.id.trim() !== ""
  ) {
    ids.push(resultArtifactRef.id);
  }

  return [...new Set(ids)];
}

function compareFactoryReplayEvents(
  left: FactoryEvent,
  right: FactoryEvent,
): number {
  const leftSequence = left.context.sessionSequence ?? left.context.sequence;
  const rightSequence = right.context.sessionSequence ?? right.context.sequence;
  if (leftSequence !== rightSequence) {
    return leftSequence - rightSequence;
  }
  if (left.context.sequence !== right.context.sequence) {
    return left.context.sequence - right.context.sequence;
  }
  return left.context.eventTime.localeCompare(right.context.eventTime);
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : {};
}

function summarizeText(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

function humanizeEventText(value: string): string {
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function mapStatusTone(
  status: string | undefined,
): FactorySessionEventReplayTone {
  switch (status) {
    case "COMPLETED":
    case "FINAL":
    case "FINISHED":
    case "SUCCEEDED":
      return "success";
    case "FAILED":
    case "FAILED_WITH_PARTIAL":
    case "INTERRUPTED":
    case "TERMINATED":
    case "TIMED_OUT":
      return "danger";
    case "CANCELED":
    case "PAUSED":
    case "UNAVAILABLE":
      return "warning";
    default:
      return "neutral";
  }
}

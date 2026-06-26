import { useId, useState } from "react";

import type { FactoryEvent } from "../../../../api/events";
import {
  AlertPanel,
  DashboardLabel,
  DashboardStatusPill,
  DashboardText,
} from "../../../../components/ui";
import { ExpandablePanelTrigger } from "../../../../components/ui/expandable-panel-trigger";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import { useFactorySessionEventReplay } from "../../hooks/use-factory-session-event-replay";
import { getFactorySessionDetailMessages } from "../../messages/factory-session-detail";

const MAX_VISIBLE_REPLAY_EVENTS = 12;

export interface FactorySessionEventReplayDisclosureProps {
  locale?: string;
  sessionID: string;
}

export function FactorySessionEventReplayDisclosure({
  locale,
  sessionID,
}: FactorySessionEventReplayDisclosureProps) {
  const messages = getFactorySessionDetailMessages(locale);
  const detailRegionID = useId();
  const [expanded, setExpanded] = useState(false);
  const replayState = useFactorySessionEventReplay(sessionID, expanded);

  return (
    <section className="grid gap-3 rounded-lg border border-outline bg-surface-container-low p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="grid gap-2">
          <DashboardLabel>{messages.eventReplayHeading}</DashboardLabel>
          <DetailCopy>{messages.eventReplayHint}</DetailCopy>
        </div>
        <ExpandablePanelTrigger
          aria-label={
            expanded
              ? messages.collapseEventReplayLabel
              : messages.expandEventReplayLabel
          }
          controlsID={detailRegionID}
          expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          variant="compact"
        >
          {messages.eventReplayHeading}
        </ExpandablePanelTrigger>
      </div>

      {expanded ? (
        <EventReplayState
          detailRegionID={detailRegionID}
          locale={locale}
          state={replayState}
        />
      ) : null}
    </section>
  );
}

function EventReplayState({
  detailRegionID,
  locale,
  state,
}: {
  detailRegionID: string;
  locale?: string;
  state: ReturnType<typeof useFactorySessionEventReplay>;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  if (state.status === "loading") {
    return (
      <DetailCopy id={detailRegionID}>{messages.eventReplayLoadingState}</DetailCopy>
    );
  }

  if (state.status === "error") {
    return (
      <AlertPanel id={detailRegionID} tone="danger">
        {state.message ?? messages.eventReplayErrorState}
      </AlertPanel>
    );
  }

  if (state.status !== "success") {
    return null;
  }

  if (state.events.length === 0) {
    return (
      <DetailCopy id={detailRegionID}>{messages.eventReplayEmptyState}</DetailCopy>
    );
  }

  const visibleEvents = state.events.slice(-MAX_VISIBLE_REPLAY_EVENTS);
  const truncatedCount = state.events.length - visibleEvents.length;

  return (
    <div
      className="grid gap-3 border-t border-outline pt-3"
      id={detailRegionID}
    >
      <DashboardText as="div" variant="supporting">
        {truncatedCount > 0
          ? messages.eventReplaySummary(
              visibleEvents.length,
              state.events.length,
            )
          : messages.eventReplayVisibleSummary(visibleEvents.length)}
      </DashboardText>
      <ol className="grid gap-2">
        {visibleEvents.map((event) => (
          <li
            className="grid gap-2 rounded-md border border-outline bg-surface px-3 py-2"
            key={event.id}
          >
            <div className="flex flex-wrap items-center gap-2">
              <DashboardStatusPill size="compact">
                {formatEventType(event.type)}
              </DashboardStatusPill>
              {event.context.sessionSequence != null ? (
                <DashboardText variant="supporting">
                  {messages.eventReplaySequenceLabel(event.context.sessionSequence)}
                </DashboardText>
              ) : null}
            </div>
            <DashboardText>{formatEventContextSummary(event, messages)}</DashboardText>
            <DashboardText variant="supporting">
              {event.context.eventTime}
            </DashboardText>
          </li>
        ))}
      </ol>
    </div>
  );
}

function formatEventContextSummary(
  event: FactoryEvent,
  messages: ReturnType<typeof getFactorySessionDetailMessages>,
): string {
  const details: string[] = [];
  const phase = event.context.phaseName ?? event.context.phaseId;
  if (phase) {
    details.push(messages.eventReplayPhaseLabel(phase));
  }
  if (event.context.dispatchId) {
    details.push(messages.eventReplayDispatchLabel(event.context.dispatchId));
  }
  if (event.context.checkpointId) {
    details.push(messages.eventReplayCheckpointLabel(event.context.checkpointId));
  }
  if (event.context.workIds && event.context.workIds.length > 0) {
    details.push(messages.eventReplayWorkLabel(event.context.workIds.length));
  }
  return details.length > 0 ? details.join(" · ") : messages.eventReplayNoContext;
}

function formatEventType(eventType: string): string {
  return eventType
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

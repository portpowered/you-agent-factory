import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { useId, useState } from "react";
import {
  AlertPanel,
  DashboardStatusPill,
  Label,
  Text,
} from "../../../../components/ui";
import { ExpandablePanelTrigger } from "../../../../components/ui/expandable-panel-trigger";
import { useFactorySessionEventReplay } from "../../hooks/use-factory-session-event-replay";
import { buildFactorySessionEventReplayTimeline } from "../../lib/factory-session-event-replay-timeline";
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
          <Label>{messages.eventReplayHeading}</Label>
          <WidgetDetailCopy>{messages.eventReplayHint}</WidgetDetailCopy>
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
      <WidgetDetailCopy id={detailRegionID}>
        {messages.eventReplayLoadingState}
      </WidgetDetailCopy>
    );
  }

  if (state.status === "error") {
    return (
      <AlertPanel id={detailRegionID} tone="danger">
        {state.message ?? messages.eventReplayErrorState}
      </AlertPanel>
    );
  }

  if (state.status === "unavailable") {
    return (
      <AlertPanel id={detailRegionID} tone="info">
        {state.message ?? messages.eventReplayUnavailableState}
      </AlertPanel>
    );
  }

  if (state.status !== "success") {
    return null;
  }

  if (state.events.length === 0) {
    return (
      <WidgetDetailCopy id={detailRegionID}>
        {messages.eventReplayEmptyState}
      </WidgetDetailCopy>
    );
  }

  const visibleEvents = state.events.slice(-MAX_VISIBLE_REPLAY_EVENTS);
  const truncatedCount = state.events.length - visibleEvents.length;
  const timelineItems = buildFactorySessionEventReplayTimeline(
    visibleEvents,
    messages,
    locale,
  );

  return (
    <div
      className="grid gap-3 border-t border-outline pt-3"
      id={detailRegionID}
    >
      <Text as="div" variant="supporting">
        {truncatedCount > 0
          ? messages.eventReplaySummary(
              visibleEvents.length,
              state.events.length,
            )
          : messages.eventReplayVisibleSummary(visibleEvents.length)}
      </Text>
      <ol className="grid gap-2">
        {timelineItems.map((item) => (
          <li
            className="grid gap-2 rounded-md border border-outline bg-surface px-3 py-2"
            key={item.id}
          >
            <div className="flex flex-wrap items-center gap-2">
              <DashboardStatusPill size="compact" tone={item.tone}>
                {item.title}
              </DashboardStatusPill>
              <DashboardStatusPill size="compact">
                {item.typeLabel}
              </DashboardStatusPill>
            </div>
            {item.detail ? <Text>{item.detail}</Text> : null}
            <Text variant="supporting">{item.referenceSummary}</Text>
            <div className="flex flex-wrap items-center gap-2">
              <Text variant="supporting">{item.orderLabel}</Text>
              <Text variant="supporting">{item.timeLabel}</Text>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

import { Heading, Label, Text } from "@you-agent-factory/components/primitives";
import { type ReactNode, useId, useState } from "react";

import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import type {
  WorkerSessionTimelineEntry,
  WorkerTimelineTerminalOutcome,
} from "../lib/worker-session-timeline-projection-types";
import type { WorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import {
  EntryStructuredDetails,
  type WorkerSessionTimelineEntryDetailsProps,
} from "./worker-session-timeline-entry-details";

export function WorkerSessionTimelineEntryView({
  entry,
  messages,
  onNavigateToWorkerSession,
  position = entry.canonical.position,
  totalEntries,
}: WorkerSessionTimelineEntryDetailsProps) {
  const categoryLabel =
    messages.categoryLabel[entry.category] ?? messages.unknownCategoryLabel;
  const phaseLabel = messages.phaseLabel(entry.phase);
  const title = `${categoryLabel} · ${phaseLabel}`;
  const detail = entryHasDetail(entry) ? (
    <WorkerSessionTimelineEntryDetails
      entry={entry}
      messages={messages}
      onNavigateToWorkerSession={onNavigateToWorkerSession}
    />
  ) : null;

  return (
    <li
      aria-posinset={position}
      aria-setsize={totalEntries}
      className="min-w-0"
      data-worker-session-timeline-entry-position={entry.canonical.position}
    >
      <article
        aria-label={`${title}, ${messages.eventPositionLabel(entry.canonical.position)}`}
        className="grid min-w-0 gap-3 rounded-lg border border-outline bg-surface-container-low p-3"
      >
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <DashboardStatusPill size="compact" tone={entryTone(entry)}>
            {categoryLabel}
          </DashboardStatusPill>
          <DashboardStatusPill size="compact" tone="neutral">
            {phaseLabel}
          </DashboardStatusPill>
          <span className="af-supporting-text text-on-surface-subtle">
            {messages.eventPositionLabel(entry.canonical.position)}
          </span>
        </div>
        <Heading as="h3" className="m-0 min-w-0 [overflow-wrap:anywhere]">
          {title}
        </Heading>
        <EntryMetadata entry={entry} messages={messages} />
        {detail ?? <Text variant="supporting">{messages.noDetailState}</Text>}
      </article>
    </li>
  );
}

function EntryMetadata({
  entry,
  messages,
}: {
  entry: WorkerSessionTimelineEntry;
  messages: WorkerSessionTimelineMessages;
}) {
  const items: Array<{ label: string; value: ReactNode }> = [];
  if (entry.identity?.provider) {
    items.push({
      label: messages.providerLabel,
      value: entry.identity.provider,
    });
  }
  if (entry.identity?.modelProvider) {
    items.push({
      label: messages.modelProviderLabel,
      value: entry.identity.modelProvider,
    });
  }
  if (entry.identity?.model) {
    items.push({ label: messages.modelLabel, value: entry.identity.model });
  }
  if (entry.attempt) {
    items.push({
      label: messages.attemptLabel(entry.attempt.number, entry.attempt.id),
      value: entry.attempt.status ?? messages.attemptIDLabel,
    });
  }
  if (entry.canonical.sourceType) {
    items.push({
      label: messages.sourceLabel,
      value: entry.canonical.sourceType,
    });
  }

  if (items.length === 0) {
    return null;
  }

  return (
    <dl className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-2">
      {items.map((item) => (
        <div
          className="grid min-w-0 gap-1"
          key={`${item.label}-${String(item.value)}`}
        >
          <Label as="dt">{item.label}</Label>
          <Text as="dd" className="m-0 min-w-0 [overflow-wrap:anywhere]">
            {item.value}
          </Text>
        </div>
      ))}
    </dl>
  );
}

function WorkerSessionTimelineEntryDetails({
  entry,
  messages,
  onNavigateToWorkerSession,
}: WorkerSessionTimelineEntryDetailsProps) {
  const controlsID = useId();
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="grid min-w-0 gap-2">
      <ExpandablePanelTrigger
        aria-label={messages.detailsLabel(expanded)}
        controlsID={controlsID}
        expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
        variant="compact"
      >
        {messages.detailsLabel(expanded)}
      </ExpandablePanelTrigger>
      {expanded ? (
        <div
          className="grid min-w-0 gap-3 border-t border-outline pt-3"
          id={controlsID}
        >
          <EntryStructuredDetails
            entry={entry}
            messages={messages}
            onNavigateToWorkerSession={onNavigateToWorkerSession}
          />
        </div>
      ) : null}
    </div>
  );
}

function entryHasDetail(entry: WorkerSessionTimelineEntry): boolean {
  return Boolean(
    entry.attempt ||
      entry.continuation ||
      entry.message ||
      entry.reasoning ||
      entry.progress ||
      entry.tool ||
      entry.usage ||
      entry.failure ||
      entry.terminal ||
      entry.generic,
  );
}

function entryTone(
  entry: WorkerSessionTimelineEntry,
): "danger" | "info" | "neutral" | "success" | "warning" {
  if (entry.terminal) {
    return terminalTone(entry.terminal.outcome);
  }
  if (entry.category === "error") {
    return "danger";
  }
  if (entry.category === "tool" || entry.category === "reasoning") {
    return "info";
  }
  return "neutral";
}

function terminalTone(
  outcome: WorkerTimelineTerminalOutcome,
): "danger" | "success" | "warning" {
  return outcome === "SUCCESS"
    ? "success"
    : outcome === "FAILURE"
      ? "danger"
      : "warning";
}

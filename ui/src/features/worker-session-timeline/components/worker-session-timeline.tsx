import { SurfacePanel } from "@you-agent-factory/components/layout";
import {
  Button,
  Heading,
  Label,
  Text,
} from "@you-agent-factory/components/primitives";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";

import { AlertPanel } from "../../../components/ui/alert-panel";
import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { cn } from "../../../lib/cn";
import { DashboardWidgetFrame } from "../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import {
  type UseWorkerSessionTimelineOptions,
  type UseWorkerSessionTimelineResult,
  useWorkerSessionTimeline,
} from "../hooks/useWorkerSessionTimeline";
import type {
  WorkerSessionTimelineEntry,
  WorkerTimelineTerminalOutcome,
} from "../lib/worker-session-timeline-projection-types";
import type { WorkerSessionTimelineStreamStatus } from "../lib/worker-session-timeline-stream";
import {
  getWorkerSessionTimelineMessages,
  type WorkerSessionTimelineMessages,
  type WorkerSessionTimelineRecordingHealth,
  type WorkerSessionTimelineViewStatus,
} from "../messages/worker-session-timeline";
import { BoundedText } from "./worker-session-timeline-detail-primitives";
import { WorkerSessionTimelineEntryView } from "./worker-session-timeline-entry";

export interface WorkerSessionTimelineProps
  extends UseWorkerSessionTimelineOptions {
  className?: string;
  locale?: string;
  onNavigateToWorkerSession?: (workerSessionID: string) => void;
  showHeading?: boolean;
  stateOverride?: UseWorkerSessionTimelineResult;
}

export interface WorkerSessionTimelineWidgetProps
  extends WorkerSessionTimelineProps {
  headerAction?: ReactNode;
  widgetId?: string;
}

export function WorkerSessionTimeline({
  className,
  locale,
  onNavigateToWorkerSession,
  showHeading = true,
  stateOverride,
  ...hookOptions
}: WorkerSessionTimelineProps) {
  const hookState = useWorkerSessionTimeline(hookOptions);
  const state = stateOverride ?? hookState;

  return (
    <WorkerSessionTimelineContent
      className={className}
      locale={locale}
      onNavigateToWorkerSession={onNavigateToWorkerSession}
      showHeading={showHeading}
      state={state}
      workerSessionID={hookOptions.workerSessionID}
    />
  );
}

/** Dashboard chrome for consumers that need a selectable Worker Session card. */
export function WorkerSessionTimelineWidget({
  headerAction,
  locale,
  widgetId = "worker-session-timeline",
  ...timelineProps
}: WorkerSessionTimelineWidgetProps) {
  const messages = getWorkerSessionTimelineMessages(locale);

  return (
    <DashboardWidgetFrame
      bodyScroll={false}
      headerAction={headerAction}
      title={messages.timelineTitle}
      widgetId={widgetId}
    >
      <WorkerSessionTimeline
        {...timelineProps}
        locale={locale}
        showHeading={false}
      />
    </DashboardWidgetFrame>
  );
}

export interface WorkerSessionTimelineContentProps {
  className?: string;
  locale?: string;
  onNavigateToWorkerSession?: (workerSessionID: string) => void;
  showHeading?: boolean;
  state: UseWorkerSessionTimelineResult;
  workerSessionID: string | null;
}

export function WorkerSessionTimelineContent({
  className,
  locale,
  onNavigateToWorkerSession,
  showHeading = true,
  state,
  workerSessionID,
}: WorkerSessionTimelineContentProps) {
  const messages = getWorkerSessionTimelineMessages(locale);
  const terminal = latestTerminal(state.entries);
  const viewStatus = toViewStatus(state.status, state.entries.length);

  return (
    <section
      aria-label={messages.ariaLabel}
      className={cn("grid min-w-0 gap-3", className)}
      data-worker-session-timeline-status={viewStatus}
      data-worker-session-timeline-worker-session-id={
        workerSessionID ?? undefined
      }
    >
      {showHeading ? <Heading as="h2">{messages.timelineTitle}</Heading> : null}
      <TimelineStatus
        messages={messages}
        state={state}
        terminal={terminal}
        viewStatus={viewStatus}
      />
      <RecordingHealthNotice messages={messages} state={state} />
      {state.status === "source-error" ? (
        <SourceErrorNotice
          messages={messages}
          onRetry={state.retry}
          state={state}
        />
      ) : null}
      {state.entries.length > 0 ? (
        <ol
          aria-label={messages.eventListLabel}
          className="grid min-w-0 gap-3"
          data-worker-session-timeline-events="true"
        >
          {state.entries.map((entry) => (
            <WorkerSessionTimelineEntryView
              entry={entry}
              key={entry.key}
              messages={messages}
              onNavigateToWorkerSession={onNavigateToWorkerSession}
            />
          ))}
        </ol>
      ) : (
        <TimelineBodyState
          messages={messages}
          state={state}
          viewStatus={viewStatus}
        />
      )}
    </section>
  );
}

function TimelineStatus({
  messages,
  state,
  terminal,
  viewStatus,
}: {
  messages: WorkerSessionTimelineMessages;
  state: UseWorkerSessionTimelineResult;
  terminal: WorkerSessionTimelineEntry["terminal"];
  viewStatus: WorkerSessionTimelineViewStatus;
}) {
  const statusLabel = statusMessage(messages, viewStatus);

  return (
    <div
      className="grid min-w-0 gap-2"
      data-worker-session-timeline-status-bar="true"
    >
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <DashboardStatusPill tone={statusPillTone(viewStatus)}>
          {statusLabel}
        </DashboardStatusPill>
        {terminal ? (
          <DashboardStatusPill
            data-worker-session-terminal-outcome={terminal.outcome}
            tone={terminalTone(terminal.outcome)}
          >
            {messages.terminalOutcomeLabel(terminal.outcome)}
          </DashboardStatusPill>
        ) : null}
      </div>
      <p
        aria-atomic="true"
        aria-live="polite"
        className="m-0 af-supporting-text text-on-surface-subtle"
      >
        {statusLabel}
      </p>
      {state.entries.length > 0 && terminal ? (
        <TerminalOutcomeSummary messages={messages} terminal={terminal} />
      ) : null}
    </div>
  );
}

function TerminalOutcomeSummary({
  messages,
  terminal,
}: {
  messages: WorkerSessionTimelineMessages;
  terminal: NonNullable<WorkerSessionTimelineEntry["terminal"]>;
}) {
  return (
    <SurfacePanel
      aria-label={messages.terminalOutcomeHeading}
      className="grid min-w-0 gap-2"
      radius="lg"
      surface="low"
    >
      <Label>{messages.terminalOutcomeHeading}</Label>
      <Text className="m-0" data-worker-session-terminal-summary="true">
        {messages.terminalOutcomeLabel(terminal.outcome)}
        {terminal.status ? ` (${terminal.status})` : ""}
      </Text>
      {terminal.failure?.message ? (
        <BoundedText
          label={messages.failureLabel}
          value={terminal.failure.message}
        />
      ) : null}
    </SurfacePanel>
  );
}

function RecordingHealthNotice({
  messages,
  state,
}: {
  messages: WorkerSessionTimelineMessages;
  state: UseWorkerSessionTimelineResult;
}) {
  const health = state.recordingHealth;
  if (health === null) {
    return (
      <div className="flex items-center gap-2">
        <DashboardStatusPill size="compact" tone="neutral">
          {messages.recordingHealthPending}
        </DashboardStatusPill>
      </div>
    );
  }

  const tone = recordingHealthTone(health);
  return (
    <AlertPanel
      role={health === "COMPLETE" ? "status" : "alert"}
      tone={
        health === "COMPLETE"
          ? "info"
          : tone === "danger"
            ? "danger"
            : "warning"
      }
    >
      <div className="grid gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <DashboardStatusPill size="compact" tone={tone}>
            {messages.recordingHealthLabel(health)}
          </DashboardStatusPill>
        </div>
        <Text className="m-0">
          {messages.recordingHealthNotice(health, state.recordingHealthReason)}
        </Text>
      </div>
    </AlertPanel>
  );
}

function SourceErrorNotice({
  messages,
  onRetry,
  state,
}: {
  messages: WorkerSessionTimelineMessages;
  onRetry: () => void;
  state: UseWorkerSessionTimelineResult;
}) {
  return (
    <AlertPanel role="alert" tone="danger">
      <div className="grid gap-3">
        <div className="grid gap-1">
          <strong>{messages.sourceErrorHeading}</strong>
          <Text className="m-0">
            {state.sourceError?.message ?? messages.sourceErrorHeading}
          </Text>
        </div>
        <div>
          <Button onClick={onRetry} tone="outline" type="button">
            {messages.retryAction}
          </Button>
        </div>
      </div>
    </AlertPanel>
  );
}

function TimelineBodyState({
  messages,
  state,
  viewStatus,
}: {
  messages: WorkerSessionTimelineMessages;
  state: UseWorkerSessionTimelineResult;
  viewStatus: WorkerSessionTimelineViewStatus;
}) {
  if (viewStatus === "source-error") {
    return null;
  }

  if (viewStatus === "loading" || viewStatus === "reconnecting") {
    return (
      <WidgetDetailCopy aria-busy="true" role="status">
        {statusMessage(messages, viewStatus)}
      </WidgetDetailCopy>
    );
  }

  if (viewStatus === "idle") {
    return <WidgetDetailCopy>{messages.idleState}</WidgetDetailCopy>;
  }

  if (viewStatus === "empty") {
    return <WidgetDetailCopy>{messages.emptyState}</WidgetDetailCopy>;
  }

  return (
    <WidgetDetailCopy>
      {state.status === "live"
        ? messages.liveFollowingState
        : messages.completedState}
    </WidgetDetailCopy>
  );
}

function latestTerminal(
  entries: readonly WorkerSessionTimelineEntry[],
): WorkerSessionTimelineEntry["terminal"] {
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const terminal = entries[index]?.terminal;
    if (terminal) {
      return terminal;
    }
  }
  return undefined;
}

function toViewStatus(
  status: WorkerSessionTimelineStreamStatus,
  entryCount: number,
): WorkerSessionTimelineViewStatus {
  if (status === "idle") {
    return "idle";
  }
  if (status === "loading") {
    return "loading";
  }
  if (status === "ready-empty") {
    return "empty";
  }
  if (status === "source-error") {
    return "source-error";
  }
  if (status === "reconnecting") {
    return "reconnecting";
  }
  if (status === "completed") {
    return "completed";
  }
  if (status === "live") {
    return "live";
  }
  return entryCount === 0 ? "empty" : "completed";
}

function statusMessage(
  messages: WorkerSessionTimelineMessages,
  status: WorkerSessionTimelineViewStatus,
): string {
  switch (status) {
    case "idle":
      return messages.idleState;
    case "loading":
      return messages.loadingState;
    case "empty":
      return messages.emptyState;
    case "live":
      return messages.liveFollowingState;
    case "reconnecting":
      return messages.reconnectingState;
    case "completed":
      return messages.completedState;
    case "source-error":
      return messages.sourceErrorHeading;
  }
}

function statusPillTone(
  status: WorkerSessionTimelineViewStatus,
): "danger" | "info" | "neutral" | "success" | "warning" {
  switch (status) {
    case "live":
      return "success";
    case "reconnecting":
      return "warning";
    case "source-error":
      return "danger";
    case "completed":
      return "neutral";
    default:
      return "info";
  }
}

function recordingHealthTone(
  health: WorkerSessionTimelineRecordingHealth,
): "danger" | "success" | "warning" {
  return health === "COMPLETE"
    ? "success"
    : health === "DEGRADED"
      ? "warning"
      : "danger";
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

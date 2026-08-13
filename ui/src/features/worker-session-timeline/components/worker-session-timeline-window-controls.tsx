import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import type { WorkerSessionTimelineWindow } from "../lib/worker-session-timeline-window";
import type { WorkerSessionTimelineMessages } from "../messages/worker-session-timeline";

export interface WorkerSessionTimelineWindowControlsProps {
  eventListID: string;
  messages: WorkerSessionTimelineMessages;
  onMove: (direction: "earlier" | "later") => void;
  onReturnToLatest: () => void;
  pendingLiveCount: number;
  timelineWindow: WorkerSessionTimelineWindow;
  totalEntries: number;
}

export function WorkerSessionTimelineWindowControls({
  eventListID,
  messages,
  onMove,
  onReturnToLatest,
  pendingLiveCount,
  timelineWindow,
  totalEntries,
}: WorkerSessionTimelineWindowControlsProps) {
  if (
    totalEntries === 0 ||
    (!timelineWindow.hasEarlier &&
      !timelineWindow.hasLater &&
      pendingLiveCount === 0)
  ) {
    return null;
  }

  return (
    <nav
      aria-label={messages.windowNavigationLabel}
      className="flex min-w-0 flex-wrap items-center gap-2"
      data-worker-session-timeline-window-controls="true"
    >
      <DashboardActionButton
        aria-controls={eventListID}
        disabled={!timelineWindow.hasEarlier}
        onClick={() => onMove("earlier")}
        type="button"
      >
        {messages.earlierEventsAction}
      </DashboardActionButton>
      <span
        aria-live="polite"
        className="min-w-0 break-words text-sm text-on-surface-subtle"
      >
        {messages.windowRangeLabel(
          timelineWindow.start + 1,
          timelineWindow.end,
          totalEntries,
        )}
      </span>
      <DashboardActionButton
        aria-controls={eventListID}
        disabled={!timelineWindow.hasLater}
        onClick={() => onMove("later")}
        type="button"
      >
        {messages.laterEventsAction}
      </DashboardActionButton>
      {pendingLiveCount > 0 ? (
        <DashboardActionButton
          aria-controls={eventListID}
          onClick={onReturnToLatest}
          type="button"
        >
          {messages.newActivityAction(pendingLiveCount)}
        </DashboardActionButton>
      ) : null}
    </nav>
  );
}

import type { WorkerSessionTimelineEntry } from "./worker-session-timeline-projection-types";

export const WORKER_SESSION_TIMELINE_WINDOW_SIZE = 200;

export interface WorkerSessionTimelineWindow {
  end: number;
  entries: WorkerSessionTimelineEntry[];
  hasEarlier: boolean;
  hasLater: boolean;
  start: number;
}

/**
 * Selects one deterministic chronological window. A null start means that
 * the window follows the newest entries, while an explicit start represents
 * a user-selected historical window.
 */
export function getWorkerSessionTimelineWindow(
  entries: readonly WorkerSessionTimelineEntry[],
  requestedStart: number | null,
): WorkerSessionTimelineWindow {
  const maxStart = Math.max(
    0,
    entries.length - WORKER_SESSION_TIMELINE_WINDOW_SIZE,
  );
  const start = clamp(requestedStart ?? maxStart, 0, maxStart);
  const end = Math.min(
    entries.length,
    start + WORKER_SESSION_TIMELINE_WINDOW_SIZE,
  );

  return {
    end,
    entries: entries.slice(start, end),
    hasEarlier: start > 0,
    hasLater: end < entries.length,
    start,
  };
}

export function moveWorkerSessionTimelineWindow(
  currentStart: number,
  direction: "earlier" | "later",
  totalEntries: number,
): number {
  const maxStart = Math.max(
    0,
    totalEntries - WORKER_SESSION_TIMELINE_WINDOW_SIZE,
  );
  const nextStart =
    direction === "earlier"
      ? currentStart - WORKER_SESSION_TIMELINE_WINDOW_SIZE
      : currentStart + WORKER_SESSION_TIMELINE_WINDOW_SIZE;
  return clamp(nextStart, 0, maxStart);
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(Math.max(value, minimum), maximum);
}

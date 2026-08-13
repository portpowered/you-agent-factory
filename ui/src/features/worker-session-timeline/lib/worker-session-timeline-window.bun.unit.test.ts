import { describe, expect, it } from "bun:test";

import type { WorkerSessionTimelineEntry } from "./worker-session-timeline-projection-types";
import {
  getWorkerSessionTimelineWindow,
  moveWorkerSessionTimelineWindow,
  WORKER_SESSION_TIMELINE_WINDOW_SIZE,
} from "./worker-session-timeline-window";

function entries(count: number): WorkerSessionTimelineEntry[] {
  return Array.from({ length: count }, (_, index) => ({
    category: "generic",
    canonical: {
      cursor: { position: index + 1 },
      position: index + 1,
      schemaId: "unknown",
      sourceEventId: `event-${index + 1}`,
      sourceId: "worker-1",
      sourceSequence: index + 1,
      sourceType: "worker_session",
    },
    key: `event-${index + 1}`,
    kind: "UNKNOWN",
    phase: "UNKNOWN",
  }));
}

describe("Worker Session timeline windows", () => {
  it("keeps the live tail bounded and exposes adjacent chronological windows", () => {
    const allEntries = entries(450);
    const tail = getWorkerSessionTimelineWindow(allEntries, null);

    expect(tail.entries).toHaveLength(WORKER_SESSION_TIMELINE_WINDOW_SIZE);
    expect(tail.start).toBe(250);
    expect(tail.end).toBe(450);
    expect(tail.hasEarlier).toBe(true);
    expect(tail.hasLater).toBe(false);

    const earlierStart = moveWorkerSessionTimelineWindow(
      tail.start,
      "earlier",
      allEntries.length,
    );
    const earlier = getWorkerSessionTimelineWindow(allEntries, earlierStart);
    expect(earlier.start).toBe(50);
    expect(earlier.entries[0]?.canonical.position).toBe(51);
    expect(earlier.entries.at(-1)?.canonical.position).toBe(250);

    expect(
      moveWorkerSessionTimelineWindow(
        earlier.start,
        "later",
        allEntries.length,
      ),
    ).toBe(tail.start);
  });

  it("clamps stale window positions when history shrinks", () => {
    const window = getWorkerSessionTimelineWindow(entries(25), 300);

    expect(window.start).toBe(0);
    expect(window.end).toBe(25);
    expect(window.entries).toHaveLength(25);
    expect(window.hasEarlier).toBe(false);
    expect(window.hasLater).toBe(false);
  });
});

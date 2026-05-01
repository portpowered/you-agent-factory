import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../api/events";
import {
  compactFactoryEventForTimeline,
  FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
  persistFactoryTimelineMemorySummary,
  readFactoryTimelineDebugOptions,
  readPersistedFactoryTimelineMemorySummary,
  summarizeFactoryTimelineMemory,
} from "./factoryTimelineDebug";

const BASE_EVENT: FactoryEvent = {
  context: {
    eventTime: "2026-04-29T10:00:00Z",
    sequence: 1,
    tick: 7,
  },
  id: "event-1",
  payload: {
    prompt: "This prompt is intentionally long so the compaction path has something to trim.",
    response: "Response text stays compact in the summarized profile.",
    stdout: "stdout",
  },
  type: FACTORY_EVENT_TYPES.inferenceRequest,
};

describe("factoryTimelineDebug", () => {
  it("reads debug flags from the browser query string", () => {
    expect(
      readFactoryTimelineDebugOptions({
        location: {
          search: "?afCompactEventText=1&afMemoryDebug=true&afMaxEventTextChars=128",
        },
      }),
    ).toEqual({
      compactEventText: true,
      maxEventTextChars: 128,
      memoryDebug: true,
    });
  });

  it("compacts heavy event text fields without mutating the original event", () => {
    const compacted = compactFactoryEventForTimeline(BASE_EVENT, {
      compactEventText: true,
      maxEventTextChars: 12,
      memoryDebug: false,
    });

    expect(compacted).not.toBe(BASE_EVENT);
    expect(compacted.payload).not.toBe(BASE_EVENT.payload);
    expect((compacted.payload as { prompt: string }).prompt).toContain("[truncated ");
    expect((compacted.payload as { prompt: string }).prompt.startsWith("This prompt ")).toBe(true);
    expect((BASE_EVENT.payload as { prompt: string }).prompt).not.toContain("[truncated ");
  });

  it("summarizes retained timeline memory and persists the latest profile", () => {
    const storage = window.localStorage;
    storage.removeItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY);

    const summary = summarizeFactoryTimelineMemory([BASE_EVENT], 7, {
      location: { search: "" },
      performance: {
        memory: {
          jsHeapSizeLimit: 100 * 1024 * 1024,
          totalJSHeapSize: 60 * 1024 * 1024,
          usedJSHeapSize: 40 * 1024 * 1024,
        },
      } as Performance & {
        memory: {
          jsHeapSizeLimit: number;
          totalJSHeapSize: number;
          usedJSHeapSize: number;
        };
      },
    });

    expect(summary.eventCount).toBe(1);
    expect(summary.selectedTick).toBe(7);
    expect(summary.jsHeapUsedMB).toBe(40);
    expect(summary.topEvents[0]).toMatchObject({
      id: "event-1",
      tick: 7,
      type: "INFERENCE_REQUEST",
    });

    persistFactoryTimelineMemorySummary(storage, summary);

    expect(readPersistedFactoryTimelineMemorySummary(storage)).toEqual(summary);
  });
});

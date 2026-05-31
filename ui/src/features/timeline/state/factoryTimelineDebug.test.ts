import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import {
  compactFactoryEventForTimeline,
  FACTORY_TIMELINE_DEBUG_GLOBAL,
  FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
  installFactoryTimelineDebugGlobal,
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
    prompt:
      "This prompt is intentionally long so the compaction path has something to trim.",
    response: "Response text stays compact in the summarized profile.",
    stdout: "stdout",
  },
  type: FACTORY_EVENT_TYPES.inferenceRequest,
};

describe("factoryTimelineDebug options and compaction", () => {
  it("reads debug flags from the browser query string", () => {
    expect(
      readFactoryTimelineDebugOptions({
        location: {
          search:
            "?afCompactEventText=1&afMemoryDebug=true&afMaxEventTextChars=128",
        },
      }),
    ).toEqual({
      compactEventText: true,
      maxEventTextChars: 128,
      memoryDebug: true,
    });
  });

  it("falls back to defaults when debug query params are invalid", () => {
    expect(
      readFactoryTimelineDebugOptions({
        location: {
          search:
            "?afCompactEventText=0&afMemoryDebug=no&afMaxEventTextChars=0",
        },
      }),
    ).toEqual({
      compactEventText: false,
      maxEventTextChars: 2_048,
      memoryDebug: false,
    });
  });

  it("returns the original event when compaction is disabled", () => {
    expect(
      compactFactoryEventForTimeline(BASE_EVENT, {
        compactEventText: false,
        maxEventTextChars: 12,
        memoryDebug: false,
      }),
    ).toBe(BASE_EVENT);
  });

  it("returns non-object payloads unchanged when compaction is enabled", () => {
    const event = {
      ...BASE_EVENT,
      payload: "plain-text",
    };

    expect(
      compactFactoryEventForTimeline(event, {
        compactEventText: true,
        maxEventTextChars: 12,
        memoryDebug: false,
      }),
    ).toBe(event);
  });

  it("compacts heavy event text fields without mutating the original event", () => {
    const compacted = compactFactoryEventForTimeline(BASE_EVENT, {
      compactEventText: true,
      maxEventTextChars: 12,
      memoryDebug: false,
    });

    expect(compacted).not.toBe(BASE_EVENT);
    expect(compacted.payload).not.toBe(BASE_EVENT.payload);
    expect((compacted.payload as { prompt: string }).prompt).toContain(
      "[truncated ",
    );
    expect(
      (compacted.payload as { prompt: string }).prompt.startsWith(
        "This prompt ",
      ),
    ).toBe(true);
    expect((BASE_EVENT.payload as { prompt: string }).prompt).not.toContain(
      "[truncated ",
    );
  });

  it("returns the original event when compaction would not change payload text", () => {
    const event = {
      ...BASE_EVENT,
      payload: {
        prompt: "short",
        response: "ok",
      },
    };

    expect(
      compactFactoryEventForTimeline(event, {
        compactEventText: true,
        maxEventTextChars: 2_048,
        memoryDebug: false,
      }),
    ).toBe(event);
  });
});

describe("factoryTimelineDebug memory summary", () => {
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

  it("skips non-object payloads when summarizing heavy byte totals", () => {
    const summary = summarizeFactoryTimelineMemory(
      [
        {
          ...BASE_EVENT,
          payload: "plain-text",
        },
      ],
      7,
      { location: { search: "" } },
    );

    expect(summary.eventCount).toBe(1);
    expect(summary.heavyPayloadBytesMB.promptBytesMB).toBe(0);
    expect(summary.topEvents[0]?.heavyPayloadBytesMB).toBe(0);
  });

  it("orders top events by heavy payload bytes before estimated JSON size", () => {
    const lightEvent: FactoryEvent = {
      ...BASE_EVENT,
      id: "light-event",
      payload: {
        prompt: "small",
      },
    };
    const heavyEvent: FactoryEvent = {
      ...BASE_EVENT,
      id: "heavy-event",
      payload: {
        prompt: "x".repeat(8_192),
      },
    };

    const summary = summarizeFactoryTimelineMemory(
      [lightEvent, heavyEvent],
      7,
      {
        location: { search: "" },
      },
    );

    expect(summary.topEvents.map((entry) => entry.id)).toEqual([
      "heavy-event",
      "light-event",
    ]);
    expect(summary.topEvents[0]?.heavyPayloadBytesMB).toBeGreaterThan(
      summary.topEvents[1]?.heavyPayloadBytesMB ?? 0,
    );
  });

  it("breaks top-event ties using estimated JSON size when heavy payload bytes match", () => {
    const sharedContext = BASE_EVENT.context;
    const smallerJson: FactoryEvent = {
      context: sharedContext,
      id: "smaller-json",
      payload: {
        prompt: "same-weight",
      },
      type: BASE_EVENT.type,
    };
    const largerJson: FactoryEvent = {
      context: sharedContext,
      id: "larger-json",
      payload: {
        debug_context: "x".repeat(200_000),
        prompt: "same-weight",
      },
      type: BASE_EVENT.type,
    };

    const summary = summarizeFactoryTimelineMemory(
      [smallerJson, largerJson],
      7,
      {
        location: { search: "" },
      },
    );

    expect(summary.topEvents.map((entry) => entry.id)).toEqual([
      "larger-json",
      "smaller-json",
    ]);
    expect(summary.topEvents[0]?.heavyPayloadBytesMB).toBe(
      summary.topEvents[1]?.heavyPayloadBytesMB,
    );
    expect(summary.topEvents[0]?.estimatedJsonBytesMB).toBeGreaterThan(
      summary.topEvents[1]?.estimatedJsonBytesMB ?? 0,
    );
  });

  it("returns null when no persisted summary exists", () => {
    const storage = {
      getItem: vi.fn().mockReturnValue(null),
      setItem: vi.fn(),
    };

    expect(readPersistedFactoryTimelineMemorySummary(storage)).toBeNull();
  });
});

describe("factoryTimelineDebug global install", () => {
  it("installs window debug globals that summarize current timeline state", () => {
    const browserWindow = {
      location: { search: "" },
      localStorage: {
        getItem: vi.fn().mockReturnValue(null),
        setItem: vi.fn(),
      },
    } as Window & {
      location: { search: string };
      localStorage: Storage;
    };

    installFactoryTimelineDebugGlobal(
      browserWindow,
      () => ({
        events: [BASE_EVENT],
        selectedTick: BASE_EVENT.context.tick,
      }),
      {
        compactEventText: false,
        maxEventTextChars: 2_048,
        memoryDebug: true,
      },
    );

    const debugGlobal = browserWindow[FACTORY_TIMELINE_DEBUG_GLOBAL];
    expect(debugGlobal?.options.memoryDebug).toBe(true);
    expect(debugGlobal?.summarize().eventCount).toBe(1);
    expect(debugGlobal?.readPersistedSummary()).toBeNull();
  });
});

import { renderHook, waitFor } from "@testing-library/react";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import {
  FACTORY_TIMELINE_DEBUG_GLOBAL,
  FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
  type FactoryTimelineDebugOptions,
} from "../../timeline/state/factoryTimelineDebug";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";

const DEBUG_OFF: FactoryTimelineDebugOptions = {
  compactEventText: false,
  maxEventTextChars: 2_048,
  memoryDebug: false,
};

const DEBUG_ON: FactoryTimelineDebugOptions = {
  compactEventText: false,
  maxEventTextChars: 2_048,
  memoryDebug: true,
};

const SAMPLE_EVENT: FactoryEvent = {
  context: {
    eventTime: "2026-04-29T10:00:00Z",
    sequence: 1,
    tick: 3,
  },
  id: "event-1",
  payload: {
    prompt: "Prompt text for memory summary coverage.",
    response: "Response text for memory summary coverage.",
  },
  type: FACTORY_EVENT_TYPES.inferenceRequest,
};

function clearTimelineDebugArtifacts(): void {
  delete window[FACTORY_TIMELINE_DEBUG_GLOBAL];
  window.localStorage.removeItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY);
}

describe("useDashboardTimelineMemoryDebug", () => {
  beforeEach(() => {
    clearTimelineDebugArtifacts();
    useFactoryTimelineStore.getState().reset();
  });

  afterEach(() => {
    clearTimelineDebugArtifacts();
  });

  it("does not install window debug globals or persist summary when memory debug is off", () => {
    useFactoryTimelineStore.setState({
      events: [SAMPLE_EVENT],
      latestTick: SAMPLE_EVENT.context.tick,
      selectedTick: SAMPLE_EVENT.context.tick,
    });

    renderHook(() =>
      useDashboardTimelineMemoryDebug({
        debugOptions: DEBUG_OFF,
        eventCount: 1,
      }),
    );

    expect(window[FACTORY_TIMELINE_DEBUG_GLOBAL]).toBeUndefined();
    expect(window.localStorage.getItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY)).toBeNull();
  });

  it("installs debug globals and persists timeline memory summary when memory debug is on", async () => {
    useFactoryTimelineStore.setState({
      events: [SAMPLE_EVENT],
      latestTick: SAMPLE_EVENT.context.tick,
      selectedTick: SAMPLE_EVENT.context.tick,
    });

    renderHook(() =>
      useDashboardTimelineMemoryDebug({
        debugOptions: DEBUG_ON,
        eventCount: 1,
      }),
    );

    await waitFor(() => {
      expect(window[FACTORY_TIMELINE_DEBUG_GLOBAL]).toBeDefined();
    });
    expect(window[FACTORY_TIMELINE_DEBUG_GLOBAL]?.options).toEqual(DEBUG_ON);
    expect(window.localStorage.getItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY)).not.toBeNull();
  });

  it("skips localStorage persistence when memory debug is on but there are no events", () => {
    renderHook(() =>
      useDashboardTimelineMemoryDebug({
        debugOptions: DEBUG_ON,
        eventCount: 0,
      }),
    );

    expect(window[FACTORY_TIMELINE_DEBUG_GLOBAL]).toBeDefined();
    expect(window.localStorage.getItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY)).toBeNull();
  });
});

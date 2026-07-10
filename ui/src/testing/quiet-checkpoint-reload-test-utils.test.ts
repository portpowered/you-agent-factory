import { afterEach, describe, expect, it, vi } from "vitest";

import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import { MockEventSource } from "./app-shell-session-stream-test-utils";
import { createQuietCheckpointReloadFixture } from "./quiet-checkpoint-reload-test-utils";

describe.each([0, 7])("quiet checkpoint reload fixture at tick %i", (tick) => {
  afterEach(() => {
    MockEventSource.instances = [];
    useFactoryTimelineStore.getState().reset();
    vi.unstubAllGlobals();
  });

  it("controls hydration, preflight, stream opening, and event arrival independently", async () => {
    const fixture = createQuietCheckpointReloadFixture(tick);
    await fixture.installCheckpoint();
    const preflight = fixture.fetchOverride(
      "/factory-sessions/~default/sync-preflight?after_sequence=0",
      "GET",
      "/factory-sessions/~default/sync-preflight?after_sequence=0",
    );

    expect(fixture.observations()).toEqual({
      checkpointHydrated: false,
      eventArrived: false,
      preflightCompleted: false,
      streamOpened: false,
    });

    fixture.completePreflight();
    await expect(preflight).resolves.toBeInstanceOf(Response);
    expect(fixture.observations().preflightCompleted).toBe(true);
    expect(fixture.observations().checkpointHydrated).toBe(false);

    fixture.hydrateCheckpoint();
    expect(fixture.observations()).toEqual({
      checkpointHydrated: true,
      eventArrived: false,
      preflightCompleted: true,
      streamOpened: false,
    });

    const stream = new MockEventSource("/factory-sessions/test/events");
    fixture.openStream(stream);

    expect(fixture.observations()).toEqual({
      checkpointHydrated: true,
      eventArrived: false,
      preflightCompleted: true,
      streamOpened: true,
    });
    expect(useFactoryTimelineStore.getState()).toMatchObject({
      events: [],
      mode: "current",
      selectedTick: tick,
    });
    expect(
      useFactoryTimelineStore.getState().worldViewCache[tick]?.factory_state,
    ).toBe(`CHECKPOINT_CURRENT_AT_${tick}`);
    expect(stream.messageEventCount).toBe(0);
  });
});

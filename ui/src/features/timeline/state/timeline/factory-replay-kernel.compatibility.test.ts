import { describe, expect, it } from "vitest";

import { projectFactoryWorldAtTick } from "../../../../../../packages/factory-replay/src/index.js";
import type { FactoryEvent } from "../../../../api/events";
import { projectSnapshot } from "./projectSnapshot";
import {
  applyReplayEvent,
  reconstructWorldState,
} from "./replayWorldState";
import { emptyReplayWorldState } from "./replayWorldStateSupport";

function factoryStateEvent(
  id: string,
  tick: number,
  sequence: number,
  state: string,
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T05:00:0${tick}Z`,
      sequence,
      tick,
    },
    id,
    payload: { state },
    type: "FACTORY_STATE_RESPONSE",
  };
}

describe("factory replay kernel compatibility", () => {
  it("matches the hosted selected-tick world projection", () => {
    const events = [
      factoryStateEvent("event-later", 2, 0, "PAUSED"),
      factoryStateEvent("event-selected", 1, 0, "RUNNING"),
    ];

    const kernel = projectFactoryWorldAtTick({
      events,
      reducer: {
        createState: emptyReplayWorldState,
        applyEvent: (state, event) => {
          applyReplayEvent(state, event as FactoryEvent);
          return state;
        },
        projectWorld: projectSnapshot,
      },
      tick: 1,
    });

    expect(kernel.world).toEqual(projectSnapshot(reconstructWorldState(events, 1)));
    expect(kernel.world.factory_state).toBe("RUNNING");
    expect(kernel.appliedEvents.map((event) => event.id)).toEqual([
      "event-selected",
    ]);
  });
});

import type { DashboardSnapshot } from "../../../../api/dashboard";
import { buildFactoryTimelineSnapshot } from "../../../timeline/state/factoryTimelineStore";
import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { factoryGraphWorkStateNodeExistsInSnapshot } from "./factoryGraphNodeSelection";

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructureRequest = event(
  "event-1",
  1,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          outputs: [{ state: "review", workType: "story" }],
          worker: "reviewer",
        },
      ],
    },
  },
);

describe("factoryGraphNodeSelection", () => {
  it("detects work-state graph nodes that exist in the factory document", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);

    expect(
      factoryGraphWorkStateNodeExistsInSnapshot(
        snapshot,
        "work-state:story:new",
      ),
    ).toBe(true);
    expect(
      factoryGraphWorkStateNodeExistsInSnapshot(
        snapshot,
        "work-state:story:removed",
      ),
    ).toBe(false);
    expect(
      factoryGraphWorkStateNodeExistsInSnapshot(snapshot, "workstation:review"),
    ).toBe(false);
  });
});

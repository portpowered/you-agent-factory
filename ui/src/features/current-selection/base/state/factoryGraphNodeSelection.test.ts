import type { DashboardSnapshot } from "../../../../api/dashboard";
import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { buildFactoryTimelineSnapshot } from "../../../timeline/state/factoryTimelineStore";
import {
  factoryGraphWorkStateNodeExistsInSnapshot,
  resolveFactoryGraphNodeSelection,
} from "./factoryGraphNodeSelection";

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
      workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
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

  it("retains topology workstation and factory-graph work-type and work-state node selections", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    const factory = snapshot.factory;
    const topologyWorkstationSelection = {
      kind: "node" as const,
      nodeId: "review",
    };
    const workTypeSelection = {
      kind: "node" as const,
      nodeId: "work-type:story",
    };
    const workStateSelection = {
      kind: "node" as const,
      nodeId: "work-state:story:new",
    };

    expect(
      resolveFactoryGraphNodeSelection(
        snapshot,
        topologyWorkstationSelection,
        factory,
      ),
    ).toEqual(topologyWorkstationSelection);
    expect(
      resolveFactoryGraphNodeSelection(snapshot, workTypeSelection, factory),
    ).toEqual(workTypeSelection);
    expect(
      resolveFactoryGraphNodeSelection(snapshot, workStateSelection, factory),
    ).toEqual(workStateSelection);
  });

  it("clears factory-graph work-type selections when the work type is missing from the factory", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    const factory = {
      ...snapshot.factory,
      workTypes: [],
    };

    expect(
      resolveFactoryGraphNodeSelection(
        snapshot,
        { kind: "node", nodeId: "work-type:story" },
        factory,
      ),
    ).toBeNull();
  });

  it("clears factory-graph work-state selections when the state is missing from the snapshot factory", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    snapshot.factory = {
      ...snapshot.factory,
      workTypes: [],
    };

    expect(
      resolveFactoryGraphNodeSelection(
        snapshot,
        { kind: "node", nodeId: "work-state:story:new" },
        snapshot.factory,
      ),
    ).toBeNull();
  });

  it("resolves work-type graph nodes using legacy work_types factory fields", () => {
    const snapshot = buildFactoryTimelineSnapshot([initialStructureRequest], 1);
    const factory = {
      work_types: [
        { name: "story", states: [{ name: "new", type: "INITIAL" }] },
      ],
    } as DashboardSnapshot["factory"];

    expect(
      resolveFactoryGraphNodeSelection(
        snapshot,
        { kind: "node", nodeId: "work-type:story" },
        factory,
      ),
    ).toEqual({ kind: "node", nodeId: "work-type:story" });
  });
});

import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { buildFactoryTimelineSnapshot } from "./factoryTimelineStore";

const eventTime = "2026-05-27T01:00:00Z";

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

describe("buildFactoryTimelineSnapshot canonical factory graph", () => {
  it("preserves the OpenAPI Factory graph beside the dashboard topology", () => {
    const initialStructure = event(
      "initial-canonical-factory",
      1,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          name: "graph-source",
          resources: [{ capacity: 2, name: "agent-slot" }],
          workers: [
            {
              model: "gpt-5.4",
              modelProvider: "CODEX",
              name: "reviewer",
              resources: [{ capacity: 1, name: "agent-slot" }],
              type: "MODEL_WORKER",
            },
          ],
          workTypes: [
            {
              name: "story",
              states: [
                { name: "new", type: "INITIAL" },
                { name: "continue", type: "PROCESSING" },
                { name: "rejected", type: "PROCESSING" },
                { name: "done", type: "TERMINAL" },
                { name: "failed", type: "FAILED" },
              ],
            },
          ],
          workstations: [
            {
              behavior: "STANDARD",
              body: "Review the work and route it.",
              id: "review",
              inputs: [{ state: "new", workType: "story" }],
              limits: { maxRetries: 3 },
              name: "Review",
              onContinue: [{ state: "continue", workType: "story" }],
              onFailure: [{ state: "failed", workType: "story" }],
              onRejection: [{ state: "rejected", workType: "story" }],
              outputs: [{ state: "done", workType: "story" }],
              resources: [{ capacity: 1, name: "agent-slot" }],
              worker: "reviewer",
            },
          ],
        },
      },
    );

    const snapshot = buildFactoryTimelineSnapshot([initialStructure], 1);

    expect(snapshot.factory).toEqual(initialStructure.payload.factory);
    expect(snapshot.factory?.resources?.[0]).toMatchObject({
      capacity: 2,
      name: "agent-slot",
    });
    expect(snapshot.factory?.workers?.[0]?.resources?.[0]).toMatchObject({
      capacity: 1,
      name: "agent-slot",
    });
    expect(snapshot.factory?.workstations?.[0]).toMatchObject({
      body: "Review the work and route it.",
      limits: { maxRetries: 3 },
      onContinue: [{ state: "continue", workType: "story" }],
      onFailure: [{ state: "failed", workType: "story" }],
      onRejection: [{ state: "rejected", workType: "story" }],
      resources: [{ capacity: 1, name: "agent-slot" }],
    });
    expect(snapshot.topology.workstation_nodes_by_id.review).toMatchObject({
      node_id: "review",
      workstation_name: "Review",
    });
  });
});

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

it("projects canonical dashboard factory snapshots without internal system-time routes", () => {
  const initialStructure = event(
    "initial-system-time-factory",
    1,
    FACTORY_EVENT_TYPES.initialStructureRequest,
    {
      factory: {
        name: "dashboard-public-factory",
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "reviewing", type: "PROCESSING" },
              { name: "done", type: "TERMINAL" },
            ],
          },
          {
            name: "__system_time",
            states: [{ name: "pending", type: "PROCESSING" }],
          },
        ],
        workstations: [
          {
            behavior: "CLASSIFIER_WORKSTATION",
            classificationRoutes: [
              {
                label: "ready",
                outputs: [{ state: "reviewing", workType: "story" }],
              },
              {
                label: "tick",
                outputs: [{ state: "pending", workType: "__system_time" }],
              },
            ],
            id: "route-story",
            inputs: [
              { state: "new", workType: "story" },
              { state: "pending", workType: "__system_time" },
            ],
            name: "Route story",
            onContinue: [{ state: "pending", workType: "__system_time" }],
            onFailure: [{ state: "pending", workType: "__system_time" }],
            onRejection: [{ state: "pending", workType: "__system_time" }],
            outputs: [
              { state: "done", workType: "story" },
              { state: "pending", workType: "__system_time" },
            ],
            worker: "router",
          },
          {
            classificationRoutes: [
              {
                label: "tick",
                outputs: [{ state: "pending", workType: "__system_time" }],
              },
            ],
            id: "system-only-public-id",
            inputs: [{ state: "new", workType: "story" }],
            name: "System-only route",
            worker: "router",
          },
          {
            id: "__system_time:expire",
            inputs: [{ state: "pending", workType: "__system_time" }],
            name: "__system_time:expire",
            outputs: [],
            worker: "",
          },
        ],
      },
    },
  );

  const snapshot = buildFactoryTimelineSnapshot([initialStructure], 1);

  expect(JSON.stringify(snapshot.factory)).not.toContain("__system_time");
  expect(snapshot.factory?.workTypes).toEqual([
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "reviewing", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ]);
  expect(snapshot.factory?.workstations).toEqual([
    expect.objectContaining({
      classificationRoutes: [
        {
          label: "ready",
          outputs: [{ state: "reviewing", workType: "story" }],
        },
      ],
      id: "route-story",
      inputs: [{ state: "new", workType: "story" }],
      outputs: [{ state: "done", workType: "story" }],
    }),
    expect.not.objectContaining({
      classificationRoutes: expect.any(Array),
    }),
  ]);
  expect(snapshot.factory?.workstations?.[0]).not.toHaveProperty("onContinue");
  expect(snapshot.factory?.workstations?.[0]).not.toHaveProperty("onFailure");
  expect(snapshot.factory?.workstations?.[0]).not.toHaveProperty("onRejection");
});

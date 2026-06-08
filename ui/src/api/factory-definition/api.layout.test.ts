import { FactoryDefinitionAPIError, normalizeFactoryDefinition } from "./api";

const factoryDefinitionWithLayout = {
  id: "agent-factory",
  layout: {
    schemaVersion: 1,
    nodes: [
      {
        id: "workstation:review",
        locked: false,
        position: { x: 420, y: 180 },
        size: { width: 156, height: 196 },
      },
    ],
    edges: [
      {
        id: "workstation-output:workstation:review->work-state:task:done",
        labelPosition: { x: 590, y: 204 },
        waypoints: [{ x: 540, y: 220 }],
      },
    ],
    groups: [
      {
        bounds: { x: 360, y: 120, width: 520, height: 360 },
        color: "blue",
        id: "review-lane",
        label: "Review",
        locked: false,
        nodeIds: ["workstation:review"],
        parentGroupId: null,
      },
    ],
    preferences: {
      direction: "RIGHT",
    },
    viewport: { x: 0, y: 0, zoom: 1 },
  },
  name: "agent-factory",
  resources: [{ capacity: 3, id: "resource:gpu", name: "gpu" }],
  workers: [{ id: "worker:writer", name: "writer", type: "MODEL_WORKER" }],
  workTypes: [
    {
      id: "task",
      name: "task",
      states: [
        { id: "queued", name: "queued", type: "INITIAL" },
        { id: "done", name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      id: "review",
      inputs: [{ state: "queued", workType: "task" }],
      name: "Review",
      outputs: [{ state: "done", workType: "task" }],
      worker: "writer",
    },
  ],
} as const;

const malformedLayoutFactoryDefinition = {
  layout: {
    nodes: [
      {
        id: "workstation:review",
        position: { x: 420, y: 180 },
      },
    ],
  },
  name: "legacy-factory",
  workers: [{ name: "writer" }],
  workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
  workstations: [
    {
      inputs: [{ state: "queued", workType: "task" }],
      name: "Review",
      outputs: [{ state: "queued", workType: "task" }],
      worker: "writer",
    },
  ],
} as const;

describe("factory-definition layout contract", () => {
  it("accepts explicit public ids and portable layout metadata", () => {
    expect(normalizeFactoryDefinition(factoryDefinitionWithLayout)).toEqual(
      factoryDefinitionWithLayout,
    );
  });

  it("rejects malformed layout payloads", () => {
    expect(() =>
      normalizeFactoryDefinition(malformedLayoutFactoryDefinition),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.layout.schemaVersion is required.",
      ),
    );
  });

  it("rejects non-finite waypoint geometry during normalization", () => {
    expect(() =>
      normalizeFactoryDefinition({
        ...factoryDefinitionWithLayout,
        layout: {
          ...factoryDefinitionWithLayout.layout,
          edges: [
            {
              id: "workstation-output:workstation:review->work-state:task:done",
              waypoints: [{ x: Number.NaN, y: 220 }],
            },
          ],
        },
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.layout.edges[0].waypoints[0].x must be a number.",
      ),
    );
  });

  it("round-trips authored edge waypoints through normalization", () => {
    const normalized = normalizeFactoryDefinition(factoryDefinitionWithLayout);

    expect(normalized.layout?.edges?.[0]).toEqual({
      id: "workstation-output:workstation:review->work-state:task:done",
      labelPosition: { x: 590, y: 204 },
      waypoints: [{ x: 540, y: 220 }],
    });
  });
});

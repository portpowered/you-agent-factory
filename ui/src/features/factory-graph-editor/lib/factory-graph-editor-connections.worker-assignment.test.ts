import type { FactoryGraphTopology } from "./factory-graph-draft-types";
import {
  buildFactoryGraphEdgeChangeFromConnection,
  createFactoryGraphWorkstationResolver,
  factoryGraphConnectionAnchorContext,
  getLocalizedFactoryGraphConnectionAnchors,
} from "./factory-graph-editor-connections";

const logicalMoveContext = factoryGraphConnectionAnchorContext({
  type: "LOGICAL_MOVE",
});

const modelWorkstationContext = factoryGraphConnectionAnchorContext({
  type: "MODEL_WORKSTATION",
  behavior: "STANDARD",
});

const workerAssignmentTopology: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "worker:writer",
      key: { kind: "worker", name: "writer" },
      kind: "worker",
      label: "writer",
    },
    {
      id: "workstation:router",
      key: { kind: "workstation", name: "router" },
      kind: "workstation",
      label: "router",
    },
    {
      id: "workstation:draft",
      key: { kind: "workstation", name: "draft" },
      kind: "workstation",
      label: "draft",
    },
  ],
};

it("omits worker-assignment-target for LOGICAL_MOVE workstations", () => {
  const anchorIds = getLocalizedFactoryGraphConnectionAnchors(
    "workstation",
    "en",
    logicalMoveContext,
  ).map((anchor) => anchor.id);

  expect(anchorIds).not.toContain("worker-assignment-target");
  expect(anchorIds).toEqual(
    expect.arrayContaining([
      "workstation-input-target",
      "workstation-resource-target",
      "workstation-output-source",
    ]),
  );
});

it("exposes worker-assignment-target for worker-backed workstations", () => {
  const anchorIds = getLocalizedFactoryGraphConnectionAnchors(
    "workstation",
    "en",
    modelWorkstationContext,
  ).map((anchor) => anchor.id);

  expect(anchorIds).toContain("worker-assignment-target");
});

it("rejects new worker-assignment edges targeting a LOGICAL_MOVE workstation", () => {
  expect(
    buildFactoryGraphEdgeChangeFromConnection(
      workerAssignmentTopology,
      {
        sourceAnchorId: "worker-assignment-source",
        sourceNodeId: "worker:writer",
        targetAnchorId: "worker-assignment-target",
        targetNodeId: "workstation:router",
      },
      createFactoryGraphWorkstationResolver([
        {
          body: "Route work downstream.",
          inputs: [],
          name: "router",
          outputs: [],
          type: "LOGICAL_MOVE",
          worker: "",
        },
      ]),
    ),
  ).toBeNull();
});

it("maps worker-assignment connections for worker-backed workstations", () => {
  expect(
    buildFactoryGraphEdgeChangeFromConnection(
      workerAssignmentTopology,
      {
        sourceAnchorId: "worker-assignment-source",
        sourceNodeId: "worker:writer",
        targetAnchorId: "worker-assignment-target",
        targetNodeId: "workstation:draft",
      },
      createFactoryGraphWorkstationResolver([
        {
          body: "Draft the story.",
          inputs: [],
          name: "draft",
          outputs: [],
          type: "MODEL_WORKSTATION",
          worker: "writer",
        },
      ]),
    ),
  ).toEqual({
    kind: "worker-assignment",
    source: { kind: "worker", name: "writer" },
    target: { kind: "workstation", name: "draft" },
  });
});

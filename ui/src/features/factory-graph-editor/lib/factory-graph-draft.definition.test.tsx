import { renderHook } from "@testing-library/react";

import type { DashboardTopology } from "../../../api/dashboard/types";
import {
  buildFactoryGraphTopologyFromDefinition,
  buildPendingFactoryDefinition,
  createEmptyFactoryGraphDraft,
  useFactoryGraphDraftState,
  validateFactoryGraphDraft,
} from "../public";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "./factory-graph-draft.test-helpers";

it("derives graph nodes and relations from the canonical editable definition", () => {
  const topology = buildFactoryGraphTopologyFromDefinition(
    baseFactoryDefinition,
  );

  expect(topology.nodes.map((node) => node.id)).toEqual([
    "resource:gpu",
    "work-state:story:done",
    "work-state:story:queued",
    "work-type:story",
    "worker:writer",
    "workstation:draft",
  ]);
  expect(topology.edges.map((edge) => edge.id)).toEqual([
    "work-type-state:work-type:story->work-state:story:done",
    "work-type-state:work-type:story->work-state:story:queued",
    "worker-assignment:worker:writer->workstation:draft",
    "workstation-input:work-state:story:queued->workstation:draft",
    "workstation-output:workstation:draft->work-state:story:done",
    "workstation-resource:resource:gpu->workstation:draft",
  ]);
});

it("builds a pending full factory definition while preserving untouched content", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.additions.workers.push({
    model: "gpt-5-mini",
    name: "reviewer",
    type: "MODEL_WORKER",
  });
  draft.additions.workstations.push({
    body: "Review the drafted story.",
    inputs: [
      {
        state: "done",
        workType: "story",
      },
    ],
    name: "review",
    outputs: [
      {
        state: "approved",
        workType: "story",
      },
    ],
    type: "MODEL_WORKSTATION",
    worker: "reviewer",
  });
  draft.additions.workStates.push({
    state: {
      name: "approved",
      type: "TERMINAL",
    },
    workTypeName: "story",
  });
  draft.edgeChanges.additions.push({
    kind: "workstation-input",
    source: {
      kind: "work-state",
      stateName: "done",
      workTypeName: "story",
    },
    target: {
      kind: "workstation",
      name: "review",
    },
  });
  draft.edgeChanges.additions.push({
    kind: "workstation-output",
    source: {
      kind: "workstation",
      name: "review",
    },
    target: {
      kind: "work-state",
      stateName: "approved",
      workTypeName: "story",
    },
  });

  const pendingFactoryDefinition = buildPendingFactoryDefinition(
    baseFactoryDefinition,
    draft,
  );

  expect(pendingFactoryDefinition).toMatchObject({
    metadata: {
      owner: "operations",
    },
    resources: [
      {
        capacity: 2,
        name: "gpu",
      },
    ],
    workers: [
      {
        model: "gpt-5",
        name: "writer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5-mini",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body: "Draft the story.",
        name: "draft",
      },
      {
        body: "Review the drafted story.",
        name: "review",
        worker: "reviewer",
      },
    ],
  });
  expect(
    pendingFactoryDefinition?.workTypes?.find(
      (workType) => workType.name === "story",
    )?.states,
  ).toEqual([
    {
      name: "queued",
      type: "INITIAL",
    },
    {
      name: "done",
      type: "TERMINAL",
    },
    {
      name: "approved",
      type: "TERMINAL",
    },
  ]);
});

it("validates duplicate identifiers, missing required fields, and incompatible edges before save", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.additions.workers.push({
    model: "gpt-5-mini",
    name: "writer",
    type: "MODEL_WORKER",
  });
  draft.additions.workstations.push({
    body: "Missing worker assignment.",
    inputs: [],
    name: "review",
    outputs: [],
    type: "MODEL_WORKSTATION",
    worker: "",
  });
  draft.edgeChanges.additions.push({
    kind: "workstation-input",
    source: {
      kind: "worker",
      name: "writer",
    },
    target: {
      kind: "workstation",
      name: "draft",
    },
  });

  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "DUPLICATE_IDENTIFIER",
        target: {
          kind: "node",
          id: "worker:writer",
        },
      }),
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
      }),
      expect.objectContaining({
        code: "INCOMPATIBLE_EDGE",
      }),
    ]),
  );
});

it("reports missing required draft names before save-building", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.additions.workers.push({
    model: "gpt-5-mini",
    name: "",
    type: "MODEL_WORKER",
  });
  draft.additions.workStates.push({
    state: {
      name: "",
      type: "TERMINAL",
    },
    workTypeName: "",
  });

  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "name",
        target: {
          kind: "field",
          field: "worker.name",
        },
      }),
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "name",
        target: {
          kind: "field",
          field: "work-state.name",
        },
      }),
      expect.objectContaining({
        code: "MISSING_REQUIRED_FIELD",
        field: "name",
        target: {
          kind: "field",
          field: "work-type.name",
        },
      }),
    ]),
  );
  expect(buildPendingFactoryDefinition(baseFactoryDefinition, draft)).toBeNull();
});

it("reports unknown edge nodes when a draft edge references a workstation outside the supported draft state", () => {
  const draft = createEmptyFactoryGraphDraft();
  draft.edgeChanges.additions.push({
    kind: "workstation-input",
    source: {
      kind: "work-state",
      stateName: "queued",
      workTypeName: "story",
    },
    target: {
      kind: "workstation",
      name: "missing",
    },
  });

  const validationErrors = validateFactoryGraphDraft(
    baseFactoryDefinition,
    draft,
  );

  expect(validationErrors).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        code: "UNKNOWN_NODE",
        target: {
          kind: "edge",
          id: "workstation-input:work-state:story:queued->workstation:missing",
        },
      }),
    ]),
  );
  expect(buildPendingFactoryDefinition(baseFactoryDefinition, draft)).toBeNull();
});

it("falls back to projection topology until the editable definition is available", () => {
  const projectedTopology: DashboardTopology = {
    edges: [],
    workstation_node_ids: ["draft"],
    workstation_nodes_by_id: {
      draft: {
        input_places: [
          {
            kind: "work_state",
            place_id: "story:queued",
            state_category: "INITIAL",
            state_value: "queued",
            type_id: "story",
          },
        ],
        node_id: "draft",
        output_places: [
          {
            kind: "work_state",
            place_id: "story:done",
            state_category: "TERMINAL",
            state_value: "done",
            type_id: "story",
          },
        ],
        transition_id: "draft",
        workstation_name: "draft",
      },
    },
  };

  const { result, rerender } = renderHook(
    (props: {
      currentFactoryDocument?: typeof currentFactoryDocument;
      projectedTopology?: DashboardTopology;
    }) => useFactoryGraphDraftState(props),
    {
      initialProps: {
        projectedTopology,
      },
    },
  );

  expect(result.current.source).toBe("projection");
  expect(result.current.graph.nodes.map((node) => node.id)).toEqual([
    "work-state:story:done",
    "work-state:story:queued",
    "workstation:draft",
  ]);

  rerender({
    currentFactoryDocument,
    projectedTopology,
  });

  expect(result.current.source).toBe("current-factory");
  expect(result.current.baseDocument?.version.logical).toBe(5);
});

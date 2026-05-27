import { describe, expect, it } from "vitest";

import {
  addFactoryGraphNode,
  applyFactoryGraphPendingEdits,
  buildFactoryGraphSaveInput,
  buildFactoryGraphState,
  connectFactoryGraphNodes,
  createEmptyFactoryGraphDraft,
  disconnectFactoryGraphEdge,
  projectFactoryGraphToReactFlow,
  removeFactoryGraphNode,
  updateFactoryGraphNodeField,
} from "../public";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: keeps the operation contract scenarios co-located.
describe("factory graph operations", () => {
  it("adds every editable graph entity through immutable operation results", () => {
    const draft = createEmptyFactoryGraphDraft();

    const withResource = addFactoryGraphNode({
      baseFactoryDefinition,
      draft,
      node: {
        capacity: "3",
        kind: "resource",
        name: "review-slot",
      },
    });
    expect(withResource.ok).toBe(true);
    expect(draft.additions.resources).toEqual([]);

    const withWorker = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: expectOk(withResource),
      node: {
        kind: "worker",
        model: "gpt-5-mini",
        name: "reviewer",
      },
    });
    const withWorkType = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: expectOk(withWorker),
      node: {
        initialStateName: "new",
        kind: "work-type",
        name: "bug",
      },
    });
    const withWorkState = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: expectOk(withWorkType),
      node: {
        kind: "work-state",
        name: "reviewed",
        stateType: "TERMINAL",
        workTypeName: "story",
      },
    });
    const withWorkstation = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: expectOk(withWorkState),
      node: {
        behavior: "STANDARD",
        body: "Review the story.",
        kind: "workstation",
        name: "review",
        workerName: "writer",
      },
    });

    expect(expectOk(withWorkstation).additions).toMatchObject({
      resources: [
        {
          capacity: 3,
          name: "review-slot",
        },
      ],
      workers: [
        {
          model: "gpt-5-mini",
          name: "reviewer",
        },
      ],
      workStates: [
        {
          state: {
            name: "reviewed",
            type: "TERMINAL",
          },
          workTypeName: "story",
        },
      ],
      workTypes: [
        {
          name: "bug",
        },
      ],
      workstations: [
        {
          body: "Review the story.",
          name: "review",
          worker: "writer",
        },
      ],
    });
  });

  it("rejects invalid add, remove, connect, and protected disconnect operations with typed reasons", () => {
    const duplicateWorker = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        kind: "worker",
        model: "gpt-5-mini",
        name: "writer",
      },
    });
    expect(duplicateWorker).toMatchObject({
      ok: false,
      reason: "DUPLICATE_IDENTIFIER",
    });

    const removeAssignedWorker = removeFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      nodeId: "worker:writer",
    });
    expect(removeAssignedWorker).toMatchObject({
      ok: false,
      reason: "BLOCKED_REMOVAL",
    });

    const invalidConnection = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "workstation-on-continue-target",
      targetNodeId: "work-state:story:done",
    });
    expect(invalidConnection).toMatchObject({
      ok: false,
      reason: "INVALID_CONNECTION",
    });

    const protectedDisconnect = disconnectFactoryGraphEdge({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      edgeId: "work-type-state:work-type:story->work-state:story:queued",
    });
    expect(protectedDisconnect).toMatchObject({
      ok: false,
      reason: "PROTECTED_EDGE",
    });
  });

  it("connects and disconnects semantic edge changes in the draft", () => {
    const connected = connectFactoryGraphNodes({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "workstation-on-failure-target",
      targetNodeId: "work-state:story:done",
    });

    expect(expectOk(connected).edgeChanges.additions).toEqual([
      {
        kind: "workstation-on-failure",
        source: {
          kind: "workstation",
          name: "draft",
        },
        target: {
          kind: "work-state",
          stateName: "done",
          workTypeName: "story",
        },
      },
    ]);

    const disconnected = disconnectFactoryGraphEdge({
      baseFactoryDefinition,
      draft: expectOk(connected),
      edgeId: "workstation-on-failure:workstation:draft->work-state:story:done",
    });
    expect(expectOk(disconnected).edgeChanges).toEqual({
      additions: [],
      removals: [],
    });
  });

  it("updates canonical graph fields with typed setters without mutating the input", () => {
    const updated = updateFactoryGraphNodeField({
      baseFactoryDefinition,
      update: {
        field: "body",
        kind: "workstation",
        name: "draft",
        value: "Draft the story with new instructions.",
      },
    });

    expect(expectOk(updated).workstations?.[0]?.body).toBe(
      "Draft the story with new instructions.",
    );
    expect(baseFactoryDefinition.workstations?.[0]?.body).toBe(
      "Draft the story.",
    );

    expect(
      updateFactoryGraphNodeField({
        baseFactoryDefinition,
        update: {
          field: "type",
          kind: "work-state",
          stateName: "missing",
          value: "TERMINAL",
          workTypeName: "story",
        },
      }),
    ).toMatchObject({
      ok: false,
      reason: "NODE_NOT_FOUND",
    });
  });

  it("builds graph state, React Flow projection, and save input from pending edits", () => {
    const removed = removeFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      nodeId: "resource:gpu",
    });
    const draft = expectOk(removed);
    const state = buildFactoryGraphState({
      baseFactoryDefinition,
      draft,
    });

    expect(state.validationErrors).toEqual([]);
    expect(state.graph.nodes.map((node) => node.id)).not.toContain(
      "resource:gpu",
    );

    const projection = projectFactoryGraphToReactFlow(state.graph);
    expect(projection.nodes[0]).toEqual(
      expect.objectContaining({
        id: "work-state:story:done",
        position: expect.objectContaining({
          x: expect.any(Number),
          y: expect.any(Number),
        }),
      }),
    );
    expect(projection.edges.map((edge) => edge.id)).toEqual(
      state.graph.edges.map((edge) => edge.id),
    );

    const applied = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft,
    });
    const saveInput = buildFactoryGraphSaveInput({
      baseFactoryDefinition,
      draft,
    });

    expect(expectOk(applied).resources).toEqual([]);
    expect(expectOk(saveInput).workstations?.[0]).toMatchObject({
      body: "Draft the story.",
      name: "draft",
      worker: "writer",
    });
    expect(expectOk(saveInput).workstations?.[0]?.resources).toBeUndefined();
  });
});

function expectOk<T>(
  result: ReturnType<
    | typeof addFactoryGraphNode
    | typeof applyFactoryGraphPendingEdits
    | typeof buildFactoryGraphSaveInput
    | typeof connectFactoryGraphNodes
    | typeof disconnectFactoryGraphEdge
    | typeof removeFactoryGraphNode
    | typeof updateFactoryGraphNodeField
  >,
) {
  expect(result.ok).toBe(true);
  return (result as { ok: true; value: T }).value;
}

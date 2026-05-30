import { act, renderHook } from "@testing-library/react";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "../lib/factory-graph-draft.test-helpers";
import { useEditableFactoryGraph } from "./use-editable-factory-graph";

const logicalMoveFactoryDocument: CurrentFactoryDocument = {
  ...baseFactoryDefinition,
  workstations: [
    ...(baseFactoryDefinition.workstations ?? []),
    {
      body: "Move work downstream.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "router",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      type: "LOGICAL_MOVE",
      worker: "",
    },
  ],
  version: {
    logical: "5",
    physical: "2026-05-18T15:00:00Z",
  },
};

function anchorIdsForWorkstation(
  projection: ReturnType<typeof useEditableFactoryGraph>["projection"],
  workstationName: string,
): string[] {
  const node = projection.nodes.find(
    (candidate) => candidate.id === `workstation:${workstationName}`,
  );
  return node?.data.connectionAnchors.map((anchor) => anchor.id) ?? [];
}

describe("useEditableFactoryGraph worker-assignment disconnect and reconnect", () => {
  it("allows worker-assignment disconnect and reconnect while save stays blocked until reassigned", async () => {
    const saveFactoryDefinition = vi.fn(async () => undefined);
    const { result } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument,
        saveFactoryDefinition,
      }),
    );

    act(() => {
      result.current.actions.addNode({
        kind: "worker",
        model: "gpt-5-mini",
        name: "reviewer",
      });
    });
    expect(result.current.blockedOperation).toBeNull();

    act(() => {
      result.current.actions.disconnectEdge(
        "worker-assignment:worker:writer->workstation:draft",
      );
    });

    expect(result.current.blockedOperation).toBeNull();
    expect(result.current.pendingState.hasChanges).toBe(true);
    expect(result.current.saveState.canSave).toBe(false);
    expect(result.current.validationState.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "MISSING_REQUIRED_FIELD",
          field: "worker",
          target: {
            kind: "node",
            id: "workstation:draft",
          },
        }),
      ]),
    );
    expect(
      result.current.draftState.graph.edges.some(
        (edge) => edge.id === "worker-assignment:worker:writer->workstation:draft",
      ),
    ).toBe(false);
    expect(
      result.current.pendingState.pendingFactoryDefinition?.workstations?.find(
        (workstation) => workstation.name === "draft",
      ),
    ).toMatchObject({
      worker: "",
    });

    act(() => {
      result.current.actions.connectNodes({
        sourceAnchorId: "worker-assignment-source",
        sourceNodeId: "worker:reviewer",
        targetAnchorId: "worker-assignment-target",
        targetNodeId: "workstation:draft",
      });
    });

    expect(result.current.blockedOperation).toBeNull();
    expect(result.current.saveState.canSave).toBe(true);
    expect(result.current.validationState.isValid).toBe(true);
    expect(
      result.current.draftState.graph.edges.some(
        (edge) =>
          edge.id === "worker-assignment:worker:reviewer->workstation:draft",
      ),
    ).toBe(true);

    let didSave = false;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(true);
    expect(saveFactoryDefinition).toHaveBeenCalledWith({
      baseVersion: currentFactoryDocument.version,
      factoryDefinition: expect.objectContaining({
        workstations: expect.arrayContaining([
          expect.objectContaining({
            name: "draft",
            worker: "reviewer",
          }),
        ]),
      }),
    });
  });
});

describe("useEditableFactoryGraph logical-move worker handles", () => {
  it("omits worker-assignment handles on LOGICAL_MOVE stations and allows save without a worker", async () => {
    const saveFactoryDefinition = vi.fn(async () => undefined);
    const { result } = renderHook(() =>
      useEditableFactoryGraph({
        currentFactoryDocument: logicalMoveFactoryDocument,
        saveFactoryDefinition,
      }),
    );

    expect(anchorIdsForWorkstation(result.current.projection, "router")).not.toContain(
      "worker-assignment-target",
    );
    expect(anchorIdsForWorkstation(result.current.projection, "draft")).toContain(
      "worker-assignment-target",
    );

    act(() => {
      result.current.actions.addNode({
        kind: "resource",
        capacity: 1,
        name: "review-slot",
      });
    });

    expect(result.current.validationState.errors).toEqual([]);
    expect(result.current.saveState.canSave).toBe(true);

    let didSave = false;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(true);
    expect(saveFactoryDefinition).toHaveBeenCalledWith({
      baseVersion: logicalMoveFactoryDocument.version,
      factoryDefinition: expect.objectContaining({
        workstations: expect.arrayContaining([
          expect.objectContaining({
            name: "router",
            type: "LOGICAL_MOVE",
            worker: "",
          }),
        ]),
        resources: expect.arrayContaining([
          expect.objectContaining({ name: "review-slot" }),
        ]),
      }),
    });
  });
});

// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: shared draft-edge coverage stays grouped around one projection seam and compact custom layouts.
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { buildGraphLayout } from "../../flowchart/lib/layout";
import { buildCurrentActivityGraphLayoutFromFactory } from "./current-activity-factory-graph-layout";
import { buildVisibleGraphEdgesWithDraft } from "./react-flow-current-activity-card-draft-edges";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  buildActiveGraphHighlights,
  buildHandleAssignments,
  buildVisibleGraphEdges,
} from "./react-flow-current-activity-card-graph";

describe("current activity graph draft edges", () => {
  it("adds supported pending draft routes onto the shared observer graph surface", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "workstation-output",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "blocked",
                  workTypeName: "story",
                },
              },
            ],
            removals: [],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });
    const handleAssignments = buildHandleAssignments(visibleGraphEdges);
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      pendingAdditionEdgeIds,
      visibleGraphEdges,
    );

    expect(pendingAdditionEdgeIds).toEqual(
      new Set(["workstation-output:workstation:review->place:story:blocked"]),
    );
    expect(edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-output:workstation:review->place:story:blocked",
          source: "workstation:review",
          sourceHandle: "workstation-output-source",
          style: expect.objectContaining({
            stroke: "var(--color-af-warning-text)",
            strokeDasharray: "9 4",
          }),
          target: "place:story:blocked",
          targetHandle: "work-state-input-target",
        }),
      ]),
    );
  });

  it("maps continue, rejection, and failure draft routes onto shared observer edges", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "workstation-on-continue",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "ready",
                  workTypeName: "story",
                },
              },
              {
                kind: "workstation-on-rejection",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "blocked",
                  workTypeName: "story",
                },
              },
              {
                kind: "workstation-on-failure",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "blocked",
                  workTypeName: "story",
                },
              },
            ],
            removals: [],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });

    expect(pendingAdditionEdgeIds).toEqual(
      new Set([
        "workstation-on-continue:workstation:review->place:story:ready",
        "workstation-on-rejection:workstation:review->place:story:blocked",
        "workstation-on-failure:workstation:review->place:story:blocked",
      ]),
    );
    expect(visibleGraphEdges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          edgeId:
            "workstation-on-continue:workstation:review->place:story:ready",
          label: "story:ready",
          outcomeKind: "continue",
          stateCategory: undefined,
        }),
        expect.objectContaining({
          edgeId:
            "workstation-on-rejection:workstation:review->place:story:blocked",
          label: "story:blocked",
          outcomeKind: "rejected",
          stateCategory: undefined,
        }),
        expect.objectContaining({
          edgeId:
            "workstation-on-failure:workstation:review->place:story:blocked",
          label: "story:blocked",
          outcomeKind: "failed",
          stateCategory: "FAILED",
        }),
      ]),
    );
  });

  it("removes matching supported draft routes and ignores unsupported or missing-node edge changes", () => {
    const graphLayout = {
      edges: [
        {
          edgeId: "workstation-output:workstation:review->place:story:complete",
          fromNodeId: "workstation:review",
          label: "story:complete",
          labelX: 0,
          labelY: 0,
          outcomeKind: "accepted",
          path: "",
          sourcePlaceKind: undefined,
          stateCategory: undefined,
          targetPlaceKind: "work_state",
          toNodeId: "place:story:complete",
        },
      ],
      height: 0,
      nodes: [
        {
          column: 0,
          height: 1,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 1,
          workstationNodeId: "review",
          x: 0,
          y: 0,
        },
        {
          column: 1,
          height: 1,
          nodeId: "place:story:complete",
          nodeKind: "state_position",
          place: {
            id: "story:complete",
            kind: "work_state",
            state_category: "TERMINAL",
            work_type_id: "story",
          },
          row: 0,
          width: 1,
          x: 1,
          y: 0,
        },
      ],
      width: 0,
    } satisfies GraphLayout;
    const baseVisibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const removedEdgeId =
      "workstation-output:workstation:review->place:story:complete";

    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "worker-assignment",
                source: { kind: "worker", name: "reviewer" },
                target: { kind: "workstation", name: "review" },
              },
              {
                kind: "workstation-output",
                source: { kind: "workstation", name: "missing-workstation" },
                target: {
                  kind: "work-state",
                  stateName: "blocked",
                  workTypeName: "story",
                },
              },
              {
                kind: "workstation-output",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "missing-state",
                  workTypeName: "story",
                },
              },
            ],
            removals: [
              {
                kind: "workstation-output",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "complete",
                  workTypeName: "story",
                },
              },
              {
                kind: "worker-resource",
                source: { kind: "worker", name: "reviewer" },
                target: { kind: "resource", name: "quality-gate" },
              },
            ],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });

    expect(pendingAdditionEdgeIds.size).toBe(0);
    expect(
      visibleGraphEdges.some((edge) => edge.edgeId === removedEdgeId),
    ).toBe(false);
    expect(visibleGraphEdges).toHaveLength(baseVisibleGraphEdges.length - 1);
  });

  it("projects supported draft edges for non-work-state observer nodes with empty labels", () => {
    const graphLayout = {
      edges: [],
      height: 0,
      nodes: [
        {
          column: 0,
          height: 1,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 1,
          workstationNodeId: "review",
          x: 0,
          y: 0,
        },
        {
          column: 1,
          height: 1,
          nodeId: "place:quality-gate",
          nodeKind: "constraint",
          place: {
            id: "quality-gate",
            kind: "constraint",
          },
          row: 0,
          width: 1,
          x: 1,
          y: 0,
        },
      ],
      width: 0,
    } satisfies GraphLayout;

    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "workstation-output",
                source: { kind: "workstation", name: "review" },
                target: { kind: "resource", name: "quality-gate" },
              },
            ],
            removals: [],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });

    expect(pendingAdditionEdgeIds).toEqual(
      new Set(["workstation-output:workstation:review->place:quality-gate"]),
    );
    expect(visibleGraphEdges).toEqual([
      expect.objectContaining({
        edgeId: "workstation-output:workstation:review->place:quality-gate",
        label: "",
        sourcePlaceKind: undefined,
        targetPlaceKind: undefined,
        toNodeId: "place:quality-gate",
      }),
    ]);
  });

  it("projects workstation-input draft edges from shared work-state nodes", () => {
    const graphLayout = {
      edges: [],
      height: 0,
      nodes: [
        {
          column: 0,
          height: 1,
          nodeId: "place:story:ready",
          nodeKind: "state_position",
          place: {
            id: "story:ready",
            kind: "work_state",
            state_category: "PROCESSING",
            work_type_id: "story",
          },
          row: 0,
          width: 1,
          x: 0,
          y: 0,
        },
        {
          column: 1,
          height: 1,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 1,
          workstationNodeId: "review",
          x: 1,
          y: 0,
        },
      ],
      width: 0,
    } satisfies GraphLayout;

    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "workstation-input",
                source: {
                  kind: "work-state",
                  stateName: "ready",
                  workTypeName: "story",
                },
                target: { kind: "workstation", name: "review" },
              },
            ],
            removals: [],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });

    expect(pendingAdditionEdgeIds).toEqual(
      new Set(["workstation-input:place:story:ready->workstation:review"]),
    );
    expect(visibleGraphEdges).toEqual([
      expect.objectContaining({
        edgeId: "workstation-input:place:story:ready->workstation:review",
        fromNodeId: "place:story:ready",
        label: "",
        outcomeKind: "accepted",
        sourcePlaceKind: "work_state",
        targetPlaceKind: undefined,
        toNodeId: "workstation:review",
      }),
    ]);
  });

  it("projects resource availability draft inputs onto the canonical resource edge", async () => {
    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory({
      name: "resource-availability-draft",
      resources: [{ capacity: 1, name: "executor-slot" }],
      workTypes: [
        {
          name: "executor-slot",
          states: [{ name: "available", type: "INITIAL" }],
        },
      ],
      workstations: [
        {
          id: "process",
          inputs: [],
          name: "process",
          outputs: [],
          type: "MODEL_WORKSTATION",
          worker: "processor",
        },
      ],
      workers: [{ name: "processor", type: "MODEL_WORKER" }],
    } as never);
    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "workstation-input",
                source: {
                  kind: "work-state",
                  stateName: "available",
                  workTypeName: "executor-slot",
                },
                target: { kind: "workstation", name: "process" },
              },
            ],
            removals: [],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });

    expect(pendingAdditionEdgeIds).toEqual(
      new Set([
        "workstation-resource:resource:executor-slot->workstation:process",
      ]),
    );
    expect(visibleGraphEdges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          edgeId:
            "workstation-resource:resource:executor-slot->workstation:process",
          fromNodeId: "resource:executor-slot",
          sourcePlaceKind: "resource",
          toNodeId: "workstation:process",
        }),
      ]),
    );
  });
});

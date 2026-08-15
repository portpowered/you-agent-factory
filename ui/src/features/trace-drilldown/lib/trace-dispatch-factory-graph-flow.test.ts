import {
  FACTORY_GRAPH_NODE_TYPES,
  projectFactoryGraphReplayFlow,
} from "@you-agent-factory/factory-graph";
import { describe, expect, it } from "vitest";
import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { buildTraceDispatchFactoryGraphFlow } from "./trace-dispatch-factory-graph-flow";

function buildWorkItem(
  workID: string,
  overrides: Partial<DashboardWorkItemRef> = {},
): DashboardWorkItemRef {
  return {
    display_name: workID,
    work_id: workID,
    work_type_id: "story",
    ...overrides,
  };
}

function buildDispatch(
  dispatchID: string,
  overrides: Partial<DashboardTraceDispatch> = {},
): DashboardTraceDispatch {
  return {
    dispatch_id: dispatchID,
    duration_millis: 1000,
    end_time: "2026-04-22T18:00:01Z",
    outcome: "ACCEPTED",
    start_time: "2026-04-22T18:00:00Z",
    transition_id: dispatchID,
    workstation_name: dispatchID,
    ...overrides,
  };
}

function buildReplayWorkstationFlow() {
  return projectFactoryGraphReplayFlow({
    factory: { name: "Trace parity" },
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: [
          {
            entityId: "review",
            handles: [
              { id: "workstation-input-target", role: "target" },
              { id: "worker-assignment-target", role: "target" },
              { id: "workstation-resource-target", role: "target" },
              { id: "workstation-output-source", role: "source" },
              { id: "workstation-approval-source", role: "source" },
              { id: "workstation-on-continue-source", role: "source" },
              { id: "workstation-on-failure-source", role: "source" },
              { id: "workstation-on-rejection-source", role: "source" },
            ],
            id: "workstation:review",
            kind: "workstation",
            label: "review",
          },
        ],
        ok: true,
        selectedTick: 4,
      },
    },
    selectedTick: 4,
  });
}

describe("buildTraceDispatchFactoryGraphFlow", () => {
  it("projects dispatch history into shared workstation nodes and editor edges", () => {
    const flow = buildTraceDispatchFactoryGraphFlow([
      buildDispatch("dispatch-plan", {
        output_items: [buildWorkItem("work-reviewed")],
      }),
      buildDispatch("dispatch-implement", {
        input_items: [buildWorkItem("work-reviewed")],
      }),
    ]);

    expect(flow.nodes).toHaveLength(2);
    expect(
      flow.nodes.find((node) => node.id === "dispatch-plan"),
    ).toMatchObject({
      id: "dispatch-plan",
      type: "workstation",
      data: {
        active: false,
        dispatchId: "dispatch-plan",
        displayLabel: "dispatch-plan",
        kind: "workstation",
        handles: expect.arrayContaining([
          expect.objectContaining({
            hidden: true,
            id: "workstation-input-target",
          }),
        ]),
        workstation: {
          node_id: "dispatch-plan",
          transition_id: "dispatch-plan",
          workstation_name: "dispatch-plan",
        },
        workstationSemantics: {
          controlRole: "UNKNOWN",
          runtimeRole: "UNKNOWN",
          runtimeType: "UNKNOWN",
          schedulingBehavior: "UNKNOWN",
        },
      },
    });
    expect(
      flow.nodes.find((node) => node.id === "dispatch-plan")?.data,
    ).not.toHaveProperty("connectionAnchors");
    expect(flow.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: "dispatch-plan",
          sourceHandle: "workstation-output-source",
          target: "dispatch-implement",
          targetHandle: "workstation-input-target",
          type: "factoryEditorEdge",
          data: expect.objectContaining({
            kind: "workstation-on-continue",
            label: "",
          }),
        }),
      ]),
    );
  });

  it("passes locale through dispatch node data", () => {
    const flow = buildTraceDispatchFactoryGraphFlow(
      [buildDispatch("dispatch-plan")],
      "zh",
    );

    expect(flow.nodes[0]?.data.locale).toBe("zh");
  });

  it("registers only shared workstation graph React Flow node types", () => {
    const flow = buildTraceDispatchFactoryGraphFlow([
      buildDispatch("dispatch-plan"),
    ]);

    expect(Object.keys(FACTORY_GRAPH_NODE_TYPES).includes("workstation")).toBe(
      true,
    );
    expect(flow.nodes.every((node) => node.type === "workstation")).toBe(true);
    expect(flow.edges.every((edge) => edge.type === "factoryEditorEdge")).toBe(
      true,
    );
  });

  it("matches replay workstation semantics, dimensions, and handles", () => {
    const flow = buildTraceDispatchFactoryGraphFlow([
      buildDispatch("dispatch-review", {
        transition_id: "review",
        workstation_name: "review",
      }),
    ]);
    const replay = buildReplayWorkstationFlow();
    const traceNode = flow.nodes[0];
    const replayNode = replay.nodes[0];

    expect(traceNode?.type).toBe("workstation");
    expect(replayNode?.type).toBe("workstation");
    expect(traceNode?.data.workstationSemantics).toEqual(
      expect.objectContaining({
        controlRole: replayNode?.data.workstationSemantics?.controlRole,
        runtimeRole: replayNode?.data.workstationSemantics?.runtimeRole,
        runtimeType: replayNode?.data.workstationSemantics?.runtimeType,
        schedulingBehavior:
          replayNode?.data.workstationSemantics?.schedulingBehavior,
      }),
    );
    expect(traceNode?.data.handles.map((handle) => handle.id)).toEqual(
      replayNode?.data.handles.map((handle) => handle.id),
    );
    expect({
      height: traceNode?.height,
      initialHeight: traceNode?.initialHeight,
      initialWidth: traceNode?.initialWidth,
      measured: traceNode?.measured,
      width: traceNode?.width,
    }).toEqual({
      height: replayNode?.height,
      initialHeight: replayNode?.initialHeight,
      initialWidth: replayNode?.initialWidth,
      measured: replayNode?.measured,
      width: replayNode?.width,
    });
  });
});

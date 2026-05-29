import { describe, expect, it } from "vitest";
import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES } from "../components/trace-dispatch-factory-graph-node";
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

describe("buildTraceDispatchFactoryGraphFlow", () => {
  it("projects dispatch history into factory entity nodes and editor edges", () => {
    const editorMessages = getFactoryGraphEditorMessages();
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
      type: "factoryEntity",
      data: {
        dispatchId: "dispatch-plan",
        displayLabel: "dispatch-plan",
        kind: "workstation",
        kindLabel: editorMessages.kindLabel("workstation"),
      },
    });
    expect(flow.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: "dispatch-plan",
          target: "dispatch-implement",
          type: "factoryEditorEdge",
          data: expect.objectContaining({
            kind: "workstation-on-continue",
            label: editorMessages.edgeKindLabel("workstation-on-continue"),
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

  it("registers only factory graph React Flow node types", () => {
    const flow = buildTraceDispatchFactoryGraphFlow([
      buildDispatch("dispatch-plan"),
    ]);

    expect(Object.keys(TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES)).toEqual([
      "factoryEntity",
    ]);
    expect(flow.nodes.every((node) => node.type === "factoryEntity")).toBe(
      true,
    );
    expect(
      flow.edges.every((edge) => edge.type === "factoryEditorEdge"),
    ).toBe(true);
  });
});

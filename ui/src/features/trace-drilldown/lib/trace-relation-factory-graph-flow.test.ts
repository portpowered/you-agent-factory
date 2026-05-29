import { describe, expect, it } from "vitest";
import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES } from "../components/trace-relation-factory-graph-node";
import { buildTraceRelationFactoryGraphFlow } from "./trace-relation-factory-graph-flow";

const RELATIONS: DashboardWorkRelation[] = [
  {
    request_id: "request-parent-child",
    required_state: "DONE",
    source_work_id: "work-plan",
    source_work_name: "Plan story",
    target_work_id: "work-implement",
    target_work_name: "Implement story",
    type: "PARENT_CHILD",
  },
  {
    request_id: "request-retry",
    required_state: "FAILED",
    source_work_id: "work-implement",
    source_work_name: "Implement story",
    target_work_id: "work-repair",
    target_work_name: "Repair story",
    type: "RETRY",
  },
];

describe("buildTraceRelationFactoryGraphFlow", () => {
  it("projects batch relations into factory entity nodes and editor edges", () => {
    const editorMessages = getFactoryGraphEditorMessages();
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS);

    expect(flow.nodes).toHaveLength(3);
    expect(
      flow.nodes.find((node) => node.id === "work-implement"),
    ).toMatchObject({
      id: "work-implement",
      type: "factoryEntity",
      data: expect.objectContaining({
        displayLabel: "Implement story",
        endpointKey: "work-implement",
        kindLabel: editorMessages.kindLabel("work-state"),
        workID: "work-implement",
      }),
    });
    expect(flow.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          source: "work-plan",
          target: "work-implement",
          type: "factoryEditorEdge",
          ariaLabel:
            "Parent-child relation from Plan story to Implement story, requiring Done",
          data: expect.objectContaining({
            kind: "work-type-state",
            label: editorMessages.edgeKindLabel("work-type-state"),
          }),
          style: expect.objectContaining({
            stroke: "var(--color-af-success)",
            strokeDasharray: "7 5",
          }),
        }),
      ]),
    );
  });

  it("registers only factory graph React Flow node types", () => {
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS);

    expect(Object.keys(TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES)).toEqual([
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

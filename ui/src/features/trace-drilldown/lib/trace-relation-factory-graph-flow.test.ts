import { describe, expect, it, vi } from "vitest";
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

  it("styles relations without required state from relation type defaults", () => {
    const parentChildFlow = buildTraceRelationFactoryGraphFlow([
      {
        request_id: "request-parent-child-plain",
        source_work_id: "work-a",
        source_work_name: "Story A",
        target_work_id: "work-b",
        target_work_name: "Story B",
        type: "PARENT_CHILD",
      },
    ]);
    const relatedFlow = buildTraceRelationFactoryGraphFlow([
      {
        request_id: "request-related",
        source_work_id: "work-c",
        source_work_name: "Story C",
        target_work_id: "work-d",
        target_work_name: "Story D",
        type: "RELATED_TO",
      },
    ]);

    expect(parentChildFlow.edges[0]?.style).toMatchObject({
      stroke: "var(--color-af-accent)",
      strokeDasharray: undefined,
      strokeWidth: 1.7,
    });
    expect(relatedFlow.edges[0]?.style).toMatchObject({
      stroke: "var(--color-af-edge-muted)",
      strokeDasharray: undefined,
      strokeWidth: 1.7,
    });
  });

  it("styles warning and danger relation edges from required state tones", () => {
    const warningFlow = buildTraceRelationFactoryGraphFlow([
      {
        request_id: "request-blocked",
        required_state: "BLOCKED",
        source_work_id: "work-a",
        source_work_name: "Story A",
        target_work_id: "work-b",
        target_work_name: "Story B",
        type: "DEPENDS_ON",
      },
    ]);
    const dangerFlow = buildTraceRelationFactoryGraphFlow([
      {
        request_id: "request-rejected",
        required_state: "REJECTED",
        source_work_id: "work-c",
        source_work_name: "Story C",
        target_work_id: "work-d",
        target_work_name: "Story D",
        type: "RETRY",
      },
    ]);

    expect(warningFlow.edges[0]?.style?.stroke).toBe(
      "var(--color-af-warning-text)",
    );
    expect(dangerFlow.edges[0]?.style?.stroke).toBe(
      "var(--color-af-danger-text)",
    );
  });

  it("marks relation nodes selectable when onSelectWorkID is provided", () => {
    const onSelectWorkID = vi.fn();
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS, {
      onSelectWorkID,
    });

    expect(
      flow.nodes.find((node) => node.id === "work-implement")?.data.selectable,
    ).toBe(true);
    expect(
      flow.nodes.find((node) => node.id === "work-implement")?.data.onSelectWorkID,
    ).toBe(onSelectWorkID);
  });
});

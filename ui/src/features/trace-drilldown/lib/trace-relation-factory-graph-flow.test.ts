import { describe, expect, it, vi } from "vitest";
import type { DashboardWorkRelation } from "../../../api/dashboard/types";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { WORK_RELATION_NODE_TYPES } from "../../graphs/public";
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
  it("projects batch relations into shared work nodes and clean edges", () => {
    const editorMessages = getFactoryGraphEditorMessages();
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS);

    expect(flow.nodes).toHaveLength(3);
    expect(flow.topology.nodes.length).toBeGreaterThan(0);
    expect(flow.endpointKeyByNodeId.size).toBe(flow.topology.nodes.length);
    expect(
      flow.nodes.find((node) => node.id === "work-implement"),
    ).toMatchObject({
      id: "work-implement",
      type: "workRelation",
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
          sourceHandle: "trace-relation-source",
          target: "work-implement",
          targetHandle: "trace-relation-target",
          type: "factoryEditorEdge",
          ariaLabel:
            "Parent-child relation from Plan story to Implement story, requiring Done",
          data: expect.objectContaining({
            kind: "work-type-state",
            label: "",
          }),
          style: expect.objectContaining({
            stroke: "var(--color-outline-variant)",
            strokeWidth: 1.7,
          }),
        }),
      ]),
    );
    expect(
      flow.nodes
        .find((node) => node.id === "work-implement")
        ?.data.connectionAnchors.map((anchor) => anchor.id),
    ).toEqual(
      expect.arrayContaining([
        "trace-relation-target",
        "trace-relation-source",
      ]),
    );
  });

  it("registers only shared work graph React Flow node types", () => {
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS);

    expect(Object.keys(WORK_RELATION_NODE_TYPES)).toEqual(["workRelation"]);
    expect(flow.nodes.every((node) => node.type === "workRelation")).toBe(true);
    expect(flow.edges.every((edge) => edge.type === "factoryEditorEdge")).toBe(
      true,
    );
  });
});

describe("buildTraceRelationFactoryGraphFlow relation styling", () => {
  it("renders relations with a shared clean edge style", () => {
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
      stroke: "var(--color-outline-variant)",
      strokeWidth: 1.7,
    });
    expect(relatedFlow.edges[0]?.style).toMatchObject({
      stroke: "var(--color-outline-variant)",
      strokeWidth: 1.7,
    });
  });

  it("keeps required-state relations on the same clean edge style", () => {
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
      "var(--color-outline-variant)",
    );
    expect(dangerFlow.edges[0]?.style?.stroke).toBe(
      "var(--color-outline-variant)",
    );
  });
});

describe("buildTraceRelationFactoryGraphFlow repeated DEPENDS_ON", () => {
  it("renders distinct dependency edges and nodes for each relation instance", () => {
    const flow = buildTraceRelationFactoryGraphFlow([
      {
        request_id: "request-dependency-a",
        required_state: "ready",
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-dependency-story",
        target_work_name: "Dependency Story",
        type: "DEPENDS_ON",
      },
      {
        request_id: "request-dependency-b",
        required_state: "ready",
        source_work_id: "work-active-story",
        source_work_name: "Active Story",
        target_work_id: "work-second-dependency-story",
        target_work_name: "Second Dependency Story",
        type: "DEPENDS_ON",
      },
    ]);

    expect(flow.nodes.map((node) => node.id).sort()).toEqual([
      "work-active-story",
      "work-dependency-story",
      "work-second-dependency-story",
    ]);
    expect(flow.edges).toHaveLength(2);
    expect(
      flow.edges.map((edge) => `${edge.source}->${edge.target}`).sort(),
    ).toEqual([
      "work-active-story->work-dependency-story",
      "work-active-story->work-second-dependency-story",
    ]);
  });
});

describe("buildTraceRelationFactoryGraphFlow selection", () => {
  it("marks relation nodes selectable when onSelectWorkID is provided", () => {
    const onSelectWorkID = vi.fn();
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS, {
      onSelectWorkID,
    });

    expect(
      flow.nodes.find((node) => node.id === "work-implement")?.data.selectable,
    ).toBe(true);
    expect(
      flow.nodes.find((node) => node.id === "work-implement")?.data
        .onSelectWorkID,
    ).toBe(onSelectWorkID);
  });

  it("marks the selected work node as active and non-selectable", () => {
    const onSelectWorkID = vi.fn();
    const flow = buildTraceRelationFactoryGraphFlow(RELATIONS, {
      onSelectWorkID,
      selectedWorkID: "work-plan",
    });

    expect(flow.nodes.find((node) => node.id === "work-plan")?.data).toEqual(
      expect.objectContaining({
        isSelectedWork: true,
        selectable: false,
        workID: "work-plan",
      }),
    );
    expect(
      flow.nodes.find((node) => node.id === "work-implement")?.data,
    ).toEqual(
      expect.objectContaining({
        isSelectedWork: false,
        selectable: true,
        workID: "work-implement",
      }),
    );
  });
});

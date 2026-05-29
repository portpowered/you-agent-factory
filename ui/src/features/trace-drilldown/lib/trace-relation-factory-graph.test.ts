import { describe, expect, it } from "vitest";
import type {
  DashboardWorkItemRef,
  DashboardWorkRelation,
} from "../../../api/dashboard/types";
import { projectTraceRelationsToFactoryGraph } from "./trace-relation-factory-graph";

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

function buildRelation(
  overrides: Partial<DashboardWorkRelation> = {},
): DashboardWorkRelation {
  return {
    request_id: "request-default",
    source_work_id: "work-source",
    source_work_name: "Source story",
    target_work_id: "work-target",
    target_work_name: "Target story",
    type: "DEPENDS_ON",
    ...overrides,
  };
}

describe("projectTraceRelationsToFactoryGraph endpoint nodes", () => {
    it("maps relation endpoints to work-state nodes when state and work type are known", () => {
      const workItemsByWorkId = new Map<string, DashboardWorkItemRef>([
        [
          "work-plan",
          { display_name: "Plan story", work_id: "work-plan", work_type_id: "story" },
        ],
        [
          "work-implement",
          {
            display_name: "Implement story",
            work_id: "work-implement",
            work_type_id: "story",
          },
        ],
      ]);
      const projection = projectTraceRelationsToFactoryGraph(
        [RELATIONS[0]],
        { workItemsByWorkId },
      );

      expect(projection.topology.nodes.map((node) => node.id)).toEqual([
        "work-state:story:done",
        "work-type:story:work-implement",
      ]);
      expect(projection.nodeIdByEndpointKey.get("work-plan")).toBe(
        "work-state:story:done",
      );
      expect(projection.nodeIdByEndpointKey.get("work-implement")).toBe(
        "work-type:story:work-implement",
      );
    });

    it("derives work-state nodes from display-name work types when required state is present", () => {
      const projection = projectTraceRelationsToFactoryGraph([RELATIONS[0]]);

      expect(projection.topology.nodes.map((node) => node.id)).toEqual([
        "work-state:plan-story:done",
        "work-state:implement-story:done",
      ]);
      expect(projection.topology.nodes.every((node) => node.kind === "work-state"))
        .toBe(true);
    });

    it("uses work_type_id from optional work items when projecting work-state nodes", () => {
      const projection = projectTraceRelationsToFactoryGraph(RELATIONS, {
        workItemsByWorkId: new Map([
          [
            "work-plan",
            { display_name: "Plan story", work_id: "work-plan", work_type_id: "story" },
          ],
          [
            "work-implement",
            {
              display_name: "Implement story",
              work_id: "work-implement",
              work_type_id: "story",
            },
          ],
          [
            "work-repair",
            {
              display_name: "Repair story",
              work_id: "work-repair",
              work_type_id: "story",
            },
          ],
        ]),
      });

      expect(projection.topology.nodes.map((node) => node.id)).toEqual([
        "work-state:story:done",
        "work-type:story:work-implement",
        "work-state:story:failed",
      ]);
    });

    it("uses work-type nodes when relations have no required state", () => {
      const projection = projectTraceRelationsToFactoryGraph([
        buildRelation({ required_state: undefined, type: "PARENT_CHILD" }),
      ]);

      expect(projection.topology.nodes.every((node) => node.kind === "work-type"))
        .toBe(true);
      expect(projection.topology.edges[0]?.kind).toBe("work-type-state");
    });
});

describe("projectTraceRelationsToFactoryGraph edges and overlays", () => {
    it("emits work-type-state edges with localized aria labels", () => {
      const projection = projectTraceRelationsToFactoryGraph(RELATIONS);

      expect(projection.topology.edges).toHaveLength(2);
      expect(
        projection.topology.edges.every((edge) => edge.kind === "work-type-state"),
      ).toBe(true);
      expect(
        projection.edgeOverlaysByEdgeId.get(projection.topology.edges[0].id)
          ?.ariaLabel,
      ).toBe(
        "Parent-child relation from Plan story to Implement story, requiring Done",
      );
      expect(
        projection.edgeOverlaysByEdgeId.get(projection.topology.edges[1].id)
          ?.ariaLabel,
      ).toBe(
        "Retry relation from Implement story to Repair story, requiring Failed",
      );
    });

    it("aggregates relation metadata onto one node per endpoint", () => {
      const projection = projectTraceRelationsToFactoryGraph(RELATIONS, {
        workItemsByWorkId: new Map([
          [
            "work-implement",
            {
              display_name: "Implement story",
              work_id: "work-implement",
              work_type_id: "story",
            },
          ],
        ]),
      });
      const implementNodeId = projection.nodeIdByEndpointKey.get("work-implement");
      const overlay = implementNodeId
        ? projection.overlaysByNodeId.get(implementNodeId)
        : undefined;

      expect(overlay).toMatchObject({
        displayLabel: "Implement story",
        endpointKey: "work-implement",
        relationStates: ["DONE", "FAILED"],
        relationTypes: ["PARENT_CHILD", "RETRY"],
        workID: "work-implement",
      });
    });

    it("keeps canonical factory labels separate from trace overlay display labels", () => {
      const projection = projectTraceRelationsToFactoryGraph([RELATIONS[0]], {
        workItemsByWorkId: new Map([
          [
            "work-plan",
            { display_name: "Plan story", work_id: "work-plan", work_type_id: "story" },
          ],
          [
            "work-implement",
            {
              display_name: "Implement story",
              work_id: "work-implement",
              work_type_id: "story",
            },
          ],
        ]),
      });
      const planNode = projection.topology.nodes.find(
        (node) => projection.endpointKeyByNodeId.get(node.id) === "work-plan",
      );

      expect(planNode?.label).toBe("story:done");
      expect(planNode?.kind).toBe("work-state");
      expect(projection.overlaysByNodeId.get(planNode?.id ?? "")?.displayLabel).toBe(
        "Plan story",
      );
    });
});

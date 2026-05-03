import type { DashboardTopology } from "../../api/dashboard/types";
import {
  dashboardTopologyFixtures,
  oneNodeDashboardTopology,
} from "../../components/dashboard/fixtures";
import { buildGraphLayout } from "./layout";
import { describe, it, expect } from "vitest";

describe("buildGraphLayout", () => {
  it("keeps a single-node graph readable without edges", async () => {
    const layout = await buildGraphLayout(oneNodeDashboardTopology);

    expect(layout.nodes).toHaveLength(3);
    expect(layout.edges).toHaveLength(2);
    expect(layout.width).toBeGreaterThan(250);
    expect(layout.height).toBeGreaterThan(130);
    expect(layout.nodes.map((node) => node.nodeId)).toEqual([
      "place:story:new",
      "workstation:intake",
      "place:story:ready",
    ]);
    expect(layout.nodes[0]).toMatchObject({
      nodeId: "place:story:new",
      nodeKind: "state_position",
      column: 0,
      row: 1,
    });
    expect(layout.nodes[0]?.x).toBeGreaterThanOrEqual(40);
    expect(layout.nodes[0]?.y).toBeGreaterThanOrEqual(36);
  });

  it.each(Object.entries(dashboardTopologyFixtures))(
    "lays out visible nodes and edges from the %s fixture",
    async (_fixtureName, topology) => {
      const layout = await buildGraphLayout(topology);
      const positionedNodeIds = layout.nodes.map((node) => node.nodeId);
      const workstationNodeIds = layout.nodes
        .filter((node) => node.nodeKind === "workstation")
        .map((node) => node.nodeId);

      expect(workstationNodeIds).toEqual(
        expect.arrayContaining(
          topology.workstation_node_ids.map((nodeId) => `workstation:${nodeId}`),
        ),
      );
      expect(positionedNodeIds.some((nodeId) => nodeId.startsWith("place:"))).toBe(true);
      expect(layout.nodes.length).toBeGreaterThan(topology.workstation_node_ids.length);
      if (_fixtureName === "mediumBranching") {
        expect(positionedNodeIds).toContain("place:quality-gate:ready");
      }
      expect(layout.edges.length).toBeGreaterThan(0);
    },
  );

  it("keeps workstation heights fixed as runtime work content changes", async () => {
    const layout = await buildGraphLayout(oneNodeDashboardTopology);

    expect(layout.nodes.find((node) => node.nodeId === "workstation:intake")).toMatchObject({
      height: 196,
    });
    expect(layout.height).toBeGreaterThan(190);
  });

  it("uses compact dimensions for exhaustion-rule transitions", async () => {
    const topology = {
      workstation_node_ids: ["process", "executor-loop-breaker"],
      workstation_nodes_by_id: {
        process: {
          node_id: "process",
          transition_id: "process",
          workstation_name: "Process",
          worker_type: "processor",
          workstation_kind: "repeater",
          input_place_ids: ["task:init"],
          input_places: [{
            kind: "work_state",
            place_id: "task:init",
            state_category: "INITIAL",
            state_value: "init",
            type_id: "task",
          }],
          output_place_ids: ["task:in-review"],
          output_places: [{
            kind: "work_state",
            place_id: "task:in-review",
            state_category: "PROCESSING",
            state_value: "in-review",
            type_id: "task",
          }],
        },
        "executor-loop-breaker": {
          node_id: "executor-loop-breaker",
          transition_id: "executor-loop-breaker",
          workstation_name: "executor-loop-breaker",
          input_place_ids: ["task:init"],
          input_places: [{
            kind: "work_state",
            place_id: "task:init",
            state_category: "INITIAL",
            state_value: "init",
            type_id: "task",
          }],
          output_place_ids: ["task:failed"],
          output_places: [{
            kind: "work_state",
            place_id: "task:failed",
            state_category: "FAILED",
            state_value: "failed",
            type_id: "task",
          }],
        },
      },
      edges: [],
    } satisfies DashboardTopology;

    const layout = await buildGraphLayout(topology);

    expect(layout.nodes.find((node) => node.nodeId === "workstation:process")).toMatchObject({
      height: 196,
      width: 156,
    });
    expect(
      layout.nodes.find((node) => node.nodeId === "workstation:executor-loop-breaker"),
    ).toMatchObject({
      height: 58,
      width: 132,
    });
  });

  it("keeps constraint and limit places as first-class graph nodes", async () => {
    const topology = constructConstraintTopology()

    const layout = await buildGraphLayout(topology);
    const nodeIds = layout.nodes.map((node) => node.nodeId);

    expect(nodeIds).toEqual(
      expect.arrayContaining([
        "place:story:new",
        "place:story:ready",
        "place:story:done",
        "place:quality-gate:ready",
        "place:visit-limit:open",
        "workstation:intake",
        "workstation:review",
      ]),
    );
    expect(layout.nodes.find((node) => node.nodeId === "place:quality-gate:ready")).toMatchObject({
      height: 58,
      nodeKind: "constraint",
      width: 156,
    });
    expect(layout.nodes.find((node) => node.nodeId === "place:visit-limit:open")).toMatchObject({
      height: 58,
      nodeKind: "constraint",
      width: 156,
    });
    expect(layout.edges).toHaveLength(8);
    expect(layout.edges.map((edge) => edge.edgeId)).toEqual(
      expect.arrayContaining([
        "workstation:intake:place:quality-gate:ready:accepted",
        "workstation:intake:place:visit-limit:open:accepted",
        "place:quality-gate:ready:workstation:review:input",
        "place:visit-limit:open:workstation:review:input",
      ]),
    );

    const nodeIdSet = new Set(nodeIds);
    for (const edge of layout.edges) {
      expect(nodeIdSet.has(edge.fromNodeId)).toBe(true);
      expect(nodeIdSet.has(edge.toNodeId)).toBe(true);
    }
  });
});

function constructConstraintTopology(): DashboardTopology {
   return {
      workstation_node_ids: ["intake", "review"],
      workstation_nodes_by_id: {
        intake: {
          node_id: "intake",
          transition_id: "intake",
          workstation_name: "Intake",
          input_places: [{
            kind: "work_state",
            place_id: "story:new",
            state_category: "INITIAL",
            state_value: "new",
            type_id: "story",
          }],
          output_place_ids: ["story:ready", "quality-gate:ready", "visit-limit:open"],
          output_places: [
            {
              kind: "work_state",
              place_id: "story:ready",
              state_category: "PROCESSING",
              state_value: "ready",
              type_id: "story",
            },
            {
              kind: "constraint",
              place_id: "quality-gate:ready",
              state_value: "ready",
              type_id: "quality-gate",
            },
            {
              kind: "limit",
              place_id: "visit-limit:open",
              state_value: "open",
              type_id: "visit-limit",
            },
          ],
          input_place_ids: ["story:new"],
          input_work_type_ids: ["story"],
          output_work_type_ids: ["story"],
        },
        review: {
          node_id: "review",
          transition_id: "review",
          workstation_name: "Review",
          input_place_ids: ["story:ready", "quality-gate:ready", "visit-limit:open"],
          input_places: [
            {
              kind: "work_state",
              place_id: "story:ready",
              state_category: "PROCESSING",
              state_value: "ready",
              type_id: "story",
            },
            {
              kind: "constraint",
              place_id: "quality-gate:ready",
              state_value: "ready",
              type_id: "quality-gate",
            },
            {
              kind: "limit",
              place_id: "visit-limit:open",
              state_value: "open",
              type_id: "visit-limit",
            },
          ],
          output_place_ids: ["story:done"],
          output_places: [{
            kind: "work_state",
            place_id: "story:done",
            state_category: "TERMINAL",
            state_value: "done",
            type_id: "story",
          }],
          input_work_type_ids: ["story"],
          output_work_type_ids: ["story"],
        },
      },
      edges: [
        {
          edge_id: "intake:review:story:ready:accepted",
          from_node_id: "intake",
          outcome_kind: "accepted",
          state_category: "PROCESSING",
          state_value: "ready",
          to_node_id: "review",
          via_place_id: "story:ready",
          work_type_id: "story",
        },
        {
          edge_id: "intake:review:quality-gate:ready:accepted",
          from_node_id: "intake",
          outcome_kind: "accepted",
          state_value: "ready",
          to_node_id: "review",
          via_place_id: "quality-gate:ready",
          work_type_id: "quality-gate",
        },
        {
          edge_id: "intake:review:visit-limit:open:accepted",
          from_node_id: "intake",
          outcome_kind: "accepted",
          state_value: "open",
          to_node_id: "review",
          via_place_id: "visit-limit:open",
          work_type_id: "visit-limit",
        },
      ],
    } satisfies DashboardTopology;
}
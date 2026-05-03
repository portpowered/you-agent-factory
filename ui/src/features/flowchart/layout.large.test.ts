import { buildGraphLayout } from "./layout";
import { twentyNodeDashboardTopology } from "../../components/dashboard/test-fixtures";
import { describe, it, expect } from "vitest";

describe("buildGraphLayout", () => {

  it("spreads a representative 20-node workflow left to right", async () => {
    const layout = await buildGraphLayout(twentyNodeDashboardTopology);
    const nodesById = new Map(layout.nodes.map((node) => [node.nodeId, node]));
    const distinctColumns = new Set(layout.nodes.map((node) => node.column));
    const workstationNodes = layout.nodes.filter((node) => node.nodeKind === "workstation");
    const statePositionNodes = layout.nodes.filter((node) => node.nodeKind === "state_position");
    const resourceNodes = layout.nodes.filter((node) => node.nodeKind === "resource");

    expect(workstationNodes).toHaveLength(20);
    expect(statePositionNodes.length).toBeGreaterThan(20);
    expect(resourceNodes).toHaveLength(1);
    expect(resourceNodes[0]?.height).toBe(86);
    expect(statePositionNodes.every((node) => node.height === resourceNodes[0]?.height)).toBe(true);
    expect(layout.edges.length).toBeGreaterThan(40);
    expect(distinctColumns.size).toBeGreaterThan(10);
    expect(layout.width).toBeGreaterThan(5000);
    expect(layout.height).toBeGreaterThan(300);
    expect(layout.height).toBeLessThan(1200);

    for (const edge of layout.edges) {
      const from = nodesById.get(edge.fromNodeId);
      const to = nodesById.get(edge.toNodeId);

      expect(from).toBeDefined();
      expect(to).toBeDefined();
      expect(edge.labelX).toBeGreaterThan(from?.x ?? 0);
      expect(edge.labelX).toBeLessThan(to?.x ?? Number.MAX_SAFE_INTEGER);
    }

    expect(
      (nodesById.get("workstation:station-1")?.x ?? 0) <
        (nodesById.get("workstation:station-20")?.x ?? 0),
    ).toBe(true);
    expect(
      (nodesById.get("workstation:station-20")?.x ?? 0) -
        (nodesById.get("workstation:station-1")?.x ?? 0),
    ).toBeGreaterThan(4000);
  });
});


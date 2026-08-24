import { describe, expect, it } from "vitest";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { buildGraphLayout } from "../../flowchart/lib/layout";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  type ActiveGraphHighlights,
  buildActiveGraphHighlights,
  buildHandleAssignments,
  buildVisibleGraphEdges,
} from "./react-flow-current-activity-card-graph";

const CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS = "agent-flow-edge--hoverable";

describe("buildGraphEdges hover emphasis", () => {
  it("marks neutral edges hoverable and suppresses hoverable class for active, semantic, and muted edges", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(visibleGraphEdges);
    const neutralEdges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
    );
    expect(
      neutralEdges.some((edge) =>
        edge.className?.includes(CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS),
      ),
    ).toBe(true);
    expect(
      neutralEdges.some((edge) =>
        edge.className?.includes("agent-flow-edge--role-muted"),
      ),
    ).toBe(true);
    expect(
      neutralEdges.some((edge) =>
        edge.className?.includes("agent-flow-edge--role-muted-soft"),
      ),
    ).toBe(true);
    expect(
      neutralEdges.some((edge) =>
        edge.className?.includes("agent-flow-edge--role-danger-muted"),
      ),
    ).toBe(true);

    const activeEdge = visibleGraphEdges.find(
      (edge) =>
        edge.outcomeKind === "accepted" && edge.stateCategory !== "FAILED",
    );
    expect(activeEdge).toBeTruthy();
    const activeHighlights: ActiveGraphHighlights = {
      activeEdgeIds: new Set([activeEdge?.edgeId ?? ""]),
      activePlaceNodeIds: new Set(),
      activeWorkstationNodeIds: new Set(),
      hasActiveFlow: true,
      relatedNodeIds: new Set([
        activeEdge?.fromNodeId ?? "",
        activeEdge?.toNodeId ?? "",
      ]),
    };
    const activeEdges = buildGraphEdges(
      activeHighlights,
      handleAssignments,
      new Set(),
      visibleGraphEdges,
    );
    const activeReactFlowEdge = activeEdges.find(
      (edge) => edge.id === activeEdge?.edgeId,
    );
    expect(activeReactFlowEdge?.className).toContain("agent-flow-edge--active");
    expect(activeReactFlowEdge?.className).not.toContain(
      CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS,
    );

    const mutedReactFlowEdge = activeEdges.find((edge) =>
      edge.className?.includes("agent-flow-edge--muted"),
    );
    expect(mutedReactFlowEdge?.className).not.toContain(
      CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS,
    );

    const semanticEdge = visibleGraphEdges.find(
      (edge) =>
        edge.outcomeKind !== "accepted" || edge.stateCategory === "FAILED",
    );
    expect(semanticEdge).toBeTruthy();
    const semanticEdges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
    );
    const semanticReactFlowEdge = semanticEdges.find(
      (edge) => edge.id === semanticEdge?.edgeId,
    );
    expect(semanticReactFlowEdge?.className).toContain(
      "agent-flow-edge--semantic",
    );
    expect(semanticReactFlowEdge?.className).not.toContain(
      CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS,
    );
  });
});

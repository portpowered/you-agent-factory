import { describe, expect, it } from "vitest";

import type { GraphLayout } from "../../../flowchart/lib/layout";
import { graphNodePositionsFromCanonicalLayout } from "./factory-graph-canonical-layout-positions";

describe("graphNodePositionsFromCanonicalLayout", () => {
  it("projects canonical layout node positions with auto-layout fallback", () => {
    const graphLayout = {
      edges: [],
      height: 300,
      nodes: [
        {
          height: 100,
          nodeId: "workstation:draft",
          nodeKind: "workstation",
          width: 160,
          workstationNodeId: "draft",
          x: 10,
          y: 20,
        },
        {
          height: 80,
          nodeId: "worker:writer",
          nodeKind: "worker",
          width: 120,
          x: 30,
          y: 40,
        },
      ],
      width: 400,
    } as GraphLayout;

    expect(
      graphNodePositionsFromCanonicalLayout(graphLayout, {
        nodes: [
          {
            id: "workstation:draft",
            position: { x: 120, y: 80 },
          },
        ],
        schemaVersion: 1,
      }),
    ).toEqual({
      "worker:writer": { x: 30, y: 40 },
      "workstation:draft": { x: 120, y: 80 },
    });
  });
});

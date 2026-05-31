import { describe, expect, it } from "vitest";

import {
  currentActivityGraphKey,
  graphKeyAfterAddingNode,
} from "./react-flow-current-activity-card-keys";

describe("graphKeyAfterAddingNode", () => {
  it("adds the new node id to the sorted graph key node segment", () => {
    const graphKey = currentActivityGraphKey({
      edges: [{ edgeId: "edge-a", fromNodeId: "a", toNodeId: "b" }],
      height: 100,
      nodes: [
        {
          column: 0,
          height: 86,
          nodeId: "worker:writer",
          nodeKind: "worker",
          row: 0,
          width: 164,
          x: 0,
          y: 0,
        },
        {
          column: 1,
          height: 196,
          nodeId: "workstation:draft",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 200,
          y: 0,
        },
      ],
      width: 400,
    });

    expect(graphKeyAfterAddingNode(graphKey, "worker:assistant")).toBe(
      "worker:assistant|worker:writer|workstation:draft::edge-a",
    );
  });
});

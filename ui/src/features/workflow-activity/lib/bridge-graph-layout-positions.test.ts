import { describe, expect, it } from "vitest";
import {
  bridgeGraphLayoutPositions,
  clearStoredNodePositionsForNodeIds,
  findBridgedNodePosition,
  nodeIdsFromGraphKey,
  resolveStoredNodePositionsForGraphKey,
} from "./bridge-graph-layout-positions";
import { currentActivityGraphKey } from "./react-flow-current-activity-card-keys";

const baseLayout = {
  height: 100,
  width: 400,
  nodes: [
    {
      column: 0,
      height: 86,
      nodeId: "worker:writer",
      nodeKind: "worker" as const,
      row: 0,
      width: 164,
      x: 0,
      y: 0,
    },
    {
      column: 1,
      height: 196,
      nodeId: "workstation:draft",
      nodeKind: "workstation" as const,
      row: 0,
      width: 156,
      x: 200,
      y: 0,
    },
  ],
};

describe("nodeIdsFromGraphKey", () => {
  it("parses sorted node ids from the graph key node segment", () => {
    const graphKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-a", fromNodeId: "a", toNodeId: "b" }],
    });

    expect([...nodeIdsFromGraphKey(graphKey)].sort()).toEqual([
      "worker:writer",
      "workstation:draft",
    ]);
  });
});

describe("findBridgedNodePosition", () => {
  it("returns a position stored under a prior graph key when edges change on save", () => {
    const beforeSaveKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [
        {
          edgeId: "edge-draft",
          fromNodeId: "workstation:draft",
          toNodeId: "worker:writer",
        },
      ],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 420,
          y: 0,
        },
      ],
    });
    const afterSaveKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [
        {
          edgeId: "edge-saved",
          fromNodeId: "workstation:draft",
          toNodeId: "worker:writer",
        },
      ],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 420,
          y: 0,
        },
      ],
    });

    const positionsByGraphKey = {
      [beforeSaveKey]: {
        "workstation:review": { x: 512, y: 288 },
      },
    };

    expect(
      findBridgedNodePosition(
        positionsByGraphKey,
        afterSaveKey,
        "workstation:review",
      ),
    ).toEqual({ x: 512, y: 288 });
    expect(positionsByGraphKey[afterSaveKey]).toBeUndefined();
  });

  it("does not bridge from graph keys that never included the node id", () => {
    const preAddKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-a", fromNodeId: "a", toNodeId: "b" }],
    });

    expect(
      findBridgedNodePosition(
        {
          [preAddKey]: {
            "workstation:draft": { x: 10, y: 20 },
          },
        },
        currentActivityGraphKey({
          ...baseLayout,
          edges: [{ edgeId: "edge-a", fromNodeId: "a", toNodeId: "b" }],
          nodes: [
            ...baseLayout.nodes,
            {
              column: 2,
              height: 196,
              nodeId: "workstation:review",
              nodeKind: "workstation",
              row: 0,
              width: 156,
              x: 420,
              y: 0,
            },
          ],
        }),
        "workstation:review",
      ),
    ).toBeUndefined();
  });
});

describe("resolveStoredNodePositionsForGraphKey", () => {
  it("merges bridged positions for layout node ids", () => {
    const storedKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-old", fromNodeId: "a", toNodeId: "b" }],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 420,
          y: 0,
        },
      ],
    });
    const lookupKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-new", fromNodeId: "a", toNodeId: "b" }],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 420,
          y: 0,
        },
      ],
    });

    const resolved = resolveStoredNodePositionsForGraphKey(
      {
        [storedKey]: {
          "workstation:review": { x: 901, y: 402 },
        },
      },
      lookupKey,
      ["workstation:review", "workstation:draft"],
    );

    expect(resolved["workstation:review"]).toEqual({ x: 901, y: 402 });
  });
});

describe("bridgeGraphLayoutPositions", () => {
  it("writes bridged positions onto the target graph key", () => {
    const storedKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-old", fromNodeId: "a", toNodeId: "b" }],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 420,
          y: 0,
        },
      ],
    });
    const targetKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-new", fromNodeId: "a", toNodeId: "b" }],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          x: 420,
          y: 0,
        },
      ],
    });
    const positionsByGraphKey = {
      [storedKey]: {
        "workstation:review": { x: 120, y: 240 },
      },
    };

    const bridged = bridgeGraphLayoutPositions({
      nodeIds: ["workstation:review"],
      positionsByGraphKey,
      targetGraphKey: targetKey,
    });

    expect(bridged?.[targetKey]?.["workstation:review"]).toEqual({
      x: 120,
      y: 240,
    });
    expect(bridged?.[storedKey]?.["workstation:review"]).toEqual({
      x: 120,
      y: 240,
    });
  });

  it("returns null when the target key already has stored positions", () => {
    const graphKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-a", fromNodeId: "a", toNodeId: "b" }],
    });

    expect(
      bridgeGraphLayoutPositions({
        nodeIds: ["workstation:draft"],
        positionsByGraphKey: {
          [graphKey]: {
            "workstation:draft": { x: 1, y: 2 },
          },
        },
        targetGraphKey: graphKey,
      }),
    ).toBeNull();
  });
});

describe("clearStoredNodePositionsForNodeIds", () => {
  it("removes consumed temporary positions and drops emptied graph keys", () => {
    const firstGraphKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-a", fromNodeId: "a", toNodeId: "b" }],
    });
    const secondGraphKey = currentActivityGraphKey({
      ...baseLayout,
      edges: [{ edgeId: "edge-b", fromNodeId: "a", toNodeId: "b" }],
      nodes: [
        ...baseLayout.nodes,
        {
          column: 2,
          height: 196,
          nodeId: "doc:factory/docs/guide.md",
          nodeKind: "doc",
          row: 0,
          targetPath: "factory/docs/guide.md",
          width: 168,
          x: 420,
          y: 0,
        },
      ],
    });

    expect(
      clearStoredNodePositionsForNodeIds(
        {
          [firstGraphKey]: {
            "worker:writer": { x: 12, y: 24 },
          },
          [secondGraphKey]: {
            "doc:factory/docs/guide.md": { x: 640, y: 180 },
            "workstation:draft": { x: 240, y: 32 },
          },
        },
        ["doc:factory/docs/guide.md", "worker:writer"],
      ),
    ).toEqual({
      [secondGraphKey]: {
        "workstation:draft": { x: 240, y: 32 },
      },
    });
  });

  it("returns null when none of the requested node ids are stored", () => {
    expect(
      clearStoredNodePositionsForNodeIds(
        {
          graph: {
            "workstation:draft": { x: 20, y: 40 },
          },
        },
        ["doc:factory/docs/guide.md"],
      ),
    ).toBeNull();
  });
});

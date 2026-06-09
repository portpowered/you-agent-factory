import { describe, expect, it } from "vitest";

import { EMPTY_GRAPH_LAYOUT } from "./react-flow-current-activity-card-graph";
import {
  CURRENT_ACTIVITY_DOC_NODE_HEIGHT,
  CURRENT_ACTIVITY_DOC_NODE_WIDTH,
  mergeDocNodesIntoGraphLayout,
} from "./current-activity-doc-graph-layout";

describe("mergeDocNodesIntoGraphLayout", () => {
  it("returns the topology layout unchanged when no bundled docs exist", () => {
    const topologyLayout = {
      ...EMPTY_GRAPH_LAYOUT,
      height: 120,
      nodes: [
        {
          column: 0,
          height: 86,
          nodeId: "workstation:writer",
          nodeKind: "workstation" as const,
          row: 0,
          width: 156,
          workstationNodeId: "writer",
          x: 0,
          y: 0,
        },
      ],
      width: 156,
    };

    expect(
      mergeDocNodesIntoGraphLayout(topologyLayout, {
        supportingFiles: {
          bundledFiles: [
            {
              content: { encoding: "utf-8", inline: "print('setup')" },
              targetPath: "factory/scripts/setup.py",
              type: "SCRIPT",
            },
          ],
        },
      }),
    ).toEqual(topologyLayout);
  });

  it("appends doc nodes to the right of the topology layout", () => {
    const topologyLayout = {
      ...EMPTY_GRAPH_LAYOUT,
      height: 120,
      nodes: [
        {
          column: 0,
          height: 86,
          nodeId: "workstation:writer",
          nodeKind: "workstation" as const,
          row: 0,
          width: 156,
          workstationNodeId: "writer",
          x: 0,
          y: 0,
        },
      ],
      width: 156,
    };

    const merged = mergeDocNodesIntoGraphLayout(topologyLayout, {
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      },
    });

    expect(merged.nodes).toHaveLength(2);
    expect(merged.nodes[1]).toMatchObject({
      displayLabel: "guide.md",
      height: CURRENT_ACTIVITY_DOC_NODE_HEIGHT,
      nodeId: "doc:factory/docs/guide.md",
      nodeKind: "doc",
      targetPath: "factory/docs/guide.md",
      width: CURRENT_ACTIVITY_DOC_NODE_WIDTH,
      x: 156 + 48,
      y: 0,
    });
    expect(merged.width).toBeGreaterThan(topologyLayout.width);
  });

  it("does not append doc nodes when the factory definition is unavailable", () => {
    const topologyLayout = {
      ...EMPTY_GRAPH_LAYOUT,
      height: 120,
      nodes: [
        {
          column: 0,
          height: 86,
          nodeId: "workstation:writer",
          nodeKind: "workstation" as const,
          row: 0,
          width: 156,
          workstationNodeId: "writer",
          x: 0,
          y: 0,
        },
      ],
      width: 156,
    };

    expect(mergeDocNodesIntoGraphLayout(topologyLayout, null)).toEqual(
      topologyLayout,
    );
    expect(mergeDocNodesIntoGraphLayout(topologyLayout, undefined)).toEqual(
      topologyLayout,
    );
  });
});

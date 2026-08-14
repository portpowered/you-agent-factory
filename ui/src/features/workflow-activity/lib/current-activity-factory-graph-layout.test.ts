import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  CURRENT_ACTIVITY_DOC_NODE_HEIGHT,
  CURRENT_ACTIVITY_DOC_NODE_WIDTH,
} from "./current-activity-factory-graph-layout";

function factoryWithSupportingFiles(
  supportingFiles: CanonicalFactoryDefinition["supportingFiles"],
): CanonicalFactoryDefinition {
  return {
    name: "Doc layout factory",
    supportingFiles,
    workTypes: [
      {
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
    ],
    workers: [{ name: "writer", type: "MODEL_WORKER" }],
    workstations: [
      {
        id: "writer",
        inputs: [{ state: "new", workType: "story" }],
        name: "writer",
        outputs: [{ state: "done", workType: "story" }],
        worker: "writer",
      },
    ],
  };
}

describe("buildCurrentActivityGraphLayoutFromFactory docs", () => {
  it("includes bundled source files in the canonical factory graph layout", async () => {
    const layout = await buildCurrentActivityGraphLayoutFromFactory(
      factoryWithSupportingFiles({
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "print('setup')" },
            targetPath: "factory/scripts/setup-workspace.py",
            type: "SCRIPT",
          },
          {
            content: { encoding: "utf-8", inline: "test:\n\tbun test" },
            targetPath: "Makefile",
            type: "ROOT_HELPER",
          },
        ],
      }),
    );

    expect(layout.nodes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          displayLabel: "Makefile",
          nodeId: "doc:Makefile",
          nodeKind: "doc",
          targetPath: "Makefile",
        }),
        expect.objectContaining({
          displayLabel: "setup-workspace.py",
          nodeId: "doc:factory/scripts/setup-workspace.py",
          nodeKind: "doc",
          targetPath: "factory/scripts/setup-workspace.py",
        }),
      ]),
    );
  });

  it("includes bundled docs through the same canonical graph layout path", async () => {
    const layout = await buildCurrentActivityGraphLayoutFromFactory(
      factoryWithSupportingFiles({
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      }),
    );
    const docNode = layout.nodes.find(
      (node) => node.nodeId === "doc:factory/docs/guide.md",
    );

    expect(docNode).toMatchObject({
      displayLabel: "guide.md",
      height: CURRENT_ACTIVITY_DOC_NODE_HEIGHT,
      nodeKind: "doc",
      targetPath: "factory/docs/guide.md",
    });
    expect(docNode?.width).toBeGreaterThanOrEqual(
      CURRENT_ACTIVITY_DOC_NODE_WIDTH,
    );
    expect(layout.width).toBeGreaterThanOrEqual(
      (docNode?.x ?? 0) + (docNode?.width ?? CURRENT_ACTIVITY_DOC_NODE_WIDTH),
    );
  });

  it("includes nested bundled docs under factory/docs subdirectories", async () => {
    const layout = await buildCurrentActivityGraphLayoutFromFactory(
      factoryWithSupportingFiles({
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Review standards" },
            targetPath: "factory/docs/standards/review.md",
            type: "DOC",
          },
        ],
      }),
    );
    const docNode = layout.nodes.find(
      (node) => node.nodeId === "doc:factory/docs/standards/review.md",
    );

    expect(docNode).toMatchObject({
      displayLabel: "review.md",
      height: CURRENT_ACTIVITY_DOC_NODE_HEIGHT,
      nodeKind: "doc",
      targetPath: "factory/docs/standards/review.md",
    });
    expect(docNode?.width).toBeGreaterThanOrEqual(
      CURRENT_ACTIVITY_DOC_NODE_WIDTH,
    );
  });

  it("uses authored node sizes while keeping invalid saved sizes finite", async () => {
    const factory = factoryWithSupportingFiles({
      bundledFiles: [
        {
          content: { encoding: "utf-8", inline: "# Guide" },
          targetPath: "factory/docs/guide.md",
          type: "DOC",
        },
      ],
    });
    const factoryWithLayout: CanonicalFactoryDefinition = {
      ...factory,
      layout: {
        nodes: [
          {
            id: "workstation:writer",
            position: { x: 0, y: 0 },
            size: { height: 420, width: 320 },
          },
          {
            id: "doc:factory/docs/guide.md",
            position: { x: 360, y: 0 },
            size: { height: Number.POSITIVE_INFINITY, width: Number.NaN },
          },
        ],
        schemaVersion: 1,
      },
    };

    const layout = await buildCurrentActivityGraphLayoutFromFactory(
      factoryWithLayout,
    );
    const workstation = layout.nodes.find(
      (node) => node.nodeId === "workstation:writer",
    );
    const doc = layout.nodes.find(
      (node) => node.nodeId === "doc:factory/docs/guide.md",
    );

    expect(workstation).toMatchObject({ height: 420, width: 320 });
    expect(doc?.height).toBeFinite();
    expect(doc?.width).toBeFinite();
  });

  it("projects the shipped goal state inventory without the removed execute state", async () => {
    const layout = await buildCurrentActivityGraphLayoutFromFactory({
      name: "@you/goal",
      workTypes: [
        {
          name: "goal",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "complete", type: "TERMINAL" },
            { name: "blocked", type: "PROCESSING" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workers: [{ name: "goal-executor", type: "AGENT_WORKER" }],
      workstations: [
        {
          inputs: [{ state: "init", workType: "goal" }],
          name: "execute-goal",
          outputs: [{ state: "complete", workType: "goal" }],
          onFailure: [{ state: "failed", workType: "goal" }],
          type: "AGENT_RUN",
          worker: "goal-executor",
        },
        {
          inputs: [{ state: "init", workType: "goal" }],
          name: "goal-loop-breaker",
          outputs: [{ state: "failed", workType: "goal" }],
          type: "LOGICAL_MOVE",
        },
      ],
    });

    const nodeIDs = layout.nodes.map((node) => node.nodeId);
    expect(nodeIDs).toEqual(
      expect.arrayContaining([
        "work-state:goal:init",
        "work-state:goal:complete",
        "work-state:goal:blocked",
        "work-state:goal:failed",
      ]),
    );
    expect(nodeIDs).not.toContain("work-state:goal:execute");
  });
});

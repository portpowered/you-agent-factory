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
      width: CURRENT_ACTIVITY_DOC_NODE_WIDTH,
    });
    expect(layout.width).toBeGreaterThanOrEqual(
      (docNode?.x ?? 0) + CURRENT_ACTIVITY_DOC_NODE_WIDTH,
    );
  });
});

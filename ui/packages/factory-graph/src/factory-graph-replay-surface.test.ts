import type { FactoryGraphSource } from "./source.js";
import { expect, test } from "vitest";

import { projectFactoryGraphReplayFlow } from "./factory-graph-replay-surface.js";

test("projects Factory-authored layout, documents, and semantic runtime nodes together", () => {
  const source = {
    factory: {
      layout: {
        edges: [],
        groups: [],
        nodes: [
          { id: "worker:alex", position: { x: 40, y: 80 } },
          { id: "doc:factory/docs/runbook.md", position: { x: 640, y: 120 } },
        ],
        viewport: { x: 12, y: 24, zoom: 0.8 },
      },
      name: "Support",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "UTF8", inline: "# Runbook" },
            targetPath: "factory/docs/runbook.md",
            type: "DOC",
          },
        ],
      },
    },
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: [
          {
            entityId: "alex",
            handles: [],
            id: "worker:alex",
            kind: "worker",
            label: "Alex",
          },
        ],
        ok: true,
        selectedTick: 4,
      },
    },
    selectedTick: 4,
  } satisfies FactoryGraphSource;

  const flow = projectFactoryGraphReplayFlow(source, "doc:factory/docs/runbook.md");

  expect(flow.nodes).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: "worker:alex",
        position: { x: 40, y: 80 },
        type: "worker",
      }),
      expect.objectContaining({
        data: expect.objectContaining({
          selectedDoc: true,
          targetPath: "factory/docs/runbook.md",
        }),
        id: "doc:factory/docs/runbook.md",
        position: { x: 640, y: 120 },
        type: "doc",
      }),
    ]),
  );
});

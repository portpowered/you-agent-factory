import { describe, expect, it } from "vitest";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import { resolveInitialPlacementTopLeft } from "./graph-editor-add-node-placement";
import { graphEditorNodeDimensionsForKind } from "./graph-editor-node-placement";

/**
 * Regression guard for add placement ignoring the live viewport (pre-pan coordinates).
 * Pure tests cover every addable kind; browser integration covers workstation pan-add only.
 */
describe("graph editor add placement regression", () => {
  const prePanCenter = { x: 120, y: 80 };
  const postPanCenter = { x: 880, y: 640 };

  const addDraftsByKind: FactoryGraphAddEntityDraft[] = [
    { capacity: 1, kind: "resource", name: "extra-gpu" },
    { kind: "worker", model: "gpt", name: "assistant" },
    { kind: "work-type", name: "review-story" },
    {
      kind: "work-state",
      name: "blocked",
      stateType: "PROCESSING",
      workTypeName: "story",
    },
    {
      behavior: "STANDARD",
      body: "",
      kind: "workstation",
      name: "review",
      workerName: "writer",
    },
  ];

  it.each(addDraftsByKind)(
    "changes initial top-left when the viewport center moves for $kind adds",
    (draft) => {
      const nearTopLeft = resolveInitialPlacementTopLeft({
        draft,
        nodes: [],
        viewportCenter: prePanCenter,
      });
      const farTopLeft = resolveInitialPlacementTopLeft({
        draft,
        nodes: [],
        viewportCenter: postPanCenter,
      });

      expect(nearTopLeft).not.toBeNull();
      expect(farTopLeft).not.toBeNull();
      expect(nearTopLeft).not.toEqual(farTopLeft);
    },
  );

  it("derives top-left from kind dimensions at the viewport center", () => {
    const viewportCenter = { x: 640, y: 360 };
    const workstationSize = graphEditorNodeDimensionsForKind("workstation");

    const topLeft = resolveInitialPlacementTopLeft({
      draft: {
        behavior: "STANDARD",
        body: "",
        kind: "workstation",
        name: "review",
        workerName: "writer",
      },
      nodes: [],
      viewportCenter,
    });

    expect(topLeft).toEqual({
      x: viewportCenter.x - workstationSize.width / 2,
      y: viewportCenter.y - workstationSize.height / 2,
    });
  });
});

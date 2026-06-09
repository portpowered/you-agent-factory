import { describe, expect, it } from "vitest";

import { createDefaultFactoryLayout } from "../../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import {
  mergeFactoryLayoutWithNodePositions,
  overlayGraphNodePositions,
  selectHydratableGraphNodePositions,
} from "./factory-graph-canonical-layout-positions";

describe("selectHydratableGraphNodePositions", () => {
  it("keeps only temporary positions that differ from the canonical layout", () => {
    expect(
      selectHydratableGraphNodePositions(
        {
          "workstation:draft": { x: 120, y: 80 },
        },
        {
          "doc:factory/docs/guide.md": { x: 640, y: 220 },
          "workstation:draft": { x: 360, y: 140 },
          "worker:writer": { x: Number.NaN, y: 0 },
        },
      ),
    ).toEqual({
      "doc:factory/docs/guide.md": { x: 640, y: 220 },
      "workstation:draft": { x: 360, y: 140 },
    });
  });
});

describe("overlayGraphNodePositions", () => {
  it("prefers hydrated temporary positions over the canonical projection", () => {
    expect(
      overlayGraphNodePositions(
        {
          "doc:factory/docs/guide.md": { x: 420, y: 0 },
          "workstation:draft": { x: 120, y: 80 },
        },
        {
          "doc:factory/docs/guide.md": { x: 640, y: 220 },
        },
      ),
    ).toEqual({
      "doc:factory/docs/guide.md": { x: 640, y: 220 },
      "workstation:draft": { x: 120, y: 80 },
    });
  });
});

describe("mergeFactoryLayoutWithNodePositions", () => {
  it("hydrates temporary positions into the editor layout draft", () => {
    expect(
      mergeFactoryLayoutWithNodePositions(createDefaultFactoryLayout(), {
        "doc:factory/docs/guide.md": { x: 640, y: 220 },
        "workstation:draft": { x: 360, y: 140 },
      }),
    ).toMatchObject({
      nodes: expect.arrayContaining([
        {
          id: "doc:factory/docs/guide.md",
          position: { x: 640, y: 220 },
        },
        {
          id: "workstation:draft",
          position: { x: 360, y: 140 },
        },
      ]),
    });
  });
});

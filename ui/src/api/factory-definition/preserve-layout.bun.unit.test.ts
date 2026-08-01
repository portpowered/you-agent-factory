import { describe, expect, it } from "bun:test";

import { preserveExistingLayoutWhenAbsent } from "./preserve-layout";

describe("preserveExistingLayoutWhenAbsent", () => {
  it("preserves layout from the existing factory when the incoming factory omits it", () => {
    const existing = {
      layout: {
        nodes: [
          {
            id: "workstation:review",
            position: { x: 320, y: 180 },
          },
        ],
        schemaVersion: 1,
        viewport: { x: 12, y: 34, zoom: 1.1 },
      },
      name: "factory",
    };
    const incoming = {
      name: "factory",
      workers: [{ name: "writer", type: "MODEL_WORKER" as const }],
    };

    expect(preserveExistingLayoutWhenAbsent(incoming, existing)).toEqual({
      ...incoming,
      layout: existing.layout,
    });
  });

  it("keeps explicit incoming layout when it is present", () => {
    const existing = {
      layout: {
        nodes: [
          {
            id: "workstation:review",
            position: { x: 320, y: 180 },
          },
        ],
        schemaVersion: 1,
      },
      name: "factory",
    };
    const incoming = {
      layout: {
        nodes: [
          {
            id: "workstation:review",
            position: { x: 640, y: 420 },
          },
        ],
        schemaVersion: 1,
      },
      name: "factory",
    };

    expect(preserveExistingLayoutWhenAbsent(incoming, existing)).toEqual(
      incoming,
    );
  });
});

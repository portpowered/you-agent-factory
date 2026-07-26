import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { resolveTopologyStableFactory } from "./use-topology-stable-factory-for-layout";

function cloneDefinition(): CanonicalFactoryDefinition {
  return structuredClone(baseFactoryDefinition);
}

describe("useTopologyStableFactoryForLayout", () => {
  it("keeps the previous factory reference when only non-topology fields change", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workstations = [
      {
        ...(next.workstations?.[0] ?? {}),
        body: "Updated workstation instructions.",
      },
    ];

    const initial = resolveTopologyStableFactory(undefined, previous);
    expect(initial).toBe(previous);
    expect(resolveTopologyStableFactory(initial ?? undefined, next)).toBe(
      previous,
    );
  });

  it("returns the new factory reference when topology changes", () => {
    const previous = cloneDefinition();
    const next = cloneDefinition();
    next.workers = [
      ...(next.workers ?? []),
      {
        model: "gpt-5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ];
    next.workstations = [
      {
        ...(next.workstations?.[0] ?? {}),
        worker: "reviewer",
      },
    ];

    const initial = resolveTopologyStableFactory(undefined, previous);
    expect(resolveTopologyStableFactory(initial ?? undefined, next)).toBe(next);
  });
});

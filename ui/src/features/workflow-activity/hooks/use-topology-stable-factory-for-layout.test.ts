import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft.test-helpers";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { useTopologyStableFactoryForLayout } from "./use-topology-stable-factory-for-layout";

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

    const { result, rerender } = renderHook(
      ({ factory }) => useTopologyStableFactoryForLayout(factory),
      { initialProps: { factory: previous } },
    );

    expect(result.current).toBe(previous);

    rerender({ factory: next });

    expect(result.current).toBe(previous);
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

    const { result, rerender } = renderHook(
      ({ factory }) => useTopologyStableFactoryForLayout(factory),
      { initialProps: { factory: previous } },
    );

    rerender({ factory: next });

    expect(result.current).toBe(next);
  });
});

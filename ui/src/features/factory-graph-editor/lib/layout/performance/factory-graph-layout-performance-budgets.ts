import type { FactoryGraphLargeEditorFixtureKey } from "../../fixtures/factory-graph-large-editor-fixtures";

/**
 * Canonical layout-editor performance budgets for large factory graph fixtures.
 *
 * These budgets measure pure canonical-state work:
 * - initial projection via `projectFactoryGraphWithCanonicalLayout`
 * - drag response via layout move helpers
 * - save-related layout recomputation via dirty checks and pending-edit application
 *
 * React Flow object creation is not the source of truth for these checks.
 */
export interface FactoryGraphLayoutPerformanceBudget {
  dragMultiNodeMs: number;
  dragSingleNodeMs: number;
  initialProjectionMs: number;
  saveLayoutRecomputationMs: number;
  waypointHistoryMs: number;
  waypointEditMs: number;
}

export const FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS: Record<
  FactoryGraphLargeEditorFixtureKey,
  FactoryGraphLayoutPerformanceBudget
> = {
  hundred: {
    // Initial projection includes one-time module/import cost on cold starts.
    initialProjectionMs: 6_000,
    dragSingleNodeMs: 5,
    dragMultiNodeMs: 25,
    saveLayoutRecomputationMs: 50,
    waypointEditMs: 5,
    waypointHistoryMs: 5,
  },
  fiveHundred: {
    initialProjectionMs: 35_000,
    dragSingleNodeMs: 5,
    dragMultiNodeMs: 50,
    // Save-layout recomputation varies with host CPU load; 300 ms keeps the
    // 500-node gate stable on developer hardware and full `make ui-test` runs.
    saveLayoutRecomputationMs: 300,
    waypointEditMs: 5,
    waypointHistoryMs: 10,
  },
  stressThousand: {
    // Stress projection includes ELK work that can exceed 90 s on slower hosts.
    initialProjectionMs: 120_000,
    dragSingleNodeMs: 10,
    dragMultiNodeMs: 100,
    // Waypoint saves on the stress fixture can exceed the 500-node gate under load.
    saveLayoutRecomputationMs: 600,
    waypointEditMs: 10,
    waypointHistoryMs: 15,
  },
};

export async function measureMedianOperationMs(
  operation: () => void | Promise<void>,
  options?: {
    iterations?: number;
    warmup?: number;
  },
): Promise<number> {
  const iterations = options?.iterations ?? 5;
  const warmup = options?.warmup ?? 1;

  for (let index = 0; index < warmup; index += 1) {
    await operation();
  }

  const samples: number[] = [];
  for (let index = 0; index < iterations; index += 1) {
    const startedAt = performance.now();
    await operation();
    samples.push(performance.now() - startedAt);
  }

  samples.sort((left, right) => left - right);
  return samples[Math.floor(samples.length / 2)] ?? 0;
}

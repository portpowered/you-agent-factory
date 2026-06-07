import type { FactoryGraphLargeEditorFixtureKey } from "./factory-graph-large-editor-fixtures";

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
}

export const FACTORY_GRAPH_LAYOUT_PERFORMANCE_BUDGETS: Record<
  FactoryGraphLargeEditorFixtureKey,
  FactoryGraphLayoutPerformanceBudget
> = {
  hundred: {
    initialProjectionMs: 4_000,
    dragSingleNodeMs: 5,
    dragMultiNodeMs: 25,
    saveLayoutRecomputationMs: 25,
  },
  fiveHundred: {
    initialProjectionMs: 35_000,
    dragSingleNodeMs: 5,
    dragMultiNodeMs: 50,
    saveLayoutRecomputationMs: 50,
  },
  stressThousand: {
    initialProjectionMs: 90_000,
    dragSingleNodeMs: 10,
    dragMultiNodeMs: 100,
    saveLayoutRecomputationMs: 100,
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

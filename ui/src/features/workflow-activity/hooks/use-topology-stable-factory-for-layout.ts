import { useMemo, useRef } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { doesFactoryDefinitionChangeAffectGraphTopology } from "../../factory-graph-editor/lib/operations/factory-graph-topology-impact";

/**
 * Keeps the factory definition reference used for graph layout stable across
 * non-topology document updates so layout cache keys and persisted positions
 * are not invalidated by prompt-only or other metadata saves.
 */
export function useTopologyStableFactoryForLayout(
  factory: DashboardSnapshot["factory"] | null | undefined,
): DashboardSnapshot["factory"] | null | undefined {
  const stableFactoryRef = useRef<
    NonNullable<DashboardSnapshot["factory"]> | undefined
  >(undefined);

  return useMemo(() => {
    const stableFactory = resolveTopologyStableFactory(
      stableFactoryRef.current,
      factory,
    );
    stableFactoryRef.current = stableFactory ?? undefined;
    return stableFactory;
  }, [factory]);
}

export function resolveTopologyStableFactory(
  previous: NonNullable<DashboardSnapshot["factory"]> | undefined,
  factory: DashboardSnapshot["factory"] | null | undefined,
): DashboardSnapshot["factory"] | null | undefined {
  if (factory === null || factory === undefined) {
    return factory;
  }
  if (
    previous &&
    !doesFactoryDefinitionChangeAffectGraphTopology(previous, factory)
  ) {
    return previous;
  }
  return factory;
}

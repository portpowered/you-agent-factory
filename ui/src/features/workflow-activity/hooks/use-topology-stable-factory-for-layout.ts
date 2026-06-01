import { useMemo, useRef } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { doesFactoryDefinitionChangeAffectGraphTopology } from "../../factory-graph-editor/lib/factory-graph-topology-impact";

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
    if (factory === null) {
      stableFactoryRef.current = undefined;
      return null;
    }

    if (factory === undefined) {
      stableFactoryRef.current = undefined;
      return undefined;
    }

    const previous = stableFactoryRef.current;
    if (
      previous &&
      !doesFactoryDefinitionChangeAffectGraphTopology(previous, factory)
    ) {
      return previous;
    }

    stableFactoryRef.current = factory;
    return factory;
  }, [factory]);
}

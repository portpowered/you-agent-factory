import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { buildCurrentActivityGraphLayoutFromFactory } from "../lib/current-activity-factory-graph-layout";
import { EMPTY_GRAPH_LAYOUT } from "../lib/react-flow-current-activity-card-graph";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "./workflow-topology-async-cache";

const GRAPH_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<GraphLayout>();

export function useCurrentActivityGraphLayout(snapshot: DashboardSnapshot) {
  return useCurrentActivityGraphLayoutForFactory(snapshot);
}

export function useCurrentActivityGraphLayoutForFactory(
  snapshot: DashboardSnapshot,
  factoryOverride?: DashboardSnapshot["factory"],
) {
  const factory = factoryOverride ?? snapshot.factory;
  const layoutSource = useMemo(
    () =>
      factory
        ? {
            factory,
            key: currentActivityFactoryKey(factory),
            kind: "factory" as const,
          }
        : {
            key: "missing-factory",
            kind: "missing-factory" as const,
          },
    [factory],
  );

  return useWorkflowTopologyAsyncCache({
    cache: GRAPH_LAYOUT_CACHE,
    dependencies: [layoutSource],
    fallbackValue: EMPTY_GRAPH_LAYOUT,
    initialValue: EMPTY_GRAPH_LAYOUT,
    loadLayout: () =>
      layoutSource.kind === "factory"
        ? buildCurrentActivityGraphLayoutFromFactory(layoutSource.factory)
        : Promise.resolve(EMPTY_GRAPH_LAYOUT),
    mapResolvedLayout: identityGraphLayout,
    topologyKey: layoutSource.key,
  });
}

function identityGraphLayout(layout: GraphLayout) {
  return layout;
}

function currentActivityFactoryKey(
  factory: NonNullable<DashboardSnapshot["factory"]>,
): string {
  const legacyFactory = factory as typeof factory & {
    work_types?: typeof factory.workTypes;
  };
  return JSON.stringify({
    resources: (factory.resources ?? [])
      .map((resource) => resource.name)
      .sort(),
    workers: (factory.workers ?? [])
      .map((worker) => ({
        name: worker.name,
        resources: (worker.resources ?? [])
          .map((resource) => resource.name)
          .sort(),
      }))
      .sort((left, right) => left.name.localeCompare(right.name)),
    workTypes: (factory.workTypes ?? legacyFactory.work_types ?? [])
      .map((workType) => ({
        name: workType.name,
        states: workType.states.map((state) => ({
          name: state.name,
          type: state.type,
        })),
      }))
      .sort((left, right) => left.name.localeCompare(right.name)),
    workstations: (factory.workstations ?? [])
      .map((workstation) => ({
        id: workstation.id ?? "",
        inputs: workstation.inputs ?? [],
        name: workstation.name,
        onContinue:
          workstation.onContinue ??
          (workstation as typeof workstation & { on_continue?: unknown })
            .on_continue ??
          [],
        onFailure:
          workstation.onFailure ??
          (workstation as typeof workstation & { on_failure?: unknown })
            .on_failure ??
          [],
        onRejection:
          workstation.onRejection ??
          (workstation as typeof workstation & { on_rejection?: unknown })
            .on_rejection ??
          [],
        outputs: workstation.outputs ?? [],
        resources: workstation.resources ?? [],
        type: workstation.type ?? "",
        worker: workstation.worker,
      }))
      .sort((left, right) => left.name.localeCompare(right.name)),
  });
}

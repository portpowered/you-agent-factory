import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { buildGraphLayout, type GraphLayout } from "../../flowchart/lib/layout";
import { buildCurrentActivityGraphLayoutFromFactory } from "../lib/current-activity-factory-graph-layout";
import { EMPTY_GRAPH_LAYOUT } from "../lib/react-flow-current-activity-card-graph";
import { currentActivityTopologyKey } from "../lib/react-flow-current-activity-card-keys";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "./workflow-topology-async-cache";

const GRAPH_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<GraphLayout>();

export function useCurrentActivityGraphLayout(snapshot: DashboardSnapshot) {
  const layoutSource = useMemo(
    () =>
      snapshot.factory
        ? {
            factory: snapshot.factory,
            key: currentActivityFactoryKey(snapshot.factory),
            kind: "factory" as const,
          }
        : {
            key: currentActivityTopologyKey(snapshot.topology),
            kind: "topology" as const,
            topology: snapshot.topology,
          },
    [snapshot.factory, snapshot.topology],
  );

  return useWorkflowTopologyAsyncCache({
    cache: GRAPH_LAYOUT_CACHE,
    dependencies: [layoutSource],
    fallbackValue: EMPTY_GRAPH_LAYOUT,
    initialValue: EMPTY_GRAPH_LAYOUT,
    loadLayout: () =>
      layoutSource.kind === "factory"
        ? buildCurrentActivityGraphLayoutFromFactory(layoutSource.factory)
        : buildGraphLayout(layoutSource.topology),
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
    workTypes: (factory.workTypes ?? [])
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
        onContinue: workstation.onContinue ?? [],
        onFailure: workstation.onFailure ?? [],
        onRejection: workstation.onRejection ?? [],
        outputs: workstation.outputs ?? [],
        resources: workstation.resources ?? [],
        type: workstation.type ?? "",
        worker: workstation.worker,
      }))
      .sort((left, right) => left.name.localeCompare(right.name)),
  });
}

import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { buildFactoryGraphLayoutTopologyKey } from "../../factory-graph-editor/lib/operations/factory-graph-topology-impact";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { buildCurrentActivityGraphLayoutFromFactory } from "../lib/current-activity-factory-graph-layout";
import { EMPTY_GRAPH_LAYOUT } from "../lib/react-flow-current-activity-card-graph";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "./workflow-topology-async-cache";

export type CurrentActivityGraphLayoutBuilder = (
  factory: NonNullable<DashboardSnapshot["factory"]>,
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>,
  visibilityPreset: FactoryGraphEditorVisibilityPreset,
) => Promise<GraphLayout>;

const GRAPH_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<GraphLayout>();

export function resetCurrentActivityGraphLayoutCacheForTests(): void {
  GRAPH_LAYOUT_CACHE.inFlightByTopologyKey.clear();
  GRAPH_LAYOUT_CACHE.resolvedByTopologyKey.clear();
}

export function useCurrentActivityGraphLayoutForFactory(
  snapshot: DashboardSnapshot,
  /** Omit to use the event-computed `snapshot.factory`; pass `null` when graph state has no renderable factory yet. */
  factoryOverride?: DashboardSnapshot["factory"] | null,
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind> = new Set(),
  visibilityPreset: FactoryGraphEditorVisibilityPreset = "all",
  buildLayout: CurrentActivityGraphLayoutBuilder = buildCurrentActivityGraphLayoutFromFactory,
) {
  const factory =
    factoryOverride === undefined ? snapshot.factory : factoryOverride;
  const hiddenClassesKey = [...hiddenNodeClasses].sort().join(",");
  const layoutSource = useMemo(
    () =>
      factory
        ? {
            factory,
            hiddenClassesKey,
            key: `${buildFactoryGraphLayoutTopologyKey(factory)}|hidden:${hiddenClassesKey}|preset:${visibilityPreset}`,
            kind: "factory" as const,
            visibilityPreset,
          }
        : {
            hiddenClassesKey,
            key: `missing-factory|hidden:${hiddenClassesKey}|preset:${visibilityPreset}`,
            kind: "missing-factory" as const,
            visibilityPreset,
          },
    [factory, hiddenClassesKey, visibilityPreset],
  );

  return useWorkflowTopologyAsyncCache({
    cache: GRAPH_LAYOUT_CACHE,
    dependencies: [buildLayout, layoutSource],
    fallbackValue: EMPTY_GRAPH_LAYOUT,
    initialValue: EMPTY_GRAPH_LAYOUT,
    loadLayout: () =>
      layoutSource.kind === "factory"
        ? buildLayout(
            layoutSource.factory,
            hiddenNodeClasses,
            layoutSource.visibilityPreset,
          )
        : Promise.resolve(EMPTY_GRAPH_LAYOUT),
    mapResolvedLayout: identityGraphLayout,
    topologyKey: layoutSource.key,
  });
}

function identityGraphLayout(layout: GraphLayout) {
  return layout;
}

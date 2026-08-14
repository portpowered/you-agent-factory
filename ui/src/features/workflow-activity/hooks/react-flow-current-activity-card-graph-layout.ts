import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  type FactoryLayout,
  factoryLayoutFromDefinition,
} from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { buildFactoryGraphLayoutTopologyKey } from "../../factory-graph-editor/lib/operations/factory-graph-topology-impact";
import type { GraphLayout } from "../../flowchart/lib/layout";
import {
  applyFactoryLayoutNodeSizesToGraphLayout,
  buildCurrentActivityGraphLayoutFromFactory,
} from "../lib/current-activity-factory-graph-layout";
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
const GRAPH_LAYOUT_BUILDER_IDS = new WeakMap<
  CurrentActivityGraphLayoutBuilder,
  number
>();
let nextGraphLayoutBuilderID = 1;

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
  canonicalLayout?: FactoryLayout,
) {
  const factory =
    factoryOverride === undefined ? snapshot.factory : factoryOverride;
  const hiddenClassesKey = [...hiddenNodeClasses].sort().join(",");
  const builderCacheKey = getGraphLayoutBuilderCacheKey(buildLayout);
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

  const topologyLayout = useWorkflowTopologyAsyncCache({
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
    mapResolvedLayout: (layout) => layout,
    topologyKey: `${layoutSource.key}|builder:${builderCacheKey}`,
  });

  return useMemo(() => {
    if (layoutSource.kind !== "factory") {
      return topologyLayout;
    }

    return applyFactoryLayoutNodeSizesToGraphLayout({
      canonicalLayout:
        canonicalLayout ?? factoryLayoutFromDefinition(layoutSource.factory),
      factory: layoutSource.factory,
      graphLayout: topologyLayout,
    });
  }, [canonicalLayout, layoutSource, topologyLayout]);
}

function getGraphLayoutBuilderCacheKey(
  buildLayout: CurrentActivityGraphLayoutBuilder,
): number {
  const existingID = GRAPH_LAYOUT_BUILDER_IDS.get(buildLayout);
  if (existingID !== undefined) {
    return existingID;
  }

  const builderID = nextGraphLayoutBuilderID;
  nextGraphLayoutBuilderID += 1;
  GRAPH_LAYOUT_BUILDER_IDS.set(buildLayout, builderID);
  return builderID;
}

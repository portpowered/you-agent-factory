import { useEffect, useState } from "react";

import { buildFactoryGraphEditorLayout } from "../../factory-graph-editor/factory-graph-editor-layout";
import type { FactoryGraphTopology } from "../../factory-graph-editor/factory-graph-draft-types";

const EDITOR_LAYOUT_CACHE = new Map<
  string,
  Awaited<ReturnType<typeof buildFactoryGraphEditorLayout>>
>();
const EDITOR_LAYOUT_PROMISE_CACHE = new Map<
  string,
  Promise<Awaited<ReturnType<typeof buildFactoryGraphEditorLayout>>>
>();

export function useFactoryGraphEditorLayoutPositions(
  topology: FactoryGraphTopology,
  topologyKey: string,
) {
  const [layoutPositionsByNodeId, setLayoutPositionsByNodeId] = useState<
    ReadonlyMap<string, { x: number; y: number }>
  >(new Map());

  useEffect(() => {
    if (topology.nodes.length === 0) {
      setLayoutPositionsByNodeId(new Map());
      return;
    }

    let cancelled = false;
    const cachedLayout = EDITOR_LAYOUT_CACHE.get(topologyKey);
    if (cachedLayout) {
      setLayoutPositionsByNodeId(layoutNodePositions(cachedLayout.nodes));
      return () => {
        cancelled = true;
      };
    }

    const inFlightLayout =
      EDITOR_LAYOUT_PROMISE_CACHE.get(topologyKey) ??
      buildFactoryGraphEditorLayout(topology);
    EDITOR_LAYOUT_PROMISE_CACHE.set(topologyKey, inFlightLayout);

    inFlightLayout
      .then((layout) => {
        EDITOR_LAYOUT_CACHE.set(topologyKey, layout);
        EDITOR_LAYOUT_PROMISE_CACHE.delete(topologyKey);
        if (!cancelled) {
          setLayoutPositionsByNodeId(layoutNodePositions(layout.nodes));
        }
      })
      .catch(() => {
        EDITOR_LAYOUT_PROMISE_CACHE.delete(topologyKey);
        if (!cancelled) {
          setLayoutPositionsByNodeId(new Map());
        }
      });

    return () => {
      cancelled = true;
    };
  }, [topology, topologyKey]);

  return layoutPositionsByNodeId;
}

function layoutNodePositions(
  nodes: Array<{ nodeId: string; x: number; y: number }>,
) {
  return new Map(nodes.map((node) => [node.nodeId, { x: node.x, y: node.y }]));
}

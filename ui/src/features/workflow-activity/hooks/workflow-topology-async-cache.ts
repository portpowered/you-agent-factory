import { useEffect, useState, type DependencyList } from "react";

type WorkflowTopologyAsyncCache<TLayout> = {
  readonly inFlightByTopologyKey: Map<string, Promise<TLayout>>;
  readonly resolvedByTopologyKey: Map<string, TLayout>;
};

type UseWorkflowTopologyAsyncCacheOptions<TLayout, TResult> = {
  cache: WorkflowTopologyAsyncCache<TLayout>;
  dependencies: DependencyList;
  disabled?: boolean;
  disabledValue?: TResult;
  fallbackValue: TResult;
  initialValue: TResult;
  loadLayout: () => Promise<TLayout>;
  mapResolvedLayout: (layout: TLayout) => TResult;
  topologyKey: string;
};

export function createWorkflowTopologyAsyncCache<TLayout>(): WorkflowTopologyAsyncCache<TLayout> {
  return {
    inFlightByTopologyKey: new Map<string, Promise<TLayout>>(),
    resolvedByTopologyKey: new Map<string, TLayout>(),
  };
}

export function useWorkflowTopologyAsyncCache<TLayout, TResult>({
  cache,
  dependencies,
  disabled = false,
  disabledValue,
  fallbackValue,
  initialValue,
  loadLayout,
  mapResolvedLayout,
  topologyKey,
}: UseWorkflowTopologyAsyncCacheOptions<TLayout, TResult>) {
  const [value, setValue] = useState<TResult>(initialValue);

  useEffect(() => {
    if (disabled) {
      setValue(disabledValue ?? initialValue);
      return;
    }

    let cancelled = false;
    const cachedLayout = cache.resolvedByTopologyKey.get(topologyKey);

    if (cachedLayout) {
      setValue(mapResolvedLayout(cachedLayout));
      return () => {
        cancelled = true;
      };
    }

    const inFlightLayout =
      cache.inFlightByTopologyKey.get(topologyKey) ?? loadLayout();
    cache.inFlightByTopologyKey.set(topologyKey, inFlightLayout);

    inFlightLayout
      .then((layout) => {
        cache.resolvedByTopologyKey.set(topologyKey, layout);
        cache.inFlightByTopologyKey.delete(topologyKey);
        if (!cancelled) {
          setValue(mapResolvedLayout(layout));
        }
      })
      .catch(() => {
        cache.inFlightByTopologyKey.delete(topologyKey);
        if (!cancelled) {
          setValue(fallbackValue);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [
    cache,
    disabled,
    disabledValue,
    fallbackValue,
    initialValue,
    loadLayout,
    mapResolvedLayout,
    topologyKey,
    ...dependencies,
  ]);

  return value;
}

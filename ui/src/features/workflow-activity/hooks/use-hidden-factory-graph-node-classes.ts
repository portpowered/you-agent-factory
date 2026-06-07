import { useCallback, useMemo, useState } from "react";

import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";

function serializeHiddenNodeClasses(
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>,
): string {
  return [...hiddenNodeClasses].sort().join(",");
}

export function useHiddenFactoryGraphNodeClasses() {
  const [hiddenNodeClasses, setHiddenNodeClasses] = useState<
    ReadonlySet<FactoryGraphNodeKind>
  >(() => new Set());
  const [baselineHiddenNodeClasses, setBaselineHiddenNodeClasses] = useState(
    () => serializeHiddenNodeClasses(new Set()),
  );
  const [hideShowMenuOpen, setHideShowMenuOpen] = useState(false);
  const preferencesDirty = useMemo(
    () =>
      serializeHiddenNodeClasses(hiddenNodeClasses) !==
      baselineHiddenNodeClasses,
    [baselineHiddenNodeClasses, hiddenNodeClasses],
  );

  const toggleHiddenNodeClass = useCallback((kind: FactoryGraphNodeKind) => {
    setHiddenNodeClasses((current) => {
      const next = new Set(current);
      if (next.has(kind)) {
        next.delete(kind);
      } else {
        next.add(kind);
      }
      return next;
    });
  }, []);
  const resetPreferences = useCallback(() => {
    setHiddenNodeClasses(new Set());
    setBaselineHiddenNodeClasses(serializeHiddenNodeClasses(new Set()));
  }, []);
  const adoptSavedPreferences = useCallback(
    (nextHiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>) => {
      setHiddenNodeClasses(new Set(nextHiddenNodeClasses));
      setBaselineHiddenNodeClasses(
        serializeHiddenNodeClasses(nextHiddenNodeClasses),
      );
    },
    [],
  );

  return {
    adoptSavedPreferences,
    hasPreferenceChanges: preferencesDirty,
    hiddenNodeClasses,
    hideShowMenuOpen,
    preferencesDirty,
    resetPreferences,
    setHideShowMenuOpen,
    toggleHiddenNodeClass,
  };
}

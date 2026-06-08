import { useCallback, useState } from "react";

import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";

export function useHiddenFactoryGraphNodeClasses() {
  const [hiddenNodeClasses, setHiddenNodeClasses] = useState<
    ReadonlySet<FactoryGraphNodeKind>
  >(() => new Set());
  const [hideShowMenuOpen, setHideShowMenuOpen] = useState(false);

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

  return {
    hiddenNodeClasses,
    hideShowMenuOpen,
    setHideShowMenuOpen,
    toggleHiddenNodeClass,
  };
}

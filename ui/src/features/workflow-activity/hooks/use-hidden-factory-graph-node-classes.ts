import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  clearFactoryGraphEditorPreferencesForScope,
  DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
  factoryGraphEditorViewPreferencesDirty,
  readFactoryGraphEditorPreferencesForScope,
  writeFactoryGraphEditorPreferencesForScope,
} from "../../factory-graph-editor/lib/preferences/factory-graph-editor-preferences";

export function useHiddenFactoryGraphNodeClasses(
  factoryViewScopeKey?: string | null,
) {
  const normalizedScopeKey = factoryViewScopeKey ?? null;
  const [hiddenNodeClasses, setHiddenNodeClasses] = useState<
    ReadonlySet<FactoryGraphNodeKind>
  >(
    () =>
      new Set(DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.hiddenNodeClasses),
  );
  const [visibilityPreset, setVisibilityPresetState] =
    useState<FactoryGraphEditorVisibilityPreset>(
      DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.visibilityPreset,
    );
  const [hideShowMenuOpen, setHideShowMenuOpen] = useState(false);
  const hasHydratedPreferencesRef = useRef(false);
  const lastScopeKeyRef = useRef<string | null>(null);

  useEffect(() => {
    if (normalizedScopeKey === lastScopeKeyRef.current) {
      return;
    }

    lastScopeKeyRef.current = normalizedScopeKey;
    hasHydratedPreferencesRef.current = false;

    if (normalizedScopeKey === null) {
      setHiddenNodeClasses(
        new Set(
          DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.hiddenNodeClasses,
        ),
      );
      setVisibilityPresetState(
        DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.visibilityPreset,
      );
      hasHydratedPreferencesRef.current = true;
      return;
    }

    const storedPreferences =
      readFactoryGraphEditorPreferencesForScope(normalizedScopeKey);
    setHiddenNodeClasses(new Set(storedPreferences.hiddenNodeClasses));
    setVisibilityPresetState(storedPreferences.visibilityPreset);
    hasHydratedPreferencesRef.current = true;
  }, [normalizedScopeKey]);

  const currentPreferences = useMemo(
    () => ({
      hiddenNodeClasses,
      visibilityPreset,
    }),
    [hiddenNodeClasses, visibilityPreset],
  );
  const preferencesDirty = useMemo(
    () => factoryGraphEditorViewPreferencesDirty(currentPreferences),
    [currentPreferences],
  );

  useEffect(() => {
    if (!hasHydratedPreferencesRef.current || normalizedScopeKey === null) {
      return;
    }

    writeFactoryGraphEditorPreferencesForScope(
      normalizedScopeKey,
      currentPreferences,
    );
  }, [currentPreferences, normalizedScopeKey]);

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

  const setVisibilityPreset = useCallback(
    (preset: FactoryGraphEditorVisibilityPreset) => {
      setVisibilityPresetState(preset);
    },
    [],
  );

  const resetPreferences = useCallback(() => {
    setHiddenNodeClasses(
      new Set(DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.hiddenNodeClasses),
    );
    setVisibilityPresetState(
      DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES.visibilityPreset,
    );
    if (normalizedScopeKey !== null) {
      clearFactoryGraphEditorPreferencesForScope(normalizedScopeKey);
    }
  }, [normalizedScopeKey]);

  const adoptSavedPreferences = useCallback(
    (nextHiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>) => {
      setHiddenNodeClasses(new Set(nextHiddenNodeClasses));
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
    setVisibilityPreset,
    toggleHiddenNodeClass,
    visibilityPreset,
  };
}

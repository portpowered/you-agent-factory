import type { FactoryGraphEditorVisibilityPreset } from "../../components/controls/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../draft/factory-graph-draft-types";
import { FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS } from "../work-state/factory-graph-node-class-visibility";

export const FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY =
  "agent-factory.factory-graph.editor-preferences.v1";

export const DEFAULT_FACTORY_GRAPH_EDITOR_VISIBILITY_PRESET: FactoryGraphEditorVisibilityPreset =
  "all";

export interface FactoryGraphEditorViewPreferences {
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
}

export const DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES: FactoryGraphEditorViewPreferences =
  {
    hiddenNodeClasses: new Set(),
    visibilityPreset: DEFAULT_FACTORY_GRAPH_EDITOR_VISIBILITY_PRESET,
  };

type StoredPreferencesRecord = Record<
  string,
  {
    hiddenNodeClasses?: unknown;
    visibilityPreset?: unknown;
  }
>;

export function serializeFactoryGraphEditorViewPreferences(
  preferences: FactoryGraphEditorViewPreferences,
): string {
  const hiddenNodeClasses = [...preferences.hiddenNodeClasses].sort().join(",");
  return `${hiddenNodeClasses}|${preferences.visibilityPreset}`;
}

export function factoryGraphEditorViewPreferencesDirty(
  preferences: FactoryGraphEditorViewPreferences,
): boolean {
  return (
    serializeFactoryGraphEditorViewPreferences(preferences) !==
    serializeFactoryGraphEditorViewPreferences(
      DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
    )
  );
}

export function readStoredFactoryGraphEditorPreferences(
  storage: Pick<Storage, "getItem"> = window.localStorage,
): StoredPreferencesRecord {
  try {
    const raw = storage.getItem(FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY);
    if (!raw) {
      return {};
    }

    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }

    return parsed as StoredPreferencesRecord;
  } catch {
    return {};
  }
}

export function readFactoryGraphEditorPreferencesForScope(
  factoryViewScopeKey: string,
  storage: Pick<Storage, "getItem"> = window.localStorage,
): FactoryGraphEditorViewPreferences {
  const storedRecord = readStoredFactoryGraphEditorPreferences(storage);
  return normalizeFactoryGraphEditorViewPreferences(
    storedRecord[factoryViewScopeKey],
  );
}

export function writeFactoryGraphEditorPreferencesForScope(
  factoryViewScopeKey: string,
  preferences: FactoryGraphEditorViewPreferences,
  storage: Pick<
    Storage,
    "getItem" | "setItem" | "removeItem"
  > = window.localStorage,
): void {
  try {
    const storedRecord = readStoredFactoryGraphEditorPreferences(storage);
    const nextRecord = { ...storedRecord };

    if (
      serializeFactoryGraphEditorViewPreferences(preferences) ===
      serializeFactoryGraphEditorViewPreferences(
        DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
      )
    ) {
      delete nextRecord[factoryViewScopeKey];
    } else {
      nextRecord[factoryViewScopeKey] = {
        hiddenNodeClasses: [...preferences.hiddenNodeClasses].sort(),
        visibilityPreset: preferences.visibilityPreset,
      };
    }

    if (Object.keys(nextRecord).length === 0) {
      storage.removeItem(FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY);
      return;
    }

    storage.setItem(
      FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY,
      JSON.stringify(nextRecord),
    );
  } catch {
    // Preference persistence is a convenience; graph interaction should keep working without it.
  }
}

export function clearFactoryGraphEditorPreferencesForScope(
  factoryViewScopeKey: string,
  storage: Pick<
    Storage,
    "getItem" | "setItem" | "removeItem"
  > = window.localStorage,
): void {
  writeFactoryGraphEditorPreferencesForScope(
    factoryViewScopeKey,
    DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
    storage,
  );
}

function normalizeFactoryGraphEditorViewPreferences(
  value: StoredPreferencesRecord[string] | undefined,
): FactoryGraphEditorViewPreferences {
  if (!value || typeof value !== "object") {
    return DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES;
  }

  const hiddenNodeClasses = normalizeHiddenNodeClasses(value.hiddenNodeClasses);
  const visibilityPreset = normalizeVisibilityPreset(value.visibilityPreset);

  return {
    hiddenNodeClasses,
    visibilityPreset,
  };
}

function normalizeHiddenNodeClasses(
  value: unknown,
): ReadonlySet<FactoryGraphNodeKind> {
  if (!Array.isArray(value)) {
    return new Set();
  }

  const allowedKinds = new Set<FactoryGraphNodeKind>(
    FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS,
  );
  const hiddenNodeClasses = value.filter(
    (kind): kind is FactoryGraphNodeKind =>
      typeof kind === "string" &&
      allowedKinds.has(kind as FactoryGraphNodeKind),
  );

  return new Set(hiddenNodeClasses);
}

function normalizeVisibilityPreset(
  value: unknown,
): FactoryGraphEditorVisibilityPreset {
  switch (value) {
    case "workflow":
    case "execution":
    case "infrastructure":
    case "all":
      return value;
    default:
      return DEFAULT_FACTORY_GRAPH_EDITOR_VISIBILITY_PRESET;
  }
}

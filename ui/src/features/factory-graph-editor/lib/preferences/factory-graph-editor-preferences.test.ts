import { describe, expect, it } from "vitest";

import {
  clearFactoryGraphEditorPreferencesForScope,
  DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
  FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY,
  factoryGraphEditorViewPreferencesDirty,
  readFactoryGraphEditorPreferencesForScope,
  readStoredFactoryGraphEditorPreferences,
  serializeFactoryGraphEditorViewPreferences,
  writeFactoryGraphEditorPreferencesForScope,
} from "../preferences/factory-graph-editor-preferences";

function createMemoryStorage(): Storage {
  const values = new Map<string, string>();

  return {
    get length() {
      return values.size;
    },
    clear() {
      values.clear();
    },
    getItem(key: string) {
      return values.get(key) ?? null;
    },
    key(index: number) {
      return [...values.keys()][index] ?? null;
    },
    removeItem(key: string) {
      values.delete(key);
    },
    setItem(key: string, value: string) {
      values.set(key, value);
    },
  };
}

describe("factoryGraphEditorViewPreferences", () => {
  it("treats non-default hidden classes and presets as dirty", () => {
    expect(
      factoryGraphEditorViewPreferencesDirty(
        DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES,
      ),
    ).toBe(false);
    expect(
      factoryGraphEditorViewPreferencesDirty({
        hiddenNodeClasses: new Set(["work-state"]),
        visibilityPreset: "all",
      }),
    ).toBe(true);
    expect(
      factoryGraphEditorViewPreferencesDirty({
        hiddenNodeClasses: new Set(),
        visibilityPreset: "workflow",
      }),
    ).toBe(true);
  });

  it("persists and reloads preferences per factory view scope", () => {
    const storage = createMemoryStorage();

    writeFactoryGraphEditorPreferencesForScope(
      "session-alpha",
      {
        hiddenNodeClasses: new Set(["resource"]),
        visibilityPreset: "execution",
      },
      storage,
    );
    writeFactoryGraphEditorPreferencesForScope(
      "session-beta",
      {
        hiddenNodeClasses: new Set(["work-state"]),
        visibilityPreset: "workflow",
      },
      storage,
    );

    expect(
      readFactoryGraphEditorPreferencesForScope("session-alpha", storage),
    ).toEqual({
      hiddenNodeClasses: new Set(["resource"]),
      visibilityPreset: "execution",
    });
    expect(
      readFactoryGraphEditorPreferencesForScope("session-beta", storage),
    ).toEqual({
      hiddenNodeClasses: new Set(["work-state"]),
      visibilityPreset: "workflow",
    });
  });

  it("removes scope entries when preferences return to defaults", () => {
    const storage = createMemoryStorage();

    writeFactoryGraphEditorPreferencesForScope(
      "session-alpha",
      {
        hiddenNodeClasses: new Set(["worker"]),
        visibilityPreset: "infrastructure",
      },
      storage,
    );
    clearFactoryGraphEditorPreferencesForScope("session-alpha", storage);

    expect(
      storage.getItem(FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY),
    ).toBeNull();
    expect(
      readFactoryGraphEditorPreferencesForScope("session-alpha", storage),
    ).toEqual(DEFAULT_FACTORY_GRAPH_EDITOR_VIEW_PREFERENCES);
  });

  it("ignores invalid stored preference payloads", () => {
    const storage = createMemoryStorage();
    storage.setItem(
      FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY,
      JSON.stringify({
        "session-alpha": {
          hiddenNodeClasses: ["not-a-kind", "work-state"],
          visibilityPreset: "unknown",
        },
      }),
    );

    expect(
      readFactoryGraphEditorPreferencesForScope("session-alpha", storage),
    ).toEqual({
      hiddenNodeClasses: new Set(["work-state"]),
      visibilityPreset: "all",
    });
  });

  it("serializes preferences deterministically", () => {
    expect(
      serializeFactoryGraphEditorViewPreferences({
        hiddenNodeClasses: new Set(["work-state", "resource"]),
        visibilityPreset: "workflow",
      }),
    ).toBe("resource,work-state|workflow");
  });

  it("returns an empty record for missing, malformed, or non-object stored payloads", () => {
    const storage = createMemoryStorage();
    storage.setItem(FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY, "{not-json");
    expect(readStoredFactoryGraphEditorPreferences(storage)).toEqual({});

    storage.setItem(
      FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY,
      JSON.stringify(["not", "an", "object"]),
    );
    expect(readStoredFactoryGraphEditorPreferences(storage)).toEqual({});
  });

  it("ignores write failures without throwing", () => {
    const storage = {
      getItem: () => null,
      setItem: () => {
        throw new Error("quota exceeded");
      },
      removeItem: () => {
        throw new Error("quota exceeded");
      },
    };

    expect(() =>
      writeFactoryGraphEditorPreferencesForScope(
        "session-alpha",
        {
          hiddenNodeClasses: new Set(["resource"]),
          visibilityPreset: "workflow",
        },
        storage,
      ),
    ).not.toThrow();
  });
});

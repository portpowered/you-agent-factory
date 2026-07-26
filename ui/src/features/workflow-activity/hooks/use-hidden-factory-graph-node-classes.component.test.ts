import "../../../testing/vitest-dom-capabilities.setup";

import { act, renderHook } from "@testing-library/react";

import { FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY } from "../../factory-graph-editor/lib/preferences/factory-graph-editor-preferences";
import { useHiddenFactoryGraphNodeClasses } from "./use-hidden-factory-graph-node-classes";

describe("useHiddenFactoryGraphNodeClasses", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("starts with every node class visible and toggles hidden classes", () => {
    const { result } = renderHook(() =>
      useHiddenFactoryGraphNodeClasses("session-alpha"),
    );

    expect(result.current.hiddenNodeClasses.size).toBe(0);

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
    });

    expect(result.current.hiddenNodeClasses.has("work-state")).toBe(true);

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
    });

    expect(result.current.hiddenNodeClasses.has("work-state")).toBe(false);
  });

  it("tracks preference-only dirty state without portable document changes", () => {
    const { result } = renderHook(() =>
      useHiddenFactoryGraphNodeClasses("session-alpha"),
    );

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
    });

    expect(result.current.preferencesDirty).toBe(true);
    expect(result.current.hasPreferenceChanges).toBe(true);
  });

  it("clears preference dirty state when preferences reset", () => {
    const { result } = renderHook(() =>
      useHiddenFactoryGraphNodeClasses("session-alpha"),
    );

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
      result.current.resetPreferences();
    });

    expect(result.current.preferencesDirty).toBe(false);
  });

  it("persists hidden classes and visibility presets per factory view scope", () => {
    const first = renderHook(() =>
      useHiddenFactoryGraphNodeClasses("session-alpha"),
    );

    act(() => {
      first.result.current.toggleHiddenNodeClass("resource");
      first.result.current.setVisibilityPreset("workflow");
    });

    first.unmount();

    const second = renderHook(() =>
      useHiddenFactoryGraphNodeClasses("session-alpha"),
    );

    expect(second.result.current.hiddenNodeClasses.has("resource")).toBe(true);
    expect(second.result.current.visibilityPreset).toBe("workflow");
    expect(second.result.current.preferencesDirty).toBe(true);
  });

  it("keeps preferences isolated across factory view scopes", () => {
    const alpha = renderHook(
      ({ scopeKey }: { scopeKey: string }) =>
        useHiddenFactoryGraphNodeClasses(scopeKey),
      { initialProps: { scopeKey: "session-alpha" } },
    );

    act(() => {
      alpha.result.current.toggleHiddenNodeClass("worker");
      alpha.result.current.setVisibilityPreset("execution");
    });

    alpha.rerender({ scopeKey: "session-beta" });

    expect(alpha.result.current.hiddenNodeClasses.size).toBe(0);
    expect(alpha.result.current.visibilityPreset).toBe("all");
    expect(alpha.result.current.preferencesDirty).toBe(false);

    alpha.rerender({ scopeKey: "session-alpha" });

    expect(alpha.result.current.hiddenNodeClasses.has("worker")).toBe(true);
    expect(alpha.result.current.visibilityPreset).toBe("execution");
  });

  it("removes persisted preferences when reset restores the shared authored view", () => {
    const { result } = renderHook(() =>
      useHiddenFactoryGraphNodeClasses("session-alpha"),
    );

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
      result.current.setVisibilityPreset("infrastructure");
      result.current.resetPreferences();
    });

    expect(
      window.localStorage.getItem(FACTORY_GRAPH_EDITOR_PREFERENCES_STORAGE_KEY),
    ).toBeNull();
    expect(result.current.visibilityPreset).toBe("all");
    expect(result.current.hiddenNodeClasses.size).toBe(0);
  });
});
// Component lane: requires DOM APIs.

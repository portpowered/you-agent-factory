import { act, renderHook } from "@testing-library/react";

import { useHiddenFactoryGraphNodeClasses } from "./use-hidden-factory-graph-node-classes";

describe("useHiddenFactoryGraphNodeClasses", () => {
  it("starts with every node class visible and toggles hidden classes", () => {
    const { result } = renderHook(() => useHiddenFactoryGraphNodeClasses());

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
    const { result } = renderHook(() => useHiddenFactoryGraphNodeClasses());

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
    });

    expect(result.current.preferencesDirty).toBe(true);
    expect(result.current.hasPreferenceChanges).toBe(true);
  });

  it("clears preference dirty state when preferences reset", () => {
    const { result } = renderHook(() => useHiddenFactoryGraphNodeClasses());

    act(() => {
      result.current.toggleHiddenNodeClass("work-state");
      result.current.resetPreferences();
    });

    expect(result.current.preferencesDirty).toBe(false);
  });
});

import { act, renderHook } from "@testing-library/react";

import { useGraphEditorSession } from "./use-graph-editor-session";

const fixtureState = vi.hoisted(() => ({
  sessionState: {
    currentFactoryDefinition: {
      name: "Current Factory",
      resources: [],
      version: { logical: "1", physical: "2026-05-25T00:00:00Z" },
      workers: [],
      workTypes: [],
      workstations: [],
    },
    definitionStatus: "success",
    hasPendingGraphChanges: false,
    isSaving: false,
    projectedFactory: undefined,
  },
}));

function renderSession(editorMode = false) {
  const onAttemptLeaveEditor = vi.fn();
  const onLeaveEditor = vi.fn();
  const setEditorMode = vi.fn();
  const setActiveTool = vi.fn();

  const hook = renderHook(() =>
    useGraphEditorSession({
      activeTool: null as never,
      editorMode,
      locale: null,
      onAttemptLeaveEditor,
      onLeaveEditor,
      sessionState: fixtureState.sessionState,
      setActiveTool,
      setEditorMode,
    }),
  );

  return {
    hook,
    onAttemptLeaveEditor,
    onLeaveEditor,
    setEditorMode,
  };
}

describe("useGraphEditorSession", () => {
  beforeEach(() => {
    fixtureState.sessionState.hasPendingGraphChanges = false;
  });

  it("enters editor mode when toggling from observe mode", () => {
    const { hook, onAttemptLeaveEditor, onLeaveEditor, setEditorMode } =
      renderSession(false);

    act(() => {
      hook.result.current.handleEditorModeToggle();
    });

    expect(setEditorMode).toHaveBeenCalledWith(true);
    expect(onAttemptLeaveEditor).not.toHaveBeenCalled();
    expect(onLeaveEditor).not.toHaveBeenCalled();
  });

  it("opens leave confirmation instead of exiting when the draft has changes", () => {
    fixtureState.sessionState.hasPendingGraphChanges = true;
    const { hook, onAttemptLeaveEditor, onLeaveEditor, setEditorMode } =
      renderSession(true);

    act(() => {
      hook.result.current.handleEditorModeToggle();
    });

    expect(onAttemptLeaveEditor).toHaveBeenCalledTimes(1);
    expect(onLeaveEditor).not.toHaveBeenCalled();
    expect(setEditorMode).not.toHaveBeenCalled();
  });

  it("leaves editor mode immediately when there are no pending changes", () => {
    const { hook, onAttemptLeaveEditor, onLeaveEditor, setEditorMode } =
      renderSession(true);

    act(() => {
      hook.result.current.handleEditorModeToggle();
    });

    expect(onLeaveEditor).toHaveBeenCalledTimes(1);
    expect(onAttemptLeaveEditor).not.toHaveBeenCalled();
    expect(setEditorMode).not.toHaveBeenCalled();
  });
});

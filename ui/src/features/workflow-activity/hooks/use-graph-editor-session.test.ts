import { act, renderHook } from "@testing-library/react";

import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useGraphEditorSession } from "./use-graph-editor-session";

const fixtureState = vi.hoisted(() => ({
  draftState: {
    hasChanges: false,
    pendingFactoryDefinition: {
      name: "Current Factory",
      resources: [],
      version: { logical: "1", physical: "2026-05-25T00:00:00Z" },
      workers: [],
      workTypes: [],
      workstations: [],
    },
    latestDocument: null,
    baseDocument: null,
  } as EditableFactoryGraphViewModel["draftState"],
}));

function renderSession(editorMode = false) {
  const onAttemptLeaveEditor = vi.fn();
  const onLeaveEditor = vi.fn();
  const setEditorMode = vi.fn();
  const setActiveTool = vi.fn();

  const hook = renderHook(() =>
    useGraphEditorSession({
      activeTool: null,
      draftState: fixtureState.draftState,
      editableDefinitionQuery: { status: "success" } as never,
      editorMode,
      onAttemptLeaveEditor,
      onLeaveEditor,
      saveEditableDefinition: { isPending: false } as never,
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
    fixtureState.draftState.hasChanges = false;
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
    fixtureState.draftState.hasChanges = true;
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

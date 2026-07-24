import { describe, expect, it } from "vitest";

import {
  isFactoryGraphEditorDeleteSelectionKeyboardEvent,
  isFactoryGraphEditorRedoKeyboardEvent,
  isFactoryGraphEditorUndoKeyboardEvent,
  shouldHandleFactoryGraphEditorKeyboardShortcut,
} from "./factory-graph-layout-keyboard-shortcuts";

describe("factory graph layout keyboard shortcuts", () => {
  it("ignores shortcuts from text inputs outside the editor canvas", () => {
    const input = document.createElement("input");
    document.body.appendChild(input);

    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(input)).toBe(false);

    input.remove();
  });

  it("accepts shortcuts from the editor canvas surface", () => {
    const canvas = document.createElement("div");
    canvas.dataset.factoryGraphEditorCanvas = "true";
    const inner = document.createElement("div");
    canvas.appendChild(inner);
    document.body.appendChild(canvas);

    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(inner)).toBe(true);

    canvas.remove();
  });

  it("ignores shortcuts from contenteditable and select elements", () => {
    const editable = document.createElement("div");
    editable.contentEditable = "true";
    const select = document.createElement("select");
    const textarea = document.createElement("textarea");
    document.body.append(editable, select, textarea);

    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(editable)).toBe(
      false,
    );
    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(select)).toBe(false);
    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(textarea)).toBe(
      false,
    );

    editable.remove();
    select.remove();
    textarea.remove();
  });

  it("ignores non-element event targets", () => {
    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(null)).toBe(false);
    expect(shouldHandleFactoryGraphEditorKeyboardShortcut(document)).toBe(
      false,
    );
  });

  it("detects undo and redo keyboard combinations", () => {
    expect(
      isFactoryGraphEditorUndoKeyboardEvent({
        ctrlKey: true,
        key: "z",
        metaKey: false,
        shiftKey: false,
      }),
    ).toBe(true);
    expect(
      isFactoryGraphEditorRedoKeyboardEvent({
        ctrlKey: true,
        key: "y",
        metaKey: false,
        shiftKey: false,
      }),
    ).toBe(true);
    expect(
      isFactoryGraphEditorRedoKeyboardEvent({
        ctrlKey: true,
        key: "z",
        metaKey: false,
        shiftKey: true,
      }),
    ).toBe(true);
    expect(
      isFactoryGraphEditorUndoKeyboardEvent({
        ctrlKey: false,
        key: "Z",
        metaKey: true,
        shiftKey: false,
      }),
    ).toBe(true);
    expect(
      isFactoryGraphEditorRedoKeyboardEvent({
        ctrlKey: false,
        key: "y",
        metaKey: true,
        shiftKey: false,
      }),
    ).toBe(false);
  });

  it("detects delete and backspace keyboard events without modifier keys", () => {
    expect(
      isFactoryGraphEditorDeleteSelectionKeyboardEvent({
        altKey: false,
        ctrlKey: false,
        key: "Delete",
        metaKey: false,
      }),
    ).toBe(true);
    expect(
      isFactoryGraphEditorDeleteSelectionKeyboardEvent({
        altKey: false,
        ctrlKey: false,
        key: "Backspace",
        metaKey: false,
      }),
    ).toBe(true);
    expect(
      isFactoryGraphEditorDeleteSelectionKeyboardEvent({
        altKey: false,
        ctrlKey: true,
        key: "Backspace",
        metaKey: false,
      }),
    ).toBe(false);
  });
});

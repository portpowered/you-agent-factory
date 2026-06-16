export function shouldHandleFactoryGraphEditorKeyboardShortcut(
  target: EventTarget | null,
): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  if (target.isContentEditable) {
    return false;
  }

  const tagName = target.tagName;
  if (tagName === "INPUT" || tagName === "TEXTAREA" || tagName === "SELECT") {
    return false;
  }

  return Boolean(target.closest("[data-factory-graph-editor-canvas='true']"));
}

export function isFactoryGraphEditorUndoKeyboardEvent(event: {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
}): boolean {
  const key = event.key.toLowerCase();
  return key === "z" && (event.metaKey || event.ctrlKey) && !event.shiftKey;
}

export function isFactoryGraphEditorRedoKeyboardEvent(event: {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
}): boolean {
  const key = event.key.toLowerCase();
  return (
    (key === "z" && (event.metaKey || event.ctrlKey) && event.shiftKey) ||
    (key === "y" && event.ctrlKey && !event.metaKey)
  );
}

export function isFactoryGraphEditorDeleteSelectionKeyboardEvent(event: {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
}): boolean {
  return (
    (event.key === "Delete" || event.key === "Backspace") &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.altKey
  );
}

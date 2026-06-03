export const PROMPT_EDITOR_RESIZE_DEFAULT_HEIGHT = "13.5rem";
export const PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX = 160;
export const PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX = 640;

export function clampPromptEditorResizeHeight(
  height: number,
  bounds: { maxHeight: number; minHeight: number },
): number {
  return Math.min(
    bounds.maxHeight,
    Math.max(bounds.minHeight, Math.round(height)),
  );
}

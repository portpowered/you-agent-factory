export const PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX = 280;
export const PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX = 960;

export function clampPromptEditorResizeWidth(
  width: number,
  bounds: { maxWidth: number; minWidth: number },
): number {
  return Math.min(
    bounds.maxWidth,
    Math.max(bounds.minWidth, Math.round(width)),
  );
}

export function resolvePromptEditorResizeMaxWidth(
  configuredMaxWidth: number,
  parentWidth: number,
): number {
  if (parentWidth <= 0) {
    return configuredMaxWidth;
  }

  return Math.min(configuredMaxWidth, parentWidth);
}

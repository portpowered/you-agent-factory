import {
  clampPromptEditorResizeWidth,
  PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX,
  PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX,
  resolvePromptEditorResizeMaxWidth,
} from "./prompt-editor-resize-bounds";

describe("prompt-editor-resize-bounds", () => {
  it("clamps width to configured min and max bounds", () => {
    expect(
      clampPromptEditorResizeWidth(120, {
        minWidth: PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX,
        maxWidth: PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX,
      }),
    ).toBe(PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX);
    expect(
      clampPromptEditorResizeWidth(1200, {
        minWidth: PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX,
        maxWidth: PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX,
      }),
    ).toBe(PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX);
    expect(
      clampPromptEditorResizeWidth(420.6, {
        minWidth: PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX,
        maxWidth: PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX,
      }),
    ).toBe(421);
  });

  it("limits max width to the parent container width when narrower", () => {
    expect(resolvePromptEditorResizeMaxWidth(960, 640)).toBe(640);
    expect(resolvePromptEditorResizeMaxWidth(960, 0)).toBe(960);
  });
});
